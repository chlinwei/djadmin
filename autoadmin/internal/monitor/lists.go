package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListTargets(context *gin.Context) {
	page, size := pagination(context)
	clauses := []string{"1=1"}
	arguments := make([]any, 0)
	for queryName, column := range map[string]string{"exporter_type": "t.exporter_type", "managed_enabled": "t.managed_enabled", "install_status": "t.install_status", "last_scrape_status": "t.last_scrape_status"} {
		if value := strings.TrimSpace(context.Query(queryName)); value != "" {
			clauses = append(clauses, column+"=?")
			arguments = append(arguments, value)
		}
	}
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		pattern := "%" + search + "%"
		clauses = append(clauses, "(h.instance_name LIKE ? OR h.ip LIKE ? OR t.exporter_type LIKE ?)")
		arguments = append(arguments, pattern, pattern, pattern)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM monitor_target t JOIN assets_host h ON h.id=t.host_id`+where, arguments)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT t.*, 'exporter' AS target_type, h.instance_name AS host_name, h.ip AS host_ip, h.agent_online AS host_agent_online FROM monitor_target t JOIN assets_host h ON h.id=t.host_id`+where+` ORDER BY t.id DESC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	paginated(context, items, count, page, size)
}

func (handler *Handler) ListPackages(context *gin.Context) {
	page, size := pagination(context)
	clauses := []string{"1=1"}
	arguments := make([]any, 0)
	for queryName, column := range map[string]string{"package_type": "p.package_type", "name": "p.name", "version": "p.version", "os": "p.os", "arch": "p.arch", "enabled": "p.enabled"} {
		if value := strings.TrimSpace(context.Query(queryName)); value != "" {
			clauses = append(clauses, column+"=?")
			arguments = append(arguments, value)
		}
	}
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		pattern := "%" + search + "%"
		clauses = append(clauses, "(p.name LIKE ? OR p.version LIKE ?)")
		arguments = append(arguments, pattern, pattern)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM monitor_software_package p`+where, arguments)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT p.*, COALESCE(i.name,'') AS install_playbook_template_name, COALESCE(i.content,'') AS install_playbook_content, COALESCE(u.name,'') AS uninstall_playbook_template_name, COALESCE(u.content,'') AS uninstall_playbook_content FROM monitor_software_package p LEFT JOIN automation_playbook_template i ON i.id=p.install_playbook_template_id LEFT JOIN automation_playbook_template u ON u.id=p.uninstall_playbook_template_id`+where+` ORDER BY p.id DESC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		fileName := fmt.Sprint(item["file"])
		item["download_url"] = ""
		if fileName != "" {
			item["download_url"] = "/media/" + strings.TrimLeft(fileName, "/")
		}
		parts := strings.Split(fileName, "/")
		item["file_name"] = parts[len(parts)-1]
		item["synced"] = fileName != "" && fmt.Sprint(item["sha256"]) != ""
	}
	paginated(context, items, count, page, size)
}

func (handler *Handler) ListOpenSearchClusters(context *gin.Context) {
	page, size := pagination(context)
	clauses := []string{"1=1"}
	arguments := make([]any, 0)
	for queryName, column := range map[string]string{"enabled": "enabled", "is_default": "is_default"} {
		if value := strings.TrimSpace(context.Query(queryName)); value != "" {
			clauses = append(clauses, column+"=?")
			arguments = append(arguments, value)
		}
	}
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		pattern := "%" + search + "%"
		clauses = append(clauses, "(name LIKE ? OR hosts LIKE ? OR remark LIKE ?)")
		arguments = append(arguments, pattern, pattern, pattern)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM monitor_opensearch_cluster`+where, arguments)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT * FROM monitor_opensearch_cluster`+where+` ORDER BY is_default DESC,id ASC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		item["password_configured"] = fmt.Sprint(item["password"]) != ""
		delete(item, "password")
	}
	paginated(context, items, count, page, size)
}

func pagination(context *gin.Context) (int, int) {
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

func queryCount(context *gin.Context, database *sql.DB, query string, arguments []any) (int64, error) {
	var count int64
	err := database.QueryRowContext(context, query, arguments...).Scan(&count)
	return count, err
}

func paginated(context *gin.Context, items []gin.H, count int64, page, size int) {
	response.Success(context, gin.H{"results": items, "count": count, "pageNumber": page, "pageSize": size, "totalPages": int64(math.Ceil(float64(count) / float64(size))), "next": nil, "previous": nil})
}

func scanRows(rows *sql.Rows) ([]gin.H, error) {
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
			item[column] = normalizeDatabaseValue(column, values[index])
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeDatabaseValue(column string, value any) any {
	bytes, ok := value.([]byte)
	if !ok {
		return value
	}
	text := string(bytes)
	if column == "labels" || strings.HasSuffix(column, "_summary") {
		var decoded any
		if json.Unmarshal(bytes, &decoded) == nil {
			return decoded
		}
	}
	return text
}
