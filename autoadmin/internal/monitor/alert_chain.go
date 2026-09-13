package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"

	"github.com/gin-gonic/gin"
)

// ---- 契约结构 ----

type alertChainMediaBrief struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	MediaType  string `json:"media_type"`
	Enabled    bool   `json:"enabled"`
}

type alertChainRouteBrief struct {
	RouteID         int64           `json:"route_id"`
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	NotifyOnFiring   bool   `json:"notify_on_firing"`
	NotifyOnResolved bool   `json:"notify_on_resolved"`
	Matchers        map[string]any `json:"matchers"`
	Issues          []string       `json:"issues"`
}

type alertChainBinding struct {
	BindingID  int64                  `json:"binding_id"`
	Enabled    bool                   `json:"enabled"`
	Recipients []string               `json:"recipients"`
	Scope      []alertChainScopeItem  `json:"scope"`
	Media      alertChainMediaBrief   `json:"media"`
	Issues     []string               `json:"issues"`
	Routes     []alertChainRouteBrief `json:"routes"`
}

type alertChainUserChain struct {
	User          gin.H                 `json:"user"`
	Bindings      []alertChainBinding   `json:"bindings"`
	CanReceive    bool                  `json:"can_receive"`
	SummaryIssues []string              `json:"summary_issues"`
}

type alertChainDelivery struct {
	UserID   *int32 `json:"user_id"`
	Username string `json:"username"`
	Address  string `json:"address"`
	Status   string `json:"status"`
	Error    string `json:"error"`
}

type alertChainEvent struct {
	ID           int64                `json:"id"`
	EventType    string               `json:"event_type"`
	Status       string               `json:"status"`
	AttemptCount int64                `json:"attempt_count"`
	Error        string               `json:"error"`
}

type alertChainMediaDetail struct {
	ID       int64                `json:"id"`
	Name     string               `json:"name"`
	Enabled  bool                 `json:"enabled"`
	Bindings []gin.H              `json:"bindings"`
	Event    *alertChainEvent     `json:"event"`
	Deliveries []alertChainDelivery `json:"deliveries"`
}

type alertChainRouteDetail struct {
	RouteID          int64                    `json:"route_id"`
	Name             string                   `json:"name"`
	Enabled          bool                     `json:"enabled"`
	Matched          bool                     `json:"matched"`
	MissReason       string                   `json:"miss_reason"`
	NotifyOnFiring   bool                     `json:"notify_on_firing"`
	NotifyOnResolved bool                     `json:"notify_on_resolved"`
	Media            []alertChainMediaDetail  `json:"media"`

	// Labels 告警原始 labels（不入 JSON），供 scope 判定取 host_id。
	Labels map[string]any `json:"-"`
}

// ---- 纯判定逻辑（可单测） ----

// chainMergeLabels 把 alertname/severity/instance 便捷键合并进 labels（显式 labels 值优先）。
func chainMergeLabels(labels map[string]any, alertname, severity, instance string) map[string]any {
	merged := make(map[string]any, len(labels)+3)
	for key, value := range labels {
		merged[key] = value
	}
	setDefault := func(key, value string) {
		if value != "" {
			if _, exists := merged[key]; !exists {
				merged[key] = value
			}
		}
	}
	setDefault("alertname", alertname)
	setDefault("severity", severity)
	setDefault("instance", instance)
	return merged
}

// matchRouteMatchers 对 matchers 做 labels 全量等值匹配，返回是否命中与第一个不匹配键。
func matchRouteMatchers(matchers map[string]any, labels map[string]any) (bool, string) {
	for key, expected := range matchers {
		actual, exists := labels[key]
		if !exists || fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", expected) {
			return false, key
		}
	}
	return true, ""
}

// routeNotifyMiss 依据告警 state 判断 notify 开关是否匹配，返回 miss 说明（空串=匹配）。
func routeNotifyMiss(state string, notifyOnFiring, notifyOnResolved bool) string {
	switch strings.ToLower(state) {
	case "firing":
		if !notifyOnFiring {
			return "告警 state=firing 但该路由未开启 firing 通知"
		}
	case "resolved":
		if !notifyOnResolved {
			return "告警 state=resolved 但该路由未开启 resolved 通知"
		}
	}
	return ""
}

