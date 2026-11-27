package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"github.com/gin-gonic/gin"
)

type maintenanceWindowItem struct {
	db.MonitorMaintenanceWindow
	SuppressedCount int64 `json:"suppressed_count"`
}

func maintenanceState(w db.MonitorMaintenanceWindow, now time.Time) (string, error) {
	switch w.Status {
	case "cancelled", "ended":
		return w.Status, nil
	case "open":
		if !w.EndsAt.After(w.StartsAt) {
			return "", errors.New("维护窗口时间范围损坏")
		}
		if now.Before(w.StartsAt) {
			return "scheduled", nil
		}
		if now.Before(w.EndsAt) {
			return "active", nil
		}
		return "expired", nil
	default:
		return "", fmt.Errorf("无效的维护窗口状态 %q", w.Status)
	}
}

func maintenanceItem(w db.MonitorMaintenanceWindow, now time.Time) (maintenanceWindowItem, error) {
	status, err := maintenanceState(w, now)
	if err != nil {
		return maintenanceWindowItem{}, err
	}
	if _, err := maintenanceMatchers(w.Matchers); err != nil {
		return maintenanceWindowItem{}, fmt.Errorf("维护窗口 %d 条件损坏: %w", w.ID, err)
	}
	w.Status = status
	return maintenanceWindowItem{MonitorMaintenanceWindow: w}, nil
}

func windowCounts(ctx context.Context, q *db.Queries, items []maintenanceWindowItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, len(items))
	index := make(map[int64]int, len(items))
	for i, v := range items {
		ids[i] = v.ID
		index[v.ID] = i
	}
	rows, err := q.ListMaintenanceWindowSuppressionCounts(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rows {
		items[index[r.WindowID]].SuppressedCount = r.SuppressedCount
	}
	return nil
}

func (h *Handler) ListMaintenanceWindows(c *gin.Context) {
	ctx := c.Request.Context()
	q := db.New(h.db)
	page, size := pagination(c)
	count, err := q.CountMaintenanceWindows(ctx)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, err := q.ListMaintenanceWindows(ctx, db.ListMaintenanceWindowsParams{Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		response.Error(c, err)
		return
	}
	now := time.Now().UTC()
	items := make([]maintenanceWindowItem, 0, len(rows))
	for _, w := range rows {
		item, err := maintenanceItem(w, now)
		if err != nil {
			response.Error(c, err)
			return
		}
		items = append(items, item)
	}
	if err = windowCounts(ctx, q, items); err != nil {
		response.Error(c, err)
		return
	}
	response.Paginated(c, items, count, int32(page), int32(size))
}

func (h *Handler) DetailMaintenanceWindow(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || id < 1 {
		response.BusinessError(c, 400, "id 必须为正整数", nil)
		return
	}
	ctx := c.Request.Context()
	w, err := db.New(h.db).GetMaintenanceWindow(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		response.BusinessError(c, 404, "维护窗口不存在", nil)
		return
	}
	if err != nil {
		response.Error(c, err)
		return
	}
	item, err := maintenanceItem(w, time.Now().UTC())
	if err != nil {
		response.Error(c, err)
		return
	}
	items := []maintenanceWindowItem{item}
	if err = windowCounts(ctx, db.New(h.db), items); err != nil {
		response.Error(c, err)
		return
	}
	// 仅解析这个窗口引用的资产，查看范围不要求额外的资产列表权限。
	names, err := maintenanceScopeNames(ctx, db.New(h.db), w.Matchers)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, struct {
		maintenanceWindowItem
		ScopeNames map[string]string `json:"scope_names"`
	}{items[0], names})
}

func maintenanceScopeNames(ctx context.Context, q *db.Queries, raw json.RawMessage) (map[string]string, error) {
	ms, err := maintenanceMatchers(raw)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, m := range ms {
		if m.Type == "label" {
			continue
		}
		kind := m.NodeType
		if m.Type == "host" {
			kind = "host"
		}
		key := fmt.Sprintf("%s:%d", kind, m.ID)
		if kind == "environment" {
			key += fmt.Sprintf(":%d", m.BusinessID)
		}
		if _, ok := names[key]; ok {
			continue
		}
		name, err := maintenanceScopeName(ctx, q, m)
		if errors.Is(err, sql.ErrNoRows) {
			name = fmt.Sprintf("资产已不存在（%s）", key)
		} else if err != nil {
			return nil, err
		}
		names[key] = name
	}
	return names, nil
}

