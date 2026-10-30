package assets

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"autoadmin/internal/agent/pb"
	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func testWebSSHAudit(t *testing.T, mock sqlmock.Sqlmock, pool *sql.DB) *webSSHAudit {
	t.Helper()
	now := time.Now().UTC()
	mock.ExpectExec("INSERT INTO assets_webssh_session_log").
		WithArgs(int64(42), int64(7), "operator", "10.0.0.9", "test-agent", now, "root").
		WillReturnResult(sqlmock.NewResult(123, 1))
	audit, err := newWebSSHAudit(context.Background(), db.New(pool), db.CreateWebSSHSessionParams{
		HostID: 42, UserID: sql.NullInt32{Int32: 7, Valid: true}, Username: "operator",
		ClientIp: "10.0.0.9", UserAgent: "test-agent", StartTime: now, RequestedUsername: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	return audit
}

func TestWebSSHAuditSharedBoundAndInputCounters(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	audit := testWebSSHAudit(t, mock, pool)
	audit.recordInput("echo 中文\r")
	audit.recordInput("\nwhoami\n")
	audit.recordOutput([]byte(strings.Repeat("x", webSSHAuditMaxContentBytes)))
	audit.connected("root")
	mock.ExpectExec("UPDATE assets_webssh_session_log").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := audit.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(audit.input)+len(audit.output) != webSSHAuditMaxContentBytes || !audit.row.IsContentTruncated ||
		audit.row.RecordedContentBytes != webSSHAuditMaxContentBytes || audit.row.CommandCount != 2 ||
		audit.row.InputBytes != int32(len("echo 中文\r\nwhoami\n")) {
		t.Fatalf("incorrect bounded recording: %+v", audit.row)
	}
	audit.recordOutput([]byte("discarded"))
	if audit.dirty {
		t.Fatal("capped output caused redundant database writes")
	}
	if err := audit.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	audit.recordInput("id\r\n")
	if audit.row.CommandCount != 3 || !audit.dirty {
		t.Fatal("capping content stopped input counters")
	}
	audit.finish()
	mock.ExpectExec("UPDATE assets_webssh_session_log").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := audit.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if audit.row.Status != "closed" || !audit.row.EndTime.Valid || !audit.row.DurationSeconds.Valid ||
		audit.row.CloseCode.Int32 != 1000 || audit.row.EffectiveUsername != "root" {
		t.Fatalf("missing terminal lifecycle metadata: %+v", audit.row)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWebSSHAuditUTF8AndWriteFailure(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	audit := testWebSSHAudit(t, mock, pool)
	bytes := []byte("中文")
	audit.recordOutput(bytes[:2])
	audit.recordOutput(bytes[2:])
	audit.recordOutput([]byte{0, 0xff})
	mock.ExpectExec("UPDATE assets_webssh_session_log").WillReturnError(errors.New("database unavailable"))
	if err := audit.flush(context.Background()); err == nil || !audit.dirty {
		t.Fatal("failed audit save was treated as successful")
	}
	mock.ExpectExec("UPDATE assets_webssh_session_log").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := audit.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if audit.row.OutputContent != "中文\uFFFD\uFFFD" || !utf8.ValidString(audit.row.OutputContent) {
		t.Fatalf("invalid text snapshot: %q", audit.row.OutputContent)
	}
	for _, limit := range []int{0, 1, 2, 3, 4, 5, 6} {
		text := webSSHText("中文", limit)
		if !utf8.ValidString(text) || len(text) > limit {
			t.Fatalf("invalid UTF-8 byte limit %d: %q", limit, text)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWebSSHAuditCreateFailure(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectExec("INSERT INTO assets_webssh_session_log").WillReturnError(errors.New("write denied"))
	if _, err := newWebSSHAudit(context.Background(), db.New(pool), db.CreateWebSSHSessionParams{}); err == nil {
		t.Fatal("audit creation failure was ignored")
	}
}

func TestWebSSHAuditZeroChangedRows(t *testing.T) {
	for _, exists := range []bool{true, false} {
		t.Run(fmt.Sprint(exists), func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			audit := testWebSSHAudit(t, mock, pool)
			audit.connected("root")
			mock.ExpectExec("UPDATE assets_webssh_session_log").WillReturnResult(sqlmock.NewResult(0, 0))
			rows := sqlmock.NewRows([]string{"id"})
			if exists {
				rows.AddRow(123)
			}
			mock.ExpectQuery("SELECT id FROM assets_webssh_session_log").WithArgs(int64(123)).WillReturnRows(rows)
			err = audit.flush(context.Background())
			if (err == nil) != exists || audit.dirty == exists {
				t.Fatalf("unchanged/deleted audit rows were conflated: exists=%v err=%v dirty=%v", exists, err, audit.dirty)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type webSSHTestTerminal struct {
	events   chan *pb.AgentFrame
	input    chan string
	closed   chan struct{}
	openErr  error
	inputErr error
}

func (gateway *webSSHTestTerminal) OpenTerminal(context.Context, string, string, string, uint32, uint32) (<-chan *pb.AgentFrame, error) {
	return gateway.events, gateway.openErr
}
func (gateway *webSSHTestTerminal) SendTerminalData(_ context.Context, _, _ string, data []byte) error {
	gateway.input <- string(data)
	return gateway.inputErr
}
func (*webSSHTestTerminal) ResizeTerminal(context.Context, string, string, uint32, uint32) error {
	return nil
}
func (gateway *webSSHTestTerminal) CloseTerminal(context.Context, string, string) error {
	close(gateway.closed)
	return nil
}

type webSSHRecordedInput struct {
	text    string
	flushed chan struct{}
}

func (match webSSHRecordedInput) Match(value driver.Value) bool {
	if value != match.text {
		return false
	}
	select {
	case match.flushed <- struct{}{}:
	default:
	}
	return true
}

func TestWebSSHBridgeAuditsLifecycle(t *testing.T) {
	for _, scenario := range []string{"exit", "exit error", "client close", "normal disconnect", "disconnect", "agent error", "agent lost", "offline", "input error", "audit error", "live flush"} {
		t.Run(scenario, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			audit := testWebSSHAudit(t, mock, pool)
			gateway := &webSSHTestTerminal{
				events: make(chan *pb.AgentFrame, 8), input: make(chan string, 1), closed: make(chan struct{}),
			}
			flushed := make(chan struct{}, 1)
			switch scenario {
			case "offline":
				gateway.openErr = errors.New("agent offline")
			case "agent error":
				gateway.events <- &pb.AgentFrame{Payload: &pb.AgentFrame_TerminalOpenResponse{
					TerminalOpenResponse: &pb.TerminalOpenResponse{Error: "target user unavailable"},
				}}
			default:
				gateway.events <- &pb.AgentFrame{Payload: &pb.AgentFrame_TerminalOpenResponse{
					TerminalOpenResponse: &pb.TerminalOpenResponse{EffectiveUser: "root"},
				}}
				update := mock.ExpectExec("UPDATE assets_webssh_session_log")
				if scenario == "audit error" {
					update.WillReturnError(errors.New("audit storage unavailable"))
				} else {
					update.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if scenario == "input error" {
				gateway.inputErr = errors.New("agent send failed")
			}
			if scenario == "live flush" {
				mock.ExpectExec("UPDATE assets_webssh_session_log").WithArgs(
					"connected", nil, nil, nil, "", int64(6), int64(1),
					webSSHRecordedInput{text: "hello\r", flushed: flushed}, "world\n", int64(12), false, "root", "", "success", int64(123),
				).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec("UPDATE assets_webssh_session_log").WillReturnResult(sqlmock.NewResult(0, 1))
			done := make(chan error, 1)
			router := gin.New()
			router.GET("/terminal", func(c *gin.Context) {
				socket, err := webSSHUpgrader.Upgrade(c.Writer, c.Request, nil)
				if err != nil {
					done <- err
					return
				}
				defer socket.Close()
				name, ip := "test-host", "10.0.0.1"
				bridgeWebSSH(c.Request.Context(), socket, gateway, Host{InstanceName: &name, IP: &ip}, "root", audit)
				audit.finish()
				writeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				done <- audit.flush(writeCtx)
			})
			server := httptest.NewServer(router)
			defer server.Close()
			socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/terminal", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			_ = socket.SetReadDeadline(time.Now().Add(10 * time.Second))
			var message map[string]any
			if err := socket.ReadJSON(&message); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "offline", "agent error", "audit error":
				if message["type"] != "error" {
					t.Fatalf("failed session reported success: %v", message)
				}
			default:
				if message["type"] != "connected" {
					t.Fatalf("missing connected message: %v", message)
				}
				if scenario == "client close" {
					if err := socket.WriteJSON(webSSHMessage{Type: "close"}); err != nil {
						t.Fatal(err)
					}
				} else if scenario == "normal disconnect" {
					if err := socket.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1000, "")); err != nil {
						t.Fatal(err)
					}
				} else if scenario == "disconnect" {
					_ = socket.Close()
				} else if scenario == "agent lost" {
					close(gateway.events)
				} else {
					if err := socket.WriteJSON(webSSHMessage{Type: "input", Data: "hello\r"}); err != nil {
						t.Fatal(err)
					}
					select {
					case input := <-gateway.input:
						if input != "hello\r" {
							t.Fatalf("terminal input changed: %q", input)
						}
					case <-time.After(time.Second):
						t.Fatal("terminal did not receive input")
					}
					if scenario != "input error" {
						gateway.events <- &pb.AgentFrame{Payload: &pb.AgentFrame_TerminalDataResponse{
							TerminalDataResponse: &pb.TerminalDataResponse{Data: []byte("world\n")},
						}}
						if err := socket.ReadJSON(&message); err != nil || message["type"] != "output" || message["data"] != "world\n" {
							t.Fatalf("terminal output changed: %v, %v", message, err)
						}
						if scenario == "live flush" {
							select {
							case <-flushed:
							case <-time.After(8 * time.Second):
								t.Fatal("running session recording was not persisted")
							}
						}
						exit := &pb.TerminalExitResponse{}
						if scenario == "exit error" {
							exit.ExitCode, exit.Error = 2, "process failed"
						}
						gateway.events <- &pb.AgentFrame{Payload: &pb.AgentFrame_TerminalExitResponse{TerminalExitResponse: exit}}
					}
				}
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("terminal exit did not finish audit")
			}
			failed := scenario != "exit" && scenario != "client close" && scenario != "normal disconnect" && scenario != "live flush"
			if (audit.row.Status == "failed") != failed || !audit.row.EndTime.Valid || !audit.row.DurationSeconds.Valid {
				t.Fatalf("wrong session final state: %+v", audit.row)
			}
			if scenario == "exit" || scenario == "live flush" {
				if audit.row.InputContent != "hello\r" || audit.row.OutputContent != "world\n" || audit.row.RecordedContentBytes != 12 {
					t.Fatalf("missing terminal recording: %+v", audit.row)
				}
			}
			if scenario != "offline" {
				select {
				case <-gateway.closed:
				default:
					t.Fatal("remote terminal was not closed")
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
