package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ---- 纯匹配逻辑（对齐 Django tasks.resolve_alert_media）----

func TestMergeAlertLabelsConvenienceKeys(t *testing.T) {
	merged := mergeAlertLabels(map[string]any{"alertname": "HighCPU", "job": "node"}, "HighCPU", "warning", "host-1")
	if merged["alertname"] != "HighCPU" || merged["severity"] != "warning" || merged["instance"] != "host-1" || merged["job"] != "node" {
		t.Fatalf("unexpected merged labels: %v", merged)
	}
	// 便捷键缺失时补空串，与 Django labels.update 行为一致。
	empty := mergeAlertLabels(map[string]any{}, "", "", "")
	if empty["severity"] != "" || empty["alertname"] != "" || empty["instance"] != "" {
		t.Fatalf("unexpected empty merged labels: %v", empty)
	}
}

func TestPolicyMatcherMatch(t *testing.T) {
	labels := map[string]string{"alertname": "HighDiskUsage", "severity": "warning"}
	scopeNodes := map[string]bool{"business:3": true, "environment:7": true}
	cases := []struct {
		matcher policyMatcher
		want    bool
	}{
		{policyMatcher{Type: "label", Label: "severity", Operator: "=", Value: "warning"}, true},
		{policyMatcher{Type: "label", Label: "severity", Operator: "=", Value: "critical"}, false},
		{policyMatcher{Type: "label", Label: "severity", Operator: "!=", Value: "critical"}, true},
		// Prometheus 语义：缺失 label 视为空串。
		{policyMatcher{Type: "label", Label: "missing", Operator: "!=", Value: "x"}, true},
		{policyMatcher{Type: "label", Label: "missing", Operator: "!~", Value: ".+"}, true},
		{policyMatcher{Type: "label", Label: "alertname", Operator: "=~", Value: "High.*"}, true},
		{policyMatcher{Type: "label", Label: "alertname", Operator: "!~", Value: "Disk$"}, true},
		{policyMatcher{Type: "label", Label: "alertname", Operator: "=~", Value: "^Low"}, false},
		{policyMatcher{Type: "tree", NodeType: "business", ID: 3}, true},
		{policyMatcher{Type: "tree", NodeType: "business", ID: 4}, false},
		{policyMatcher{Type: "tree", NodeType: "service", ID: 9}, false},
	}
	for _, testCase := range cases {
		matched, _ := policyMatcherMatch(testCase.matcher, labels, scopeNodes)
		if matched != testCase.want {
			t.Fatalf("matcher %+v: got %v want %v", testCase.matcher, matched, testCase.want)
		}
	}
}

func TestNormalizePolicyMatchers(t *testing.T) {
	if _, errMsg := normalizePolicyMatchers(nil); errMsg != "" {
		t.Fatalf("nil matchers must pass: %s", errMsg)
	}
	valid, _ := json.Marshal([]policyMatcher{
		{Type: "label", Label: "severity", Operator: "=", Value: "critical"},
		{Type: "tree", NodeType: "business", ID: 3},
	})
	if _, errMsg := normalizePolicyMatchers(valid); errMsg != "" {
		t.Fatalf("valid matchers must pass: %s", errMsg)
	}
	bad, _ := json.Marshal([]policyMatcher{{Type: "label", Label: "severity", Operator: "~", Value: "x"}})
	if _, errMsg := normalizePolicyMatchers(bad); errMsg == "" {
		t.Fatal("unknown operator must fail")
	}
	badRegex, _ := json.Marshal([]policyMatcher{{Type: "label", Label: "a", Operator: "=~", Value: "("}})
	if _, errMsg := normalizePolicyMatchers(badRegex); errMsg == "" {
		t.Fatal("invalid regex must fail")
	}
}

