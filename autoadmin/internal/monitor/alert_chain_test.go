package monitor

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// ---- 纯判定逻辑 ----

func TestMergeAlertLabels(t *testing.T) {
	merged := chainMergeLabels(map[string]any{"alertname": "Explicit", "job": "node"}, "Fallback", "warning", "host-1")
	if merged["alertname"] != "Explicit" {
		t.Fatalf("explicit labels must win, got %v", merged["alertname"])
	}
	if merged["severity"] != "warning" || merged["instance"] != "host-1" || merged["job"] != "node" {
		t.Fatalf("unexpected merged labels: %v", merged)
	}
}

func TestUserBindingIssues(t *testing.T) {
	issues := userBindingIssues(false, false, "webhook", nil)
	want := []string{"该绑定已禁用", "媒介已停用", "非邮件媒介，暂不支持自动发送", "绑定未配置收件地址"}
	if len(issues) != len(want) {
		t.Fatalf("got %v, want %v", issues, want)
	}
	for index := range want {
		if issues[index] != want[index] {
			t.Fatalf("case %d: got %q want %q", index, issues[index], want[index])
		}
	}
	if issues := userBindingIssues(true, true, "email", []string{"a@b.com"}); len(issues) != 0 {
		t.Fatalf("healthy binding should have no issues, got %v", issues)
	}
}

func TestUserCanReceive(t *testing.T) {
	ok := []alertChainBinding{{
		Enabled: true, Recipients: []string{"a@b.com"},
		Media:    alertChainMediaBrief{Enabled: true, MediaType: "email"},
		Policies: []alertChainPolicyBrief{{NotifyOnFiring: true}},
	}}
	if !userCanReceive(ok) {
		t.Fatal("complete path should be receivable")
	}
	broken := []alertChainBinding{{
		Enabled: true, Recipients: []string{"a@b.com"},
		Media:    alertChainMediaBrief{Enabled: true, MediaType: "email"},
		Policies: []alertChainPolicyBrief{{NotifyOnFiring: false, NotifyOnResolved: true}},
	}}
	if userCanReceive(broken) {
		t.Fatal("policy without firing notify breaks the path")
	}
	disabledBinding := []alertChainBinding{{Enabled: false, Recipients: []string{"a@b.com"},
		Media:    alertChainMediaBrief{Enabled: true, MediaType: "email"},
		Policies: []alertChainPolicyBrief{{NotifyOnFiring: true}}}}
	if userCanReceive(disabledBinding) {
		t.Fatal("disabled binding breaks the path")
	}
}

func TestTruncateError(t *testing.T) {
	if got := truncateError("  smtp timeout \n"); got != "smtp timeout" {
		t.Fatalf("got %q", got)
	}
	long := ""
	for index := 0; index < 50; index++ {
		long += "error"
	}
	got := truncateError(long)
	if len(got) != 123 || got[120:] != "..." {
		t.Fatalf("truncate length=%d suffix=%q", len(got), got[120:])
	}
}

// ---- SQL handler：user-chain ----

func userChainEngine(handler *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/monitor/alert-notification/user-chain/", func(context *gin.Context) {
		context.Set(identity.ClaimsContextKey, &identity.Claims{UserID: 5, Username: "zhang"})
		handler.UserAlertChain(context)
	})
	return engine
}

func policyTreeRow(id, parentID int64, name string, mediaIDs any, matchers string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "parent_id", "name", "position", "remark", "matchers", "media_ids", "user_group_ids", "notify_on_firing", "notify_on_resolved"})
	rows.AddRow(id, parentID, name, 0, "", matchers, mediaIDs, nil, true, true)
	return rows
}

