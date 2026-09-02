package monitor

import (
	"database/sql"
	"encoding/json"
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
