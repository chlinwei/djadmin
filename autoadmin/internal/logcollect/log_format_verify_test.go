package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
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

// 日志格式认证的回归用例（架构文档 §4.8）。
//
// 这里钉住的是"样例怎么变成 pipeline 输入"和"实例依据怎么取到真实日志"两段——
// 它们最容易在真实环境里悄悄失效：多行合并还原错了，认证结果就和真实采集不一致；
// 反向读取的窗口起点没丢残行，半行日志会被当成一条完整记录。

func TestLogSampleDocs(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		multiline    bool
		startPattern string
		want         []string
		wantError    string
	}{
		{
			name: "单行模式：逐行成记录并丢掉空行",
			text: "2026-09-19 10:00:00 INFO start\n\n2026-09-19 10:00:01 ERROR boom\n",
			want: []string{"2026-09-19 10:00:00 INFO start", "2026-09-19 10:00:01 ERROR boom"},
		},
		{
			name: "单行模式：CRLF 也要切干净",
			text: "line-a\r\nline-b\r\n",
			want: []string{"line-a", "line-b"},
		},
		{
			name: "多行模式：不以首行正则开头的行并入上一条（Filebeat negate+after）",
			text: strings.Join([]string{
				"2026-09-19 10:00:00 ERROR boom",
				"  at com.example.Foo.bar(Foo.java:42)",
				"  at com.example.Foo.baz(Foo.java:43)",
				"2026-09-19 10:00:01 INFO recovered",
			}, "\n"),
			multiline:    true,
			startPattern: `^\d{4}-\d{2}-\d{2}`,
			want: []string{
				"2026-09-19 10:00:00 ERROR boom\n  at com.example.Foo.bar(Foo.java:42)\n  at com.example.Foo.baz(Foo.java:43)",
				"2026-09-19 10:00:01 INFO recovered",
			},
		},
		{
			// 口径 2026-09-19 改：多行模式下首行之前的前导行**丢掉**。
			// 那些行是反向读取窗口切出来的残尾（上一条记录的堆栈续行），真实采集时会被
			// Filebeat 并进上一条记录；当成独立记录送进认证只会误报"缺少 log_level"
			//（现场：catalina.out 认证一直被它挡住，与规则本身无关）。
			name:      "多行模式：首行之前的前导行丢掉（窗口切出来的残尾，不是一条记录）",
			text:      "\tat com.example.Foo.bar(Foo.java:42)\nbanner line\n2026-09-19 10:00:00 ERROR boom",
			multiline: true, startPattern: `^\d{4}-\d{2}-\d{2}`,
			want: []string{"2026-09-19 10:00:00 ERROR boom"},
		},
		{
			name:      "多行模式缺首行正则：报错而不是猜",
			text:      "anything",
			multiline: true,
			wantError: "未配置首行正则",
		},
		{
			name:      "多行模式首行正则非法：报错",
			text:      "anything",
			multiline: true, startPattern: "([unclosed",
			wantError: "首行正则不合法",
		},
		{
			// 现场规则 `springboot-tomcat-exception` 的准确保真：catalina.out 里既有
			// "2026-09-19 10:00:00" 这种数字日期开头的行，也有 "19-Sep-2026"（英文月）/ 中文这种
			// `\d{2}-[字母或汉字]{3,4}-\d{4}` 开头，堆栈行必须并到上一行。
			name: "多行模式：现场 tomcat 规则（含中文范围）能还原堆栈",
			text: strings.Join([]string{
				"19-Sep-2026 10:00:00.123 SEVERE [main] org.apache.catalina.startup.Catalina.start 启动失败",
				"\tat java.base/java.lang.Thread.run(Thread.java:833)",
				"\tCaused by: java.lang.IllegalStateException: boom",
				"2026-09-19 10:00:01.456 INFO [main] 已恢复",
			}, "\n"),
			multiline:    true,
			startPattern: `^(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\x{4e00}-\x{9fa5}]{3,4}-\d{4})`,
			want: []string{
				"19-Sep-2026 10:00:00.123 SEVERE [main] org.apache.catalina.startup.Catalina.start 启动失败\n" +
					"\tat java.base/java.lang.Thread.run(Thread.java:833)\n" +
					"\tCaused by: java.lang.IllegalStateException: boom",
				"2026-09-19 10:00:01.456 INFO [main] 已恢复",
			},
		},
		{
			// 同一条规则改回 Java/JS 的 `\uXXXX` 就会走到这里：报错文案必须带上等价写法，
			// 否则用户只看到 "invalid escape sequence: `\u`"，不知道该怎么改。
			name:      "多行模式：Java/JS 的 \\uXXXX 写法被拒，并提示 \\x{XXXX}",
			text:      "anything",
			multiline: true, startPattern: `^(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\u4e00-\u9fa5]{3,4}-\d{4})`,
			wantError: `\u4e00 → \x{4e00}`,
		},
		{
			name:      "样例为空：报错",
			text:      "\n\n",
			wantError: "样例日志为空",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			docs, err := logSampleDocs(testCase.text, testCase.multiline, testCase.startPattern)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("error = %v, want 包含 %q", err, testCase.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("log sample docs: %v", err)
			}
			if len(docs) != len(testCase.want) {
				t.Fatalf("docs 数 = %d, want %d（%v）", len(docs), len(testCase.want), docs)
			}
			for index, want := range testCase.want {
				message, ok := docs[index].(map[string]any)["message"].(string)
				if !ok || message != want {
					t.Fatalf("docs[%d].message = %#v, want %q", index, docs[index], want)
				}
			}
		})
	}
}