func TestUserAlertChainViaEngine(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	engine := userChainEngine(handler)

	mock.ExpectQuery("SELECT username FROM sys_user").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("zhang"))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "enabled", "recipients", "media_id", "name", "media_type", "media_enabled"}).
			AddRow(1, true, `["a@b.com"]`, 2, "公司邮箱", "email", true).
			AddRow(2, true, `[]`, 3, "钉钉", "webhook", false))
	// 策略树：根出口 [2]，critical 子策略继承。
	mock.ExpectQuery("FROM monitor_notification_policy").
		WillReturnRows(policyTreeRow(1, 0, "默认策略", `[2]`, `[]`).
			AddRow(2, 1, "critical", 0, "", `[{"type":"label","label":"severity","operator":"=","value":"critical"}]`, nil, nil, true, true))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/monitor/alert-notification/user-chain/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	var payload struct {
		User struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
		Bindings      []alertChainBinding `json:"bindings"`
		CanReceive    bool                `json:"can_receive"`
		SummaryIssues []string            `json:"summary_issues"`
	}
	if err = json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if payload.User.ID != 5 || payload.User.Username != "zhang" {
		t.Fatalf("unexpected user: %+v", payload.User)
	}
	if len(payload.Bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(payload.Bindings))
	}
	healthy := payload.Bindings[0]
	if healthy.BindingID != 1 || len(healthy.Issues) != 0 || len(healthy.Policies) != 2 {
		t.Fatalf("unexpected healthy binding: %+v", healthy)
	}
	if healthy.Policies[0].ID != 1 || healthy.Policies[0].Path != "默认策略" {
		t.Fatalf("policy brief unexpected: %+v", healthy.Policies[0])
	}
	// critical 子策略继承根出口 [2]，同样生效。
	if healthy.Policies[1].ID != 2 || healthy.Policies[1].Path != "默认策略 / critical" {
		t.Fatalf("inherited policy brief unexpected: %+v", healthy.Policies[1])
	}
	broken := payload.Bindings[1]
	if len(broken.Issues) != 3 {
		t.Fatalf("expected 3 issues on webhook binding, got %v", broken.Issues)
	}
	if len(broken.Policies) != 0 {
		t.Fatalf("webhook media (3) must not be in root outlet [2], got %+v", broken.Policies)
	}
	if !containsString(payload.SummaryIssues, "媒介 钉钉 未被任何通知策略出口命中") {
		t.Fatalf("summary missing outlet issue, got %v", payload.SummaryIssues)
	}
	if !payload.CanReceive {
		t.Fatal("healthy path via binding 1 should be receivable")
	}
}

