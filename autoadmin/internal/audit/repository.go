package audit

import (
	"context"
	"database/sql"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type Entry struct {
	Username     string
	UserID       int32
	Method       string
	Path         string
	RouteName    string
	ClientIP     string
	UserAgent    string
	StatusCode   int32
	DurationMS   int32
	Message      string
	RequestData  string
	ResponseData string
}

type Filter struct {
	Keyword     string
	Method      string
	StatusCode  *int32
	CreatedFrom sql.NullTime
	CreatedTo   sql.NullTime
}

type OperationLog struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	UserID       *int32 `json:"user_id"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	RouteName    string `json:"route_name"`
	ClientIP     string `json:"client_ip"`
	UserAgent    string `json:"user_agent"`
	StatusCode   int32  `json:"status_code"`
	DurationMS   *int32 `json:"duration_ms"`
	Message      string `json:"message"`
	CreatedAt    string `json:"created_at"`
	RequestData  string `json:"request_data"`
	ResponseData string `json:"response_data"`
}

func operationLog(row db.AuditOperationLog) OperationLog {
	return OperationLog{
		ID: row.ID, Username: row.Username, UserID: int32Ptr(row.UserID),
		Method: row.Method, Path: row.Path, RouteName: row.RouteName,
		ClientIP: row.ClientIp, UserAgent: row.UserAgent, StatusCode: row.StatusCode,
		DurationMS: int32Ptr(row.DurationMs), Message: row.Message,
		CreatedAt: formatTime(row.CreatedAt), RequestData: row.RequestData, ResponseData: row.ResponseData,
	}
}

type Recorder interface {
	Record(context.Context, Entry) error
}

type Repository struct {
	queries *db.Queries
}

func NewRepository(pool *sql.DB) *Repository {
	return &Repository{queries: db.New(pool)}
}

func (repository *Repository) Record(ctx context.Context, entry Entry) error {
	return repository.queries.CreateOperationAudit(ctx, db.CreateOperationAuditParams{
		Username: entry.Username,
		UserID:   sql.NullInt32{Int32: entry.UserID, Valid: entry.UserID > 0},
		Method:   entry.Method, Path: entry.Path, RouteName: entry.RouteName,
		ClientIp: entry.ClientIP, UserAgent: entry.UserAgent,
		StatusCode: entry.StatusCode,
		DurationMs: sql.NullInt32{Int32: entry.DurationMS, Valid: true},
		Message:    entry.Message, CreatedAt: nowUTC(),
		RequestData: entry.RequestData, ResponseData: entry.ResponseData,
	})
}

func (repository *Repository) List(ctx context.Context, filter Filter, page pagination.Page) ([]OperationLog, int64, error) {
	countParams := countParams(filter)
	count, err := repository.queries.CountOperationAudits(ctx, countParams)
	if err != nil {
		return nil, 0, err
	}
	rows, err := repository.queries.ListOperationAudits(ctx, db.ListOperationAuditsParams{
		UsernamePattern: countParams.UsernamePattern, MethodPattern: countParams.MethodPattern,
		PathPattern: countParams.PathPattern, RoutePattern: countParams.RoutePattern,
		ClientIpPattern: countParams.ClientIpPattern, MessagePattern: countParams.MessagePattern,
		ExactMethod: countParams.ExactMethod, ExactStatusCode: countParams.ExactStatusCode,
		CreatedFrom: countParams.CreatedFrom, CreatedTo: countParams.CreatedTo,
		Limit: page.Size, Offset: page.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	logs := make([]OperationLog, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, operationLog(row))
	}
	return logs, count, nil
}

func countParams(filter Filter) db.CountOperationAuditsParams {
	pattern := sql.NullString{}
	if filter.Keyword != "" {
		pattern = sql.NullString{String: "%" + filter.Keyword + "%", Valid: true}
	}
	method := sql.NullString{}
	if filter.Method != "" {
		method = sql.NullString{String: filter.Method, Valid: true}
	}
	status := sql.NullInt32{}
	if filter.StatusCode != nil {
		status = sql.NullInt32{Int32: *filter.StatusCode, Valid: true}
	}
	return db.CountOperationAuditsParams{
		UsernamePattern: pattern, MethodPattern: pattern, PathPattern: pattern,
		RoutePattern: pattern, ClientIpPattern: pattern, MessagePattern: pattern,
		ExactMethod: method, ExactStatusCode: status,
		CreatedFrom: filter.CreatedFrom, CreatedTo: filter.CreatedTo,
	}
}
