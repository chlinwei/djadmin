package assets

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	db "autoadmin/internal/platform/database/generated"
)

const (
	webSSHAuditMaxContentBytes = 1024 * 1024
	webSSHAuditFlushInterval   = 5 * time.Second
	webSSHAuditWriteTimeout    = 5 * time.Second
)

// Only the terminal bridge loop mutates a recorder. Both directions share one
// budget; capped output must not keep allocating or writing unchanged snapshots.
type webSSHAudit struct {
	queries *db.Queries
	started time.Time
	row     db.UpdateWebSSHSessionParams
	input   []byte
	output  []byte
	lastCR  bool
	dirty   bool
}

func newWebSSHAudit(ctx context.Context, queries *db.Queries, params db.CreateWebSSHSessionParams) (*webSSHAudit, error) {
	result, err := queries.CreateWebSSHSession(ctx, params)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	if id < 1 {
		return nil, fmt.Errorf("WebSSH 会话审计记录 ID 无效")
	}
	return &webSSHAudit{
		queries: queries, started: params.StartTime,
		row: db.UpdateWebSSHSessionParams{ID: id, Status: "connected", SwitchUserStatus: "pending"},
	}, nil
}

func (audit *webSSHAudit) recordInput(data string) {
	audit.row.InputBytes = webSSHCounter(int64(audit.row.InputBytes) + int64(len(data)))
	// This counts submitted input lines, not proven shell command execution.
	// CRLF may be split across WebSocket frames and must count only once.
	for _, char := range []byte(data) {
		if char == '\r' || (char == '\n' && !audit.lastCR) {
			audit.row.CommandCount = webSSHCounter(int64(audit.row.CommandCount) + 1)
		}
		audit.lastCR = char == '\r'
	}
	audit.record(&audit.input, []byte(data))
	audit.dirty = true
}

func (audit *webSSHAudit) recordOutput(data []byte) {
	audit.record(&audit.output, data)
}

func (audit *webSSHAudit) record(destination *[]byte, data []byte) {
	remaining := webSSHAuditMaxContentBytes - len(audit.input) - len(audit.output)
	n := min(remaining, len(data))
	if n > 0 {
		*destination = append(*destination, data[:n]...)
		audit.dirty = true
	}
	if n < len(data) && !audit.row.IsContentTruncated {
		audit.row.IsContentTruncated = true
		audit.dirty = true
	}
}

func (audit *webSSHAudit) connected(effectiveUser string) {
	audit.row.EffectiveUsername = webSSHText(effectiveUser, 100)
	audit.row.SwitchUserStatus = "success"
	audit.dirty = true
}

func (audit *webSSHAudit) fail(message string) {
	audit.row.Status = "failed"
	audit.row.ErrorMessage = webSSHText(message, 4096)
	if audit.row.SwitchUserStatus == "pending" {
		audit.row.SwitchUserStatus = "failed"
		audit.row.SwitchUserError = audit.row.ErrorMessage
	}
	audit.row.CloseCode = sql.NullInt32{Int32: 1011, Valid: true}
	audit.dirty = true
}

func (audit *webSSHAudit) finish() {
	now := time.Now().UTC()
	if audit.row.Status != "failed" {
		audit.row.Status = "closed"
	}
	if !audit.row.CloseCode.Valid {
		audit.row.CloseCode = sql.NullInt32{Int32: 1000, Valid: true}
	}
	if audit.row.SwitchUserStatus == "pending" {
		audit.row.SwitchUserStatus = "cancelled"
	}
	audit.row.EndTime = sql.NullTime{Time: now, Valid: true}
	audit.row.DurationSeconds = sql.NullInt32{Int32: webSSHCounter(max(0, int64(now.Sub(audit.started)/time.Second))), Valid: true}
	audit.dirty = true
}

func (audit *webSSHAudit) flush(ctx context.Context) error {
	if !audit.dirty {
		return nil
	}
	// Keep raw bytes until snapshotting so UTF-8 characters split across Agent
	// frames reassemble. Normalize invalid bytes/NUL for both SQL dialects, then
	// enforce the budget on the stored UTF-8 text as well.
	input := webSSHText(string(audit.input), webSSHAuditMaxContentBytes)
	output := webSSHText(string(audit.output), webSSHAuditMaxContentBytes-len(input))
	normalizedSize := len(webSSHNormalize(string(audit.input))) + len(webSSHNormalize(string(audit.output)))
	audit.row.InputContent, audit.row.OutputContent = input, output
	audit.row.RecordedContentBytes = int32(len(input) + len(output))
	audit.row.IsContentTruncated = audit.row.IsContentTruncated || normalizedSize > webSSHAuditMaxContentBytes
	affected, err := audit.queries.UpdateWebSSHSession(ctx, audit.row)
	if err != nil {
		return err
	}
	// MySQL can report zero for unchanged values; distinguish that from a
	// deleted audit row rather than silently continuing an unrecorded terminal.
	if affected == 0 {
		if _, err := audit.queries.GetWebSSHAuditID(ctx, audit.row.ID); err != nil {
			return fmt.Errorf("核对 WebSSH 会话审计记录失败: %w", err)
		}
	}
	audit.dirty = false
	return nil
}

func webSSHCounter(value int64) int32 {
	return int32(min(value, math.MaxInt32))
}

func webSSHNormalize(value string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(value, "\uFFFD"), "\x00", "\uFFFD")
}

func webSSHText(value string, limit int) string {
	value = webSSHNormalize(value)
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
