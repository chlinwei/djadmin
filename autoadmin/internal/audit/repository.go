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

func (repository *Repository) List(ctx context.Context, filter Filter, page pagination.Page) ([]db.AuditOperationLog, int64, error) {
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
	return rows, count, err
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