// 样例行数上限：窗口 1MiB 可能包含上万行，只取尾部若干行送进 _simulate。
func TestLogSampleDocsKeepsOnlyTailLines(t *testing.T) {
	lines := make([]string, 0, logFormatSampleLines+10)
	for index := 0; index < logFormatSampleLines+10; index++ {
		lines = append(lines, "line-"+string(rune('a'+index%26)))
	}
	docs, err := logSampleDocs(strings.Join(lines, "\n"), false, "")
	if err != nil {
		t.Fatalf("log sample docs: %v", err)
	}
	if len(docs) != logFormatSampleLines {
		t.Fatalf("docs 数 = %d, want %d", len(docs), logFormatSampleLines)
	}
	// 留下的是尾部：最后一条必须是原文最后一行。
	last := docs[len(docs)-1].(map[string]any)["message"]
	if last != lines[len(lines)-1] {
		t.Fatalf("最后一条 = %v, want %q", last, lines[len(lines)-1])
	}
}

// 多行模式必须按**记录**截断，不能按原始行截断（2026-09-20 现场：Nacos/Tomcat 的
// log_error.log 尾部是一条超长堆栈，尾 50 行全是 `\tat …`，按行截断会误报
// "未命中首行正则，还原不出任何记录"）。
func TestLogSampleDocsKeepsTailRecordsNotTailLines(t *testing.T) {
	const start = "2026-09-18 09:40:27.953 "
	// 先来两条普通记录，最后一条是首行 + 80 行续行（共 81 行，远超 50 行上限）。
	text := strings.Join([]string{
		start + "INFO first",
		"\tat A", "\tat B",
		start + "ERROR second",
		start + "ERROR giant",
		strings.Repeat("\tat java.base/Foo.bar(Foo.java:1)\n", 80),
	}, "\n")
	docs, err := logSampleDocs(text, true, `^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}`)
	if err != nil {
		t.Fatalf("长堆栈应能还原出记录，得到错误：%v", err)
	}
	// 三条记录都在：尾部超长记录必须完整保留（含它的首行）。
	if len(docs) != 3 {
		t.Fatalf("docs 数 = %d, want 3", len(docs))
	}
	last := docs[len(docs)-1].(map[string]any)["message"].(string)
	if !strings.HasPrefix(last, start+"ERROR giant") {
		t.Fatalf("最后一条必须以首行开始，得到 %q", last[:min(60, len(last))])
	}
	if !strings.Contains(last, "\tat java.base/Foo.bar") {
		t.Fatalf("最后一条应包含续行堆栈")
	}
}

