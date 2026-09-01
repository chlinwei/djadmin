package automation

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/response"

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
	search, category := strings.TrimSpace(context.Query("search")), strings.TrimSpace(context.Query("category"))
	pattern := "%" + search + "%"
	where := ` WHERE (?='' OR name LIKE ? OR description LIKE ? OR COALESCE(remark,'') LIKE ?) AND (?='' OR category=?)`
	var count int64
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM automation_playbook_template`+where, search, pattern, pattern, pattern, category, category).Scan(&count); err != nil {
		response.Error(context, err)
		return
	}
	orders := map[string]string{"id": "id", "name": "name", "create_time": "create_time", "update_time": "update_time"}
	rawOrder := context.DefaultQuery("ordering", "-id")
	direction := "ASC"
	key := rawOrder
	if strings.HasPrefix(rawOrder, "-") {
		direction = "DESC"
		key = strings.TrimPrefix(rawOrder, "-")
	}
	column, ok := orders[key]
	if !ok {
		column = "id"
		direction = "DESC"
	}
	rows, err := handler.db.QueryContext(context, `SELECT id,create_time,update_time,remark,name,description,content,category FROM automation_playbook_template`+where+` ORDER BY `+column+` `+direction+` LIMIT ? OFFSET ?`, search, pattern, pattern, pattern, category, category, size, (page-1)*size)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	items := make([]Playbook, 0)
	for rows.Next() {
		var item Playbook
		if err = rows.Scan(&item.ID, &item.CreateTime, &item.UpdateTime, &item.Remark, &item.Name, &item.Description, &item.Content, &item.Category); err != nil {
			response.Error(context, err)
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	response.Paginated(context, items, count, int32(page), int32(size))
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

func bindPlaybook(context *gin.Context) (playbookInput, bool) {
	var input playbookInput
	if context.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Name) == "" || validatePlaybook(input.Content) != nil {
		context.JSON(200, gin.H{"code": 400, "msg": "Playbook 名称或内容无效", "data": nil})
		return input, false
	}
	if input.Category == "" {
		input.Category = "general"
	}
	if input.Category != "general" && input.Category != "software_package" {
		context.JSON(200, gin.H{"code": 400, "msg": "category 无效", "data": nil})
		return input, false
	}
	return input, true
}

func (handler *Handler) Create(context *gin.Context) {
	input, ok := bindPlaybook(context)
	if !ok {
		return
	}
	now := time.Now().UTC()
	result, err := handler.db.ExecContext(context, `INSERT INTO automation_playbook_template(create_time,update_time,remark,name,description,content,category) VALUES(?,?,?,?,?,?,?)`, now, now, nullString(input.Remark), strings.TrimSpace(input.Name), input.Description, input.Content, input.Category)
	if err != nil {
		response.Error(context, err)
		return
	}
	id, _ := result.LastInsertId()
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
	_, err = handler.db.ExecContext(context, `UPDATE automation_playbook_template SET update_time=?,remark=?,name=?,description=?,content=?,category=? WHERE id=?`, time.Now().UTC(), nullString(input.Remark), strings.TrimSpace(input.Name), input.Description, input.Content, input.Category, id)
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
	var item Playbook
	err := handler.db.QueryRowContext(context, `SELECT id,create_time,update_time,remark,name,description,content,category FROM automation_playbook_template WHERE id=?`, id).Scan(&item.ID, &item.CreateTime, &item.UpdateTime, &item.Remark, &item.Name, &item.Description, &item.Content, &item.Category)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, item)
}
func (handler *Handler) Delete(context *gin.Context) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err == nil {
		_, err = handler.db.ExecContext(context, `DELETE FROM automation_playbook_template WHERE id=?`, id)
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": id})
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
