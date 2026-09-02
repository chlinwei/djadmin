package automation

import (
	"database/sql"
	"math"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func automationPagination(context *gin.Context) (int, int) {
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
	return page, size
}

func automationSearchPattern(context *gin.Context) sql.NullString {
	search := strings.TrimSpace(context.Query("search"))
	if search == "" {
		search = strings.TrimSpace(context.Query("keyword"))
	}
	if search == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: "%" + search + "%", Valid: true}
}

func automationOptionalInt64(context *gin.Context, name string) sql.NullInt64 {
	value := strings.TrimSpace(context.Query(name))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if value == "" || err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: parsed, Valid: true}
}

func automationOptionalString(context *gin.Context, name string) sql.NullString {
	value := strings.TrimSpace(context.Query(name))
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func (handler *Handler) ListInventories(context *gin.Context) {
	page, size := automationPagination(context)
	queries := db.New(handler.db)
	pattern := automationSearchPattern(context)
	count, err := queries.CountInventories(context, db.CountInventoriesParams{Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListInventoriesTyped(context, db.ListInventoriesTypedParams{Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := inventoryRowToMap(row)
		handler.decorateInventory(context, item)
		items = append(items, item)
	}
	automationPaginated(context, items, count, page, size)
}

func (handler *Handler) ListTasks(context *gin.Context) {
	page, size := automationPagination(context)
	queries := db.New(handler.db)
	idFilter, pattern := automationOptionalInt64(context, "task_id"), automationSearchPattern(context)
	count, err := queries.CountTasks(context, db.CountTasksParams{ID: idFilter, Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListTasksTyped(context, db.ListTasksTypedParams{ID: idFilter, Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := taskRowToMapFromList(row)
		item["playbook_template"] = item["playbook_template_id"]
		item["inventory"] = item["inventory_id"]
		templateName, _ := item["raw_template_name"].(string)
		if templateName != "" {
			item["template_name"] = "[Playbook] " + templateName
		} else {
			item["template_name"] = ""
		}
		delete(item, "raw_template_name")
		items = append(items, item)
	}
	automationPaginated(context, items, count, page, size)
}

func (handler *Handler) ListJobs(context *gin.Context) {
	page, size := automationPagination(context)
	queries := db.New(handler.db)
	idFilter, pattern := automationOptionalInt64(context, "job_id"), automationSearchPattern(context)
	status, taskID := automationOptionalString(context, "status"), automationOptionalInt64(context, "task_id")
	count, err := queries.CountJobs(context, db.CountJobsParams{ID: idFilter, Status: status, TaskID: taskID, Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListJobsTyped(context, db.ListJobsTypedParams{ID: idFilter, Status: status, TaskID: taskID, Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := jobRowToMap(row)
		item["job_id"] = item["id"]
		item["template_name"] = item["template_name_snapshot"]
		item["task_name"] = item["task_name_snapshot"]
		items = append(items, item)
	}
	automationPaginated(context, items, count, page, size)
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
