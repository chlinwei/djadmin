package logcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func liveUserRows(username string, status int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "username", "password", "avatar", "phonenumber", "login_date", "status", "create_time", "update_time", "remark", "timezone"}).
		AddRow(1, username, "unused", nil, nil, nil, status, nil, nil, nil, "UTC")
}

func expectLiveSource(mock sqlmock.Sqlmock, paths ...string) {
	path := "/logs/app.log"
	if len(paths) > 0 {
		path = paths[0]
	}
	mock.ExpectQuery("SELECT d.host_id, COALESCE\\(h.instance_name").
		WithArgs(int64(7), "app", "10.0.0.1").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "host_instance_name"}).AddRow(42, "host-agent"))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_host WHERE instance_name").WillReturnRows(
		sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("FROM assets_application_service_deployment").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "service_id", "service_code", "instance_name", "runtime_variables", "app_home", "host_ip"}).
			AddRow(42, 7, "svc", "app", []byte(`{}`), "", "10.0.0.1"))
	mock.ExpectQuery("FROM assets_application_service s").WillReturnRows(sqlmock.NewRows([]string{
		"host_id", "project_code", "environment_code", "business_system_code", "service_code", "service_id",
		"application_code", "tier_code", "pipeline_name", "log_name", "path_pattern", "macro_values", "macro_definitions",
		"multiline_enabled", "start_pattern", "flush_timeout", "filter_include_rule_id", "filter_exclude_rule_id",
		"collection_filter_rule_id", "collection_exclude_filter_rule_id", "extra_processors",
	}).AddRow(42, "proj", "prod", "biz", "svc", 7, "app", "std", "rule", "main", path,
		[]byte(`{}`), []byte(`[]`), false, "", 2000, nil, nil, nil, nil, []byte(`[]`)))
}