func maintenanceScopeName(ctx context.Context, q *db.Queries, m maintenanceMatcher) (string, error) {
	if m.Type == "host" {
		host, err := q.GetHostTargetIdentity(ctx, m.ID)
		if err != nil {
			return "", err
		}
		switch {
		case host.InstanceName != "" && host.Ip != "":
			return host.InstanceName + "(" + host.Ip + ")", nil
		case host.InstanceName != "" || host.Ip != "":
			return host.InstanceName + host.Ip, nil
		default:
			return "未命名主机", nil
		}
	}
	if m.NodeType == "project" {
		return q.GetProjectNameByID(ctx, m.ID)
	}
	businessID := m.ID
	var suffix []string
	switch m.NodeType {
	case "environment":
		name, err := q.GetBusinessEnvironmentNameByID(ctx, m.ID)
		if err != nil {
			return "", err
		}
		businessID, suffix = m.BusinessID, []string{name}
	case "service":
		s, err := q.GetApplicationServiceDetail(ctx, m.ID)
		if err != nil {
			return "", err
		}
		env := s.EnvironmentName
		if env == "" {
			env = "未指定环境"
		}
		businessID, suffix = s.BusinessSystemID, []string{env, s.Name}
	}
	b, err := q.GetBusinessSystem(ctx, businessID)
	if err != nil {
		return "", err
	}
	project := b.ProjectName
	if project == "" {
		project = "未分配项目"
	}
	return strings.Join(append([]string{project, b.Name}, suffix...), " / "), nil
}

type maintenanceInput struct {
	ID       int64           `json:"id"`
	Title    string          `json:"title"`
	Reason   string          `json:"reason"`
	StartsAt time.Time       `json:"starts_at"`
	EndsAt   time.Time       `json:"ends_at"`
	Matchers json.RawMessage `json:"matchers"`
}

func (in *maintenanceInput) normalize() (ms []maintenanceMatcher, err error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 200 {
		return nil, errors.New("名称必填且不超过 200 个字符")
	}
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 4000 {
		return nil, errors.New("原因必填且不超过 4000 个字符")
	}
	in.StartsAt = in.StartsAt.UTC().Truncate(time.Microsecond)
	in.EndsAt = in.EndsAt.UTC().Truncate(time.Microsecond)
	if in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) {
		return nil, errors.New("必须指定有效开始、结束时间，且结束晚于开始")
	}
	ms, err = maintenanceMatchers(in.Matchers)
	if err == nil {
		in.Matchers, err = json.Marshal(ms)
	}
	return ms, err
}

