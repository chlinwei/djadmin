package monitor

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func optionalLikeParam(context *gin.Context, name string) sql.NullString {
	value := strings.TrimSpace(context.Query(name))
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: "%" + value + "%", Valid: true}
}

// optionalTimeParam 兼容前端可能传的几种时间格式；解析失败时当作未传，交给调用方决定要不要报错。
func optionalTimeParam(context *gin.Context, name string) sql.NullTime {
	value := strings.TrimSpace(context.Query(name))
	if value == "" {
		return sql.NullTime{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return sql.NullTime{Time: parsed, Valid: true}
		}
	}
	return sql.NullTime{}
}

func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
func nullInt32Ptr(value sql.NullInt32) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}
func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
func nullFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

// ---- monitor_target_install_history ----

type installHistoryResponse struct {
	ID                        int64           `json:"id"`
	CreateTime                time.Time       `json:"create_time"`
	UpdateTime                time.Time       `json:"update_time"`
	Remark                    string          `json:"remark"`
	Action                    string          `json:"action"`
	TriggerType               string          `json:"trigger_type"`
	Status                    string          `json:"status"`
	HostIDSnapshot            *int32          `json:"host_id_snapshot"`
	HostNameSnapshot          string          `json:"host_name_snapshot"`
	HostIPSnapshot            string          `json:"host_ip_snapshot"`
	ExporterTypeSnapshot      string          `json:"exporter_type_snapshot"`
	SummaryMessage            string          `json:"summary_message"`
	StdoutSnapshot            string          `json:"stdout_snapshot"`
	StderrSnapshot            string          `json:"stderr_snapshot"`
	ErrorMessageSnapshot      string          `json:"error_message_snapshot"`
	ResultSummarySnapshot     json.RawMessage `json:"result_summary_snapshot"`
	RequestedUserIDSnapshot   *int32          `json:"requested_user_id_snapshot"`
	RequestedUsernameSnapshot string          `json:"requested_username_snapshot"`
	StartTime                 *time.Time      `json:"start_time"`
	EndTime                   *time.Time      `json:"end_time"`
	DurationSeconds           *float64        `json:"duration_seconds"`
	HostID                    *int64          `json:"host_id"`
	TargetID                  *int64          `json:"target_id"`
	LogCollectionTargetID     *int64          `json:"log_collection_target_id"`
	HostName                  string          `json:"host_name"`
	HostIP                    string          `json:"host_ip"`
	TargetExporterType        string          `json:"target_exporter_type"`
	ManagedTargetID           *int64          `json:"managed_target_id"`
	TargetType                string          `json:"target_type"`
}

func installHistoryResponseFrom(row db.ListInstallHistoriesRow) installHistoryResponse {
	return installHistoryResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark.String,
		Action: row.Action, TriggerType: row.TriggerType, Status: row.Status,
		HostIDSnapshot: nullInt32Ptr(row.HostIDSnapshot), HostNameSnapshot: row.HostNameSnapshot,
		HostIPSnapshot: row.HostIpSnapshot, ExporterTypeSnapshot: row.ExporterTypeSnapshot,
		SummaryMessage: row.SummaryMessage, StdoutSnapshot: row.StdoutSnapshot, StderrSnapshot: row.StderrSnapshot,
		ErrorMessageSnapshot: row.ErrorMessageSnapshot, ResultSummarySnapshot: row.ResultSummarySnapshot,
		RequestedUserIDSnapshot: nullInt32Ptr(row.RequestedUserIDSnapshot), RequestedUsernameSnapshot: row.RequestedUsernameSnapshot,
		StartTime: nullTimePtr(row.StartTime), EndTime: nullTimePtr(row.EndTime), DurationSeconds: nullFloat64Ptr(row.DurationSeconds),
		HostID: nullInt64Ptr(row.HostID), TargetID: nullInt64Ptr(row.TargetID), LogCollectionTargetID: nullInt64Ptr(row.LogCollectionTargetID),
		HostName: row.HostName, HostIP: row.HostIp, TargetExporterType: row.TargetExporterType,
		ManagedTargetID: nullInt64Ptr(row.ManagedTargetID), TargetType: row.TargetType,
	}
}

func (handler *Handler) ListInstallHistories(context *gin.Context) {
	handler.installHistories(context, 0)
}
func (handler *Handler) GetInstallHistory(context *gin.Context) {
	handler.installHistories(context, parseID(context.Param("id")))
}