// Opt-in cross-module test. A dj_agent TestTailExternalPeer connects to the
// published loopback address, exercising the real native reader, both generated
// protocol copies, gateway, authorization and WebSocket without production data.
func TestLiveTailRealAgentEndToEnd(t *testing.T) {
	coordinate := os.Getenv("DJADMIN_LIVE_TAIL_E2E_COORD")
	if coordinate == "" {
		t.Skip("set DJADMIN_LIVE_TAIL_E2E_COORD and run the agent peer test concurrently")
	}
	t.Setenv("LOG_LIVE_TAIL_MODE", "single-api-experimental")
	name := ".live-tail-e2e-source.log"
	if err := os.WriteFile(name, []byte("native snapshot\n"), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(name)
	path, err := filepath.Abs(name)
	if err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("FROM sys_user WHERE id").WillReturnRows(liveUserRows("admin", 1))
	expectLiveSource(mock, path)
	gateway := agent.NewGateway(func(instance, token string) bool { return instance == "host-agent" && token == "e2e-agent-secret" }, nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	gateway.Register(server)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	if err = os.WriteFile(coordinate, []byte(listener.Addr().String()), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(coordinate)
	deadline := time.Now().Add(25 * time.Second)
	for !gateway.IsOnline("host-agent") {
		if time.Now().After(deadline) {
			t.Fatal("real agent peer did not connect")
		}
		time.Sleep(10 * time.Millisecond)
	}
	tokens := identity.NewTokenManager("test-secret", time.Hour)
	token, err := tokens.Issue(1, "admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(database, gateway, nil, nil, "")
	handler.ConfigureLiveTail(tokens, []string{"*"})
	router := gin.New()
	router.GET("/ws/monitor/log-tail/", handler.LiveTail)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	socket, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws/monitor/log-tail/", http.Header{"Origin": []string{httpServer.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = socket.WriteJSON(map[string]any{"token": token, "source": liveSource{
		ServiceID: "7", HostIP: "10.0.0.1", Instance: "app", LogName: "main", Path: path,
	}}); err != nil {
		t.Fatal(err)
	}
	await := func(expected string) {
		t.Helper()
		for {
			var event struct {
				Type    string `json:"type"`
				Data    []byte `json:"data"`
				Message string `json:"message"`
			}
			if err := socket.ReadJSON(&event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "error" {
				t.Fatal(event.Message)
			}
			if strings.Contains(string(event.Data), expected) {
				return
			}
		}
	}
	await("native snapshot")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("native incremental 日志\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	await("native incremental 日志")
	if err = socket.WriteJSON(map[string]string{"type": "stop"}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		handler.liveTail.mu.Lock()
		count := len(handler.liveTail.slots)
		handler.liveTail.mu.Unlock()
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real-agent stop leaked API quota")
		}
		time.Sleep(time.Millisecond)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveTailWebSocketGatewayRoundTripAndStop(t *testing.T) {
	t.Setenv("LOG_LIVE_TAIL_MODE", "single-api-experimental")
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("FROM sys_user WHERE id").WillReturnRows(liveUserRows("viewer", 1))
	mock.ExpectQuery("SELECT DISTINCT m.perms").WillReturnRows(
		sqlmock.NewRows([]string{"perms"}).AddRow("monitor:view"))
	expectLiveSource(mock)
	gateway := agent.NewGateway(func(instance, token string) bool { return instance == "host-agent" && token == "agent-secret" }, nil)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateway.Register(server)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	stream, err := pb.NewAgentChannelClient(client).Session(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_Hello{Hello: &pb.Hello{InstanceName: "host-agent", Token: "agent-secret"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Recv(); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan bool, 1)
	go func() {
		prepared, recvErr := stream.Recv()
		if recvErr != nil || prepared.GetTailRequest().Action != "prepare" || prepared.GetTailRequest().Path != "/logs/app.log" {
			stopped <- false
			return
		}
		id := prepared.GetTailRequest().RequestId
		_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_TailEvent{TailEvent: &pb.TailEvent{
			RequestId: id, Type: "ready", Identity: "1:123",
		}}})
		start, recvErr := stream.Recv()
		if recvErr != nil || start.GetTailRequest().Action != "start" || start.GetTailRequest().Identity != "1:123" {
			stopped <- false
			return
		}
		_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_TailEvent{TailEvent: &pb.TailEvent{
			RequestId: id, Type: "data", Data: []byte("原始日志\n"),
		}}})
		stop, recvErr := stream.Recv()
		stopped <- recvErr == nil && stop.GetTailRequest().Action == "stop"
	}()
	tokens := identity.NewTokenManager("test-secret", time.Hour)
	token, err := tokens.Issue(1, "viewer", []string{"monitor:view"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(database, gateway, nil, nil, "")
	handler.ConfigureLiveTail(tokens, []string{"*"})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ws/monitor/log-tail/", handler.LiveTail)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	headers := http.Header{"Origin": []string{httpServer.URL}}
	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws/monitor/log-tail/", headers)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	_ = socket.SetReadDeadline(time.Now().Add(4 * time.Second))
	if err = socket.WriteJSON(map[string]any{"token": token, "source": liveSource{
		ServiceID: "7", HostIP: "10.0.0.1", Instance: "app", LogName: "main", Path: "/logs/app.log",
	}}); err != nil {
		t.Fatal(err)
	}
	var event struct {
		Type    string `json:"type"`
		Data    []byte `json:"data"`
		Message string `json:"message"`
	}
	if err = socket.ReadJSON(&event); err != nil || event.Type != "ready" {
		t.Fatalf("ready: %+v, %v", event, err)
	}
	if err = socket.ReadJSON(&event); err != nil || event.Type != "data" || string(event.Data) != "原始日志\n" {
		t.Fatalf("data: %+v, %v", event, err)
	}
	if err = socket.WriteJSON(map[string]string{"type": "stop"}); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-stopped:
		if !ok {
			t.Fatal("agent prepare/start/stop protocol failed")
		}
	case <-ctx.Done():
		t.Fatal("agent stop not received")
	}
	deadline := time.Now().Add(time.Second)
	for {
		handler.liveTail.mu.Lock()
		remaining := len(handler.liveTail.slots)
		handler.liveTail.mu.Unlock()
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("quota reservation leaked after stop")
		}
		time.Sleep(time.Millisecond)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveTailRevokedPermissionAndDisabledAccount(t *testing.T) {
	for _, status := range []int{0, 1} {
		database, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery("FROM sys_user WHERE id").WillReturnRows(liveUserRows("viewer", status))
		if status == 1 {
			mock.ExpectQuery("SELECT DISTINCT m.perms").WillReturnRows(
				sqlmock.NewRows([]string{"perms"}).AddRow("assets:hosts:view"))
		}
		tokens := identity.NewTokenManager("secret", time.Hour)
		token, err := tokens.Issue(1, "viewer", []string{"monitor:view"})
		if err != nil {
			t.Fatal(err)
		}
		claims, err := tokens.Parse(token)
		if err != nil {
			t.Fatal(err)
		}
		handler := &Handler{db: database}
		if err = handler.authorizeLiveTail(context.Background(), claims); err == nil {
			t.Fatal("stale JWT permissions bypassed current account/role state")
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}

func TestLiveTailRejectsDuplicateAgentRoute(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("SELECT d.host_id, COALESCE\\(h.instance_name").
		WithArgs(int64(7), "app", "10.0.0.1").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "host_instance_name"}).AddRow(42, "duplicate"))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_host WHERE instance_name").WillReturnRows(
		sqlmock.NewRows([]string{"count"}).AddRow(1))
	handler := &Handler{db: database}
	if _, err = handler.resolveLiveTail(context.Background(), liveSource{
		ServiceID: "7", HostIP: "10.0.0.1", Instance: "app", LogName: "main", Path: "/logs/app.log",
	}); err == nil {
		t.Fatal("ambiguous agent routing accepted")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveTailResolvesHostFromDeploymentWithoutDocumentHostID(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	expectLiveSource(mock)
	handler := &Handler{db: database}
	target, err := handler.resolveLiveTail(context.Background(), liveSource{
		ServiceID: "7", HostIP: "10.0.0.1", Instance: "app", LogName: "main", Path: "/logs/app.log",
	})
	if err != nil || target.HostID != 42 || target.HostInstanceName != "host-agent" {
		t.Fatalf("deployment association: target=%+v err=%v", target, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveTailRejectsMissingOrAmbiguousDeployment(t *testing.T) {
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			rows := sqlmock.NewRows([]string{"host_id", "host_instance_name"})
			for i := 0; i < count; i++ {
				rows.AddRow(42+i, fmt.Sprint("agent-", i))
			}
			mock.ExpectQuery("SELECT d.host_id, COALESCE\\(h.instance_name").
				WithArgs(int64(7), "app", "10.0.0.1").WillReturnRows(rows)
			handler := &Handler{db: database}
			if _, err = handler.resolveLiveTail(context.Background(), liveSource{
				ServiceID: "7", HostIP: "10.0.0.1", Instance: "app", LogName: "main", Path: "/logs/app.log",
			}); err == nil {
				t.Fatal("non-unique deployment accepted")
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLiveTailRejectsURLTokenAndBoundsUnauthenticatedConnections(t *testing.T) {
	t.Setenv("LOG_LIVE_TAIL_MODE", "single-api-experimental")
	tokens := identity.NewTokenManager("test-secret", time.Hour)
	token, err := tokens.Issue(1, "admin", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := &Handler{}
	handler.ConfigureLiveTail(tokens, []string{"*"})
	router := gin.New()
	router.GET("/ws/monitor/log-tail/", handler.LiveTail)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	socket, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws/monitor/log-tail/?token="+token, http.Header{"Origin": []string{httpServer.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	_ = socket.SetReadDeadline(time.Now().Add(time.Second))
	if err = socket.WriteJSON(map[string]string{"type": "start"}); err != nil {
		t.Fatal(err)
	}
	var event struct {
		Type string `json:"type"`
	}
	if err = socket.ReadJSON(&event); err != nil || event.Type != "error" {
		t.Fatalf("URL JWT authenticated a connection: %+v %v", event, err)
	}
	deadline := time.Now().Add(time.Second)
	for len(handler.liveTail.pending) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("authentication failure leaked pending slot")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < cap(handler.liveTail.pending); i++ {
		handler.liveTail.pending <- struct{}{}
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ws/monitor/log-tail/", nil))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("unbounded pending connections: status %d", recorder.Code)
	}
}

func TestLiveTailHandshakeUsesConfiguredCORS(t *testing.T) {
	t.Setenv("LOG_LIVE_TAIL_MODE", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://not-the-injected-origin.example")
	const origin = "http://10.25.66.150:5173"
	for _, tc := range []struct {
		name    string
		allowed []string
		accept  bool
	}{
		{name: "wildcard", allowed: []string{"*"}, accept: true},
		{name: "explicit list", allowed: []string{origin}, accept: true},
		{name: "unlisted", allowed: []string{"http://other.example"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := &Handler{}
			handler.ConfigureLiveTail(identity.NewTokenManager("test-secret", time.Hour), tc.allowed)
			router := gin.New()
			router.GET("/ws/monitor/log-tail/", handler.LiveTail)
			server := httptest.NewServer(router)
			defer server.Close()
			socket, response, err := websocket.DefaultDialer.Dial(
				"ws"+strings.TrimPrefix(server.URL, "http")+"/ws/monitor/log-tail/",
				http.Header{"Origin": []string{origin}})
			if !tc.accept {
				if socket != nil {
					socket.Close()
				}
				if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
					t.Fatalf("unlisted origin accepted: response=%v err=%v", response, err)
				}
				response.Body.Close()
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			_ = socket.SetReadDeadline(time.Now().Add(time.Second))
			if err := socket.WriteJSON(map[string]string{"token": "invalid"}); err != nil {
				t.Fatal(err)
			}
			var event struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}
			if err := socket.ReadJSON(&event); err != nil || event.Type != "error" ||
				event.Message != "登录已失效" {
				t.Fatalf("wildcard must not bypass authentication: %+v err=%v", event, err)
			}
		})
	}
}

func TestLiveTailModeDefaultsAndExplicitDisable(t *testing.T) {
	for _, tc := range []struct {
		name, mode, reason string
		unset, enabled     bool
	}{
		{name: "unset", unset: true, enabled: true},
		{name: "empty", enabled: true},
		{name: "whitespace", mode: "  ", enabled: true},
		{name: "explicit enabled", mode: "single-api-experimental", enabled: true},
		{name: "explicit disabled", mode: "disabled", reason: "已关闭"},
		{name: "invalid", mode: "typo", reason: "配置无效"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOG_LIVE_TAIL_MODE", tc.mode)
			if tc.unset {
				if err := os.Unsetenv("LOG_LIVE_TAIL_MODE"); err != nil {
					t.Fatal(err)
				}
			}
			handler := &Handler{}
			handler.ConfigureLiveTail(identity.NewTokenManager("test-secret", time.Hour), []string{"*"})
			router := gin.New()
			router.GET("/ws/monitor/log-tail/", handler.LiveTail)
			server := httptest.NewServer(router)
			defer server.Close()
			socket, response, err := websocket.DefaultDialer.Dial(
				"ws"+strings.TrimPrefix(server.URL, "http")+"/ws/monitor/log-tail/",
				http.Header{"Origin": []string{server.URL}})
			if !tc.enabled {
				if socket != nil {
					socket.Close()
				}
				if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
					t.Fatalf("disabled mode accepted handshake: response=%v err=%v", response, err)
				}
				defer response.Body.Close()
				var body struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				if err := json.NewDecoder(response.Body).Decode(&body); err != nil ||
					body.Code != 503 || !strings.Contains(body.Msg, tc.reason) {
					t.Fatalf("unavailable reason: %+v err=%v", body, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			_ = socket.SetReadDeadline(time.Now().Add(time.Second))
			if err := socket.WriteJSON(map[string]string{"token": "invalid"}); err != nil {
				t.Fatal(err)
			}
			var event struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}
			if err := socket.ReadJSON(&event); err != nil || event.Type != "error" ||
				!strings.Contains(event.Message, "登录已失效") {
				t.Fatalf("default enable must not bypass authentication: %+v err=%v", event, err)
			}
		})
	}
}
