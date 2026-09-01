package inspection

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"autoadmin/internal/agent"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	db      *sql.DB
	gateway *agent.Gateway
}

func NewHandler(db *sql.DB, gateway *agent.Gateway) *Handler {
	return &Handler{db: db, gateway: gateway}
}

func parseID(value string) int64 {
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
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

func scanRows(rows *sql.Rows) ([]gin.H, error) {
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	items := make([]gin.H, 0)
	for rows.Next() {
		values, destinations := make([]any, len(columns)), make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err = rows.Scan(destinations...); err != nil {
			return nil, err
		}
		item := gin.H{}
		for index, column := range columns {
			item[column] = normalizeValue(column, values[index])
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeValue(column string, value any) any {
	if column == "enabled" {
		switch typed := value.(type) {
		case bool:
			return typed
		case int64:
			return typed != 0
		case []byte:
			return string(typed) == "1" || strings.EqualFold(string(typed), "true")
		case string:
			return typed == "1" || strings.EqualFold(typed, "true")
		}
	}
	raw, ok := value.([]byte)
	if !ok {
		return value
	}
	text := string(raw)
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var decoded any
		if json.Unmarshal(raw, &decoded) == nil {
			return decoded
		}
	}
	return text
}

func queryCount(context *gin.Context, db *sql.DB, query string, arguments ...any) (int64, error) {
	var count int64
	err := db.QueryRowContext(context, query, arguments...).Scan(&count)
	return count, err
}