// 记录数上限：超过 logFormatSampleLines 条时只留尾部，且每条都从首行开始。
func TestLogSampleDocsCapsTailRecords(t *testing.T) {
	start := "2026-09-18 09:40:27.953 "
	lines := make([]string, 0, (logFormatSampleLines+10)*2)
	for index := 0; index < logFormatSampleLines+10; index++ {
		lines = append(lines, start+fmt.Sprintf("ERROR record-%d", index), "\tat stack")
	}
	docs, err := logSampleDocs(strings.Join(lines, "\n"), true, `^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}`)
	if err != nil {
		t.Fatalf("log sample docs: %v", err)
	}
	if len(docs) != logFormatSampleLines {
		t.Fatalf("docs 数 = %d, want %d", len(docs), logFormatSampleLines)
	}
	// 尾部保留：最后一条是原文最后一条记录。
	last := docs[len(docs)-1].(map[string]any)["message"].(string)
	if !strings.HasPrefix(last, start+fmt.Sprintf("ERROR record-%d", logFormatSampleLines+9)) {
		t.Fatalf("最后一条 = %q, want record-%d", last, logFormatSampleLines+9)
	}
	// 第一条是第 10 条记录（前 10 条被裁掉）。
	first := docs[0].(map[string]any)["message"].(string)
	if !strings.HasPrefix(first, start+"ERROR record-10") {
		t.Fatalf("第一条 = %q, want record-10", first)
	}
}

const (
	listServiceTemplateLogsQuery = "FROM assets_application_log_definition ld"
	getLogProcessingRuleQuery    = "FROM monitor_log_processing_rule"
)

func logFormatDefinitionColumns() []string {
	return []string{
		"id", "name", "path_pattern", "processing_rule_id", "processing_rule_name",
		"processing_rule_update_time", "application_version_id", "retention_tier_id",
		"override_collection_enabled", "collection_filter_rule_id", "collection_exclude_filter_rule_id",
		"filter_include_rule_id", "filter_exclude_rule_id", "format_verified_at",
		"format_verified_fingerprint", "format_verified_source", "format_verified_by",
		"service_code", "project_code", "environment_code", "business_system_code",
		"macro_values", "macro_definitions", "app_home", "tier_code",
	}
}

func expectLogFormatDefinition(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).WillReturnRows(
		sqlmock.NewRows(logFormatDefinitionColumns()).AddRow(
			int64(24), "error.log", "${APP_HOME}/nginx/logs/error.log", sql.NullInt64{Int64: 7, Valid: true},
			"nginx 规则", time.Date(2026, 9, 19, 1, 2, 3, 0, time.UTC), int64(3), sql.NullInt64{},
			nil, sql.NullInt64{},
			// 服务级 exclude 覆盖 + 模板级两个方向的默认值：本例都不配。
			sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
			sql.NullTime{}, sql.NullString{}, sql.NullString{}, sql.NullString{},
			"nginx", "yilake", sql.NullString{String: "poc", Valid: true}, "tib", []byte("{}"),
			[]byte(`[{"name":"APP_HOME","value":"/home/esb/tomcat"}]`), "/home/esb/tomcat", "std",
		))
}

func processingRuleColumns() []string {
	return []string{
		"id", "create_time", "update_time", "remark", "name", "description", "input_format",
		"multiline_enabled", "start_pattern", "continuation_pattern", "sample_log",
		"flush_timeout", "pipeline_body", "cluster_id", "application_id",
	}
}

func addProcessingRuleRow(rows *sqlmock.Rows, sampleLog string, pipelineBody string) *sqlmock.Rows {
	now := time.Date(2026, 9, 19, 1, 2, 3, 0, time.UTC)
	return rows.AddRow(
		int64(7), now, now, nil, "nginx 规则", "", "plain", false, "", "",
		sampleLog, uint32(2000), []byte(pipelineBody), int64(1), nil,
	)
}

// 命中必备字段的 pipeline：dissect 出 log_level / log_message / error_fingerprint。
const passingPipelineBody = `{"processors":[{"dissect":{"field":"message","pattern":"%{log_level} %{log_message} %{error_fingerprint}"}}]}`

// 只产出 log_message 的 pipeline：缺 log_level 与 error_fingerprint。
const partialPipelineBody = `{"processors":[{"set":{"field":"log_message","value":"{{message}}"}}]}`

