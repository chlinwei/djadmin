package audit

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"time"
	"unicode"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type LoginFilter struct {
	Keyword, Status string
	From, To        sql.NullTime
}
type WebSSHFilter struct {
	Status, Username, Keyword, OutputKeyword string
	From, To                                 sql.NullTime
}

type LoginLog struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	UserID    *int32 `json:"user_id"`
	Status    string `json:"status"`
	ClientIP  string `json:"client_ip"`
	UserAgent string `json:"user_agent"`
	Message   string `json:"message"`
	LoginTime string `json:"login_time"`
}
type WebSSHSession struct {
	ID                   int64   `json:"id"`
	Host                 int64   `json:"host"`
	HostName             *string `json:"host_name"`
	HostIP               *string `json:"host_ip"`
	UserID               *int32  `json:"user_id"`
	Username             string  `json:"username"`
	ClientIP             string  `json:"client_ip"`
	UserAgent            string  `json:"user_agent"`
	Status               string  `json:"status"`
	StartTime            string  `json:"start_time"`
	EndTime              *string `json:"end_time"`
	DurationSeconds      *int32  `json:"duration_seconds"`
	CloseCode            *int32  `json:"close_code"`
	ErrorMessage         string  `json:"error_message"`
	InputBytes           int32   `json:"input_bytes"`
	CommandCount         int32   `json:"command_count"`
	RecordedContentBytes int32   `json:"recorded_content_bytes"`
	IsContentTruncated   bool    `json:"is_content_truncated"`
}
type WebSSHContent struct {
	ID                    int64    `json:"id"`
	Status                string   `json:"status"`
	StartTime             string   `json:"start_time"`
	EndTime               *string  `json:"end_time"`
	DurationSeconds       *int32   `json:"duration_seconds"`
	RawInputContent       string   `json:"raw_input_content"`
	RawOutputContent      string   `json:"raw_output_content"`
	InputContent          string   `json:"input_content"`
	OutputContent         string   `json:"output_content"`
	ReadableInputContent  string   `json:"readable_input_content"`
	ReadableInputCommands []string `json:"readable_input_commands"`
	RecordedContentBytes  int32    `json:"recorded_content_bytes"`
	IsContentTruncated    bool     `json:"is_content_truncated"`
	HostID                int64    `json:"host_id"`
	HostName              *string  `json:"host_name"`
	HostIP                *string  `json:"host_ip"`
	Username              string   `json:"username"`
	EffectiveUsername     string   `json:"effective_username"`
	ClientIP              string   `json:"client_ip"`
	CloseCode             *int32   `json:"close_code"`
	ErrorMessage          string   `json:"error_message"`
}

