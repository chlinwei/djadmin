package baseline

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
	rows, err := handler.db.QueryContext(context, `SELECT b.id, b.name, b.version, b.description, b.enabled, b.create_time, b.update_time,
       (SELECT COUNT(*) FROM baseline_item i WHERE i.baseline_id = b.id) AS item_count,
       (SELECT COUNT(*) FROM security_scan sc WHERE sc.baseline_id = b.id) AS scan_count
FROM baseline b
WHERE (? = '' OR b.name LIKE ? OR b.description LIKE ?)
ORDER BY b.id DESC LIMIT ? OFFSET ?`, search, "%"+search+"%", "%"+search+"%", pageSize, 0)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var name, version, description string
		var enabled bool
		var createTime, updateTime time.Time
		var itemCount, scanCount int64
		if err := rows.Scan(&id, &name, &version, &description, &enabled, &createTime, &updateTime, &itemCount, &scanCount); err != nil {
			response.Error(context, err)
			return
		}
		items = append(items, gin.H{"id": id, "name": name, "version": version, "description": description,
			"enabled": enabled, "item_count": itemCount, "scan_count": scanCount, "create_time": createTime, "update_time": updateTime})
	}
	response.Paginated(context, items, int64(len(items)), int32(pageNumber), int32(pageSize))
}