// newESVerifyFixture 起一个假 Elasticsearch 与 sql mock，返回 handler 与假 ES 地址。
// 注意 sqlmock 默认按调用顺序匹配：集群查询要由用例在"日志定义 → 规则"之后再注册。
func newESVerifyFixture(t *testing.T, esHandler http.HandlerFunc) (*sqlmock.Sqlmock, *Handler, string) {
	t.Helper()
	elasticsearchServer := httptest.NewServer(esHandler)
	t.Cleanup(elasticsearchServer.Close)

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatalf("create secret encryptor: %v", err)
	}
	return &mock, &Handler{db: database, secrets: secrets}, elasticsearchServer.URL
}

func expectLogFormatCluster(mock sqlmock.Sqlmock, serverURL string) {
	mock.ExpectQuery(regexp.QuoteMeta(elasticsearchClusterQuery)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled"},
	).AddRow(int64(1), serverURL, "", "", false, "", "logs", 5, true))
}

// sample_log 依据的端到端：样例行 → inline _simulate → 必备字段判定。
func TestVerifyLogFormatFromRuleSampleLog(t *testing.T) {
	cases := []struct {
		name         string
		sampleLog    string
		pipelineBody string
		wantMissing  []string
	}{
		{
			name:         "样例能解析出必备字段：通过",
			sampleLog:    "INFO hello world fp-1",
			pipelineBody: passingPipelineBody,
		},
		{
			name:         "样例缺字段：返回缺失清单",
			sampleLog:    "INFO hello world fp-1",
			pipelineBody: partialPipelineBody,
			wantMissing:  []string{"error_fingerprint", "log_level"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mock, handler, clusterURL := newESVerifyFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodGet {
					// 索引模板不存在 → 回退内置标准字段。
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatalf("decode upstream body: %v", err)
				}
				// 认证必须用规则里的 pipeline body 走 inline 模拟：不依赖集群上是否已发布同名 pipeline。
				if _, exists := body["pipeline"]; !exists {
					t.Fatalf("认证应发送 inline pipeline，实际 body = %#v", body)
				}
				// 让 mock 的响应体现"pipeline 产出什么"：按 body 里的 pipeline 是否含 dissect 决定。
				pipeline, _ := body["pipeline"].(map[string]any)
				source := map[string]any{"message": "INFO hello world fp-1"}
				if processors, ok := pipeline["processors"].([]any); ok && len(processors) > 0 {
					if _, isDissect := processors[0].(map[string]any)["dissect"]; isDissect {
						source = map[string]any{"log_level": "INFO", "log_message": "hello world", "error_fingerprint": "fp-1"}
					} else {
						source = map[string]any{"log_message": "hello world"}
					}
				}
				payload, _ := json.Marshal(map[string]any{"docs": []any{map[string]any{"doc": map[string]any{"_source": source}}}})
				_, _ = writer.Write(payload)
			})
			expectLogFormatDefinition(*mock)
			(*mock).ExpectQuery(regexp.QuoteMeta(getLogProcessingRuleQuery)).
				WithArgs(int64(7)).
				WillReturnRows(addProcessingRuleRow(sqlmock.NewRows(processingRuleColumns()), testCase.sampleLog, testCase.pipelineBody))
			expectLogFormatCluster(*mock, clusterURL)

			report, err := handler.VerifyLogFormat(context.Background(), assets.LogFormatVerifyRequest{
				ServiceID: 15, LogDefinitionID: 24, Source: assets.LogFormatSourceSampleLog,
			})
			if err != nil {
				t.Fatalf("verify: %v", err)
			}
			if strings.Join(report.MissingFields, ",") != strings.Join(testCase.wantMissing, ",") {
				t.Fatalf("missing = %v, want %v", report.MissingFields, testCase.wantMissing)
			}
			if err = (*mock).ExpectationsWereMet(); err != nil {
				t.Fatalf("database expectations: %v", err)
			}
		})
	}
}