func TestResolvePolicyRouteAndEffectiveMedia(t *testing.T) {
	root := &policyNode{ID: 1, Name: "root", MediaIDs: []int64{2}}
	child := &policyNode{ID: 2, ParentID: 1, Name: "critical", Matchers: []policyMatcher{{Type: "label", Label: "severity", Operator: "=", Value: "critical"}}}
	grandchild := &policyNode{ID: 3, ParentID: 2, Name: "mute", Matchers: []policyMatcher{{Type: "label", Label: "env", Operator: "=", Value: "prod"}}, MediaIDs: []int64{}}
	root.Children = []*policyNode{child}
	child.Children = []*policyNode{grandchild}

	labels := map[string]string{"severity": "critical", "env": "prod"}
	path := resolvePolicyRoute(root, labels, map[string]bool{})
	if len(path) != 3 || path[len(path)-1].ID != 3 {
		t.Fatalf("unexpected path: %+v", path)
	}
	if ids := effectivePolicyMedia(path); len(ids) != 0 {
		t.Fatalf("explicit empty media must mute: %v", ids)
	}

	// env 不命中 grandchild：停在 child，继承根出口。
	path = resolvePolicyRoute(root, map[string]string{"severity": "critical"}, map[string]bool{})
	if len(path) != 2 || path[len(path)-1].ID != 2 {
		t.Fatalf("unexpected path: %+v", path)
	}
	if ids := effectivePolicyMedia(path); len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("inherited media expected: %v", ids)
	}
}

func TestPolicyAllowsEvent(t *testing.T) {
	node := &policyNode{NotifyOnFiring: true, NotifyOnResolved: false}
	if !policyAllowsEvent(node, "firing") || policyAllowsEvent(node, "resolved") {
		t.Fatal("unexpected event allowance")
	}
}

// ---- enqueue 预判：无媒介不建事件 ----

func notificationTestHandler(t *testing.T) (*Handler, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &Handler{db: database, maintenanceLookup: func(context.Context, alertNotificationTarget, time.Time) ([]maintenanceWindowHit, error) {
		return nil, nil
	}}, mock, database
}

// policyTreeRows 根节点：matchers=[] 恒命中，出口媒介 [2]。
// 列集与 ListNotificationPolicyNodes 一致（树加载与管理列表共用一条查询，含建/改时间）。
func policyTreeRows() *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{"id", "parent_id", "name", "position", "remark", "matchers", "media_ids", "user_group_ids", "notify_on_firing", "notify_on_resolved", "create_time", "update_time"}).
		AddRow(int64(1), int64(0), "默认策略", int32(0), "", []byte(`[]`), "[2]", nil, true, true, now, now)
}

func expectPolicyMediaHit(mock sqlmock.Sqlmock, name, mediaType string) {
	mock.ExpectQuery("FROM monitor_notification_policy").WillReturnRows(policyTreeRows())
	mock.ExpectQuery("SELECT id, name, media_type, config FROM monitor_alert_media").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "media_type", "config"}).
			AddRow(int64(2), name, mediaType, []byte(`{"smtpServer":"smtp.x.com"}`)))
}

func expectPolicyMediaMiss(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM monitor_notification_policy").WillReturnRows(policyTreeRows())
	mock.ExpectQuery("SELECT id, name, media_type, config FROM monitor_alert_media").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "media_type", "config"}))
}

