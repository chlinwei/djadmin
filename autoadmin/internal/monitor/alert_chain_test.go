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

func TestMatchRouteMatchers(t *testing.T) {
	labels := map[string]any{"alertname": "HighDiskUsage", "severity": "warning", "env": "prod"}
	if matched, key := matchRouteMatchers(map[string]any{"severity": "warning"}, labels); !matched || key != "" {
		t.Fatalf("expected match, got matched=%v key=%q", matched, key)
	}
	if matched, key := matchRouteMatchers(map[string]any{"env": "staging"}, labels); matched || key != "env" {
		t.Fatalf("expected mismatch on env, got matched=%v key=%q", matched, key)
	}
	if matched, key := matchRouteMatchers(map[string]any{"missing": "x"}, labels); matched || key != "missing" {
		t.Fatalf("expected mismatch on missing key, got matched=%v key=%q", matched, key)
	}
	// 空 matchers 视为命中全部。
	if matched, _ := matchRouteMatchers(map[string]any{}, labels); !matched {
		t.Fatal("empty matchers should match")
	}
}

func TestRouteNotifyMiss(t *testing.T) {
	if miss := routeNotifyMiss("firing", false, true); miss == "" {
		t.Fatal("firing without notify_on_firing must miss")
	}
	if miss := routeNotifyMiss("firing", true, false); miss != "" {
		t.Fatalf("firing with notify_on_firing should pass, got %q", miss)
	}
	if miss := routeNotifyMiss("resolved", true, false); miss == "" {
		t.Fatal("resolved without notify_on_resolved must miss")
	}
	if miss := routeNotifyMiss("resolved", true, true); miss != "" {
		t.Fatalf("resolved with both flags should pass, got %q", miss)
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

func TestUserRouteIssues(t *testing.T) {
	if issues := userRouteIssues(true, false, true); len(issues) != 1 || issues[0] != "该路由仅通知 resolved，firing 通知未开启" {
		t.Fatalf("unexpected issues: %v", issues)
	}
	if issues := userRouteIssues(true, true, true); len(issues) != 0 {
		t.Fatalf("fully enabled route should have no issues, got %v", issues)
	}
	if issues := userRouteIssues(false, true, true); len(issues) != 1 || issues[0] != "路由已禁用" {
		t.Fatalf("unexpected issues: %v", issues)
	}
}

func TestUserCanReceive(t *testing.T) {
	ok := []alertChainBinding{{
		Enabled: true, Recipients: []string{"a@b.com"},
		Media: alertChainMediaBrief{Enabled: true, MediaType: "email"},
		Routes: []alertChainRouteBrief{{Enabled: true, NotifyOnFiring: true}},
	}}
	if !userCanReceive(ok) {
		t.Fatal("complete path should be receivable")
	}
	broken := []alertChainBinding{{
		Enabled: true, Recipients: []string{"a@b.com"},
		Media: alertChainMediaBrief{Enabled: true, MediaType: "email"},
		Routes: []alertChainRouteBrief{{Enabled: true, NotifyOnFiring: false}},
	}}
	if userCanReceive(broken) {
		t.Fatal("route without firing notify breaks the path")
	}
	disabledBinding := []alertChainBinding{{Enabled: false, Recipients: []string{"a@b.com"},
		Media:   alertChainMediaBrief{Enabled: true, MediaType: "email"},
		Routes:  []alertChainRouteBrief{{Enabled: true, NotifyOnFiring: true}}}}
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

func TestUserAlertChainViaEngine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	engine := gin.New()
	engine.GET("/monitor/alert-notification/user-chain/", func(context *gin.Context) {
		context.Set(identity.ClaimsContextKey, &identity.Claims{UserID: 5, Username: "zhang"})
		handler.UserAlertChain(context)
	})

	mock.ExpectQuery("SELECT username FROM sys_user").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("zhang"))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "enabled", "recipients", "media_id", "name", "media_type", "media_enabled", "scope"}).
			AddRow(1, true, `["a@b.com"]`, 2, "公司邮箱", "email", true, nil).
			AddRow(2, true, `[]`, 3, "钉钉", "webhook", false, nil))
	mock.ExpectQuery("FROM monitor_alert_route r JOIN monitor_alert_route_media rm").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled", "notify_on_firing", "notify_on_resolved", "matchers"}).
			AddRow(3, "disk", true, true, false, `{"severity":"warning"}`))
	mock.ExpectQuery("FROM monitor_alert_route r JOIN monitor_alert_route_media rm").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled", "notify_on_firing", "notify_on_resolved", "matchers"}))

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
		Bindings []alertChainBinding `json:"bindings"`
		CanReceive bool              `json:"can_receive"`
		SummaryIssues []string      `json:"summary_issues"`
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
	if healthy.BindingID != 1 || len(healthy.Issues) != 0 || len(healthy.Routes) != 1 {
		t.Fatalf("unexpected healthy binding: %+v", healthy)
	}
	if healthy.Routes[0].Matchers["severity"] != "warning" {
		t.Fatalf("matchers should be returned, got %v", healthy.Routes[0].Matchers)
	}
	broken := payload.Bindings[1]
	if len(broken.Issues) != 3 {
		t.Fatalf("expected 3 issues on webhook binding, got %v", broken.Issues)
	}
	if !payload.CanReceive {
		t.Fatal("healthy path via binding 1 should be receivable")
	}
}