func (handler *Handler) GetBaseline(context *gin.Context) {
	var id int64
	fmt.Sscanf(strings.TrimSpace(context.Param("id")), "%d", &id)
	var name, version, description string
	var enabled bool
	var createTime, updateTime time.Time
	err := handler.db.QueryRowContext(context, `SELECT name,version,description,enabled,create_time,update_time FROM baseline WHERE id=?`, id).
		Scan(&name, &version, &description, &enabled, &createTime, &updateTime)
	if isNoRows(err) {
		response.BusinessError(context, 404, "基线不存在", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	categories := make([]gin.H, 0)
	categoryRows, categoryErr := handler.db.QueryContext(context, `SELECT id, sort, name FROM baseline_category WHERE baseline_id=? ORDER BY sort, id`, id)
	if categoryErr != nil {
		response.Error(context, categoryErr)
		return
	}
	defer categoryRows.Close()
	categoryNames := make(map[int64]string)
	for categoryRows.Next() {
		var categoryID, sort int64
		var categoryName string
		if categoryRows.Scan(&categoryID, &sort, &categoryName) == nil {
			categoryNames[categoryID] = categoryName
			categories = append(categories, gin.H{"id": categoryID, "sort": sort, "name": categoryName})
		}
	}
	items := make([]gin.H, 0)
	itemRows, itemErr := handler.db.QueryContext(context, `SELECT id,category_id,sort,name,description,config,severity FROM baseline_item WHERE baseline_id=? ORDER BY sort,id`, id)
	if itemErr != nil {
		response.Error(context, itemErr)
		return
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var itemID, categoryID, sort int64
		var itemName, severity string
		var itemDescription string
		var config []byte
		if itemRows.Scan(&itemID, &categoryID, &sort, &itemName, &itemDescription, &config, &severity) == nil {
			var configDecoded any
			_ = json.Unmarshal(config, &configDecoded)
			items = append(items, gin.H{"id": itemID, "category_id": categoryID, "category": categoryNames[categoryID], "sort": sort, "name": itemName,
				"description": itemDescription, "config": configDecoded, "severity": severity})
		}
	}
	response.Success(context, gin.H{"baseline": gin.H{"id": id, "name": name, "version": version,
		"description": description, "enabled": enabled, "create_time": createTime, "update_time": updateTime},
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
		categoryIDArgs := make([]any, 0, len(*input.Items))
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
			categoryIDArgs = append(categoryIDArgs, item.CategoryID)
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
		if id > 0 && len(categoryIDs) > 0 {
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(categoryIDs)), ",")
			args := make([]any, 0, len(categoryIDArgs)+1)
			args = append(args, id)
			args = append(args, categoryIDArgs...)
			rows, err := handler.db.QueryContext(context, `SELECT COUNT(*) FROM baseline_category WHERE baseline_id=? AND id IN (`+placeholders+`)`, args...)
			if err != nil {
				response.Error(context, err)
				return
			}
			var matched int64
			if rows.Next() {
				if err := rows.Scan(&matched); err != nil {
					rows.Close()
					response.Error(context, err)
					return
				}
			}
			rows.Close()
			if int(matched) != len(categoryIDs) {
				response.BusinessError(context, 400, "策略的类目不存在或不属于当前基线", nil)
				return
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
	if id == 0 {
		result, execErr := tx.ExecContext(context, `INSERT INTO baseline(create_time,update_time,name,version,description,enabled) VALUES(NOW(6),NOW(6),?,?,?,?)`, name, version, description, enabled)
		if execErr != nil {
			response.BusinessError(context, 400, "基线名称已存在", nil)
			return
		}
		id, err = result.LastInsertId()
	} else {
		_, err = tx.ExecContext(context, `UPDATE baseline SET name=?,version=?,description=?,enabled=?,update_time=NOW(6) WHERE id=?`, name, version, description, enabled, id)
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if input.Items != nil {
		if _, err = tx.ExecContext(context, `DELETE FROM baseline_item WHERE baseline_id=?`, id); err != nil {
			response.Error(context, err)
			return
		}
		for index, item := range *input.Items {
			severity := item.Severity
			if severity != "high" && severity != "medium" && severity != "low" {
				severity = "high"
			}
			// 类目归属已在保存前校验（迁移 000011：chapter 列已删除，策略挂 category_id）。
			if _, err = tx.ExecContext(context, `INSERT INTO baseline_item(create_time,update_time,baseline_id,category_id,sort,name,description,config,severity) VALUES(NOW(6),NOW(6),?,?,?,?,?,?,?)`,
				id, item.CategoryID, index, strings.TrimSpace(item.Name), item.Description, jsonBytes(item.Config), severity); err != nil {
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
	if _, err = tx.ExecContext(context, `DELETE FROM baseline_item WHERE baseline_id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	result, err := tx.ExecContext(context, `DELETE FROM baseline WHERE id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
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
	var exists int
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM baseline WHERE id=?`, baselineID).Scan(&exists); err != nil || exists == 0 {
		response.BusinessError(context, 404, "基线不存在", nil)
		return
	}
	var maxSort int64
	if err := handler.db.QueryRowContext(context, `SELECT COALESCE(MAX(sort), -1) FROM baseline_category WHERE baseline_id=?`, baselineID).Scan(&maxSort); err != nil {
		response.Error(context, err)
		return
	}
	result, err := handler.db.ExecContext(context, `INSERT INTO baseline_category(create_time,update_time,name,sort,baseline_id) VALUES(NOW(6),NOW(6),?,?,?)`, name, maxSort+1, baselineID)
	if err != nil {
		response.Error(context, err)
		return
	}
	categoryID, _ := result.LastInsertId()
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
	var baselineID, sort int64
	var currentName string
	err := handler.db.QueryRowContext(context, `SELECT baseline_id, sort, name FROM baseline_category WHERE id=?`, categoryID).Scan(&baselineID, &sort, &currentName)
	if isNoRows(err) {
		response.BusinessError(context, 404, "类目不存在", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	switch {
	case input.Name != nil:
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			response.BusinessError(context, 400, "类目名称不能为空", nil)
			return
		}
		if _, err := handler.db.ExecContext(context, `UPDATE baseline_category SET name=?, update_time=NOW(6) WHERE id=?`, name, categoryID); err != nil {
			response.Error(context, err)
			return
		}
		response.Success(context, gin.H{"id": categoryID, "name": name})
	case input.Direction == "up" || input.Direction == "down":
		rows, err := handler.db.QueryContext(context, `SELECT id, sort FROM baseline_category WHERE baseline_id=? ORDER BY sort, id`, baselineID)
		if err != nil {
			response.Error(context, err)
			return
		}
		defer rows.Close()
		var ids []int64
		var sorts []int64
		selfIndex := -1
		for rows.Next() {
			var rowID, rowSort int64
			if err := rows.Scan(&rowID, &rowSort); err != nil {
				response.Error(context, err)
				return
			}
			ids = append(ids, rowID)
			sorts = append(sorts, rowSort)
			if rowID == categoryID {
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
		if _, err := handler.db.ExecContext(context, `UPDATE baseline_category SET sort=?, update_time=NOW(6) WHERE id=?`, sorts[neighbor], ids[selfIndex]); err != nil {
			response.Error(context, err)
			return
		}
		if _, err := handler.db.ExecContext(context, `UPDATE baseline_category SET sort=?, update_time=NOW(6) WHERE id=?`, sorts[selfIndex], ids[neighbor]); err != nil {
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
	var itemCount int64
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM baseline_item WHERE category_id=?`, categoryID).Scan(&itemCount); err != nil {
		response.Error(context, err)
		return
	}
	if itemCount > 0 {
		response.BusinessError(context, 400, fmt.Sprintf("类目下还有 %d 个策略，请先移走或删除", itemCount), nil)
		return
	}
	result, err := handler.db.ExecContext(context, `DELETE FROM baseline_category WHERE id=?`, categoryID)
	if err != nil {
		response.Error(context, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
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
	var belongs int64
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM baseline_category WHERE id=? AND baseline_id=?`, categoryID, baselineID).Scan(&belongs); err != nil {
		return "", "", err
	}
	if belongs == 0 {
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
	result, err := handler.db.ExecContext(context, `INSERT INTO baseline_item(create_time,update_time,baseline_id,category_id,sort,name,description,config,severity) VALUES(NOW(6),NOW(6),?,?,?,?,?,?,?)`,
		baselineID, input.CategoryID, sort+1, strings.TrimSpace(input.Name), input.Description, jsonBytes(input.Config), severity)
	if err != nil {
		response.Error(context, err)
		return
	}
	itemID, _ := result.LastInsertId()
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
	var ownerBaseline int64
	err := handler.db.QueryRowContext(context, `SELECT baseline_id FROM baseline_item WHERE id=?`, itemID).Scan(&ownerBaseline)
	if isNoRows(err) {
		response.BusinessError(context, 404, "策略不存在", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if ownerBaseline != baselineID {
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
	if _, err := handler.db.ExecContext(context, `UPDATE baseline_item SET category_id=?, name=?, description=?, config=?, severity=?, update_time=NOW(6) WHERE id=?`,
		input.CategoryID, strings.TrimSpace(input.Name), input.Description, jsonBytes(input.Config), severity, itemID); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": itemID})
}

// DeleteBaselineItem DELETE /sys/security/baseline/{id}/items/{itemId}/ —— 即时删除。
func (handler *Handler) DeleteBaselineItem(context *gin.Context) {
	baselineID := pathID(context)
	itemID := pathParamID(context, "itemId")
	result, err := handler.db.ExecContext(context, `DELETE FROM baseline_item WHERE id=? AND baseline_id=?`, itemID, baselineID)
	if err != nil {
		response.Error(context, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
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