// userBindingIssues 计算用户视角单条绑定的媒介级 issues。
func userBindingIssues(bindingEnabled bool, mediaEnabled bool, mediaType string, recipients []string) []string {
	issues := make([]string, 0, 4)
	if !bindingEnabled {
		issues = append(issues, "该绑定已禁用")
	}
	if !mediaEnabled {
		issues = append(issues, "媒介已停用")
	}
	if mediaType != "email" {
		issues = append(issues, "非邮件媒介，暂不支持自动发送")
	}
	if len(recipients) == 0 {
		issues = append(issues, "绑定未配置收件地址")
	}
	return issues
}

// userRouteIssues 计算用户视角路由 issues（matchers 是对告警的，与用户无关，故不校验）。
func userRouteIssues(routeEnabled, notifyOnFiring, notifyOnResolved bool) []string {
	var issues []string
	if !routeEnabled {
		issues = append(issues, "路由已禁用")
	}
	if !notifyOnFiring {
		if notifyOnResolved {
			issues = append(issues, "该路由仅通知 resolved，firing 通知未开启")
		} else {
			issues = append(issues, "该路由未开启任何事件通知")
		}
	}
	return issues
}

// userCanReceive 判断是否存在绑定启用+媒介启用+email+收件地址非空+挂在启用且开启 firing 通知的路由上的完整通路。

// alertChainScopeItem 订阅范围条目；missing=true 表示节点已被删除。
type alertChainScopeItem struct {
	Type    string `json:"type"`
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Missing bool   `json:"missing"`
}

