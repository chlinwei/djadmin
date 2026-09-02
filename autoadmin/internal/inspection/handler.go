package inspection

import (
	"database/sql"
	"strconv"

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

func queryCount(context *gin.Context, db *sql.DB, query string, arguments ...any) (int64, error) {
	var count int64
	err := db.QueryRowContext(context, query, arguments...).Scan(&count)
	return count, err
}
