package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type alertMailTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

const defaultAlertMailBody = `Alert: {{alert_name}}
Severity: {{severity}}
State: {{state}}
Instance: {{instance}}
Environment: {{labels.environment}}

Summary: {{summary}}
Description: {{description}}

Started at: {{started_at}}
`

const defaultAlertMailRecovery = `Resolved at: {{resolved_at}}
Duration: {{duration}}
`

const defaultAlertMailFooter = `
Source URL: {{generator_url}}
Labels: {{labels}}
`

var alertMailToken = regexp.MustCompile(`\{\{([^{}]*)\}\}|\{\{|\}\}`)

func sampleAlertMailTarget(event string) (alertNotificationTarget, error) {
	if event != "firing" && event != "resolved" {
		return alertNotificationTarget{}, fmt.Errorf("请选择触发或恢复事件")
	}
	target := alertNotificationTarget{
		state: event, alertname: "HighLoad5m", severity: "warning", instance: "vm-example:9100",
		labels: map[string]any{"job": "node", "environment": "test"},
		annotations: map[string]any{
			"summary":     "主机负载过高",
			"description": "过去 5 分钟主机负载持续高于阈值。",
		},
		startedAt:    time.Date(2026, 1, 15, 2, 0, 0, 0, time.UTC),
		generatorURL: "https://prometheus.example.com/graph",
	}
	if event == "resolved" {
		resolvedAt := target.startedAt.Add(15 * time.Minute)
		target.resolvedAt = &resolvedAt
	}
	return target, nil
}

func alertMailUserLocation(userID int32, name string) *time.Location {
	if name == "" || name == "UTC" {
		return time.UTC
	}
	location, err := time.LoadLocation(name)
	if err != nil || name == "Local" {
		// Invalid profile data must not prevent an alert from being sent.
		slog.Warn("invalid alert email user timezone; using UTC", "user_id", userID, "timezone", name)
		return time.UTC
	}
	return location
}

func (handler *Handler) currentAlertMailLocation(ctx *gin.Context) (*time.Location, error) {
	claims, ok := identity.ClaimsFromContext(ctx)
	if !ok {
		return nil, apperror.ErrTokenInvalid
	}
	name, err := db.New(handler.db).GetUserTimezone(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	return alertMailUserLocation(claims.UserID, name), nil
}

func formatAlertMailTime(value time.Time, location *time.Location) string {
	if value.IsZero() {
		return "—"
	}
	if location == time.UTC {
		return value.UTC().Format(time.RFC3339)
	}
	return value.In(location).Format("2006-01-02 15:04:05 (UTC-07:00)")
}

func renderAlertMail(target alertNotificationTarget, template alertMailTemplate, location *time.Location) (alertMailTemplate, error) {
	labels, err := json.Marshal(nonNilMap(target.labels))
	if err != nil {
		return alertMailTemplate{}, err
	}
	state := "Firing"
	resolvedAt := "—"
	duration := "—"
	if target.state == "resolved" {
		state = "Resolved"
		// A delayed firing notification must not expose recovery data.
		if target.resolvedAt != nil {
			resolvedAt = formatAlertMailTime(*target.resolvedAt, location)
			if !target.startedAt.IsZero() && !target.resolvedAt.IsZero() && !target.resolvedAt.Before(target.startedAt) {
				duration = target.resolvedAt.Sub(target.startedAt).String()
			}
		}
	}
	variables := map[string]string{
		"alert_name":         defaultString(target.alertname, "Unknown Alert"),
		"severity":           defaultString(target.severity, "unknown"),
		"state":              state,
		"instance":           defaultString(target.instance, "N/A"),
		"labels":             string(labels),
		"labels.environment": defaultString(strings.TrimSpace(stringValue(target.labels["environment"])), "—"),
		"summary":            defaultString(strings.TrimSpace(stringValue(target.annotations["summary"])), "—"),
		"description":        defaultString(strings.TrimSpace(stringValue(target.annotations["description"])), "—"),
		"started_at":         formatAlertMailTime(target.startedAt, location),
		"resolved_at":        resolvedAt,
		"duration":           duration,
		"generator_url":      defaultString(strings.TrimSpace(target.generatorURL), "—"),
	}
	render := func(source string) (string, error) {
		var renderErr error
		result := alertMailToken.ReplaceAllStringFunc(source, func(token string) string {
			parts := alertMailToken.FindStringSubmatch(token)
			if value, ok := variables[strings.TrimSpace(parts[1])]; ok {
				return value
			}
			if renderErr == nil {
				renderErr = fmt.Errorf("模板变量无效：%s", token)
			}
			return ""
		})
		return result, renderErr
	}
	if len(template.Subject) > 1024 || len(template.Body) > 16*1024 || strings.ContainsAny(template.Subject, "\r\n") {
		return alertMailTemplate{}, fmt.Errorf("主题必须为单行且不超过 1024 字节，正文不超过 16 KiB")
	}
	// Empty fields use the built-in content independently.
	result := alertMailTemplate{Subject: buildAlertEmailSubject(target)}
	if strings.TrimSpace(template.Subject) != "" {
		result.Subject, err = render(template.Subject)
		if err != nil {
			return alertMailTemplate{}, err
		}
	}
	body := template.Body
	if strings.TrimSpace(body) == "" {
		body = defaultAlertMailBody
		if target.state == "resolved" {
			body += defaultAlertMailRecovery
		}
		body += defaultAlertMailFooter
	}
	result.Body, err = render(body)
	if err != nil {
		return alertMailTemplate{}, err
	}
	// Alert values can contain newlines even when the subject template does not.
	result.Subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(result.Subject)
	return result, nil
}

func alertMailTemplates(config map[string]any) (map[string]alertMailTemplate, error) {
	value, ok := config["message_templates"]
	if !ok {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("消息模板必须是对象")
	}
	templates := make(map[string]alertMailTemplate, len(object))
	for event, value := range object {
		sample, err := sampleAlertMailTarget(event)
		if err != nil {
			return nil, err
		}
		if _, ok := value.(map[string]any); !ok {
			return nil, fmt.Errorf("%s 模板必须是对象", event)
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var template alertMailTemplate
		if err := decoder.Decode(&template); err != nil {
			return nil, fmt.Errorf("%s 模板格式无效：%w", event, err)
		}
		if _, err := renderAlertMail(sample, template, time.UTC); err != nil {
			return nil, fmt.Errorf("%s 模板：%w", event, err)
		}
		templates[event] = template
	}
	return templates, nil
}

func renderConfiguredAlertMail(config map[string]any, target alertNotificationTarget, location *time.Location) (alertMailTemplate, error) {
	templates, err := alertMailTemplates(config)
	if err != nil {
		return alertMailTemplate{}, err
	}
	return renderAlertMail(target, templates[target.state], location)
}

func (handler *Handler) PreviewAlertMediaTemplate(ctx *gin.Context) {
	// Account for JSON escaping in addition to the decoded template limits.
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 128*1024)
	var input struct {
		Event    string            `json:"event"`
		Template alertMailTemplate `json:"template"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		response.BusinessError(ctx, 400, "消息模板格式无效", nil)
		return
	}
	target, err := sampleAlertMailTarget(input.Event)
	if err != nil {
		response.BusinessError(ctx, 400, err.Error(), nil)
		return
	}
	location, err := handler.currentAlertMailLocation(ctx)
	if err != nil {
		response.Error(ctx, err)
		return
	}
	mail, err := renderAlertMail(target, input.Template, location)
	if err != nil {
		response.BusinessError(ctx, 400, err.Error(), nil)
		return
	}
	response.Success(ctx, mail)
}