func TestUserAlertChainAllBroken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	engine := gin.New()
	engine.GET("/monitor/alert-notification/user-chain/", func(context *gin.Context) {
		context.Set(identity.ClaimsContextKey, &identity.Claims{UserID: 5, Username: "zhang"})
		handler.UserAlertChain(context)
	})

	mock.ExpectQuery("SELECT username FROM sys_user").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("zhang"))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "enabled", "recipients", "media_id", "name", "media_type", "media_enabled", "scope"}).
			AddRow(7, false, `[]`, 8, "公司邮箱", "email", true, nil))
	mock.ExpectQuery("FROM monitor_alert_route r JOIN monitor_alert_route_media rm").
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled", "notify_on_firing", "notify_on_resolved", "matchers"}).
			AddRow(9, "disk", true, false, true, `{}`))

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
	for _, want := range []string{"绑定 7 已禁用", "绑定 7：绑定未配置收件地址", "路由 disk：该路由仅通知 resolved，firing 通知未开启"} {
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
	gin.SetMode(gin.TestMode)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	engine := gin.New()
	engine.GET("/monitor/alert-notification/chain/:historyId/", handler.AlertChainEvaluation)

	startedAt := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT alertname,severity,instance,labels,state,started_at FROM monitor_alert_history").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"alertname", "severity", "instance", "labels", "state", "started_at"}).
			AddRow("HighDiskUsage", "warning", "host-1", `{"env":"prod"}`, "firing", startedAt))

	// 路由 3：labels 命中且 firing 开启；路由 4：labels 不匹配；路由 5：disabled。
	mock.ExpectQuery("FROM monitor_alert_route").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled", "notify_on_firing", "notify_on_resolved", "matchers"}).
			AddRow(3, "disk", true, true, false, `{"severity":"warning","env":"prod"}`).
			AddRow(4, "cpu", true, true, false, `{"severity":"critical"}`).
			AddRow(5, "legacy", false, true, true, `{}`))

	// 路由 3 的媒介。
	mock.ExpectQuery("FROM monitor_alert_route_media rm JOIN monitor_alert_media m").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled"}).AddRow(2, "公司邮箱", true))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN sys_user u").
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "recipients", "enabled", "scope"}).
			AddRow(5, "zhang", `["a@b.com"]`, true, nil))
	mock.ExpectQuery("FROM monitor_alert_notification_event WHERE alert_id=.*AND event_type").
		WithArgs(int64(1), "firing").
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_type", "status", "attempt_count", "error_message"}).
			AddRow(9, "firing", "success", 1, ""))
	mock.ExpectQuery("FROM monitor_alert_notification_delivery d LEFT JOIN sys_user u").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "username", "address", "status", "error_message"}).
			AddRow(5, "zhang", "a@b.com", "failed", "smtp timeout"))

	// 路由 4、5 未命中，不应再查媒介——但 evaluateRouteMedia 在判定前执行；当前实现先取媒介再判定，
	// 因此每个路由都会查一次媒介。
	mock.ExpectQuery("FROM monitor_alert_route_media rm JOIN monitor_alert_media m").
		WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled"}))
	mock.ExpectQuery("FROM monitor_alert_route_media rm JOIN monitor_alert_media m").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "enabled"}))

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
			ID        int64           `json:"id"`
			Alertname string          `json:"alertname"`
			State     string          `json:"state"`
			Labels    map[string]any  `json:"labels"`
			StartedAt time.Time       `json:"started_at"`
		} `json:"alert"`
		Routes []alertChainRouteDetail `json:"routes"`
		SummaryIssues []string         `json:"summary_issues"`
	}
	if err = json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if payload.Alert.Alertname != "HighDiskUsage" || payload.Alert.State != "firing" {
		t.Fatalf("unexpected alert: %+v", payload.Alert)
	}
	// 便捷键合并。
	if payload.Alert.Labels["alertname"] != "HighDiskUsage" || payload.Alert.Labels["severity"] != "warning" || payload.Alert.Labels["instance"] != "host-1" {
		t.Fatalf("labels convenience keys missing: %v", payload.Alert.Labels)
	}
	if len(payload.Routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(payload.Routes))
	}
	if !payload.Routes[0].Matched || payload.Routes[0].MissReason != "" {
		t.Fatalf("route 3 should match, got %+v", payload.Routes[0])
	}
	if len(payload.Routes[0].Media) != 1 || payload.Routes[0].Media[0].Event == nil {
		t.Fatalf("route 3 media/event missing: %+v", payload.Routes[0].Media)
	}
	event := payload.Routes[0].Media[0].Event
	if event.ID != 9 || event.EventType != "firing" || event.Status != "success" || event.AttemptCount != 1 {
		t.Fatalf("unexpected event: %+v", event)
	}
	deliveries := payload.Routes[0].Media[0].Deliveries
	if len(deliveries) != 1 || deliveries[0].Status != "failed" || deliveries[0].Error != "smtp timeout" {
		t.Fatalf("unexpected deliveries: %+v", deliveries)
	}
	if payload.Routes[1].Matched || payload.Routes[1].MissReason == "" ||
		payload.Routes[1].MissReason != "labels 不匹配（matchers 需要 severity=critical）" {
		t.Fatalf("route 4 miss reason unexpected: %+v", payload.Routes[1])
	}
	if payload.Routes[2].Enabled || payload.Routes[2].MissReason != "路由已禁用" {
		t.Fatalf("route 5 should be disabled miss: %+v", payload.Routes[2])
	}
	if !containsString(payload.SummaryIssues, "用户 zhang 的投递失败：smtp timeout") {
		t.Fatalf("summary missing delivery failure, got %v", payload.SummaryIssues)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAlertChainEvaluationNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
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
