package automation

import (
	"context"
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
	// shellcheckDir 是平台上传的 shellcheck 二进制的存储目录（<media>/shellcheck）。
	// 校验时解析顺序：此目录下的 shellcheck → 系统 PATH；都缺失则引导上传或安装。
	shellcheckDir string
}

func NewHandler(db *sql.DB, gateway *agent.Gateway, shellcheckDir string) *Handler {
	return &Handler{db: db, gateway: gateway, shellcheckDir: shellcheckDir}
}

type Playbook struct {
	ID            int64          `json:"id"`
	CreateTime    time.Time      `json:"create_time"`
	UpdateTime    time.Time      `json:"update_time"`
	Remark        sql.NullString `json:"remark"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Content       string         `json:"content"`
	ContentFormat string         `json:"content_format"`
	Category      string         `json:"category"`
}
type playbookInput struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Content       string `json:"content"`
	ContentFormat string `json:"content_format"`
	Category      string `json:"category"`
	Remark        string `json:"remark"`
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
			Name: row.Name, Description: row.Description, Content: row.Content,
			ContentFormat: row.ContentFormat, Category: row.Category,
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

// playbookContentFormat 归一内容形态：缺省 = playbook（存量行/旧客户端语义不变）。
func playbookContentFormat(raw string) string {
	if strings.TrimSpace(raw) == playbookFormatShell {
		return playbookFormatShell
	}
	return playbookFormatPlaybook
}

// validatePlaybookContent 按形态分流校验。返回：
//   - fatal：不可保存的错误消息（空串 = 可保存）；
//   - warnings：shellcheck 的非 error 级告警（可保存，随响应带回前端黄条提示）。
//
// playbook 形态保持原有的 YAML 结构校验；shell 形态交给 shellcheck（error 级拒绝保存）。
func (handler *Handler) validatePlaybookContent(ctx context.Context, format, content string) (fatal string, warnings []shellcheckFinding) {
	if strings.TrimSpace(content) == "" {
		return "Playbook content cannot be empty", nil
	}
	if format != playbookFormatShell {
		return messageOrNil(validatePlaybook(content)), nil
	}
	findings, err := handler.validateShellScript(ctx, content)
	if err != nil {
		return err.Error(), nil
	}
	if fatal := fatalShellcheckFindings(findings); len(fatal) > 0 {
		messages := make([]string, 0, len(fatal))
		for _, finding := range fatal {
			messages = append(messages, fmt.Sprintf("line %d (SC%d): %s", finding.Line, finding.Code, finding.Message))
		}
		return "Shell 脚本存在语法错误，无法保存：" + strings.Join(messages, "；"), nil
	}
	return "", findings
}

func messageOrNil(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// playbookCategoryAgent 是 Agent 安装专用模板分类：它是 Agent 安装/更新功能的唯一配置源
// （assets 包按 category 定位读取），只允许通过种子 SQL 落库并修改内容，
// 禁止在模板管理中新建、删除或改回其他分类。
const playbookCategoryAgent = "agent"

func bindPlaybook(context *gin.Context) (playbookInput, bool) {
	var input playbookInput
	if context.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Name) == "" {
		context.JSON(200, gin.H{"code": 400, "msg": "Playbook 名称或内容无效", "data": nil})
		return input, false
	}
	input.ContentFormat = playbookContentFormat(input.ContentFormat)
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
	fatal, warnings := handler.validatePlaybookContent(context.Request.Context(), input.ContentFormat, input.Content)
	if fatal != "" {
		context.JSON(200, gin.H{"code": 400, "msg": fatal, "data": nil})
		return
	}
	now := time.Now().UTC()
	id, err := db.New(handler.db).CreateAutomationPlaybook(context, db.CreateAutomationPlaybookParams{
		CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name),
		Description: input.Description, Content: input.Content, ContentFormat: input.ContentFormat, Category: input.Category,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	handler.respondPlaybook(context, id, warnings)
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
	fatal, warnings := handler.validatePlaybookContent(context.Request.Context(), input.ContentFormat, input.Content)
	if fatal != "" {
		context.JSON(200, gin.H{"code": 400, "msg": fatal, "data": nil})
		return
	}
	err = db.New(handler.db).UpdateAutomationPlaybook(context, db.UpdateAutomationPlaybookParams{
		UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name),
		Description: input.Description, Content: input.Content, ContentFormat: input.ContentFormat,
		Category: input.Category, ID: id,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	handler.respondPlaybook(context, id, warnings)
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
	handler.respondPlaybook(context, id, nil)
}

// respondPlaybook 返回模板详情；warnings 非 nil 时（shellcheck 非 error 级告警）一并带回，
// 前端据此显示黄条提示。读取失败由本函数处理。
func (handler *Handler) respondPlaybook(context *gin.Context, id int64, warnings []shellcheckFinding) {
	row, err := db.New(handler.db).GetAutomationPlaybook(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	playbook := Playbook{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark,
		Name: row.Name, Description: row.Description, Content: row.Content,
		ContentFormat: row.ContentFormat, Category: row.Category,
	}
	if warnings == nil {
		response.Success(context, playbook)
		return
	}
	response.Success(context, gin.H{"playbook": playbook, "warnings": warnings})
}
func (handler *Handler) Validate(context *gin.Context) {
	var input struct {
		Content       string `json:"content"`
		ContentFormat string `json:"content_format"`
	}
	if context.ShouldBindJSON(&input) != nil {
		context.JSON(200, gin.H{"code": 400, "msg": "请求参数错误", "data": nil})
		return
	}
	format := playbookContentFormat(input.ContentFormat)
	fatal, warnings := handler.validatePlaybookContent(context.Request.Context(), format, input.Content)
	if fatal != "" {
		// data 里带 shellcheck 的逐条告警（含行号），前端据此在编辑器里定位。
		context.JSON(200, gin.H{"code": 400, "msg": fatal, "data": warnings})
		return
	}
	response.Success(context, gin.H{"valid": true, "warnings": warnings})
}
func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