func TestEnqueueAlertNotificationSkipsWithoutMedia(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	// 策略树出口为空：不得出现任何 INSERT。
	expectPolicyMediaMiss(mock)
	_, created, err := handler.enqueueAlertNotification(alertNotificationTarget{id: 7, alertname: "HighDiskUsage", severity: "warning", labels: map[string]any{"severity": "warning"}}, "firing")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if created {
		t.Fatal("must not create event without deliverable media")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestEnqueueAlertNotificationCreatesDedupedEvent(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	expectPolicyMediaHit(mock, "公司邮箱", "email")
	expectAlertNotificationEventInsert(mock, "firing", "7:firing", 7, 9, true)
	eventID, created, err := handler.enqueueAlertNotification(alertNotificationTarget{id: 7, alertname: "HighDiskUsage", severity: "warning", labels: map[string]any{"severity": "warning"}}, "firing")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if !created || eventID != 9 {
		t.Fatalf("expected event created with id 9, got id=%d created=%v", eventID, created)
	}
	// 第二次同 key 入队：预判仍命中但 INSERT IGNORE 去重，不计入也不派发。
	expectPolicyMediaHit(mock, "公司邮箱", "email")
	expectAlertNotificationEventInsert(mock, "firing", "7:firing", 7, 9, false)
	_, created, err = handler.enqueueAlertNotification(alertNotificationTarget{id: 7, alertname: "HighDiskUsage", severity: "warning", labels: map[string]any{"severity": "warning"}}, "firing")
	if err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	if created {
		t.Fatal("duplicate key must not create a second event")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestEnqueueWebhookNotificationsCounts(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	// 第一个目标命中并创建事件；第二个目标预判无媒介不建事件。
	expectPolicyMediaHit(mock, "公司邮箱", "email")
	expectAlertNotificationEventInsert(mock, "firing", "7:firing", 7, 9, true)
	mock.ExpectExec("CompleteNotificationIntent").WithArgs(7, "firing").WillReturnResult(sqlmock.NewResult(0, 1))
	expectPolicyMediaMiss(mock)
	mock.ExpectExec("CompleteNotificationIntent").WithArgs(8, "firing").WillReturnResult(sqlmock.NewResult(0, 1))
	count, ids := handler.enqueueWebhookNotifications([]alertNotificationTarget{
		{id: 7, alertname: "HighDiskUsage", severity: "warning", state: "firing", labels: map[string]any{"severity": "warning"}},
		{id: 8, alertname: "Other", severity: "info", state: "firing", labels: map[string]any{"severity": "info"}},
	})
	if count != 1 || len(ids) != 1 || ids[0] != 9 {
		t.Fatalf("unexpected enqueue results: count=%d ids=%v", count, ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ---- 发送：绑定筛选 ----

// eventRow 对应 GetAlertNotificationEventDispatch：事件字段（含 status/attempt_count，
// 原实现分两条语句读同一行，现在合并为一条）与告警关键字段一次返回。
func eventRow() *sqlmock.Rows {
	return dispatchTestRow("firing", "pending", "resolved", 0)
}

func dispatchTestRow(eventType, status, state string, suppressed int64) *sqlmock.Rows {
	startedAt := time.Date(2026, 9, 17, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	return sqlmock.NewRows([]string{"event_type", "attempt_count", "status", "id", "alertname", "severity", "instance", "state", "labels", "annotations", "started_at", "resolved_at", "generator_url", "suppression_count"}).
		AddRow(eventType, int64(0), status, int64(7), "HighDiskUsage", "warning", "host-1", state,
			[]byte(`{"severity":"warning","environment":"production"}`), []byte(`{"summary":"磁盘空间不足","description":"可用空间低于 10%"}`),
			startedAt, startedAt.Add(time.Minute), "https://prometheus.example.com/graph?expr=disk", suppressed)
}

func TestSendAlertNotificationEventNoBindings(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	handler.smtpSend = func(config map[string]any, subject, body string, recipients []string) (bool, string) {
		t.Fatal("smtp must not be called without bindings")
		return true, ""
	}
	mock.ExpectQuery("FROM monitor_alert_notification_event e JOIN monitor_alert_history").WillReturnRows(eventRow())
	expectPolicyMediaHit(mock, "公司邮箱", "email")
	mock.ExpectExec("SET status='sending', attempt_count=attempt_count\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
	// 该媒介没有任何 enabled 绑定。
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN sys_user u").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "recipients", "timezone"}))
	mock.ExpectExec("SET status='failed'").
		WithArgs("媒介 \"公司邮箱\" 没有任何用户绑定", sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	retryable, err := handler.sendAlertNotificationEvent(1)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if retryable {
		t.Fatal("no-binding failure must not be retryable")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSendAlertNotificationEventNonEmailMediaOnly(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	handler.smtpSend = func(config map[string]any, subject, body string, recipients []string) (bool, string) {
		t.Fatal("smtp must not be called for non-email media")
		return true, ""
	}
	mock.ExpectQuery("FROM monitor_alert_notification_event e JOIN monitor_alert_history").WillReturnRows(eventRow())
	// 预判能命中媒介（非 email），但发送阶段全部被跳过。
	expectPolicyMediaHit(mock, "钉钉群", "webhook")
	mock.ExpectExec("SET status='sending', attempt_count=attempt_count\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SET status='failed'").
		WithArgs("匹配的告警媒介没有可投递的用户地址", sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	retryable, err := handler.sendAlertNotificationEvent(1)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if retryable {
		t.Fatal("no-address failure must not be retryable")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ---- 发送：delivery 状态流转（成功）----

func TestSendAlertNotificationEventDeliverySuccess(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	wantTimes := map[string]string{
		"a@b.com": "2026-09-17 08:00:00 (UTC+08:00)",
		"b@b.com": "2026-09-16 20:00:00 (UTC-04:00)",
		"c@b.com": "2026-09-17T00:00:00Z",
	}
	handler.smtpSend = func(config map[string]any, subject, body string, recipients []string) (bool, string) {
		if len(recipients) != 1 || wantTimes[recipients[0]] == "" {
			t.Fatalf("unexpected recipients: %v", recipients)
		}
		if config["smtpServer"] != "smtp.x.com" {
			t.Fatalf("unexpected config: %v", config)
		}
		if subject != "生产 HighDiskUsage：磁盘空间不足" ||
			body != "Firing @ host-1 (production)\n可用空间低于 10%\n"+wantTimes[recipients[0]]+" → — (—)\nhttps://prometheus.example.com/graph?expr=disk" {
			t.Fatalf("unexpected rendered alert mail: %q / %q", subject, body)
		}
		delete(wantTimes, recipients[0])
		return true, ""
	}
	mock.ExpectQuery("FROM monitor_alert_notification_event e JOIN monitor_alert_history").WillReturnRows(eventRow())
	mock.ExpectQuery("FROM monitor_notification_policy").WillReturnRows(policyTreeRows())
	mock.ExpectQuery("SELECT id, name, media_type, config FROM monitor_alert_media").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "media_type", "config"}).
			AddRow(int64(2), "公司邮箱", "email", []byte(`{"smtpServer":"smtp.x.com","message_templates":{"firing":{"subject":"生产 {{alert_name}}：{{summary}}","body":"{{state}} @ {{instance}} ({{labels.environment}})\n{{description}}\n{{started_at}} → {{resolved_at}} ({{duration}})\n{{generator_url}}"},"resolved":{"subject":"恢复"}}}`)))
	mock.ExpectExec("SET status='sending', attempt_count=attempt_count\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN sys_user u").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "recipients", "timezone"}).
			AddRow(int64(5), "zhang", []byte(`["a@b.com", "a@b.com", " "]`), "Asia/Kuala_Lumpur").
			AddRow(int64(6), "lee", []byte(`["b@b.com"]`), "America/New_York").
			AddRow(int64(7), "chen", []byte(`["c@b.com"]`), "Local"))
	for _, delivery := range []struct {
		id, userID int64
		address    string
	}{{11, 5, "a@b.com"}, {12, 6, "b@b.com"}, {13, 7, "c@b.com"}} {
		expectAlertNotificationDeliveryCreate(mock, delivery.id, delivery.userID, delivery.address)
		mock.ExpectQuery("SELECT status FROM monitor_alert_notification_delivery").
			WithArgs(delivery.id).
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))
		mock.ExpectExec("SET status='sending', attempt_count=attempt_count\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("SET status='success', sent_at").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	// 片段不写占位符：两侧是 `?` / `$n`。
	mock.ExpectExec("SET status='success', sent_at").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	retryable, err := handler.sendAlertNotificationEvent(1)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if retryable {
		t.Fatal("success must not be retryable")
	}
	if len(wantTimes) != 0 {
		t.Fatalf("missing deliveries: %v", wantTimes)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSendAlertNotificationEventDeliveryFailureIsRetryable(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	handler.smtpSend = func(config map[string]any, subject, body string, recipients []string) (bool, string) {
		return false, "邮件发送失败: connection refused"
	}
	mock.ExpectQuery("FROM monitor_alert_notification_event e JOIN monitor_alert_history").WillReturnRows(eventRow())
	expectPolicyMediaHit(mock, "公司邮箱", "email")
	mock.ExpectExec("SET status='sending', attempt_count=attempt_count\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN sys_user u").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "recipients", "timezone"}).AddRow(int64(5), "zhang", []byte(`["a@b.com"]`), "UTC"))
	expectAlertNotificationDeliveryCreate(mock, 11, 5, "a@b.com")
	mock.ExpectQuery("SELECT status FROM monitor_alert_notification_delivery").
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))
	mock.ExpectExec("SET status='sending', attempt_count=attempt_count\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SET status='failed', error_message").WillReturnResult(sqlmock.NewResult(0, 1))
	// attempt_count 尚未用尽：事件回 pending 等待退避重试。
	mock.ExpectExec("SET status='pending'").WillReturnResult(sqlmock.NewResult(0, 1))

	retryable, err := handler.sendAlertNotificationEvent(1)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !retryable {
		t.Fatal("delivery failure must be retryable")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
