package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectCount(mock sqlmock.Sqlmock, query string, id, n int64) {
	mock.ExpectQuery(query).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(n))
}
func expectDecision(mock sqlmock.Sqlmock, id int64, status string) {
	mock.ExpectExec("MarkMaintenanceDecision").WithArgs(status, sqlmock.AnyArg(), sqlmock.AnyArg(), id).WillReturnResult(sqlmock.NewResult(0, 1))
}
func expectHits(mock sqlmock.Sqlmock, id int64, hits []maintenanceWindowHit) {
	for _, hit := range hits {
		mock.ExpectExec("RecordMaintenanceSuppression").WithArgs(id, hit.ID, hit.Title, hit.EndsAt, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	}
}
func requireNotificationMocks(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceOverlapSurvivesRestartAndFirstEnd(t *testing.T) {
	h, m, pool := notificationTestHandler(t)
	now := time.Now().UTC()
	hits := []maintenanceWindowHit{{1, "数据库升级", now.Add(time.Hour)}, {2, "网络升级", now.Add(2 * time.Hour)}}
	h.maintenanceLookup = func(context.Context, alertNotificationTarget, time.Time) ([]maintenanceWindowHit, error) {
		return hits, nil
	}
	m.ExpectBegin()
	expectAlertNotificationEventInsert(m, "firing", "7:firing", 7, 1, true)
	expectHits(m, 1, hits)
	expectDecision(m, 1, "suppressed")
	m.ExpectCommit()
	if _, ok, err := h.enqueueAlertNotification(alertNotificationTarget{id: 7}, "firing"); !ok || err != nil {
		t.Fatal(ok, err)
	}

	// 新 Handler 无进程内记忆；第一个窗口已结束，但另一个窗口保持静音。
	restarted := &Handler{db: pool, maintenanceLookup: func(context.Context, alertNotificationTarget, time.Time) ([]maintenanceWindowHit, error) {
		return hits[1:], nil
	}}
	m.ExpectQuery("ListUnfinishedNotificationIDs").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "suppressed", "firing", 2))
	m.ExpectBegin()
	expectCount(m, "CountSuccessfulEventDeliveries", 1, 0)
	expectHits(m, 1, hits[1:])
	expectDecision(m, 1, "suppressed")
	m.ExpectExec("SuppressOutstandingDeliveries").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectCommit()
	if err := restarted.scanMaintenanceDelivery(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	requireNotificationMocks(t, m)
}

func expectBindingEmails(m sqlmock.Sqlmock, addresses string) {
	m.ExpectQuery("ListAlertMediaBindingsByMedia").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "recipients", "timezone"}).AddRow(5, "zhang", []byte(addresses), "UTC"))
}
func expectAddress(m sqlmock.Sqlmock, id int64, address, status string) {
	expectAlertNotificationDeliveryCreate(m, id, 5, address)
	m.ExpectQuery("GetAlertNotificationDeliveryStatus").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(status))
}
func expectSend(m sqlmock.Sqlmock) {
	m.ExpectExec("MarkAlertNotificationDeliverySending").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("MarkAlertNotificationDeliverySuccess").WillReturnResult(sqlmock.NewResult(0, 1))
}
func TestMaintenanceStartsBetweenAddressesAndReleasesOnlyUnsent(t *testing.T) {
	h, m, _ := notificationTestHandler(t)
	sent := []string{}
	released := false
	h.smtpSend = func(_ map[string]any, subject, body string, to []string) (bool, string) {
		if released && (!strings.Contains(subject, "维护已结束") || !strings.Contains(body, "原始触发时间")) {
			t.Fatal(subject, body)
		}
		sent = append(sent, to...)
		return true, ""
	}
	hits := []maintenanceWindowHit{{3, "网络维护", time.Now().UTC().Add(time.Hour)}}
	h.maintenanceLookup = func(context.Context, alertNotificationTarget, time.Time) ([]maintenanceWindowHit, error) {
		if len(sent) > 0 && !released {
			return hits, nil
		}
		return nil, nil
	}
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "pending", "firing", 0))
	expectPolicyMediaHit(m, "邮件", "email")
	m.ExpectExec("MarkAlertNotificationEventSending").WillReturnResult(sqlmock.NewResult(0, 1))
	expectBindingEmails(m, `["a@b.com","b@b.com"]`)
	expectAddress(m, 11, "a@b.com", "pending")
	expectSend(m)
	expectAddress(m, 12, "b@b.com", "pending")
	m.ExpectBegin()
	expectCount(m, "CountSuccessfulEventDeliveries", 1, 1)
	expectHits(m, 1, hits)
	expectDecision(m, 1, "partial_muted")
	m.ExpectExec("SuppressOutstandingDeliveries").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	if retry, err := h.sendAlertNotificationEvent(1); retry || err != nil {
		t.Fatal(retry, err)
	}
	if strings.Join(sent, ",") != "a@b.com" {
		t.Fatal(sent)
	}
	released = true
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "partial_muted", "firing", 1))
	expectPolicyMediaHit(m, "邮件", "email")
	m.ExpectExec("MarkAlertNotificationEventSending").WillReturnResult(sqlmock.NewResult(0, 1))
	expectBindingEmails(m, `["a@b.com","b@b.com"]`)
	expectAddress(m, 11, "a@b.com", "success")
	expectAddress(m, 12, "b@b.com", "suppressed")
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "sending", "firing", 1))
	expectSend(m)
	m.ExpectQuery("ListAlertNotificationDeliveries").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	m.ExpectExec("MarkAlertNotificationEventSuccess").WillReturnResult(sqlmock.NewResult(0, 1))
	if retry, err := h.sendAlertNotificationEvent(1); retry || err != nil {
		t.Fatal(retry, err)
	}
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "success", "firing", 1))
	if retry, err := h.sendAlertNotificationEvent(1); retry || err != nil {
		t.Fatal(retry, err)
	}
	if strings.Join(sent, ",") != "a@b.com,b@b.com" {
		t.Fatal(sent)
	}
	requireNotificationMocks(t, m)
}