func TestUserAlertChainAllBroken(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	engine := userChainEngine(handler)

	mock.ExpectQuery("SELECT username FROM sys_user").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("zhang"))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "enabled", "recipients", "media_id", "name", "media_type", "media_enabled"}).
			AddRow(7, false, `[]`, 8, "公司邮箱", "email", true))
	// 根出口为空：没有任何策略出口命中。
	mock.ExpectQuery("FROM monitor_notification_policy").
		WillReturnRows(policyTreeRow(1, 0, "默认策略", `[]`, `[]`))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/monitor/alert-notification/user-chain/", nil))

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	var payload struct {
		CanReceive    bool     `json:"can_receive"`
		SummaryIssues []string `json:"summary_issues"`
	}
	if err = json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if payload.CanReceive {
		t.Fatal("disabled binding with empty recipients must not be receivable")
	}
	joined := ""
	for _, issue := range payload.SummaryIssues {
		joined += issue + "\n"
	}
	for _, want := range []string{"绑定 7：该绑定已禁用", "绑定 7：绑定未配置收件地址", "媒介 公司邮箱 未被任何通知策略出口命中"} {
		if !containsString(payload.SummaryIssues, want) {
			t.Fatalf("summary missing %q, got %v", want, joined)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// ---- SQL handler：chain/:historyId/ ----

func TestAlertChainEvaluation(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/monitor/alert-notification/chain/:historyId/", handler.AlertChainEvaluation)

	startedAt := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT alertname,severity,instance,labels,state,started_at FROM monitor_alert_history").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"alertname", "severity", "instance", "labels", "state", "started_at"}).
			AddRow("HighDiskUsage", "warning", "host-1", `{"env":"prod"}`, "firing", startedAt))

	// 根出口 [2]；子策略 severity=warning 命中（selected），severity=critical 未命中。
	mock.ExpectQuery("FROM monitor_notification_policy").
		WillReturnRows(policyTreeRow(1, 0, "默认策略", `[2]`, `[]`).
			AddRow(2, 1, "disk", 0, "", `[{"type":"label","label":"severity","operator":"=","value":"warning"}]`, nil, nil, true, true).
			AddRow(3, 1, "cpu", 0, "", `[{"type":"label","label":"severity","operator":"=","value":"critical"}]`, nil, nil, true, true))

	// 出口媒介 [2] 的绑定与事件/投递记录。
	mock.ExpectQuery("FROM monitor_alert_media WHERE id IN").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled"}).AddRow(2, "公司邮箱", true))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN sys_user u").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "recipients", "enabled"}).
			AddRow(5, "zhang", `["a@b.com"]`, true))
	mock.ExpectQuery("FROM monitor_alert_notification_event WHERE alert_id=.*AND event_type").
		WithArgs(int64(1), "firing").
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_type", "status", "attempt_count", "error_message"}).
			AddRow(9, "firing", "success", 1, ""))
	mock.ExpectQuery("FROM monitor_alert_notification_delivery d LEFT JOIN sys_user u").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "address", "status", "error_message"}).
			AddRow(5, "zhang", "a@b.com", "failed", "smtp timeout"))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/monitor/alert-notification/chain/1/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	var payload struct {
		Alert struct {
			ID        int64          `json:"id"`
			Alertname string         `json:"alertname"`
			State     string         `json:"state"`
			Labels    map[string]any `json:"labels"`
			StartedAt time.Time      `json:"started_at"`
		} `json:"alert"`
		PolicyTree struct {
			MatchedPathIDs []int64                  `json:"matched_path_ids"`
			MatchedPath    string                   `json:"matched_path"`
			Levels         [][]alertChainPolicyEval `json:"levels"`
			FinalPolicy    alertChainPolicyDetail   `json:"final_policy"`
		} `json:"policy_tree"`
		Medias        []alertChainMediaDetail `json:"medias"`
		SummaryIssues []string                `json:"summary_issues"`
	}
	if err = json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if payload.Alert.Alertname != "HighDiskUsage" || payload.Alert.State != "firing" {
		t.Fatalf("unexpected alert: %+v", payload.Alert)
	}
	if payload.Alert.Labels["alertname"] != "HighDiskUsage" || payload.Alert.Labels["severity"] != "warning" || payload.Alert.Labels["instance"] != "host-1" {
		t.Fatalf("labels convenience keys missing: %v", payload.Alert.Labels)
	}
	if len(payload.PolicyTree.Levels) != 1 || len(payload.PolicyTree.Levels[0]) != 2 {
		t.Fatalf("expected 1 level with 2 siblings, got %+v", payload.PolicyTree.Levels)
	}
	first, second := payload.PolicyTree.Levels[0][0], payload.PolicyTree.Levels[0][1]
	if !first.Matched || !first.Selected || first.ID != 2 {
		t.Fatalf("disk policy should be selected, got %+v", first)
	}
	if second.Matched || second.Selected || second.MissReason == "" {
		t.Fatalf("critical policy should miss, got %+v", second)
	}
	if len(payload.PolicyTree.MatchedPathIDs) != 2 || payload.PolicyTree.MatchedPathIDs[1] != 2 {
		t.Fatalf("unexpected matched path: %+v", payload.PolicyTree.MatchedPathIDs)
	}
	if payload.PolicyTree.MatchedPath != "默认策略 / disk" {
		t.Fatalf("unexpected path label: %q", payload.PolicyTree.MatchedPath)
	}
	if !payload.PolicyTree.FinalPolicy.EventAllowed {
		t.Fatalf("firing should be allowed: %+v", payload.PolicyTree.FinalPolicy)
	}
	if len(payload.PolicyTree.FinalPolicy.EffectiveMedia) != 1 || payload.PolicyTree.FinalPolicy.EffectiveMedia[0] != 2 {
		t.Fatalf("effective media should inherit root [2], got %+v", payload.PolicyTree.FinalPolicy.EffectiveMedia)
	}
	if len(payload.Medias) != 1 || payload.Medias[0].ID != 2 || payload.Medias[0].Event == nil {
		t.Fatalf("media detail missing: %+v", payload.Medias)
	}
	if deliveries := payload.Medias[0].Deliveries; len(deliveries) != 1 || deliveries[0].Status != "failed" || deliveries[0].Error != "smtp timeout" {
		t.Fatalf("unexpected deliveries: %+v", deliveries)
	}
	if !containsString(payload.SummaryIssues, "用户 zhang 的投递失败：smtp timeout") {
		t.Fatalf("summary missing delivery failure, got %v", payload.SummaryIssues)
	}
	if !containsString(payload.SummaryIssues, `策略 cpu 未命中：labels.severity="warning" 不等于 "critical"`) {
		t.Fatalf("summary missing sibling miss, got %v", payload.SummaryIssues)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAlertChainEvaluationNotFound(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/monitor/alert-notification/chain/:historyId/", handler.AlertChainEvaluation)

	mock.ExpectQuery("SELECT alertname,severity,instance,labels,state,started_at FROM monitor_alert_history").
		WithArgs(int64(99)).
		WillReturnError(sql.ErrNoRows)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/monitor/alert-notification/chain/99/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope response.Envelope
	if err = json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.Code != 404 {
		t.Fatalf("expected business code 404, got %d body=%s", envelope.Code, recorder.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
