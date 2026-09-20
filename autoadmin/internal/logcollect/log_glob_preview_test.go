package logcollect

import (
	"context"
	"database/sql"
	"net"
	"regexp"
	"testing"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
	"autoadmin/internal/assets"

	"github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// 日志路径通配的按需展开（界面「解析后」列）：后端用 ListFiles 逐层展开（不依赖 agent
// 版本），按承载实例分组。某台主机离线只影响它自己那一项，不能拖垮整条日志。
func TestPreviewLogGlobReturnsMatches(t *testing.T) {
	const token = "test-token"
	gateway := agent.NewGateway(func(instanceName, receivedToken string) bool {
		return instanceName == "host-01" && receivedToken == token
	}, nil)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateway.Register(server)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.Close()
	stream, err := pb.NewAgentChannelClient(connection).Session(ctx)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if err = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_Hello{Hello: &pb.Hello{InstanceName: "host-01", Token: token}}}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	if _, err = stream.Recv(); err != nil {
		t.Fatalf("recv hello ack: %v", err)
	}

	// 目录树：logs/{app1,app2}/log_error.log，另有一个非匹配文件。
	dirs := map[string][]*pb.FileEntry{
		"/home/esb/data/logs": {
			{Name: "app1", IsDir: true},
			{Name: "app2", IsDir: true},
			{Name: "other.log", IsDir: false, Size: 1},
		},
		"/home/esb/data/logs/app1": {
			{Name: "log_error.log", IsDir: false, Size: 11, Mtime: 2},
		},
		"/home/esb/data/logs/app2": {
			{Name: "log_error.log", IsDir: false, Size: 22, Mtime: 1},
		},
	}
	go func() {
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				return
			}
			if listRequest := frame.GetListRequest(); listRequest != nil {
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_ListResponse{ListResponse: &pb.ListResponse{
					RequestId:   listRequest.RequestId,
					CurrentPath: listRequest.Path,
					Entries:     dirs[listRequest.Path],
				}}})
			}
		}
	}()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).WillReturnRows(
		sqlmock.NewRows(logFormatDefinitionColumns()).AddRow(
			int64(24), "error.log", "/home/esb/data/logs/*/log_error.log", sql.NullInt64{Int64: 7, Valid: true},
			"nginx 规则", time.Date(2026, 9, 19, 1, 2, 3, 0, time.UTC), int64(3), sql.NullInt64{},
			nil, sql.NullInt64{},
			sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
			sql.NullTime{}, sql.NullString{}, sql.NullString{}, sql.NullString{},
			"nginx", "yilake", sql.NullString{String: "poc", Valid: true}, "tib", []byte("{}"),
			[]byte("[]"), "/home/esb/data", "std",
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service_deployment")).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(int64(101)))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_deployment d")).
		WithArgs(int64(101), int64(15), int64(24)).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "host_id", "host_instance_name", "deployment_instance_name",
			"runtime_variables", "app_home", "macro_definitions", "macro_values", "path_pattern",
		}).AddRow(int64(101), int64(5), "host-01", "inst-1", []byte("{}"), "", []byte("[]"), []byte("{}"),
			"/home/esb/data/logs/*/log_error.log"))

	handler := &Handler{db: database, gateway: gateway}
	preview, err := handler.PreviewLogGlob(context.Background(), assets.LogGlobPreviewRequest{
		ServiceID: 15, LogDefinitionID: 24,
	})
	if err != nil {
		t.Fatalf("preview log glob: %v", err)
	}
	if preview.PathPattern != "/home/esb/data/logs/*/log_error.log" {
		t.Fatalf("path_pattern = %q", preview.PathPattern)
	}
	if len(preview.Instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(preview.Instances))
	}
	item := preview.Instances[0]
	if item.Error != "" {
		t.Fatalf("instance error = %q, want 空", item.Error)
	}
	if item.HostInstanceName != "host-01" || item.DeploymentInstanceName != "inst-1" {
		t.Fatalf("instance = %#v", item)
	}
	want := []string{
		"/home/esb/data/logs/app1/log_error.log",
		"/home/esb/data/logs/app2/log_error.log",
	}
	if len(item.Matches) != 2 || item.Matches[0] != want[0] || item.Matches[1] != want[1] {
		t.Fatalf("matches = %v, want %v", item.Matches, want)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