func TestMaintenanceRecoveryIntersectsActualAndCurrentRecipients(t *testing.T) {
	h, m, _ := notificationTestHandler(t)
	// 恢复发生在窗口到期之后，resolved 事件本身没有抑制快照。
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("resolved", "pending", "resolved", 0))
	expectCount(m, "CountFiringMaintenanceDecisions", 7, 1)
	expectPolicyMediaHit(m, "邮件", "email")
	m.ExpectExec("MarkAlertNotificationEventSending").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("ListMaintenanceRecoveryRecipients").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"user_id", "media_id", "address"}).
		AddRow(5, 2, "current@b.com").AddRow(5, 2, "removed@b.com").AddRow(7, 2, "other-user@b.com"))
	expectBindingEmails(m, `["current@b.com","new@b.com","other-user@b.com"]`)
	expectAddress(m, 12, "current@b.com", "pending")
	expectSend(m)
	m.ExpectExec("MarkAlertNotificationEventSuccess").WillReturnResult(sqlmock.NewResult(0, 1))
	sent := 0
	h.smtpSend = func(_ map[string]any, _, _ string, to []string) (bool, string) {
		if len(to) != 1 || to[0] != "current@b.com" {
			t.Fatal(to)
		}
		sent++
		return true, ""
	}
	if retry, err := h.sendAlertNotificationEvent(1); retry || err != nil || sent != 1 {
		t.Fatal(retry, err, sent)
	}
	requireNotificationMocks(t, m)
}

func TestMaintenanceRecoveryRespectsDisabledPolicy(t *testing.T) {
	h, m, _ := notificationTestHandler(t)
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("resolved", "suppressed", "resolved", 1))
	now := time.Now().UTC()
	m.ExpectQuery("ListNotificationPolicyNodes").WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "name", "position", "remark", "matchers", "media_ids", "user_group_ids", "notify_on_firing", "notify_on_resolved", "create_time", "update_time"}).
		AddRow(1, 0, "默认", 0, "", []byte("[]"), "[2]", nil, true, false, now, now))
	expectCount(m, "CountSuccessfulEventDeliveries", 1, 0)
	expectDecision(m, 1, "skipped")
	if retry, err := h.sendAlertNotificationEvent(1); retry || err != nil {
		t.Fatal(retry, err)
	}
	requireNotificationMocks(t, m)
}