// resolveScopeItems 把绑定的 scope JSON 解析为带节点名的条目列表。
// 名称按类型查服务树表；查不到即 missing（节点已删除）。
func (handler *Handler) resolveScopeItems(context context.Context, raw []byte) []alertChainScopeItem {
	if len(raw) == 0 || string(raw) == "null" {
		return []alertChainScopeItem{}
	}
	var entries []struct {
		Type string `json:"type"`
		ID   int64  `json:"id"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return []alertChainScopeItem{}
	}
	items := make([]alertChainScopeItem, 0, len(entries))
	for _, entry := range entries {
		item := alertChainScopeItem{Type: entry.Type, ID: entry.ID, Missing: true}
		var name string
		var table string
		switch entry.Type {
		case "service":
			table = "assets_application_service"
		case "environment":
			table = "assets_business_environment"
		case "business":
			table = "assets_business_system"
		case "project":
			table = "assets_project"
		default:
			table = ""
		}
		if table != "" {
			query := fmt.Sprintf("SELECT name FROM `%s` WHERE id=?", table)
			if err := handler.db.QueryRowContext(context, query, entry.ID).Scan(&name); err == nil {
				item.Name = name
				item.Missing = false
			}
		}
		items = append(items, item)
	}
	return items
}

// scopeIssues：scope 绑定的范围相关断点（missing 节点、全部 missing）。
func scopeIssues(items []alertChainScopeItem) []string {
	if len(items) == 0 {
		return nil
	}
	issues := make([]string, 0)
	missingCount := 0
	for _, item := range items {
		if item.Missing {
			missingCount++
		}
	}
	if missingCount > 0 {
		issues = append(issues, "订阅范围包含已删除的服务树节点")
	}
	if missingCount == len(items) {
		issues = append(issues, "订阅范围不含任何存在的服务树节点，等同于收不到告警")
	}
	return issues
}

// scopeCoversAlert：告警历史 labels 的 host_id 解析出的服务树归属节点，
// 是否落在该用户绑定 scope 内（scope 为空 = 全局恒命中；复用分发侧的解析与匹配语义）。
func (handler *Handler) scopeCoversAlert(context context.Context, detail *alertChainRouteDetail, userID int32) bool {
	var scopeRaw []byte
	if err := handler.db.QueryRowContext(context,
		`SELECT scope FROM monitor_user_alert_media_binding WHERE user_id=? LIMIT 1`, userID).Scan(&scopeRaw); err != nil {
		return true
	}
	var entries []struct {
		Type string `json:"type"`
		ID   int64  `json:"id"`
	}
	if len(scopeRaw) == 0 || string(scopeRaw) == "null" || json.Unmarshal(scopeRaw, &entries) != nil || len(entries) == 0 {
		return true
	}
	var hostID int64
	if detail.Labels != nil {
		if raw, ok := detail.Labels["host_id"]; ok {
			_, _ = fmt.Sscan(strings.TrimSpace(fmt.Sprint(raw)), &hostID)
		}
	}
	if hostID <= 0 {
		return false
	}
	nodeSet := map[string]bool{}
	rows, err := handler.db.QueryContext(context, `SELECT DISTINCT s.id, s.business_system_id, s.environment_id, bs.project
		FROM assets_application_deployment d
		JOIN assets_application_service_deployment sd ON sd.deployment_id=d.id
		JOIN assets_application_service s ON s.id=sd.service_id
		JOIN assets_business_system bs ON bs.id=s.business_system_id
		WHERE d.host_id=? AND d.enabled=TRUE AND sd.enabled=TRUE`, hostID)
	if err != nil {
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var serviceID, businessID int64
		var environmentID, projectID sql.NullInt64
		if err = rows.Scan(&serviceID, &businessID, &environmentID, &projectID); err != nil {
			return true
		}
		nodeSet[fmt.Sprintf("service:%d", serviceID)] = true
		nodeSet[fmt.Sprintf("business:%d", businessID)] = true
		if environmentID.Valid {
			nodeSet[fmt.Sprintf("environment:%d", environmentID.Int64)] = true
		}
		if projectID.Valid {
			nodeSet[fmt.Sprintf("project:%d", projectID.Int64)] = true
		}
	}
	for _, entry := range entries {
		if entry.ID > 0 && nodeSet[fmt.Sprintf("%s:%d", entry.Type, entry.ID)] {
			return true
		}
	}
	return false
}

func userCanReceive(bindings []alertChainBinding) bool {
	for _, binding := range bindings {
		if !binding.Enabled || !binding.Media.Enabled || binding.Media.MediaType != "email" || len(binding.Recipients) == 0 {
			continue
		}
		for _, route := range binding.Routes {
			if route.Enabled && route.NotifyOnFiring {
				return true
			}
		}
	}
	return false
}

func truncateError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 120 {
		return message[:120] + "..."
	}
	return message
}

// ---- SQL 行结构 ----

type userBindingRow struct {
	BindingID    int64
	Enabled      bool
	Recipients   []byte
	MediaID      int64
	MediaName    string
	MediaType    string
	MediaEnabled bool
	Scope        []byte
}

type routeRow struct {
	ID               int64
	Name             string
	Enabled          bool
	NotifyOnFiring   bool
	NotifyOnResolved bool
	Matchers         []byte
}

// ---- API 1: GET /monitor/alert-notification/user-chain/ ----

func (handler *Handler) UserAlertChain(context *gin.Context) {
	userID, ok := resolveTargetUser(context)
	if !ok {
		return
	}
	var username string
	if err := handler.db.QueryRowContext(context, `SELECT username FROM sys_user WHERE id=?`, userID).Scan(&username); err != nil {
		response.Error(context, err)
		return
	}

	bindingRows, err := handler.db.QueryContext(context, `SELECT b.id,b.enabled,b.recipients,m.id,m.name,m.media_type,m.enabled,b.scope
		FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m ON m.id=b.media_id
		WHERE b.user_id=? ORDER BY b.id`, userID)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer bindingRows.Close()

	bindings := make([]alertChainBinding, 0)
	summaryIssues := make([]string, 0)
	seenSummary := map[string]bool{}
	addSummary := func(item string) {
		if !seenSummary[item] {
			seenSummary[item] = true
			summaryIssues = append(summaryIssues, item)
		}
	}
	for bindingRows.Next() {
		var row userBindingRow
		if err = bindingRows.Scan(&row.BindingID, &row.Enabled, &row.Recipients, &row.MediaID, &row.MediaName, &row.MediaType, &row.MediaEnabled, &row.Scope); err != nil {
			response.Error(context, err)
			return
		}
		recipients := make([]string, 0)
		_ = json.Unmarshal(row.Recipients, &recipients)

		scopeItems := handler.resolveScopeItems(context, row.Scope)
		issues := userBindingIssues(row.Enabled, row.MediaEnabled, row.MediaType, recipients)
		issues = append(issues, scopeIssues(scopeItems)...)
		for _, issue := range issues {
			switch issue {
			case "该绑定已禁用":
				addSummary(fmt.Sprintf("绑定 %d 已禁用", row.BindingID))
			case "媒介已停用":
				addSummary(fmt.Sprintf("媒介 %d 已停用", row.MediaID))
			case "订阅范围包含已删除的服务树节点":
				addSummary(fmt.Sprintf("绑定 %d：%s", row.BindingID, issue))
			default:
				addSummary(fmt.Sprintf("绑定 %d：%s", row.BindingID, issue))
			}
		}

		routes, routeErr := handler.routesForMedia(context, row.MediaID)
		if routeErr != nil {
			response.Error(context, routeErr)
			return
		}
		routeBriefs := make([]alertChainRouteBrief, 0, len(routes))
		for _, route := range routes {
			var matchers map[string]any
			_ = json.Unmarshal(route.Matchers, &matchers)
			if matchers == nil {
				matchers = map[string]any{}
			}
			routeIssues := userRouteIssues(route.Enabled, route.NotifyOnFiring, route.NotifyOnResolved)
			for _, issue := range routeIssues {
				addSummary(fmt.Sprintf("路由 %s：%s", route.Name, issue))
			}
			routeBriefs = append(routeBriefs, alertChainRouteBrief{
				RouteID: route.ID, Name: route.Name, Enabled: route.Enabled,
				NotifyOnFiring: route.NotifyOnFiring, NotifyOnResolved: route.NotifyOnResolved,
				Matchers: matchers, Issues: routeIssues,
			})
		}
		bindings = append(bindings, alertChainBinding{
			BindingID: row.BindingID, Enabled: row.Enabled, Recipients: recipients, Scope: scopeItems,
			Media: alertChainMediaBrief{ID: row.MediaID, Name: row.MediaName, MediaType: row.MediaType, Enabled: row.MediaEnabled},
			Issues: issues, Routes: routeBriefs,
		})
	}
	if err = bindingRows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	if len(bindings) == 0 {
		addSummary(fmt.Sprintf("用户 %s 未配置任何告警媒介绑定", username))
	}

	response.Success(context, alertChainUserChain{
		User: gin.H{"id": userID, "username": username},
		Bindings: bindings, CanReceive: userCanReceive(bindings), SummaryIssues: summaryIssues,
	})
}

func resolveTargetUser(context *gin.Context) (int64, bool) {
	if raw := strings.TrimSpace(context.Query("user_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			response.BusinessError(context, 400, "user_id 无效", nil)
			return 0, false
		}
		return parsed, true
	}
	claims, ok := identity.ClaimsFromContext(context)
	if !ok {
		response.Error(context, fmt.Errorf("未获取到登录用户"))
		return 0, false
	}
	return int64(claims.UserID), true
}

func (handler *Handler) routesForMedia(context *gin.Context, mediaID int64) ([]routeRow, error) {
	rows, err := handler.db.QueryContext(context, `SELECT r.id,r.name,r.enabled,r.notify_on_firing,r.notify_on_resolved,r.matchers
		FROM monitor_alert_route r JOIN monitor_alert_route_media rm ON rm.alertroute_id=r.id
		WHERE rm.alertmedia_id=? ORDER BY r.id`, mediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	routes := make([]routeRow, 0)
	for rows.Next() {
		var route routeRow
		if err = rows.Scan(&route.ID, &route.Name, &route.Enabled, &route.NotifyOnFiring, &route.NotifyOnResolved, &route.Matchers); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

// ---- API 2: GET /monitor/alert-notification/chain/:historyId/ ----

func (handler *Handler) AlertChainEvaluation(context *gin.Context) {
	historyID := parseID(context.Param("historyId"))
	var alertname, severity, instance, state string
	var labelsRaw []byte
	var startedAt time.Time
	err := handler.db.QueryRowContext(context,
		`SELECT alertname,severity,instance,labels,state,started_at FROM monitor_alert_history WHERE id=?`, historyID,
	).Scan(&alertname, &severity, &instance, &labelsRaw, &state, &startedAt)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "alert history not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	labels := map[string]any{}
	_ = json.Unmarshal(labelsRaw, &labels)
	mergedLabels := chainMergeLabels(labels, alertname, severity, instance)

	routeRows, err := handler.db.QueryContext(context, `SELECT id,name,enabled,notify_on_firing,notify_on_resolved,matchers
		FROM monitor_alert_route ORDER BY id`)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer routeRows.Close()

	routeDetails := make([]alertChainRouteDetail, 0)
	summaryIssues := make([]string, 0)
	anyMatched := false
	for routeRows.Next() {
		var route routeRow
		if err = routeRows.Scan(&route.ID, &route.Name, &route.Enabled, &route.NotifyOnFiring, &route.NotifyOnResolved, &route.Matchers); err != nil {
			response.Error(context, err)
			return
		}
		var matchers map[string]any
		_ = json.Unmarshal(route.Matchers, &matchers)
		if matchers == nil {
			matchers = map[string]any{}
		}

		detail := alertChainRouteDetail{
			RouteID: route.ID, Name: route.Name, Enabled: route.Enabled,
			NotifyOnFiring: route.NotifyOnFiring, NotifyOnResolved: route.NotifyOnResolved,
			Labels: labels, Media: make([]alertChainMediaDetail, 0),
		}
		mediaErr := handler.evaluateRouteMedia(context, &detail, historyID, state)
		if mediaErr != nil {
			response.Error(context, mediaErr)
			return
		}
		detail.Matched = false
		switch {
		case !route.Enabled:
			detail.MissReason = "路由已禁用"
		default:
			if matched, missKey := matchRouteMatchers(matchers, mergedLabels); !matched {
				detail.MissReason = fmt.Sprintf("labels 不匹配（matchers 需要 %s=%v）", missKey, matchers[missKey])
			} else if miss := routeNotifyMiss(state, route.NotifyOnFiring, route.NotifyOnResolved); miss != "" {
				detail.MissReason = miss
			} else {
				detail.Matched = true
				anyMatched = true
			}
		}
		if !detail.Matched {
			summaryIssues = append(summaryIssues, fmt.Sprintf("路由 %s 未命中：%s", route.Name, detail.MissReason))
		} else {
			for _, media := range detail.Media {
				if media.Event == nil {
					summaryIssues = append(summaryIssues, fmt.Sprintf("路由 %s：无 %s 事件记录，通知未触发", route.Name, state))
					break
				}
			}
		}
		for _, media := range detail.Media {
			for _, delivery := range media.Deliveries {
				if delivery.Status != "success" {
					name := delivery.Username
					if name == "" {
						name = fmt.Sprintf("%d", derefInt32(delivery.UserID))
					}
					summaryIssues = append(summaryIssues, fmt.Sprintf("用户 %s 的投递失败：%s", name, truncateError(delivery.Error)))
				}
			}
		}
		routeDetails = append(routeDetails, detail)
	}
	if err = routeRows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	if !anyMatched && len(routeDetails) == 0 {
		summaryIssues = append(summaryIssues, "未配置任何告警路由，通知不会触发")
	}

	response.Success(context, gin.H{
		"alert": gin.H{
			"id": historyID, "alertname": alertname, "state": state,
			"labels": mergedLabels, "started_at": startedAt,
		},
		"routes":         routeDetails,
		"summary_issues": summaryIssues,
	})
}

func derefInt32(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

// evaluateRouteMedia 填充命中路由下的媒介、用户绑定与实际 event/delivery 记录。
func (handler *Handler) evaluateRouteMedia(context *gin.Context, detail *alertChainRouteDetail, historyID int64, state string) error {
	mediaRows, err := handler.db.QueryContext(context, `SELECT m.id,m.name,m.enabled
		FROM monitor_alert_route_media rm JOIN monitor_alert_media m ON m.id=rm.alertmedia_id
		WHERE rm.alertroute_id=? ORDER BY m.id`, detail.RouteID)
	if err != nil {
		return err
	}
	defer mediaRows.Close()

	type mediaRow struct {
		ID      int64
		Name    string
		Enabled bool
	}
	medias := make([]mediaRow, 0)
	for mediaRows.Next() {
		var media mediaRow
		if err = mediaRows.Scan(&media.ID, &media.Name, &media.Enabled); err != nil {
			return err
		}
		medias = append(medias, media)
	}
	if err = mediaRows.Err(); err != nil {
		return err
	}

	for _, media := range medias {
		mediaDetail := alertChainMediaDetail{
			ID: media.ID, Name: media.Name, Enabled: media.Enabled,
			Bindings: make([]gin.H, 0), Deliveries: make([]alertChainDelivery, 0),
		}
		bindingRows, err := handler.db.QueryContext(context, `SELECT b.user_id,u.username,b.recipients,b.enabled,b.scope
			FROM monitor_user_alert_media_binding b JOIN sys_user u ON u.id=b.user_id
			WHERE b.media_id=? ORDER BY b.id`, media.ID)
		if err != nil {
			return err
		}
		for bindingRows.Next() {
			var userID int32
			var username string
			var recipientsRaw, scopeRaw []byte
			var enabled bool
			if err = bindingRows.Scan(&userID, &username, &recipientsRaw, &enabled, &scopeRaw); err != nil {
				bindingRows.Close()
				return err
			}
			recipients := make([]string, 0)
			_ = json.Unmarshal(recipientsRaw, &recipients)
			scopeItems := handler.resolveScopeItems(context, scopeRaw)
			scopedIn := true
			if len(scopeItems) > 0 {
				scopedIn = handler.scopeCoversAlert(context, detail, userID)
			}
			bindingView := gin.H{
				"user_id": userID, "username": username, "recipients": recipients, "enabled": enabled,
				"scope": scopeItems, "scoped_in": scopedIn,
			}
			if !scopedIn {
				bindingView["issue"] = "订阅范围不含该告警的归属节点"
			}
			mediaDetail.Bindings = append(mediaDetail.Bindings, bindingView)
		}
		if err = bindingRows.Err(); err != nil {
			bindingRows.Close()
			return err
		}
		bindingRows.Close()

		eventType := strings.ToLower(state)
		var eventID int64
		var dbEventType, eventStatus, eventError string
		var attemptCount int64
		eventErr := handler.db.QueryRowContext(context, `SELECT id,event_type,status,attempt_count,error_message
			FROM monitor_alert_notification_event WHERE alert_id=? AND event_type=? ORDER BY id DESC LIMIT 1`,
			historyID, eventType).Scan(&eventID, &dbEventType, &eventStatus, &attemptCount, &eventError)
		switch {
		case eventErr == sql.ErrNoRows:
			mediaDetail.Event = nil
		case eventErr != nil:
			return eventErr
		default:
			mediaDetail.Event = &alertChainEvent{
				ID: eventID, EventType: dbEventType, Status: eventStatus,
				AttemptCount: attemptCount, Error: eventError,
			}
			deliveryRows, err := handler.db.QueryContext(context, `SELECT d.user_id,u.username,d.address,d.status,d.error_message
				FROM monitor_alert_notification_delivery d LEFT JOIN sys_user u ON u.id=d.user_id
				WHERE d.event_id=? ORDER BY d.id`, eventID)
			if err != nil {
				return err
			}
			for deliveryRows.Next() {
				var delivery alertChainDelivery
				var userID sql.NullInt32
				var username sql.NullString
				if err = deliveryRows.Scan(&userID, &username, &delivery.Address, &delivery.Status, &delivery.Error); err != nil {
					deliveryRows.Close()
					return err
				}
				if userID.Valid {
					value := userID.Int32
					delivery.UserID = &value
				}
				delivery.Username = username.String
				mediaDetail.Deliveries = append(mediaDetail.Deliveries, delivery)
			}
			if err = deliveryRows.Err(); err != nil {
				deliveryRows.Close()
				return err
			}
			deliveryRows.Close()
		}
		detail.Media = append(detail.Media, mediaDetail)
	}
	return nil
}