func (handler *Handler) installHistories(context *gin.Context, id int64) {
	page, size := pagination(context)
	idFilter := sql.NullInt64{}
	if id > 0 {
		idFilter = sql.NullInt64{Int64: id, Valid: true}
	}
	targetID, logCollectionTargetID := optionalInt64Param(context, "target_id"), optionalInt64Param(context, "log_collection_target_id")
	action, triggerType, status := optionalStringParam(context, "action"), optionalStringParam(context, "trigger_type"), optionalStringParam(context, "status")
	keyword := optionalLikeParam(context, "keyword")
	startTime, endTime := optionalTimeParam(context, "start_time"), optionalTimeParam(context, "end_time")

	queries := db.New(handler.db)
	count, err := queries.CountInstallHistories(context, db.CountInstallHistoriesParams{
		ID: idFilter, TargetID: targetID, LogCollectionTargetID: logCollectionTargetID,
		Action: action, TriggerType: triggerType, Status: status, Keyword: keyword, StartTime: startTime, EndTime: endTime,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListInstallHistories(context, db.ListInstallHistoriesParams{
		ID: idFilter, TargetID: targetID, LogCollectionTargetID: logCollectionTargetID,
		Action: action, TriggerType: triggerType, Status: status, Keyword: keyword, StartTime: startTime, EndTime: endTime,
		Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]installHistoryResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, installHistoryResponseFrom(row))
	}
	if id > 0 {
		if len(items) == 0 {
			response.BusinessError(context, 404, "install history not found", nil)
			return
		}
		response.Success(context, items[0])
		return
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

// ---- monitor_alert_history ----

type alertHistoryResponse struct {
	ID                        int64           `json:"id"`
	CreateTime                time.Time       `json:"create_time"`
	UpdateTime                time.Time       `json:"update_time"`
	Remark                    string          `json:"remark"`
	Fingerprint               string          `json:"fingerprint"`
	Alertname                 string          `json:"alertname"`
	Severity                  string          `json:"severity"`
	Instance                  string          `json:"instance"`
	Labels                    json.RawMessage `json:"labels"`
	Annotations               json.RawMessage `json:"annotations"`
	GeneratorURL              string          `json:"generator_url"`
	State                     string          `json:"state"`
	StartedAt                 time.Time       `json:"started_at"`
	ResolvedAt                *time.Time      `json:"resolved_at"`
	LastSeenAt                time.Time       `json:"last_seen_at"`
	ResolvedByReconciliation  bool            `json:"resolved_by_reconciliation"`
	RuleGroup                 string          `json:"rule_group"`
	RuleSnapshot              json.RawMessage `json:"rule_snapshot"`
	RuleDetails               json.RawMessage `json:"rule_details"`
	Source                    string          `json:"source"`
	NotificationCount         int64           `json:"notification_count"`
	NotificationDeliveryCount int64           `json:"notification_delivery_count"`
	NotificationStatus        string          `json:"notification_status"`
}

// notificationStatusFrom 判断口径与旧版 notificationStatus(map) 完全一致，只是参数从 gin.H 换成具名 int64。
func notificationStatusFrom(count, failed, active, delivery int64) string {
	if count == 0 {
		return "none"
	}
	if failed > 0 || (active == 0 && delivery == 0) {
		return "failed"
	}
	if active > 0 {
		return "in_progress"
	}
	return "success"
}

func alertHistoryResponseFrom(row db.ListAlertHistoriesRow) alertHistoryResponse {
	return alertHistoryResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark.String,
		Fingerprint: row.Fingerprint, Alertname: row.Alertname, Severity: row.Severity, Instance: row.Instance,
		Labels: row.Labels, Annotations: row.Annotations, GeneratorURL: row.GeneratorUrl, State: row.State,
		StartedAt: row.StartedAt, ResolvedAt: nullTimePtr(row.ResolvedAt), LastSeenAt: row.LastSeenAt,
		ResolvedByReconciliation: row.ResolvedByReconciliation, RuleGroup: row.RuleGroup,
		RuleSnapshot: row.RuleSnapshot, RuleDetails: row.RuleSnapshot, Source: row.Source,
		NotificationCount: row.NotificationCount, NotificationDeliveryCount: row.NotificationDeliveryCount,
		NotificationStatus: notificationStatusFrom(row.NotificationCount, row.NotificationFailedCount, row.NotificationActiveCount, row.NotificationDeliveryCount),
	}
}

func (handler *Handler) ListAlertHistories(context *gin.Context) { handler.alertHistories(context, 0) }
func (handler *Handler) GetAlertHistory(context *gin.Context) {
	handler.alertHistories(context, parseID(context.Param("id")))
}

func (handler *Handler) alertHistories(context *gin.Context, id int64) {
	page, size := pagination(context)
	idFilter := sql.NullInt64{}
	if id > 0 {
		idFilter = sql.NullInt64{Int64: id, Valid: true}
	}
	state, severity := optionalStringParam(context, "state"), optionalStringParam(context, "severity")
	keyword := optionalLikeParam(context, "keyword")
	startTime, endTime := optionalTimeParam(context, "start_time"), optionalTimeParam(context, "end_time")

	queries := db.New(handler.db)
	count, err := queries.CountAlertHistories(context, db.CountAlertHistoriesParams{
		ID: idFilter, State: state, Severity: severity, Keyword: keyword, StartTime: startTime, EndTime: endTime,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListAlertHistories(context, db.ListAlertHistoriesParams{
		ID: idFilter, State: state, Severity: severity, Keyword: keyword, StartTime: startTime, EndTime: endTime,
		Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]alertHistoryResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, alertHistoryResponseFrom(row))
	}
	if id > 0 {
		if len(items) == 0 {
			response.BusinessError(context, 404, "alert history not found", nil)
			return
		}
		response.Success(context, items[0])
		return
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

// ---- monitor_alert_notification_event/delivery ----

type notificationDeliveryResponse struct {
	ID           int64      `json:"id"`
	UserID       *int32     `json:"user_id"`
	Username     string     `json:"username"`
	MediaID      *int64     `json:"media_id"`
	MediaName    string     `json:"media_name"`
	MediaType    string     `json:"media_type"`
	Address      string     `json:"address"`
	Status       string     `json:"status"`
	AttemptCount int64      `json:"attempt_count"`
	ErrorMessage string     `json:"error_message"`
	SentAt       *time.Time `json:"sent_at"`
	CreateTime   time.Time  `json:"create_time"`
}

type notificationEventResponse struct {
	ID               int64                          `json:"id"`
	CreateTime       time.Time                      `json:"create_time"`
	UpdateTime       time.Time                      `json:"update_time"`
	Remark           string                         `json:"remark"`
	EventType        string                         `json:"event_type"`
	DeduplicationKey string                         `json:"deduplication_key"`
	Status           string                         `json:"status"`
	AttemptCount     int64                          `json:"attempt_count"`
	ErrorMessage     string                         `json:"error_message"`
	SentAt           *time.Time                     `json:"sent_at"`
	AlertID          int64                          `json:"alert_id"`
	Deliveries       []notificationDeliveryResponse `json:"deliveries"`
}

func (handler *Handler) AlertNotificationStatus(context *gin.Context) {
	id := parseID(context.Param("id"))
	queries := db.New(handler.db)
	alert, err := queries.GetAlertHistoryAlertnameInstance(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	eventRows, err := queries.ListAlertNotificationEvents(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	events := make([]notificationEventResponse, 0, len(eventRows))
	for _, eventRow := range eventRows {
		deliveryRows, deliveryErr := queries.ListAlertNotificationDeliveries(context, eventRow.ID)
		if deliveryErr != nil {
			response.Error(context, deliveryErr)
			return
		}
		deliveries := make([]notificationDeliveryResponse, 0, len(deliveryRows))
		for _, deliveryRow := range deliveryRows {
			deliveries = append(deliveries, notificationDeliveryResponse{
				ID: deliveryRow.ID, UserID: nullInt32Ptr(deliveryRow.UserID), Username: deliveryRow.Username,
				MediaID: nullInt64Ptr(deliveryRow.MediaID), MediaName: deliveryRow.MediaName, MediaType: deliveryRow.MediaType,
				Address: deliveryRow.Address, Status: deliveryRow.Status, AttemptCount: int64(deliveryRow.AttemptCount),
				ErrorMessage: deliveryRow.ErrorMessage, SentAt: nullTimePtr(deliveryRow.SentAt), CreateTime: deliveryRow.CreateTime,
			})
		}
		event := notificationEventResponse{
			ID: eventRow.ID, CreateTime: eventRow.CreateTime, UpdateTime: eventRow.UpdateTime, Remark: eventRow.Remark.String,
			EventType: eventRow.EventType, DeduplicationKey: eventRow.DeduplicationKey, Status: eventRow.Status,
			AttemptCount: int64(eventRow.AttemptCount), ErrorMessage: eventRow.ErrorMessage, SentAt: nullTimePtr(eventRow.SentAt),
			AlertID: eventRow.AlertID, Deliveries: deliveries,
		}
		// 没有任何投递明细却标记成功，说明状态不可信——不知道到底发没发给谁，必须改判失败并给出原因。
		if len(deliveries) == 0 && event.Status == "success" {
			event.Status = "failed"
			event.ErrorMessage = "没有投递明细，无法确认实际接收用户、媒介和地址"
		}
		events = append(events, event)
	}
	response.Success(context, gin.H{"alert_id": id, "alertname": alert.Alertname, "instance": alert.Instance, "events": events})
}