// 取不到样例必须报错，不能返回"空缺失"——否则编排层会把"没验成"写成"已验证"。
func TestVerifyLogFormatErrorsWhenSampleUnavailable(t *testing.T) {
	mock, handler, _ := newESVerifyFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("取不到样例时不该去问 ES")
	})
	expectLogFormatDefinition(*mock)
	// 规则没配样例日志。
	(*mock).ExpectQuery(regexp.QuoteMeta(getLogProcessingRuleQuery)).
		WithArgs(int64(7)).
		WillReturnRows(addProcessingRuleRow(sqlmock.NewRows(processingRuleColumns()), "", passingPipelineBody))

	_, err := handler.VerifyLogFormat(context.Background(), assets.LogFormatVerifyRequest{
		ServiceID: 15, LogDefinitionID: 24, Source: assets.LogFormatSourceSampleLog,
	})
	if err == nil {
		t.Fatalf("规则没配样例日志时应报错")
	}
	if !strings.Contains(err.Error(), "未配置样例日志") {
		t.Fatalf("error = %v, want 提示改用实例抽样", err)
	}
}

// startTestAgent 起一个 bufconn 上的真 gRPC 会话，测试进程扮演 agent：responder 收到一帧
// 请求后返回要回给 backend 的响应帧（nil 表示不回）。
func startTestAgent(t *testing.T, responder func(*pb.ServerFrame) *pb.AgentFrame) *agent.Gateway {
	t.Helper()
	const token = "test-token"
	gateway := agent.NewGateway(func(instanceName, receivedToken string) bool {
		return instanceName == "host-01" && receivedToken == token
	}, nil)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateway.Register(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	connection, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
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
	go func() {
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				return
			}
			if response := responder(frame); response != nil {
				_ = stream.Send(response)
			}
		}
	}()
	return gateway
}