func (r *Repository) ListLoginLogs(ctx context.Context, filter LoginFilter, page pagination.Page) ([]LoginLog, int64, error) {
	params := loginParams(filter)
	count, err := r.queries.CountLoginAudits(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListLoginAudits(ctx, db.ListLoginAuditsParams{KeywordPattern: params.KeywordPattern, ClientIpPattern: params.ClientIpPattern, MessagePattern: params.MessagePattern, ExactStatus: params.ExactStatus, TimeFrom: params.TimeFrom, TimeTo: params.TimeTo, Limit: page.Size, Offset: page.Offset})
	result := make([]LoginLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, LoginLog{ID: row.ID, Username: row.Username, UserID: int32Ptr(row.UserID), Status: row.Status, ClientIP: row.ClientIp, UserAgent: row.UserAgent, Message: row.Message, LoginTime: formatTime(row.LoginTime)})
	}
	return result, count, err
}
func loginParams(filter LoginFilter) db.CountLoginAuditsParams {
	pattern := nullPattern(filter.Keyword)
	status := sql.NullString{}
	if filter.Status == "success" || filter.Status == "failed" {
		status = sql.NullString{String: filter.Status, Valid: true}
	}
	return db.CountLoginAuditsParams{KeywordPattern: pattern, ClientIpPattern: pattern, MessagePattern: pattern, ExactStatus: status, TimeFrom: filter.From, TimeTo: filter.To}
}
func (r *Repository) ListWebSSHSessions(ctx context.Context, filter WebSSHFilter, page pagination.Page) ([]WebSSHSession, int64, error) {
	params := webSSHParams(filter)
	count, err := r.queries.CountWebSSHSessions(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListWebSSHSessions(ctx, db.ListWebSSHSessionsParams{ExactStatus: params.ExactStatus, AlternateStatus: params.AlternateStatus, ExactUsername: params.ExactUsername, KeywordUser: params.KeywordUser, KeywordHost: params.KeywordHost, KeywordIp: params.KeywordIp, OutputPattern: params.OutputPattern, TimeFrom: params.TimeFrom, TimeTo: params.TimeTo, Limit: page.Size, Offset: page.Offset})
	result := make([]WebSSHSession, 0, len(rows))
	for _, row := range rows {
		status := row.Status
		if status == "connected" {
			status = "closed"
		}
		result = append(result, WebSSHSession{ID: row.ID, Host: row.Host, HostName: stringPtr(row.HostName), HostIP: stringPtr(row.HostIp), UserID: int32Ptr(row.UserID), Username: row.Username, ClientIP: row.ClientIp, UserAgent: row.UserAgent, Status: status, StartTime: formatTime(row.StartTime), EndTime: nullTimePtr(row.EndTime), DurationSeconds: int32Ptr(row.DurationSeconds), CloseCode: int32Ptr(row.CloseCode), ErrorMessage: row.ErrorMessage, InputBytes: row.InputBytes, CommandCount: row.CommandCount, RecordedContentBytes: row.RecordedContentBytes, IsContentTruncated: row.IsContentTruncated})
	}
	return result, count, err
}
func webSSHParams(filter WebSSHFilter) db.CountWebSSHSessionsParams {
	status, alternate := sql.NullString{}, sql.NullString{}
	if filter.Status != "" {
		status = sql.NullString{String: filter.Status, Valid: true}
		alternate = status
		if filter.Status == "closed" {
			alternate.String = "connected"
		}
	}
	username := nullPattern(filter.Username)
	keyword := nullPattern(filter.Keyword)
	output := nullPattern(filter.OutputKeyword)
	return db.CountWebSSHSessionsParams{ExactStatus: status, AlternateStatus: alternate, ExactUsername: username, KeywordUser: keyword, KeywordHost: keyword, KeywordIp: keyword, OutputPattern: output, TimeFrom: filter.From, TimeTo: filter.To}
}
func (r *Repository) GetWebSSHContent(ctx context.Context, id int64) (WebSSHContent, error) {
	row, err := r.queries.GetWebSSHSessionContent(ctx, id)
	if err != nil {
		return WebSSHContent{}, err
	}
	input, output := cleanTerminal(row.InputContent), cleanTerminal(row.OutputContent)
	commands := readableCommands(input)
	return WebSSHContent{ID: row.ID, Status: row.Status, StartTime: formatTime(row.StartTime), EndTime: nullTimePtr(row.EndTime), DurationSeconds: int32Ptr(row.DurationSeconds), RawInputContent: row.InputContent, RawOutputContent: row.OutputContent, InputContent: input, OutputContent: output, ReadableInputContent: strings.Join(commands, "\n"), ReadableInputCommands: commands, RecordedContentBytes: row.RecordedContentBytes, IsContentTruncated: row.IsContentTruncated, HostID: row.HostID, HostName: stringPtr(row.HostName), HostIP: stringPtr(row.HostIp), Username: row.Username, EffectiveUsername: row.EffectiveUsername, ClientIP: row.ClientIp, CloseCode: int32Ptr(row.CloseCode), ErrorMessage: row.ErrorMessage}, nil
}
func nullPattern(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: "%" + value + "%", Valid: true}
}
func int32Ptr(value sql.NullInt32) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}
func stringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
func nullTimePtr(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	text := formatTime(value.Time)
	return &text
}
func formatTime(value time.Time) string { return value.UTC().Format("2006-01-02T15:04:05.999999Z") }

var ansiPattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var oscPattern = regexp.MustCompile(`\x1b\][^\x07]*(\x07|\x1b\\)`)

func cleanTerminal(value string) string {
	value = oscPattern.ReplaceAllString(value, "")
	value = ansiPattern.ReplaceAllString(value, "")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, value)
}
func readableCommands(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r", "\n"), "\n")
	result := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
