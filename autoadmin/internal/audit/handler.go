package audit

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	repository *Repository
}

func NewHandler(repository *Repository) *Handler {
	return &Handler{repository: repository}
}

func (handler *Handler) List(context *gin.Context) {
	page, err := auditPage(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	filter, err := auditFilter(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, count, err := handler.repository.List(context.Request.Context(), filter, page)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrQueryInternal, err))
		return
	}
	response.Paginated(context, rows, count, page.Number, page.Size)
}

func auditPage(context *gin.Context) (pagination.Page, error) {
	number, err := parsePositive(context.DefaultQuery("page", "1"))
	if err != nil {
		return pagination.Page{}, apperror.ErrPageInvalid
	}
	size, err := parsePositive(context.DefaultQuery("page_size", "10"))
	if err != nil {
		return pagination.Page{}, apperror.ErrPageSizeInvalid
	}
	return pagination.New(number, size), nil
}

func auditFilter(context *gin.Context) (Filter, error) {
	filter := Filter{Keyword: strings.TrimSpace(context.Query("keyword"))}
	if method := strings.ToUpper(strings.TrimSpace(context.Query("method"))); method != "" {
		if method != httpMethodPost && method != httpMethodPut && method != httpMethodPatch && method != httpMethodDelete {
			return Filter{}, ErrMethodInvalid
		}
		filter.Method = method
	}
	if rawStatus := strings.TrimSpace(context.Query("status_code")); rawStatus != "" {
		status, err := strconv.ParseInt(rawStatus, 10, 32)
		if err != nil {
			return Filter{}, ErrStatusInvalid
		}
		value := int32(status)
		filter.StatusCode = &value
	}
	filter.CreatedFrom = parseAuditTime(context.Query("created_at_from"))
	filter.CreatedTo = parseAuditTime(context.Query("created_at_to"))
	return filter, nil
}

func parsePositive(raw string) (int32, error) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 1 {
		return 0, apperror.ErrInvalidRequest
	}
	return int32(value), nil
}

func parseAuditTime(raw string) sql.NullTime {
	if raw == "" {
		return sql.NullTime{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	return sql.NullTime{Time: parsed.UTC(), Valid: err == nil}
}

const (
	httpMethodPost   = "POST"
	httpMethodPut    = "PUT"
	httpMethodPatch  = "PATCH"
	httpMethodDelete = "DELETE"
)
