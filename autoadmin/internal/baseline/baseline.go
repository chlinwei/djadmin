package baseline

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/opapolicy"

	"github.com/gin-gonic/gin"
)

// OS 基线扫描：基线标准（章节分组条目）→ 挂载解析（复用 inspection 的
// 项目/环境主机集合查询）→ OPA check_plan 下发 → 结果落 baseline_scan_result，
// 按主机聚合符合率。目标只解析主机上下文（OS 基线与应用无关，禁应用变量）。

type Handler struct {
	db      *sql.DB
	gateway *agent.Gateway
	// canceledScans 内存镜像已取消扫描（扫描 goroutine 据此丢弃结果/提前收尾）；
	// Agent 协议无 cancel 帧，无法真正终止已下发的执行，与巡检 CancelExecution 同语义。
	canceledScans sync.Map
}

// markCanceled / isCanceled：取消标记的进程内读写。
func (handler *Handler) markCanceled(scanID int64) { handler.canceledScans.Store(scanID, struct{}{}) }
func (handler *Handler) isCanceled(scanID int64) bool {
	_, canceled := handler.canceledScans.Load(scanID)
	return canceled
}

func NewHandler(database *sql.DB, gateway *agent.Gateway) *Handler {
	return &Handler{db: database, gateway: gateway}
}