func validateMaintenanceAssets(ctx context.Context, q *db.Queries, ms []maintenanceMatcher) error {
	for _, m := range ms {
		var err error
		switch {
		case m.Type == "host":
			_, err = q.GetMaintenanceHost(ctx, m.ID)
		case m.Type == "tree":
			switch m.NodeType {
			case "business":
				_, err = q.GetBusinessSystem(ctx, m.ID)
			case "service":
				_, err = q.GetApplicationServiceName(ctx, m.ID)
			case "project":
				_, err = q.GetProjectNameByID(ctx, m.ID)
			case "environment":
				_, err = q.GetBusinessEnvironmentNameByID(ctx, m.ID)
				if err == nil {
					_, err = q.GetBusinessSystem(ctx, m.BusinessID)
				}
			}
		}
		if errors.Is(err, sql.ErrNoRows) {
			return apperror.New(400, fmt.Sprintf("所选资产不存在：%s %s #%d", m.Type, m.NodeType, m.ID))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) CreateMaintenanceWindow(c *gin.Context) { h.mutateMaintenance(c, "create") }
func (h *Handler) UpdateMaintenanceWindow(c *gin.Context) { h.mutateMaintenance(c, "update") }
func (h *Handler) CancelMaintenanceWindow(c *gin.Context) { h.mutateMaintenance(c, "cancel") }
func (h *Handler) EndMaintenanceWindow(c *gin.Context)    { h.mutateMaintenance(c, "end") }

func (h *Handler) mutateMaintenance(c *gin.Context, action string) {
	var in maintenanceInput
	if err := c.ShouldBindJSON(&in); err != nil || (action != "create" && in.ID <= 0) || (action == "create" && in.ID != 0) {
		response.BusinessError(c, 400, "无效的维护窗口请求", nil)
		return
	}
	var ms []maintenanceMatcher
	var err error
	if action == "create" || action == "update" {
		ms, err = in.normalize()
		if err != nil {
			response.BusinessError(c, 400, err.Error(), nil)
			return
		}
	}
	claims, ok := identity.ClaimsFromContext(c)
	if !ok {
		response.Error(c, apperror.ErrTokenInvalid)
		return
	}
	ctx := c.Request.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		response.Error(c, err)
		return
	}
	defer tx.Rollback()
	q := db.New(tx)
	var before *db.MonitorMaintenanceWindow
	if action != "create" {
		w, e := q.LockMaintenanceWindow(ctx, in.ID)
		if errors.Is(e, sql.ErrNoRows) {
			response.BusinessError(c, 404, "维护窗口不存在", nil)
			return
		}
		if e != nil {
			response.Error(c, e)
			return
		}
		before = &w
	}
	// 行锁取得后才判定时间状态，防止排队等待的修改覆盖已生效或已结束的窗口。
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor := claims.Username
	if before != nil {
		state, e := maintenanceState(*before, now)
		if e != nil {
			response.Error(c, e)
			return
		}
		activeEdit := state == "active" && action == "update"
		if activeEdit && !in.StartsAt.Equal(before.StartsAt) {
			response.BusinessError(c, 409, "生效中的窗口不能修改开始时间", nil)
			return
		}
		if !activeEdit && ((action == "end" && state != "active") || (action != "end" && state != "scheduled")) {
			response.BusinessError(c, 409, "当前窗口状态不允许此操作", nil)
			return
		}
	}
	if ms != nil {
		if !in.EndsAt.After(now) {
			response.BusinessError(c, 400, "结束时间必须晚于当前时间", nil)
			return
		}
		if err = validateMaintenanceAssets(ctx, q, ms); err != nil {
			response.Error(c, err)
			return
		}
	}
	switch action {
	case "create":
		in.ID, err = q.CreateMaintenanceWindow(ctx, db.CreateMaintenanceWindowParams{Title: in.Title, Reason: in.Reason, StartsAt: in.StartsAt, EndsAt: in.EndsAt, Matchers: in.Matchers, CreatedBy: actor, UpdatedBy: actor, CreatedAt: now, UpdatedAt: now})
	case "update":
		err = q.EditMaintenanceWindow(ctx, db.EditMaintenanceWindowParams{ID: in.ID, Title: in.Title, Reason: in.Reason, StartsAt: in.StartsAt, EndsAt: in.EndsAt, Matchers: in.Matchers, UpdatedAt: now, UpdatedBy: actor})
	default:
		status := "cancelled"
		endsAt := before.EndsAt
		if action == "end" {
			status = "ended"
			endsAt = now
		}
		err = q.CloseMaintenanceWindow(ctx, db.CloseMaintenanceWindowParams{ID: in.ID, Status: status, EndsAt: endsAt, UpdatedBy: actor, UpdatedAt: now})
	}
	if err != nil {
		response.Error(c, err)
		return
	}
	after, err := q.GetMaintenanceWindow(ctx, in.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	item, err := maintenanceItem(after, now)
	if err != nil {
		response.Error(c, err)
		return
	}
	if action == "end" {
		items := []maintenanceWindowItem{item}
		if err := windowCounts(ctx, q, items); err != nil {
			response.Error(c, err)
			return
		}
		item = items[0]
	}
	// 快照与窗口变更同事务提交；审计落库失败不能留下无法追溯的静音。
	err = auditMaintenanceChange(ctx, q, before, after, action, actor, now)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, item)
}

func auditMaintenanceChange(ctx context.Context, q *db.Queries, before *db.MonitorMaintenanceWindow, after db.MonitorMaintenanceWindow, action, actor string, now time.Time) error {
	snapshot, err := json.Marshal(gin.H{"before": before, "after": after})
	if err != nil {
		return err
	}
	return q.AddMaintenanceWindowAudit(ctx, db.AddMaintenanceWindowAuditParams{WindowID: after.ID, Action: action, Actor: actor, DecidedAt: now, Snapshot: snapshot})
}

func (h *Handler) BatchDeleteMaintenanceWindows(c *gin.Context) {
	claims, ok := identity.ClaimsFromContext(c)
	if !ok {
		response.Error(c, apperror.ErrTokenInvalid)
		return
	}
	ctx := c.Request.Context()
	batchDeleteMonitorRows(c, h, func(_ *gin.Context, id int64) error {
		return database.InTransaction(ctx, h.db, func(q *db.Queries) error {
			before, err := q.LockMaintenanceWindow(ctx, id)
			if err != nil {
				return err
			}
			// 只退出活动扫描和管理列表，不级联删除历史抑制决定或审计。
			now := time.Now().UTC().Truncate(time.Microsecond)
			after := before
			after.Status, after.UpdatedBy, after.UpdatedAt = "deleted", claims.Username, now
			if err = q.CloseMaintenanceWindow(ctx, db.CloseMaintenanceWindowParams{ID: id, Status: after.Status, EndsAt: after.EndsAt, UpdatedBy: after.UpdatedBy, UpdatedAt: now}); err != nil {
				return err
			}
			return auditMaintenanceChange(ctx, q, &before, after, "delete", claims.Username, now)
		})
	})
}

func (h *Handler) PreviewMaintenanceWindow(c *gin.Context) {
	ms, err := maintenanceMatchers(json.RawMessage(c.Query("matchers")))
	if err != nil {
		response.BusinessError(c, 400, err.Error(), nil)
		return
	}
	ctx := c.Request.Context()
	q := db.New(h.db)
	if err = validateMaintenanceAssets(ctx, q, ms); err != nil {
		response.Error(c, err)
		return
	}
	_, payload, err := h.prometheusGet(ctx, "/api/v1/alerts", nil)
	if err != nil {
		prometheusReadError(c, payload, err)
		return
	}
	var upstream struct {
		Alerts *[]struct {
			Labels   map[string]any `json:"labels"`
			State    string         `json:"state"`
			ActiveAt string         `json:"activeAt"`
		} `json:"alerts"`
	}
	if err = json.Unmarshal(payload.Data, &upstream); err != nil || upstream.Alerts == nil {
		response.BusinessError(c, 502, "Prometheus 当前告警响应无效", nil)
		return
	}
	results := []gin.H{}
	for _, a := range *upstream.Alerts {
		if a.Labels == nil {
			response.BusinessError(c, 502, "Prometheus 告警缺少 labels", nil)
			return
		}
		t := alertNotificationTarget{labels: a.Labels, alertname: mapString(a.Labels, "alertname"), instance: mapString(a.Labels, "instance"), severity: mapString(a.Labels, "severity")}
		f := newMaintenanceFacts(t)
		ok, err := h.maintenanceScopeMatch(ctx, ms, f)
		if err != nil {
			response.Error(c, err)
			return
		}
		if !ok {
			continue
		}
		if err = h.loadMaintenanceAssets(ctx, f); err != nil {
			response.Error(c, err)
			return
		}
		matched := []maintenanceMatcher{}
		for _, m := range ms {
			if m.matches(f) {
				matched = append(matched, m)
			}
		}
		results = append(results, gin.H{"name": t.alertname, "severity": t.severity, "instance": t.instance, "labels": a.Labels, "state": a.State, "active_at": a.ActiveAt, "host_id": f.hostID, "host_name": f.hostName, "project_ids": f.projectIDs, "matched_by": matched})
	}
	response.Success(c, gin.H{"results": results, "count": len(results)})
}
