package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestAlertMediaTemplatePreviewAndSend(t *testing.T) {
	handler, mock, _ := notificationTestHandler(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(ctx *gin.Context) {
		ctx.Set(identity.ClaimsContextKey, &identity.Claims{UserID: 17})
	})
	engine.POST("/preview", handler.PreviewAlertMediaTemplate)
	engine.POST("/media/:id/test", handler.TestAlertMedia)
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	draft := `{"subject":"恢复 {{ alert_name }}","body":"{{state}} @ {{instance}} ({{labels.environment}})\n{{summary}}：{{description}}\n{{started_at}} → {{resolved_at}} ({{duration}})\n{{generator_url}}"}`
	expectOperatorTimezone := func() {
		mock.ExpectQuery("SELECT timezone FROM sys_user WHERE id").
			WithArgs(int32(17)).
			WillReturnRows(sqlmock.NewRows([]string{"timezone"}).AddRow("Asia/Kuala_Lumpur"))
	}
	expectOperatorTimezone()
	preview := post("/preview", `{"event":"resolved","template":`+draft+`}`)
	var envelope struct {
		Code int               `json:"code"`
		Data alertMailTemplate `json:"data"`
	}
	want := alertMailTemplate{
		Subject: "恢复 HighLoad5m",
		Body:    "Resolved @ vm-example:9100 (test)\n主机负载过高：过去 5 分钟主机负载持续高于阈值。\n2026-01-15 10:00:00 (UTC+08:00) → 2026-01-15 10:15:00 (UTC+08:00) (15m0s)\nhttps://prometheus.example.com/graph",
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &envelope); err != nil || envelope.Code != 200 || envelope.Data != want {
		t.Fatalf("unexpected preview: %s (%v)", preview.Body, err)
	}
	now := time.Now().UTC()
	mock.ExpectQuery("FROM monitor_alert_media WHERE id").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "name", "media_type", "config", "enabled", "recipients"}).
			AddRow(int64(7), now, now, "", "mail", "email",
				[]byte(`{"message_templates":{"firing":{"subject":"触发"},"resolved":`+draft+`}}`), true, []byte(`[]`)))
	expectOperatorTimezone()
	called := false
	handler.smtpSend = func(_ map[string]any, subject, body string, recipients []string) (bool, string) {
		called = true
		if subject != want.Subject || body != want.Body || len(recipients) != 1 || recipients[0] != "to@example.com" {
			t.Fatalf("unexpected test mail: %q, %q, %v", subject, body, recipients)
		}
		return true, ""
	}
	sent := post("/media/7/test", `{"event":"resolved","recipients":["to@example.com"]}`)
	if !called || !strings.Contains(sent.Body.String(), `"sent":true`) {
		t.Fatalf("test mail was not sent: %s", sent.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertMailOptionalFieldsMissing(t *testing.T) {
	mail, err := renderAlertMail(alertNotificationTarget{state: "resolved"}, alertMailTemplate{
		Body: "{{summary}} / {{description}} / {{started_at}} / {{resolved_at}} / {{generator_url}} / {{duration}} / {{labels.environment}}",
	}, time.UTC)
	if err != nil || mail.Body != "— / — / — / — / — / — / —" {
		t.Fatalf("unexpected missing field rendering: %q (%v)", mail.Body, err)
	}
}

func TestAlertMailDurationRejectsReversedTimes(t *testing.T) {
	startedAt := time.Date(2026, 1, 15, 2, 0, 0, 0, time.UTC)
	resolvedAt := startedAt.Add(-time.Second)
	mail, err := renderAlertMail(alertNotificationTarget{state: "resolved", startedAt: startedAt, resolvedAt: &resolvedAt},
		alertMailTemplate{Body: "{{duration}}"}, time.UTC)
	if err != nil || mail.Body != "—" {
		t.Fatalf("unexpected duration with reversed times: %q (%v)", mail.Body, err)
	}
}

func TestAlertMailDefaultBodyByEvent(t *testing.T) {
	for _, tc := range []struct {
		event string
		state string
	}{
		{event: "firing", state: "Firing"},
		{event: "resolved", state: "Resolved"},
	} {
		t.Run(tc.event, func(t *testing.T) {
			target, err := sampleAlertMailTarget(tc.event)
			if err != nil {
				t.Fatal(err)
			}
			mail, err := renderConfiguredAlertMail(nil, target, time.UTC)
			if err != nil || mail.Subject != "["+tc.state+"] HighLoad5m - warning" {
				t.Fatalf("unexpected default content: %+v (%v)", mail, err)
			}
			lines := []string{
				"Alert: HighLoad5m", "Severity: warning", "State: " + tc.state, "Instance: vm-example:9100",
				"Environment: test", "Summary: 主机负载过高", "Description: 过去 5 分钟主机负载持续高于阈值。",
				"Started at: 2026-01-15T02:00:00Z", "Source URL: https://prometheus.example.com/graph",
				`Labels: {"environment":"test","job":"node"}`,
			}
			if tc.event == "resolved" {
				lines = append(lines, "Resolved at: 2026-01-15T02:15:00Z", "Duration: 15m0s")
			} else if strings.Contains(mail.Body, "Resolved at") || strings.Contains(mail.Body, "Duration:") {
				t.Fatalf("firing body exposes recovery fields: %q", mail.Body)
			}
			for _, line := range lines {
				if !strings.Contains(mail.Body, line+"\n") {
					t.Errorf("default body missing %q:\n%s", line, mail.Body)
				}
			}
		})
	}
}

func TestAlertMailTemplateValidation(t *testing.T) {
	target, err := sampleAlertMailTarget("firing")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []alertMailTemplate{
		{Body: "{{missing}}"},
		{Body: "{{alert_name"},
		{Subject: "subject\nsecond line"},
		{Subject: strings.Repeat("x", 1025)},
		{Body: strings.Repeat("x", 16*1024+1)},
	} {
		if _, err := renderAlertMail(target, invalid, time.UTC); err == nil {
			t.Fatalf("accepted invalid template (subject=%d bytes, body=%d bytes)", len(invalid.Subject), len(invalid.Body))
		}
	}
	target.instance = "<b>{{state}}</b>\r\nnext"
	mail, err := renderAlertMail(target, alertMailTemplate{Subject: "{{instance}}", Body: "{{instance}}"}, time.UTC)
	if err != nil || mail.Subject != "<b>{{state}}</b>  next" || mail.Body != target.instance {
		t.Fatalf("alert values must remain data: %+v (%v)", mail, err)
	}
	if got := templateHTMLBody(mail.Body); strings.Contains(got, "<b>") || !strings.Contains(got, "&lt;b&gt;{{state}}&lt;/b&gt;") {
		t.Fatalf("HTML body did not escape alert data: %s", got)
	}
}

// 收件人解析回归：与 Django test action 一致，接受数组或逗号/分号分隔字符串。
func TestParseAlertMediaTestRecipients(t *testing.T) {
	cases := []struct {
		raw  any
		want []string
	}{
		{"a@x.com, b@x.com;c@x.com", []string{"a@x.com", "b@x.com", "c@x.com"}},
		{[]any{"a@x.com", " b@x.com ", ""}, []string{"a@x.com", "b@x.com"}},
		{[]any{"a@x.com", "b@x.com;c@x.com"}, []string{"a@x.com", "b@x.com", "c@x.com"}},
		{nil, nil},
		{123, nil},
		{"   ", nil},
	}
	for index, item := range cases {
		got := parseAlertMediaTestRecipients(item.raw)
		if len(got) != len(item.want) {
			t.Fatalf("case %d: got %v, want %v", index, got, item.want)
		}
		for position := range got {
			if got[position] != item.want[position] {
				t.Fatalf("case %d: got %v, want %v", index, got, item.want)
			}
		}
	}
}

func TestBuildSMTPMessageText(t *testing.T) {
	message := string(buildSMTPMessage("alert@djadmin.local", []string{"ops@x.com"}, "test alert", "内容正文", "", "text"))
	for _, want := range []string{
		"From: alert@djadmin.local\r\n",
		"To: ops@x.com\r\n",
		"Subject: test alert\r\n",
		"Content-Type: text/plain; charset=UTF-8",
		"内容正文",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "multipart/alternative") {
		t.Fatalf("text message must not contain html part:\n%s", message)
	}
}

func TestBuildSMTPMessageHTML(t *testing.T) {
	message := string(buildSMTPMessage("alert@djadmin.local", []string{"a@x.com", "b@x.com"}, "server down", "<b>down</b>", "<b>down</b>", "html"))
	if !strings.Contains(message, `Content-Type: multipart/alternative; boundary="`) {
		t.Fatalf("missing multipart header:\n%s", message)
	}
	if strings.Count(message, "Content-Type: text/plain; charset=UTF-8") != 1 ||
		strings.Count(message, "Content-Type: text/html; charset=UTF-8") != 1 {
		t.Fatalf("alternative parts missing:\n%s", message)
	}
	if !strings.Contains(message, "<b>down</b>") {
		t.Fatalf("html body missing:\n%s", message)
	}
	if !strings.Contains(message, "To: a@x.com, b@x.com\r\n") {
		t.Fatalf("recipients missing:\n%s", message)
	}
}

// 主题含非 ASCII 时必须做 RFC 2047 编码，否则部分 SMTP 服务端拒收或乱码。
func TestBuildSMTPMessageEncodedSubject(t *testing.T) {
	message := string(buildSMTPMessage("a@x.local", []string{"b@x.local"}, "告警通知", "正文", "", "text"))
	subjectLine := ""
	for _, line := range regexp.MustCompile("\r\n").Split(message, -1) {
		if strings.HasPrefix(line, "Subject: ") {
			subjectLine = line
		}
	}
	if !strings.HasPrefix(subjectLine, "Subject: =?UTF-8?q?") {
		t.Fatalf("subject not RFC 2047 encoded: %q", subjectLine)
	}
}