type baselineItemInput struct {
	CategoryID  int64           `json:"category_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
	Severity    string          `json:"severity"`
}

type baselineInput struct {
	Name        *string              `json:"name"`
	Version     *string              `json:"version"`
	Description *string              `json:"description"`
	Enabled     *bool                `json:"enabled"`
	Items       *[]baselineItemInput `json:"items"`
}

func pathID(context *gin.Context) int64 {
	return pathParamID(context, "id")
}

func pathParamID(context *gin.Context, name string) int64 {
	var id int64
	fmt.Sscanf(strings.TrimSpace(context.Param(name)), "%d", &id)
	return id
}

func isNoRows(err error) bool { return err == sql.ErrNoRows }

// ---- 基线标准 CRUD ----

func (handler *Handler) ListBaselines(context *gin.Context) {
	search := strings.TrimSpace(context.Query("search"))
	pageNumber, pageSize := 1, 20
	// 空搜索传 NULL：查询里写的是 `LIKE narg(pattern) OR narg(pattern) IS NULL`，
	// 传 NULL 才走"不过滤"分支（传空串会变成 LIKE ''，只剩空名字能命中）。
	pattern := sql.NullString{}
	if search != "" {
		pattern = sql.NullString{String: "%" + search + "%", Valid: true}
	}
	queries := db.New(handler.db)
	total, err := queries.CountBaselines(context, db.CountBaselinesParams{Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListBaselines(context, db.ListBaselinesParams{Pattern: pattern, Limit: int32(pageSize), Offset: 0})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"id": row.ID, "name": row.Name, "version": row.Version, "description": row.Description,
			"enabled": row.Enabled, "item_count": row.ItemCount, "scan_count": row.ScanCount,
			"create_time": row.CreateTime, "update_time": row.UpdateTime})
	}
	response.Paginated(context, items, total, int32(pageNumber), int32(pageSize))
}

func (handler *Handler) GetBaseline(context *gin.Context) {
	id := pathID(context)
	queries := db.New(handler.db)
	row, err := queries.GetBaseline(context, id)
	if isNoRows(err) {
		response.BusinessError(context, 404, "基线不存在", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	categories := make([]gin.H, 0)
	categoryRows, err := queries.ListBaselineCategories(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, category := range categoryRows {
		categories = append(categories, gin.H{"id": category.ID, "sort": category.Sort, "name": category.Name})
	}
	// 这里用的是 ListBaselineItems（带 JOIN 类目名），因此条目按"类目顺序 → 类目内 sort"排列，
	// 与前端按类目分组展示的顺序一致。
	itemRows, err := queries.ListBaselineItems(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(itemRows))
	for _, item := range itemRows {
		var configDecoded any
		_ = json.Unmarshal(item.Config, &configDecoded)
		items = append(items, gin.H{"id": item.ID, "category_id": item.CategoryID, "category": item.Category, "sort": item.Sort, "name": item.Name,
			"description": item.Description, "config": configDecoded, "severity": item.Severity})
	}
	response.Success(context, gin.H{"baseline": gin.H{"id": row.ID, "name": row.Name, "version": row.Version,
		"description": row.Description, "enabled": row.Enabled, "create_time": row.CreateTime, "update_time": row.UpdateTime},
		"categories": categories, "items": items})
}

func (handler *Handler) SaveBaseline(context *gin.Context) {
	var input struct {
		Name        *string              `json:"name"`
		Version     *string              `json:"version"`
		Description *string              `json:"description"`
		Enabled     *bool                `json:"enabled"`
		Items       *[]baselineItemInput `json:"items"`
	}
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	if input.Name == nil || strings.TrimSpace(*input.Name) == "" {
		response.BusinessError(context, 400, "基线名称不能为空", nil)
		return
	}
	id := pathID(context)
	if id == 0 && input.Items != nil && len(*input.Items) > 0 {
		response.BusinessError(context, 400, "请先保存基线，再添加策略", nil)
		return
	}
	if input.Items != nil {
		seen := make(map[string]bool)
		categoryIDs := make(map[int64]bool)
		for _, item := range *input.Items {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				response.BusinessError(context, 400, "基线策略名称不能为空", nil)
				return
			}
			if item.CategoryID <= 0 {
				response.BusinessError(context, 400, fmt.Sprintf("基线策略 %q 缺少类目", name), nil)
				return
			}
			categoryIDs[item.CategoryID] = true
			if seen[name] {
				response.BusinessError(context, 400, fmt.Sprintf("基线策略名称不能重复: %q", name), nil)
				return
			}
			seen[name] = true
			// 唯一执行器 OPA：config 必须是对象且通过共享校验。
			var opaConfig map[string]any
			if err := json.Unmarshal(item.Config, &opaConfig); err != nil {
				response.BusinessError(context, 400, fmt.Sprintf("基线策略 %q 的 config 必须是对象", name), nil)
				return
			}
			if message := opapolicy.Validate(opaConfig); message != "" {
				response.BusinessError(context, 400, fmt.Sprintf("基线策略 %q: %s", name, message), nil)
				return
			}
		}
		// 提交的类目必须全部属于当前基线（防止跨基线挂策略）；新建基线（id=0）
		// 还没有类目，提交策略直接拒绝——前端流程是先建基线再进条目编辑。
		//
		// 这里取回基线的全部类目在应用层比对，而不是拼 `id IN (?,?,?)`：占位符个数随入参变化
		// 正是 sqlc 表达不了的形状（SQL_DESIGN §1 的"可变长 IN → 内联"），
		// 而基线的类目数量很小，一次查询足够。
		if id > 0 && len(categoryIDs) > 0 {
			ownedCategories, err := db.New(handler.db).ListBaselineCategories(context, id)
			if err != nil {
				response.Error(context, err)
				return
			}
			owned := make(map[int64]bool, len(ownedCategories))
			for _, category := range ownedCategories {
				owned[category.ID] = true
			}
			for categoryID := range categoryIDs {
				if !owned[categoryID] {
					response.BusinessError(context, 400, "策略的类目不存在或不属于当前基线", nil)
					return
				}
			}
		}
	}
	version, description, enabled := "v1", deref(input.Description), true
	if input.Version != nil && strings.TrimSpace(*input.Version) != "" {
		version = strings.TrimSpace(*input.Version)
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	name := strings.TrimSpace(*input.Name)
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	queries := db.New(tx)
	now := time.Now().UTC()
	if id == 0 {
		id, err = queries.CreateBaseline(context, db.CreateBaselineParams{CreateTime: now, UpdateTime: now,
			Name: name, Version: version, Description: description, Enabled: enabled})
		if err != nil {
			response.BusinessError(context, 400, "基线名称已存在", nil)
			return
		}
	} else {
		err = queries.UpdateBaseline(context, db.UpdateBaselineParams{UpdateTime: now, Name: name, Version: version,
			Description: description, Enabled: enabled, ID: id})
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if input.Items != nil {
		if err = queries.DeleteBaselineItems(context, id); err != nil {
			response.Error(context, err)
			return
		}
		for index, item := range *input.Items {
			severity := item.Severity
			if severity != "high" && severity != "medium" && severity != "low" {
				severity = "high"
			}
			// 类目归属已在保存前校验（迁移 000011：chapter 列已删除，策略挂 category_id）。
			if _, err = queries.CreateBaselineItem(context, db.CreateBaselineItemParams{
				CreateTime: now, UpdateTime: now, BaselineID: id, CategoryID: item.CategoryID, Sort: uint32(index),
				Name: strings.TrimSpace(item.Name), Description: item.Description, Config: item.Config, Severity: severity,
			}); err != nil {
				response.Error(context, err)
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": id})
}

func (handler *Handler) DeleteBaseline(context *gin.Context) {
	id := pathID(context)
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	queries := db.New(tx)
	// 扫描历史随基线一并删除（外键未设级联，须先删子表：明细 → 目标 → 扫描记录）。
	if err = queries.DeleteBaselineScanResults(context, id); err != nil {
		response.Error(context, err)
		return
	}
	if err = queries.DeleteBaselineScanTargets(context, id); err != nil {
		response.Error(context, err)
		return
	}
	if err = queries.DeleteBaselineScans(context, id); err != nil {
		response.Error(context, err)
		return
	}
	if err = queries.DeleteBaselineItems(context, id); err != nil {
		response.Error(context, err)
		return
	}
	affected, err := queries.DeleteBaseline(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	if affected == 0 {
		response.BusinessError(context, 404, "基线不存在", nil)
		return
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, nil)
}

// ---- 类目（策略分组，迁移 000011 起）----

// CreateBaselineCategory POST /sys/security/baseline/{id}/categories/  {name}
func (handler *Handler) CreateBaselineCategory(context *gin.Context) {
	baselineID := pathID(context)
	var input struct {
		Name string `json:"name"`
	}
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		response.BusinessError(context, 400, "类目名称不能为空", nil)
		return
	}
	queries := db.New(handler.db)
	if _, err := queries.GetBaselineForScan(context, baselineID); err != nil {
		if isNoRows(err) {
			response.BusinessError(context, 404, "基线不存在", nil)
			return
		}
		response.Error(context, err)
		return
	}
	maxSort, err := queries.MaxBaselineCategorySort(context, baselineID)
	if err != nil {
		response.Error(context, err)
		return
	}
	now := time.Now().UTC()
	categoryID, err := queries.CreateBaselineCategory(context, db.CreateBaselineCategoryParams{
		CreateTime: now, UpdateTime: now, Name: name, Sort: uint32(maxSort + 1), BaselineID: baselineID,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": categoryID, "name": name, "sort": maxSort + 1})
}

// UpdateBaselineCategory PATCH /sys/security/baseline/categories/{id}/  {name} 或 {direction: "up"|"down"}
func (handler *Handler) UpdateBaselineCategory(context *gin.Context) {
	categoryID := pathParamID(context, "categoryId")
	var input struct {
		Name      *string `json:"name"`
		Direction string  `json:"direction"`
	}
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	queries := db.New(handler.db)
	current, err := queries.GetBaselineCategory(context, categoryID)
	if isNoRows(err) {
		response.BusinessError(context, 404, "类目不存在", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	baselineID := current.BaselineID
	now := time.Now().UTC()
	switch {
	case input.Name != nil:
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			response.BusinessError(context, 400, "类目名称不能为空", nil)
			return
		}
		if _, err := queries.RenameBaselineCategory(context, db.RenameBaselineCategoryParams{Name: name, UpdateTime: now, ID: categoryID}); err != nil {
			response.Error(context, err)
			return
		}
		response.Success(context, gin.H{"id": categoryID, "name": name})
	case input.Direction == "up" || input.Direction == "down":
		rows, err := queries.ListBaselineCategories(context, baselineID)
		if err != nil {
			response.Error(context, err)
			return
		}
		ids := make([]int64, 0, len(rows))
		sorts := make([]int64, 0, len(rows))
		selfIndex := -1
		for _, row := range rows {
			ids = append(ids, row.ID)
			sorts = append(sorts, int64(row.Sort))
			if row.ID == categoryID {
				selfIndex = len(ids) - 1
			}
		}
		if selfIndex < 0 {
			response.BusinessError(context, 404, "类目不存在", nil)
			return
		}
		neighbor := selfIndex - 1 // up
		if input.Direction == "down" {
			neighbor = selfIndex + 1 // down
		}
		if neighbor < 0 || neighbor >= len(ids) {
			response.BusinessError(context, 400, "类目已在边界，无法移动", nil)
			return
		}
		// 交换与相邻类目的 sort 值。
		if _, err := queries.UpdateBaselineCategorySort(context, db.UpdateBaselineCategorySortParams{Sort: uint32(sorts[neighbor]), UpdateTime: now, ID: ids[selfIndex]}); err != nil {
			response.Error(context, err)
			return
		}
		if _, err := queries.UpdateBaselineCategorySort(context, db.UpdateBaselineCategorySortParams{Sort: uint32(sorts[selfIndex]), UpdateTime: now, ID: ids[neighbor]}); err != nil {
			response.Error(context, err)
			return
		}
		response.Success(context, nil)
	default:
		response.BusinessError(context, 400, "请提交 name 或 direction（up/down）", nil)
	}
}

// DeleteBaselineCategory DELETE /sys/security/baseline/categories/{id}/ —— 非空禁止删除。
func (handler *Handler) DeleteBaselineCategory(context *gin.Context) {
	categoryID := pathParamID(context, "categoryId")
	queries := db.New(handler.db)
	itemCount, err := queries.CountBaselineItemsByCategory(context, categoryID)
	if err != nil {
		response.Error(context, err)
		return
	}
	if itemCount > 0 {
		response.BusinessError(context, 400, fmt.Sprintf("类目下还有 %d 个策略，请先移走或删除", itemCount), nil)
		return
	}
	affected, err := queries.DeleteBaselineCategory(context, categoryID)
	if err != nil {
		response.Error(context, err)
		return
	}
	if affected == 0 {
		response.BusinessError(context, 404, "类目不存在", nil)
		return
	}
	response.Success(context, nil)
}

// ---- 策略单条 CRUD（弹窗确认即落库，无"保存策略"批量语义）----

// validateItemPayload 校验单条策略输入：infraErr 非 nil 为基础设施错误，
// 否则 bizMessage 非 "" 为面向用户的 400 文案，两者都空表示通过（severity 已归一）。
func (handler *Handler) validateItemPayload(context *gin.Context, baselineID, categoryID int64, name string, config json.RawMessage, severity string) (normalizedSeverity, bizMessage string, infraErr error) {
	if strings.TrimSpace(name) == "" {
		return "", "策略名称不能为空", nil
	}
	if categoryID <= 0 {
		return "", "策略缺少类目", nil
	}
	category, err := db.New(handler.db).GetBaselineCategory(context, categoryID)
	if err != nil && !isNoRows(err) {
		return "", "", err
	}
	if isNoRows(err) || category.BaselineID != baselineID {
		return "", "类目不存在或不属于当前基线", nil
	}
	var opaConfig map[string]any
	if err := json.Unmarshal(config, &opaConfig); err != nil {
		return "", "config 必须是对象", nil
	}
	if message := opapolicy.Validate(opaConfig); message != "" {
		return "", message, nil
	}
	if severity != "high" && severity != "medium" && severity != "low" {
		severity = "high"
	}
	return severity, "", nil
}

// AddBaselineItem POST /sys/security/baseline/{id}/items/ —— 弹窗确认即落库。
func (handler *Handler) AddBaselineItem(context *gin.Context) {
	baselineID := pathID(context)
	var input baselineItemInput
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	severity, message, infraErr := handler.validateItemPayload(context, baselineID, input.CategoryID, input.Name, input.Config, input.Severity)
	if infraErr != nil {
		response.Error(context, infraErr)
		return
	}
	if message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	sort, err := db.New(handler.db).MaxBaselineItemSort(context, input.CategoryID)
	if err != nil {
		response.Error(context, err)
		return
	}
	now := time.Now().UTC()
	itemID, err := db.New(handler.db).CreateBaselineItem(context, db.CreateBaselineItemParams{
		CreateTime: now, UpdateTime: now, BaselineID: baselineID, CategoryID: input.CategoryID, Sort: uint32(sort + 1),
		Name: strings.TrimSpace(input.Name), Description: input.Description, Config: input.Config, Severity: severity,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": itemID})
}

// UpdateBaselineItem PATCH /sys/security/baseline/{id}/items/{itemId}/ —— 弹窗确认即落库。
func (handler *Handler) UpdateBaselineItem(context *gin.Context) {
	baselineID := pathID(context)
	itemID := pathParamID(context, "itemId")
	var input baselineItemInput
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	queries := db.New(handler.db)
	current, err := queries.GetBaselineItem(context, itemID)
	if isNoRows(err) {
		response.BusinessError(context, 404, "策略不存在", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if current.BaselineID != baselineID {
		response.BusinessError(context, 404, "策略不存在", nil)
		return
	}
	severity, message, infraErr := handler.validateItemPayload(context, baselineID, input.CategoryID, input.Name, input.Config, input.Severity)
	if infraErr != nil {
		response.Error(context, infraErr)
		return
	}
	if message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	if _, err := queries.UpdateBaselineItem(context, db.UpdateBaselineItemParams{
		CategoryID: input.CategoryID, Name: strings.TrimSpace(input.Name), Description: input.Description,
		Config: input.Config, Severity: severity, UpdateTime: time.Now().UTC(), ID: itemID,
	}); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": itemID})
}

// DeleteBaselineItem DELETE /sys/security/baseline/{id}/items/{itemId}/ —— 即时删除。
func (handler *Handler) DeleteBaselineItem(context *gin.Context) {
	baselineID := pathID(context)
	itemID := pathParamID(context, "itemId")
	affected, err := db.New(handler.db).DeleteBaselineItem(context, db.DeleteBaselineItemParams{ID: itemID, BaselineID: baselineID})
	if err != nil {
		response.Error(context, err)
		return
	}
	if affected == 0 {
		response.BusinessError(context, 404, "策略不存在", nil)
		return
	}
	response.Success(context, nil)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func jsonBytes(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}
