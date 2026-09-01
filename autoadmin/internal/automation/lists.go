package automation

import (
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

type listSpec struct {
	from      string
	search    []string
	idFilter  string
	selectSQL string
	orderBy   string
}

func (handler *Handler) ListInventories(context *gin.Context) {
	items, count, page, size, ok := handler.listRows(context, listSpec{from: "automation_inventory a", search: []string{"a.name", "a.remark"}, selectSQL: "a.*", orderBy: "a.id DESC"})
	if !ok {
		return
	}
	for _, item := range items {
		handler.decorateInventory(context, item)
	}
	automationPaginated(context, items, count, page, size)
}

func (handler *Handler) ListTasks(context *gin.Context) {
	spec := listSpec{from: "automation_task a LEFT JOIN automation_playbook_template p ON p.id=a.playbook_template_id LEFT JOIN automation_inventory i ON i.id=a.inventory_id", search: []string{"a.name", "p.name", "i.name", "a.remark"}, idFilter: "task_id", selectSQL: "a.*,COALESCE(p.name,'') AS raw_template_name,COALESCE(i.name,'') AS inventory_name", orderBy: "a.id DESC"}
	items, count, page, size, ok := handler.listRows(context, spec)
	if !ok {
		return
	}
	for _, item := range items {
		item["playbook_template"] = item["playbook_template_id"]
		item["inventory"] = item["inventory_id"]
		templateName, _ := item["raw_template_name"].(string)
		if templateName != "" {
			item["template_name"] = "[Playbook] " + templateName
		} else {
			item["template_name"] = ""
		}
		delete(item, "raw_template_name")
	}
	automationPaginated(context, items, count, page, size)
}

func (handler *Handler) ListJobs(context *gin.Context) {
	spec := listSpec{from: "automation_execution_job a", search: []string{"a.requested_username", "a.template_name_snapshot", "a.task_name_snapshot", "a.remark"}, idFilter: "job_id", selectSQL: "a.*", orderBy: "a.id DESC"}
	items, count, page, size, ok := handler.listRows(context, spec)
	if !ok {
		return
	}
	for _, item := range items {
		item["job_id"] = item["id"]
		item["template_name"] = item["template_name_snapshot"]
		item["task_name"] = item["task_name_snapshot"]
	}
	automationPaginated(context, items, count, page, size)
}

func (handler *Handler) listRows(context *gin.Context, spec listSpec) ([]gin.H, int64, int, int, bool) {
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
	clauses := []string{"1=1"}
	arguments := make([]any, 0)
	if exactID := strings.TrimSpace(context.Query(spec.idFilter)); spec.idFilter != "" && exactID != "" {
		if _, err := strconv.ParseInt(exactID, 10, 64); err == nil {
			clauses = append(clauses, "a.id=?")
			arguments = append(arguments, exactID)
		}
	}
	search := strings.TrimSpace(context.Query("search"))
	if search == "" {
		search = strings.TrimSpace(context.Query("keyword"))
	}
	if search != "" && len(spec.search) > 0 {
		parts := make([]string, len(spec.search))
		pattern := "%" + search + "%"
		for index, column := range spec.search {
			parts[index] = column + " LIKE ?"
			arguments = append(arguments, pattern)
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}
	if status := strings.TrimSpace(context.Query("status")); status != "" && strings.Contains(spec.from, "execution_job") {
		clauses = append(clauses, "a.status=?")
		arguments = append(arguments, status)
	}
	if taskID := strings.TrimSpace(context.Query("task_id")); taskID != "" && strings.Contains(spec.from, "execution_job") {
		clauses = append(clauses, "a.task_id=?")
		arguments = append(arguments, taskID)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var count int64
	if err := handler.db.QueryRowContext(context, "SELECT COUNT(DISTINCT a.id) FROM "+spec.from+where, arguments...).Scan(&count); err != nil {
		response.Error(context, err)
		return nil, 0, page, size, false
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, "SELECT "+spec.selectSQL+" FROM "+spec.from+where+" ORDER BY "+spec.orderBy+" LIMIT ? OFFSET ?", queryArguments...)
	if err != nil {
		response.Error(context, err)
		return nil, 0, page, size, false
	}
	items, err := scanAutomationRows(rows)
	if err != nil {
		response.Error(context, err)
		return nil, 0, page, size, false
	}
	return items, count, page, size, true
}

func (handler *Handler) decorateInventory(context *gin.Context, item gin.H) {
	hostIDs := intSlice(item["selected_host_ids"])
	if len(hostIDs) == 0 {
		item["scope_summary"] = gin.H{"label": "0组 / 0台主机", "group_count": 0, "host_count": 0, "is_empty_scope": true}
		item["health_status"] = gin.H{"status": "empty", "label": "空范围", "message": "当前 Inventory 无可用主机"}
		item["resolved_host_count"] = 0
		return
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(hostIDs)), ",")
	arguments := make([]any, len(hostIDs))
	for index, id := range hostIDs {
		arguments[index] = id
	}
	var existing, resolved, groups int
	handler.db.QueryRowContext(context, "SELECT COUNT(*),COALESCE(SUM(ip IS NOT NULL),0),COUNT(DISTINCT group_id) FROM assets_host WHERE id IN ("+placeholders+")", arguments...).Scan(&existing, &resolved, &groups)
	item["scope_summary"] = gin.H{"label": strconv.Itoa(groups) + "组 / " + strconv.Itoa(resolved) + "台主机", "group_count": groups, "host_count": resolved, "is_empty_scope": false}
	item["resolved_host_count"] = resolved
	if existing < len(hostIDs) {
		item["health_status"] = gin.H{"status": "invalid", "label": "范围失效", "message": "存在已删除主机"}
	} else if resolved == 0 {
		item["health_status"] = gin.H{"status": "empty", "label": "空范围", "message": "当前 Inventory 无可用主机"}
	} else {
		item["health_status"] = gin.H{"status": "healthy", "label": "正常", "message": "当前可执行主机 " + strconv.Itoa(resolved) + " 台"}
	}
}

func scanAutomationRows(rows *sql.Rows) ([]gin.H, error) {
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	items := make([]gin.H, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err = rows.Scan(destinations...); err != nil {
			return nil, err
		}
		item := gin.H{}
		for index, column := range columns {
			item[column] = normalizeAutomationValue(column, values[index])
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func normalizeAutomationValue(column string, value any) any {
	if column == "enabled" || column == "update_on_launch" {
		return boolValue(value)
	}
	bytes, ok := value.([]byte)
	if !ok {
		return value
	}
	if column == "selected_host_ids" || column == "env_vars" || column == "nodes" || column == "edges" || column == "default_extra_vars" || column == "inventory_snapshot" || column == "extra_vars" || column == "result_summary" {
		var decoded any
		if json.Unmarshal(bytes, &decoded) == nil {
			return decoded
		}
	}
	return string(bytes)
}
func intSlice(value any) []int64 {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]int64, 0, len(raw))
	for _, item := range raw {
		if number, ok := item.(float64); ok {
			result = append(result, int64(number))
		}
	}
	return result
}
func jsonArrayLength(value any) int {
	items, ok := value.([]any)
	if !ok {
		return 0
	}
	return len(items)
}
func automationPaginated(context *gin.Context, items []gin.H, count int64, page, size int) {
	total := int64(0)
	if count > 0 {
		total = int64(math.Ceil(float64(count) / float64(size)))
	}
	response.Success(context, gin.H{"results": items, "count": count, "pageNumber": page, "pageSize": size, "totalPages": total, "next": nil, "previous": nil})
}