func TestMaintenanceResolvedWithinWindowNeverReplaysFiring(t *testing.T) {
	h, m, _ := notificationTestHandler(t)
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "suppressed", "resolved", 1))
	expectCount(m, "CountSuccessfulEventDeliveries", 1, 0)
	expectDecision(m, 1, "skipped")
	if retry, err := h.sendAlertNotificationEvent(1); retry || err != nil {
		t.Fatal(retry, err)
	}
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("resolved", "suppressed", "resolved", 1))
	expectPolicyMediaHit(m, "邮件", "email")
	m.ExpectExec("MarkAlertNotificationEventSending").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectQuery("ListMaintenanceRecoveryRecipients").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"user_id", "media_id", "address"}))
	expectCount(m, "CountSuccessfulEventDeliveries", 2, 0)
	expectDecision(m, 2, "skipped")
	if retry, err := h.sendAlertNotificationEvent(2); retry || err != nil {
		t.Fatal(retry, err)
	}
	requireNotificationMocks(t, m)
}

func TestMaintenanceLookupFailureRoutesNormally(t *testing.T) {
	h, m, _ := notificationTestHandler(t)
	h.maintenanceLookup = func(context.Context, alertNotificationTarget, time.Time) ([]maintenanceWindowHit, error) {
		return nil, errors.New("asset lookup failed")
	}
	expectPolicyMediaHit(m, "公司邮箱", "email")
	expectAlertNotificationEventInsert(m, "firing", "7:firing", 7, 9, true)
	if _, created, err := h.enqueueAlertNotification(alertNotificationTarget{id: 7}, "firing"); err != nil || !created {
		t.Fatal(created, err)
	}
	requireNotificationMocks(t, m)
}

func TestRestartDrainsPersistedIntentOnlyAfterSuccessfulEnqueue(t *testing.T) {
	h, m, _ := notificationTestHandler(t)
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"alert_id", "event_type", "alertname", "severity", "instance", "labels"}).AddRow(7, "firing", "disk", "warning", "h", []byte("{}"))
	}
	m.ExpectQuery("ListNotificationIntents").WillReturnRows(rows())
	m.ExpectQuery("ListNotificationPolicyNodes").WillReturnError(errors.New("temporarily unavailable"))
	if err := h.resumeNotificationIntents(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 第二次扫描时幂等事件已存在（进程可能恰好在 INSERT 后崩溃），只清意图，不再建第二封邮件。
	m.ExpectQuery("ListNotificationIntents").WillReturnRows(rows())
	expectPolicyMediaHit(m, "邮件", "email")
	expectAlertNotificationEventInsert(m, "firing", "7:firing", 7, 1, false)
	m.ExpectExec("CompleteNotificationIntent").WithArgs(7, "firing").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := h.resumeNotificationIntents(context.Background()); err != nil {
		t.Fatal(err)
	}
	requireNotificationMocks(t, m)
}

func TestRestartDoesNotResendUnconfirmedSMTP(t *testing.T) {
	h, m, _ := notificationTestHandler(t)
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "sending", "firing", 1))
	expectPolicyMediaHit(m, "邮件", "email")
	m.ExpectExec("MarkAlertNotificationEventSending").WillReturnResult(sqlmock.NewResult(0, 1))
	expectBindingEmails(m, `["a@b.com"]`)
	expectAddress(m, 11, "a@b.com", "sending")
	m.ExpectQuery("GetAlertNotificationEventDispatch").WillReturnRows(dispatchTestRow("firing", "sending", "firing", 1))
	m.ExpectExec("MarkAlertNotificationEventFailed").WithArgs("zhang / 邮件 / a@b.com: 上次投递结果未确认，须人工核对", sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(0, 1))
	if retry, err := h.sendAlertNotificationEvent(1); retry || err != nil {
		t.Fatal(retry, err)
	}
	requireNotificationMocks(t, m)
}