// 全部实例 + 每个实例上所有匹配文件都要认证：通配展开出的两个文件都读、都送进 _simulate。
func TestVerifyLogFormatAllDeploymentsAllFiles(t *testing.T) {
	gateway := startTestAgent(t, func(frame *pb.ServerFrame) *pb.AgentFrame {
		if request := frame.GetListRequest(); request != nil {
			entries := map[string][]*pb.FileEntry{
				"/var/log":      {{Name: "app1", IsDir: true}, {Name: "app2", IsDir: true}},
				"/var/log/app1": {{Name: "error.log", IsDir: false, Size: 100, Mtime: 1}},
				"/var/log/app2": {{Name: "error.log", IsDir: false, Size: 200, Mtime: 2}},
			}
			return &pb.AgentFrame{Payload: &pb.AgentFrame_ListResponse{ListResponse: &pb.ListResponse{
				RequestId: request.RequestId, CurrentPath: request.Path, Entries: entries[request.Path],
			}}}
		}
		if request := frame.GetReadRequest(); request != nil {
			content := []byte("2026-09-19 10:00:00 ERROR boom\n")
			return &pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
				RequestId: request.RequestId, Data: content, Eof: true, FileSize: int64(len(content)),
			}}}
		}
		return nil
	})

	mock, handler, clusterURL := newESVerifyFixture(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		wrapped, _ := body["docs"].([]any)
		out := make([]any, 0, len(wrapped))
		for range wrapped {
			out = append(out, map[string]any{"doc": map[string]any{"_source": map[string]any{
				"log_level": "INFO", "log_message": "m", "error_fingerprint": "fp",
			}}})
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"docs": out})
	})

	expectLogFormatDefinition(*mock)
	(*mock).ExpectQuery(regexp.QuoteMeta(getLogProcessingRuleQuery)).WithArgs(int64(7)).
		WillReturnRows(addProcessingRuleRow(sqlmock.NewRows(processingRuleColumns()), "", passingPipelineBody))
	expectLogFormatCluster(*mock, clusterURL)
	(*mock).ExpectQuery(regexp.QuoteMeta("FROM assets_application_service_deployment")).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(int64(101)))
	(*mock).ExpectQuery(regexp.QuoteMeta("FROM assets_application_deployment d")).
		WithArgs(int64(101), int64(15), int64(24)).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "host_id", "host_instance_name", "deployment_instance_name",
			"runtime_variables", "app_home", "macro_definitions", "macro_values", "path_pattern",
		}).AddRow(int64(101), int64(5), "host-01", "inst-1", []byte("{}"), "", []byte("[]"), []byte("{}"),
			"/var/log/*/error.log"))

	handler.gateway = gateway
	report, err := handler.VerifyLogFormat(context.Background(), assets.LogFormatVerifyRequest{
		ServiceID: 15, LogDefinitionID: 24, AllDeployments: true, Source: assets.LogFormatSourceInstance,
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !report.Passed() {
		t.Fatalf("应通过，得到 %+v", report)
	}
	if len(report.Targets) != 1 {
		t.Fatalf("targets = %d, want 1（一台承载实例）", len(report.Targets))
	}
	if files := report.Targets[0].LogFiles; len(files) != 2 {
		t.Fatalf("该实例所有匹配文件都要认证，log_files = %v", files)
	}
	if err = (*mock).ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 实例依据：反向读取尾部窗口，窗口起点落在行中间的残行必须丢掉。
// 用 bufconn 起真 gRPC 会话，测试进程扮演 agent 回应 Stat/Read 请求。
func TestTailRemoteFileDropsPartialFirstLine(t *testing.T) {
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

	// 文件比窗口大：agent 必须收到一个"从尾部算起"的 offset，且只有一次 Read。
	const fileSize = logFormatSampleWindow + 4096
	tailData := []byte("half-line-cut-by-window\n2026-09-19 10:00:00 ERROR first\n2026-09-19 10:00:01 ERROR second\n")
	go func() {
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				return
			}
			if statRequest := frame.GetStatRequest(); statRequest != nil {
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_StatResponse{StatResponse: &pb.StatResponse{
					RequestId: statRequest.RequestId, NormalizedPath: "/var/log/nginx/error.log", Size: fileSize,
				}}})
				continue
			}
			if readRequest := frame.GetReadRequest(); readRequest != nil {
				if readRequest.Offset != fileSize-logFormatSampleWindow {
					t.Errorf("ReadRequest.Offset = %d, want %d（只读尾部窗口）", readRequest.Offset, fileSize-logFormatSampleWindow)
				}
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
					RequestId: readRequest.RequestId, Data: tailData, Eof: true, FileSize: fileSize,
				}}})
			}
		}
	}()

	handler := &Handler{gateway: gateway}
	text, err := handler.tailRemoteFile(ctx, "host-01", "/var/log/nginx/error.log")
	if err != nil {
		t.Fatalf("tail remote file: %v", err)
	}
	if strings.Contains(text, "half-line-cut-by-window") {
		t.Fatalf("窗口起点落在行中间，残行必须丢掉，得到 %q", text)
	}
	if !strings.HasPrefix(text, "2026-09-19 10:00:00 ERROR first") {
		t.Fatalf("尾部内容 = %q, want 从完整行开始", text)
	}
}

// 小文件（小于窗口）整个读：offset=0 时不该丢任何行。
func TestTailRemoteFileKeepsAllLinesForSmallFile(t *testing.T) {
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

	content := []byte("2026-09-19 10:00:00 ERROR only-line\n")
	go func() {
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				return
			}
			if statRequest := frame.GetStatRequest(); statRequest != nil {
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_StatResponse{StatResponse: &pb.StatResponse{
					RequestId: statRequest.RequestId, NormalizedPath: "/var/log/nginx/error.log", Size: int64(len(content)),
				}}})
				continue
			}
			if readRequest := frame.GetReadRequest(); readRequest != nil {
				if readRequest.Offset != 0 {
					t.Errorf("小文件应从 0 开始读，实际 offset = %d", readRequest.Offset)
				}
				_ = stream.Send(&pb.AgentFrame{Payload: &pb.AgentFrame_ReadChunk{ReadChunk: &pb.ReadChunk{
					RequestId: readRequest.RequestId, Data: content, Eof: true, FileSize: int64(len(content)),
				}}})
			}
		}
	}()

	handler := &Handler{gateway: gateway}
	text, err := handler.tailRemoteFile(ctx, "host-01", "/var/log/nginx/error.log")
	if err != nil {
		t.Fatalf("tail remote file: %v", err)
	}
	if text != string(content) {
		t.Fatalf("内容 = %q, want %q", text, string(content))
	}
}
