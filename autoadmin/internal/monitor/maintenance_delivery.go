package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

func (handler *Handler) activeMaintenance(ctx context.Context, target alertNotificationTarget, now time.Time) []maintenanceWindowHit {
	lookup := handler.maintenanceLookup
	if lookup == nil {
		lookup = handler.matchedMaintenanceWindows
	}
	hits, err := lookup(ctx, target, now)
	if err != nil {
		// 查询/范围解析异常不是抑制依据；优先正常投递，同时留下可定位的运维诊断。
		slog.Warn("维护匹配失败，继续按通知策略投递", "alert_id", target.id, "error", err)
		return nil
	}
	return hits
}

func saveMaintenanceDecision(ctx context.Context, q *db.Queries, eventID int64, hits []maintenanceWindowHit, now time.Time, partial bool) error {
	for _, hit := range hits {
		if err := q.RecordMaintenanceSuppression(ctx, db.RecordMaintenanceSuppressionParams{
			EventID: eventID, WindowID: hit.ID, Title: hit.Title, EndsAt: hit.EndsAt, DecidedAt: now,
		}); err != nil {
			return err
		}
	}
	status := "suppressed"
	if partial {
		status = "partial_muted"
	}
	return q.MarkMaintenanceDecision(ctx, db.MarkMaintenanceDecisionParams{
		Status: status, ErrorMessage: fmt.Sprintf("维护窗口抑制通知：%s（共 %d 个窗口）", hits[0].Title, len(hits)), UpdateTime: now, ID: eventID,
	})
}

func (handler *Handler) holdNotification(ctx context.Context, eventID int64, target alertNotificationTarget) (bool, error) {
	now := time.Now().UTC()
	hits := handler.activeMaintenance(ctx, target, now)
	if len(hits) == 0 {
		return false, nil
	}
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return true, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	n, err := q.CountSuccessfulEventDeliveries(ctx, eventID)
	if err != nil {
		return true, err
	}
	if err := saveMaintenanceDecision(ctx, q, eventID, hits, now, n > 0); err != nil {
		return true, err
	}
	if err := q.SuppressOutstandingDeliveries(ctx, db.SuppressOutstandingDeliveriesParams{UpdateTime: now, EventID: eventID}); err != nil {
		return true, err
	}
	return true, tx.Commit()
}

func (handler *Handler) finishMuted(ctx context.Context, eventID int64, reason string) error {
	q := db.New(handler.db)
	n, err := q.CountSuccessfulEventDeliveries(ctx, eventID)
	if err != nil {
		return err
	}
	status := "skipped"
	if n > 0 {
		status = "partial_skipped"
	}
	return q.MarkMaintenanceDecision(ctx, db.MarkMaintenanceDecisionParams{ID: eventID, UpdateTime: time.Now().UTC(), Status: status, ErrorMessage: reason})
}

// API 单实例。按主键游标遍历，长期重叠窗口不应饿死后面的事件；不为每个窗口开定时器。
// pending/sending 也要拾取：重启可能发生在已创建事件或在一次补发中间。
func (handler *Handler) RunMaintenanceDelivery(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	startup := true
	for {
		before := time.Now().UTC()
		if !startup {
			before = before.Add(-6 * time.Minute)
		}
		if err := handler.resumeNotificationIntents(ctx); err != nil && ctx.Err() == nil {
			slog.Error("扫描未入队告警失败", "error", err)
		}
		if err := handler.scanMaintenanceDelivery(ctx, before); err != nil && ctx.Err() == nil {
			slog.Error("扫描维护通知待办失败", "error", err)
		}

		startup = false
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (handler *Handler) consumeNotificationIntent(target alertNotificationTarget) (int64, bool, error) {
	id, created, err := handler.enqueueAlertNotification(target, target.state)
	if err != nil {
		return 0, false, err
	}
	// event 的唯一键使先建事件后清意图可重入；清理失败也不能阻止已创建的事件尽快发送。
	if err := db.New(handler.db).CompleteNotificationIntent(context.Background(), db.CompleteNotificationIntentParams{AlertID: target.id, EventType: target.state}); err != nil {
		slog.Error("告警入队意图清理失败，稍后重试", "alert_id", target.id, "error", err)
	}
	return id, created, nil
}

func (handler *Handler) resumeNotificationIntents(ctx context.Context) error {
	var cursor db.ListNotificationIntentsParams
	for ctx.Err() == nil {
		rows, err := db.New(handler.db).ListNotificationIntents(ctx, cursor)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			cursor.AfterID, cursor.AfterType = row.AlertID, row.EventType
			id, created, err := handler.consumeNotificationIntent(alertNotificationTarget{id: row.AlertID, state: row.EventType, alertname: row.Alertname, severity: row.Severity, instance: row.Instance, labels: unmarshalAlertLabels(row.Labels)})
			if err != nil {
				slog.Error("恢复未入队告警失败，保留待办", "alert_id", row.AlertID, "error", err)
				continue
			}
			if created {
				handler.scanNotificationEvent(id)
			}
		}
		if len(rows) < 200 {
			return nil
		}
	}
	return ctx.Err()
}
func (handler *Handler) scanMaintenanceDelivery(ctx context.Context, staleBefore time.Time) error {
	var cursor int64
	for ctx.Err() == nil {
		ids, err := db.New(handler.db).ListUnfinishedNotificationIDs(ctx, db.ListUnfinishedNotificationIDsParams{AfterID: cursor, StaleBefore: staleBefore})
		if err != nil {
			return err
		}
		for _, id := range ids {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			cursor = id
			// 不阻塞在单事件的指数退避；失败的 pending 留给后续扫描。
			handler.scanNotificationEvent(id)
		}

		if len(ids) < 200 {
			return nil
		}
	}
	return ctx.Err()
}

func (handler *Handler) scanNotificationEvent(id int64) {
	if _, busy := handler.activeDispatch.LoadOrStore(id, true); busy {
		return
	}
	defer handler.activeDispatch.Delete(id)
	if _, err := handler.sendAlertNotificationEvent(id); err != nil {
		slog.Error("维护通知重评失败，保留待办", "event_id", id, "error", err)
	}
}
