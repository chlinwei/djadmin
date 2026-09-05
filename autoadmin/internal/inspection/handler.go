package inspection

import (
	"database/sql"
	"strconv"
	"sync"

	"autoadmin/internal/agent"

	"github.com/gin-gonic/gin"
)

// maxGlobalConcurrentTargets caps in-flight Agent executions across ALL running
// inspection executions, so two large tasks cannot double the fan-out.
const maxGlobalConcurrentTargets = 200

type Handler struct {
	db      *sql.DB
	gateway *agent.Gateway
	// canceled holds execution IDs cancelled after dispatch; Agent responses
	// arriving afterwards are dropped without hitting the database.
	canceled sync.Map
	// globalSlots caps concurrent targets across all executions (see maxGlobalConcurrentTargets).
	globalSlots chan struct{}
}

func NewHandler(db *sql.DB, gateway *agent.Gateway) *Handler {
	return &Handler{db: db, gateway: gateway, globalSlots: make(chan struct{}, maxGlobalConcurrentTargets)}
}

func (handler *Handler) markCanceled(executionID int64) {
	handler.canceled.Store(executionID, struct{}{})
}

func (handler *Handler) isCanceled(executionID int64) bool {
	_, canceled := handler.canceled.Load(executionID)
	return canceled
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
