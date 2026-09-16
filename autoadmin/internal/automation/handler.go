package automation

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type Handler struct {
	db      *sql.DB
	gateway *agent.Gateway
}

func NewHandler(db *sql.DB, gateway *agent.Gateway) *Handler {
	return &Handler{db: db, gateway: gateway}
}

type Playbook struct {
	ID          int64          `json:"id"`
	CreateTime  time.Time      `json:"create_time"`
	UpdateTime  time.Time      `json:"update_time"`
	Remark      sql.NullString `json:"remark"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Content     string         `json:"content"`
	Category    string         `json:"category"`
}
type playbookInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Category    string `json:"category"`
	Remark      string `json:"remark"`
}

func (handler *Handler) List(context *gin.Context) {
	page, _ := strconv.Atoi(context.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(context.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 30 {
		size = 30
	}
	// 搜索：空串传 NULL，查询里是 `LIKE narg(pattern) OR narg(pattern) IS NULL` 的"不过滤"分支。
	search := strings.TrimSpace(context.Query("search"))
	category := strings.TrimSpace(context.Query("category"))
	pattern := sql.NullString{}
	if search != "" {
		pattern = sql.NullString{String: "%" + search + "%", Valid: true}
	}
	// 排序参数走白名单：合法值原样（含 '-' 前缀表示倒序）交给 SQL 的 CASE 表达式，
	// 非法值回落到默认的 `id DESC`（sort_key 传 NULL）。
	rawOrder := context.DefaultQuery("ordering", "-id")
	var sortKey any
	if _, valid := playbookSortKeys[strings.TrimPrefix(rawOrder, "-")]; valid {
		sortKey = rawOrder
	}
	queries := db.New(handler.db)
	params := db.CountAutomationPlaybooksParams{Pattern: pattern, Category: nullIfEmpty(category)}
	count, err := queries.CountAutomationPlaybooks(context, params)
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListAutomationPlaybooks(context, db.ListAutomationPlaybooksParams{
		Pattern: params.Pattern, Category: params.Category, SortKey: sortKey,
		Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]Playbook, 0, len(rows))
	for _, row := range rows {
		items = append(items, Playbook{
			ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark,
			Name: row.Name, Description: row.Description, Content: row.Content, Category: row.Category,
		})
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

// playbookSortKeys 是可排序的列白名单（与前端表头一一对应）。
var playbookSortKeys = map[string]bool{"id": true, "name": true, "create_time": true, "update_time": true}

// optionalString 把空串转成 NULL（清单里的分类过滤用"NULL 表示不过滤"）。
func nullIfEmpty(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func validatePlaybook(content string) error {
	var plays []any
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("Playbook content cannot be empty")
	}
	if err := yaml.Unmarshal([]byte(content), &plays); err != nil {
		return fmt.Errorf("Playbook YAML syntax error: %w", err)
	}
	if len(plays) == 0 {
		return fmt.Errorf("Playbook YAML must be a nonempty list of plays")
	}
	return nil
}

// playbookCategoryAgent 是 Agent 安装专用模板分类：它是 Agent 安装/更新功能的唯一配置源
// （assets 包按 category 定位读取），只允许通过种子 SQL 落库并修改内容，
// 禁止在模板管理中新建、删除或改回其他分类。
const playbookCategoryAgent = "agent"

func bindPlaybook(context *gin.Context) (playbookInput, bool) {
	var input playbookInput
	if context.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Name) == "" || validatePlaybook(input.Content) != nil {
		context.JSON(200, gin.H{"code": 400, "msg": "Playbook 名称或内容无效", "data": nil})
		return input, false
	}
	if input.Category == "" {
		input.Category = "general"
	}
	if input.Category != "general" && input.Category != "software_package" && input.Category != playbookCategoryAgent {
		context.JSON(200, gin.H{"code": 400, "msg": "category 无效", "data": nil})
		return input, false
	}
	return input, true
}

func (handler *Handler) playbookCategoryByID(context *gin.Context, id int64) (string, bool) {
	row, err := db.New(handler.db).GetAutomationPlaybook(context, id)
	if err != nil {
		response.Error(context, err)
		return "", false
	}
	return row.Category, true
}

func (handler *Handler) Create(context *gin.Context) {
	input, ok := bindPlaybook(context)
	if !ok {
		return
	}
	if input.Category == playbookCategoryAgent {
		context.JSON(200, gin.H{"code": 400, "msg": "Agent 安装专用模板由系统种子数据维护，禁止手动新建", "data": nil})
		return
	}
	now := time.Now().UTC()
	id, err := db.New(handler.db).CreateAutomationPlaybook(context, db.CreateAutomationPlaybookParams{
		CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name),
		Description: input.Description, Content: input.Content, Category: input.Category,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	handler.GetByID(context, id)
}

func (handler *Handler) Update(context *gin.Context) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil {
		response.Error(context, err)
		return
	}
	input, ok := bindPlaybook(context)
	if !ok {
		return
	}
	if category, exists := handler.playbookCategoryByID(context, id); !exists {
		return
	} else if category == playbookCategoryAgent && input.Category != playbookCategoryAgent {
		context.JSON(200, gin.H{"code": 400, "msg": "该模板是 Agent 安装/更新的唯一配置源，无法改为其他分类", "data": nil})
		return
	}
	err = db.New(handler.db).UpdateAutomationPlaybook(context, db.UpdateAutomationPlaybookParams{
		UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name),
		Description: input.Description, Content: input.Content, Category: input.Category, ID: id,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	handler.GetByID(context, id)
}

func (handler *Handler) Get(context *gin.Context) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil {
		response.Error(context, err)
		return
	}
	handler.GetByID(context, id)
}
func (handler *Handler) GetByID(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetAutomationPlaybook(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, Playbook{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark,
		Name: row.Name, Description: row.Description, Content: row.Content, Category: row.Category,
	})
}
func (handler *Handler) Validate(context *gin.Context) {
	var input struct {
		Content string `json:"content"`
	}
	if context.ShouldBindJSON(&input) != nil {
		context.JSON(200, gin.H{"code": 400, "msg": "请求参数错误", "data": nil})
		return
	}
	if err := validatePlaybook(input.Content); err != nil {
		context.JSON(200, gin.H{"code": 400, "msg": err.Error(), "data": nil})
		return
	}
	response.Success(context, gin.H{"valid": true})
}
func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
