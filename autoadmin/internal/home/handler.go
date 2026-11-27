package home

import (
	"context"
	"database/sql"
	"log"
	"time"

	"autoadmin/internal/api/middleware"
	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

const recentLimit = 5

type Handler struct {
	queries *db.Queries
}

func NewHandler(queries *db.Queries) *Handler {
	return &Handler{queries: queries}
}

type overview struct {
	GeneratedAt time.Time `json:"generated_at"`
	Hosts       *section  `json:"hosts,omitempty"`
	Alerts      *section  `json:"alerts,omitempty"`
	Jobs        *section  `json:"jobs,omitempty"`
	Inspections *section  `json:"inspections,omitempty"`
}

type section struct {
	Status string  `json:"status"`
	Count  *int64  `json:"count,omitempty"`
	Recent *[]item `json:"recent,omitempty"`
}

type item struct {
	ID     int64     `json:"id"`
	Name   string    `json:"name"`
	Status string    `json:"status"`
	Detail string    `json:"detail,omitempty"`
	Time   time.Time `json:"time"`
}

func (h *Handler) Overview(c *gin.Context) {
	claims, ok := identity.ClaimsFromContext(c)
	if !ok {
		response.Error(c, apperror.ErrTokenInvalid)
		return
	}
	now := time.Now().UTC()
	ctx := c.Request.Context()
	data := overview{GeneratedAt: now}
	if middleware.HasPermission(claims, "assets:hosts:view") {
		count, err := h.queries.CountHosts(ctx, db.CountHostsParams{
			Pattern: text("%%"), GroupID: sql.NullInt64{Valid: true}, EnvironmentID: sql.NullInt64{Valid: true},
		})
		data.Hosts = result("hosts", count, nil, err)
	}
	if middleware.HasPermission(claims, "monitor:view") {
		data.Alerts = h.alerts(ctx)
	}
	if middleware.HasPermission(claims, "automation:jobs:view") {
		data.Jobs = h.jobs(ctx)
	}
	if middleware.HasPermission(claims, "inspection:view") {
		data.Inspections = h.inspections(ctx, now)
	}
	response.Success(c, data)
}

func result(key string, count int64, items *[]item, err error) *section {
	if err != nil {
		log.Printf("[home] overview %s: %v", key, err)
		return &section{Status: "unavailable"}
	}
	return &section{Status: "ok", Count: &count, Recent: items}
}

func text(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func (h *Handler) alerts(ctx context.Context) *section {
	state := text("firing")
	count, err := h.queries.CountAlertHistories(ctx, db.CountAlertHistoriesParams{
		State: state, LabelKey: "alertname", SkipNotificationStatus: true,
	})
	if err != nil {
		return result("alerts", 0, nil, err)
	}
	rows, err := h.queries.ListAlertHistories(ctx, db.ListAlertHistoriesParams{
		State: state, LabelKey: "alertname", SkipNotificationStatus: true, Limit: recentLimit,
	})
	items := make([]item, 0, len(rows))
	for _, row := range rows {
		items = append(items, item{ID: row.ID, Name: row.Alertname, Status: row.Severity, Detail: row.Instance, Time: row.StartedAt.UTC()})
	}
	return result("alerts", count, &items, err)
}

func (h *Handler) jobs(ctx context.Context) *section {
	count, err := h.queries.CountJobs(ctx, db.CountJobsParams{Status: text("running")})
	if err != nil {
		return result("jobs", 0, nil, err)
	}
	rows, err := h.queries.ListJobsTyped(ctx, db.ListJobsTypedParams{Limit: recentLimit})
	items := make([]item, 0, len(rows))
	for _, row := range rows {
		name := row.TaskNameSnapshot
		if name == "" {
			name = row.TemplateNameSnapshot
		}
		items = append(items, item{ID: row.ID, Name: name, Status: row.Status, Time: row.CreateTime.UTC()})
	}
	return result("jobs", count, &items, err)
}

func (h *Handler) inspections(ctx context.Context, now time.Time) *section {
	count, err := h.queries.CountInspectionExecutions(ctx, db.CountInspectionExecutionsParams{
		Status: text("failed"), StartTime: sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
		EndTime: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return result("inspections", 0, nil, err)
	}
	rows, err := h.queries.ListInspectionExecutions(ctx, db.ListInspectionExecutionsParams{Limit: recentLimit})
	items := make([]item, 0, len(rows))
	for _, row := range rows {
		items = append(items, item{ID: row.ID, Name: row.TaskName, Status: row.Status, Time: row.CreateTime.UTC()})
	}
	return result("inspections", count, &items, err)
}
