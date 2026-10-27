package assets

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

// 日志格式认证的回归用例（架构文档 §4.8）。
//
// 这批用例钉住三件容易悄悄坏掉的事：
//  1. format_state 的三态判定——尤其是 needs_recheck：存量数据没有这一态时，
//     "规则改了"会继续显示"已认证"，认证就失去意义了；
//  2. 认证结果**必须** upsert——覆盖行不存在时 UPDATE-only 会静默影响 0 行，
//     于是"认证通过"这个动作在库里什么也没留下（这正是接线前的状态）；
//  3. 不通过 / 参数非法时**不能写库**：把"没验成"记成"已验证"比不认证更糟。

// listServiceTemplateLogsQuery 片段到 WHERE 为止：占位符形态两侧不同（`?` / `$1`），断言不依赖它。
const listServiceTemplateLogsQuery = "FROM assets_application_log_definition ld"

var testRuleUpdatedAt = time.Date(2026, 9, 19, 1, 2, 3, 456000000, time.UTC)

func serviceTemplateLogColumns() []string {
	return []string{
		"id", "name", "path_pattern", "processing_rule_id", "processing_rule_name",
		"processing_rule_input_format", "processing_rule_multiline_enabled", "processing_rule_start_pattern",
		"processing_rule_continuation_pattern", "processing_rule_flush_timeout", "processing_rule_pipeline_body",
		"application_version_id", "retention_tier_id",
		"override_collection_enabled", "collection_filter_rule_id", "collection_exclude_filter_rule_id",
		// 服务级后处理器（2026-09-28）：本套测试统一返回未启用（[]）。
		"extra_processors",
		"filter_include_rule_id", "filter_exclude_rule_id", "format_verified_at",
		"format_verified_fingerprint", "format_verified_source", "format_verified_by",
		"service_code", "project_code", "environment_code", "business_system_code",
		"macro_values", "macro_definitions", "app_home", "tier_code",
	}
}

// currentLogFingerprint 与 ListServiceTemplateLogs 现场算出的指纹同源（同一个函数、同一组输入）。
func currentLogFingerprint() string {
	return logFormatFingerprintOf(logFormatFingerprintInput{
		LogDefinition: 24, LogName: "error.log", PathPattern: "${APP_HOME}/nginx/logs/error.log",
		RuleID: 7, RuleParsingSignature: testLogRuleSignature(), MacroValues: "{}", ApplicationVersion: 3,
	})
}

func expectNoLogFormatInstances(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM assets_log_format_target WHERE service_id").
		WillReturnRows(sqlmock.NewRows([]string{"service_id", "log_definition_id", "deployment_id", "fingerprint", "status", "attempts", "next_attempt_at", "lease_token", "report", "actor", "reason", "update_time"}))
	mock.ExpectQuery("FROM assets_application_service_deployment").
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}))
}

func addServiceTemplateLogRow(rows *sqlmock.Rows, stored sql.NullString) *sqlmock.Rows {
	return addServiceTemplateLogRowWithRule(rows, stored, "nginx 规则", logFormatRuleInput{
		InputFormat: "plain", FlushTimeout: 2000, PipelineBody: []byte(`{"processors":[]}`),
	})
}

func addServiceTemplateLogRowWithRule(rows *sqlmock.Rows, stored sql.NullString, ruleName string, rule logFormatRuleInput) *sqlmock.Rows {
	return rows.AddRow(
		int64(24), "error.log", "${APP_HOME}/nginx/logs/error.log", sql.NullInt64{Int64: 7, Valid: true},
		ruleName, rule.InputFormat, rule.MultilineEnabled, rule.StartPattern, rule.ContinuationPattern,
		rule.FlushTimeout, rule.PipelineBody,
		int64(3), sql.NullInt64{}, nil, sql.NullInt64{},
		// 服务级 exclude 覆盖 + 模板级两个方向的默认值：本例都不配。
		sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
		[]byte(`[]`),
		sql.NullTime{}, stored, sql.NullString{}, sql.NullString{},
		"nginx", "yilake", sql.NullString{String: "poc", Valid: true}, "tib", []byte("{}"),
		[]byte(`[{"name":"APP_HOME","value":"/home/esb/tomcat"}]`), "/home/esb/tomcat", "std",
	)
}

func TestListServiceTemplateLogsUsesRuleBehavior(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		ruleName string
		body     string
		want     string
		wantErr  bool
	}{
		{"原样保存", "nginx 规则", `{"processors":[]}`, formatStateVerified, false},
		{"名称和描述变化", "修改后的规则名称", `{"description":"只改描述","processors":[]}`, formatStateVerified, false},
		{"仅 JSON 排版变化", "nginx 规则", "{\n \"processors\": []\n}", formatStateVerified, false},
		{"解析逻辑变化", "nginx 规则", `{"processors":[{"drop":{}}]}`, formatStateNeedsRecheck, false},
		{"无效配置不生成默认指纹", "nginx 规则", `{`, "", true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
				WithArgs(int64(15)).
				WillReturnRows(addServiceTemplateLogRowWithRule(
					sqlmock.NewRows(serviceTemplateLogColumns()),
					sql.NullString{String: currentLogFingerprint(), Valid: true},
					testCase.ruleName,
					logFormatRuleInput{InputFormat: "plain", FlushTimeout: 2000, PipelineBody: []byte(testCase.body)},
				))
			items, err := NewRepository(database).ListServiceTemplateLogs(context.Background(), 15)
			if testCase.wantErr {
				if err == nil || items != nil {
					t.Fatalf("无效规则应明确返回错误，items=%v err=%v", items, err)
				}
			} else if err != nil || len(items) != 1 || items[0].FormatState != testCase.want {
				t.Fatalf("items=%v err=%v, want %s", items, err, testCase.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLogFormatQueryExcludesRuleMetadata(t *testing.T) {
	for _, path := range []string{"db/queries/mysql/assets.sql", "db/queries/postgres/assets.sql"} {
		content, err := readRepositoryFile(path)
		if err != nil {
			t.Fatal(err)
		}
		statement := namedStatement(content, "ListServiceTemplateLogs")
		for _, field := range []string{"rule.update_time", "rule.description", "rule.remark", "rule.sample_log"} {
			if strings.Contains(statement, field) {
				t.Fatalf("%s 的认证查询不应依赖 %s", path, field)
			}
		}
		for _, field := range []string{"rule.pipeline_body", "rule.input_format", "rule.multiline_enabled",
			"rule.start_pattern", "rule.continuation_pattern", "rule.flush_timeout"} {
			if !strings.Contains(statement, field) {
				t.Fatalf("%s 的认证查询缺少 %s", path, field)
			}
		}
	}
}

func TestListServiceTemplateLogsFormatState(t *testing.T) {
	current := currentLogFingerprint()
	cases := []struct {
		name   string
		stored sql.NullString
		want   string
	}{
		{"从未认证：指纹为空", sql.NullString{}, formatStateUnverified},
		{"认证过且指纹一致", sql.NullString{String: current, Valid: true}, formatStateVerified},
		{"认证过但规则/模板/宏/版本变了", sql.NullString{String: "0123456789abcdef0123456789abcdef", Valid: true}, formatStateNeedsRecheck},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("create sql mock: %v", err)
			}
			defer database.Close()
			mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
				WithArgs(int64(15)).
				WillReturnRows(addServiceTemplateLogRow(sqlmock.NewRows(serviceTemplateLogColumns()), testCase.stored))

			items, err := NewRepository(database).ListServiceTemplateLogs(context.Background(), 15)
			if err != nil {
				t.Fatalf("list service template logs: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("rows = %d, want 1", len(items))
			}
			if items[0].FormatState != testCase.want {
				t.Fatalf("format_state = %q, want %q", items[0].FormatState, testCase.want)
			}
			// 生效档位编码要带出来：日志中心页靠它区分"当前档位"与"改档位留下的历史流"。
			if items[0].TierCode != "std" {
				t.Fatalf("tier_code = %q, want %q", items[0].TierCode, "std")
			}
			if items[0].FormatFingerprint != current {
				t.Fatalf("format_fingerprint = %q, want %q（展示与认证必须同源）", items[0].FormatFingerprint, current)
			}
		})
	}
}

// fakeLogFormatVerifier 记录调用并按 source 返回预设结果。
type fakeLogFormatVerifier struct {
	report  *LogFormatVerifyReport
	missing map[string][]string
	failOn  map[string]error
	calls   []LogFormatVerifyRequest
}

func (fake *fakeLogFormatVerifier) VerifyLogFormat(_ context.Context, request LogFormatVerifyRequest) (LogFormatVerifyReport, error) {
	fake.calls = append(fake.calls, request)
	if err, exists := fake.failOn[request.Source]; exists {
		return LogFormatVerifyReport{}, err
	}
	if fake.report != nil {
		return *fake.report, nil
	}
	return LogFormatVerifyReport{SampleCount: 1, MissingFields: fake.missing[request.Source]}, nil
}

func newLogFormatFixture(t *testing.T, stored sql.NullString) (*sqlmock.Sqlmock, *sql.DB, *Service) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WillReturnRows(addServiceTemplateLogRow(sqlmock.NewRows(serviceTemplateLogColumns()), stored))
	return &mock, database, newTestService(t, NewRepository(database))
}

func TestRuleSampleDoesNotCertifyProductionInstances(t *testing.T) {
	mock, _, service := newLogFormatFixture(t, sql.NullString{})
	verifier := &fakeLogFormatVerifier{}
	service.SetLogFormatVerifier(verifier)

	result, err := service.VerifyServiceLogFormat(context.Background(), 15, 24, 0, false, LogFormatSourceSampleLog, "zhangsan")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !result.Passed || result.FormatState != formatStateUnverified {
		t.Fatalf("样例测试通过不能认证真实实例: %+v", result)
	}
	if len(verifier.calls) != 1 || verifier.calls[0].Source != LogFormatSourceSampleLog {
		t.Fatalf("只能测试规则样例，得到 %+v", verifier.calls)
	}
	if err = (*mock).ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestVerifyServiceLogFormatKeepsUnverifiedWhenFieldsMissing(t *testing.T) {
	mock, _, service := newLogFormatFixture(t, sql.NullString{})
	service.SetLogFormatVerifier(&fakeLogFormatVerifier{missing: map[string][]string{
		LogFormatSourceSampleLog: {"log_level", "error_fingerprint"},
	}})

	// 不通过时**不写库**：没有 Exec 预期，多写一次会被 ExpectationsWereMet 抓到。
	result, err := service.VerifyServiceLogFormat(context.Background(), 15, 24, 0, false, LogFormatSourceSampleLog, "zhangsan")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Passed {
		t.Fatalf("缺必备字段不应判通过：%+v", result)
	}
	if len(result.MissingFields) != 2 || result.MissingFields[0] != "log_level" {
		t.Fatalf("missing_fields = %v, want [log_level error_fingerprint]", result.MissingFields)
	}
	if result.FormatState != formatStateUnverified {
		t.Fatalf("未写入时状态应保持 unverified，得到 %q", result.FormatState)
	}
	if err = (*mock).ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestWaiverRequiresReasonAndActor(t *testing.T) {
	for _, actor := range []string{"", "admin"} {
		database, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		service := newTestService(t, NewRepository(database))
		_, err = service.VerifyServiceLogFormat(context.Background(), 15, 24, 0, false, LogFormatSourceWaiver, actor, "")
		if err == nil {
			t.Fatal("没有原因或操作人的豁免必须拒绝")
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}

func TestVerifyServiceLogFormatRejectsInvalidArgumentsBeforeReading(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	service.SetLogFormatVerifier(&fakeLogFormatVerifier{})

	// 两条都应在碰数据库之前就拒绝：没有任何查询预期。
	if _, err = service.VerifyServiceLogFormat(context.Background(), 15, 24, 0, false, LogFormatSourceInstance, "admin"); !errors.Is(err, ErrLogFormatDeploymentRequired) {
		t.Fatalf("instance 依据缺 deployment_id 应报 ErrLogFormatDeploymentRequired，得到 %v", err)
	}
	if _, err = service.VerifyServiceLogFormat(context.Background(), 15, 24, 20, false, "bogus", "admin"); !errors.Is(err, ErrLogFormatSourceInvalid) {
		t.Fatalf("非法依据应报 ErrLogFormatSourceInvalid，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生任何查询: %v", err)
	}
}

func TestLogFormatOutcomesAndRetryBudget(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		report LogFormatVerifyReport
		err    error
		status string
	}{
		{"真实通过", LogFormatVerifyReport{SampleCount: 50}, nil, "verified"},
		{"无记录", LogFormatVerifyReport{}, nil, "failed"},
		{"全部过滤", LogFormatVerifyReport{SampleCount: 5, DroppedSamples: 5}, nil, "failed"},
		{"部分过滤", LogFormatVerifyReport{SampleCount: 5, DroppedSamples: 2}, nil, "verified"},
		{"字段缺失", LogFormatVerifyReport{SampleCount: 5, MissingFields: []string{"log_level"}}, nil, "failed"},
		{"离线", LogFormatVerifyReport{}, errors.New("agent offline"), "waiting"},
		{"空文件", LogFormatVerifyReport{Targets: []LogFormatVerifyTarget{{Error: "empty file"}}}, nil, "waiting"},
		{"首行规则错误", LogFormatVerifyReport{Targets: []LogFormatVerifyTarget{{Error: "invalid pattern", FailureKind: "format"}}}, nil, "failed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			status, _ := logFormatOutcome(testCase.report, testCase.err)
			if status != testCase.status {
				t.Fatalf("status=%s want=%s", status, testCase.status)
			}
		})
	}
	if logFormatRetryDelay(1) != time.Minute || logFormatRetryDelay(4) != 8*time.Minute || logFormatRetryDelay(100) != 8*time.Minute {
		t.Fatal("重试必须指数退避并封顶")
	}
}

func logFormatInstanceFixture() db.GetLogVerifyInstanceContextRow {
	return db.GetLogVerifyInstanceContextRow{
		DeploymentID: 20, HostID: 5, HostInstanceName: "host-01", DeploymentInstanceName: "instance-1",
		RuntimeVariables: []byte(`{"PORT":8080}`), MacroDefinitions: []byte(`[]`), AppHome: "/app",
	}
}

func expectCurrentLogFormatTarget(mock sqlmock.Sqlmock, instance db.GetLogVerifyInstanceContextRow) {
	mock.ExpectQuery("SELECT log_collection_enabled, log_retention_tier_id").
		WillReturnRows(sqlmock.NewRows([]string{"log_collection_enabled", "log_retention_tier_id"}).AddRow(true, int64(4)))
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WillReturnRows(addServiceTemplateLogRow(sqlmock.NewRows(serviceTemplateLogColumns()), sql.NullString{}))
	expectLogFormatInstanceContext(mock, instance)
}

func expectLogFormatInstanceContext(mock sqlmock.Sqlmock, instance db.GetLogVerifyInstanceContextRow) {
	mock.ExpectQuery("FROM assets_application_deployment d").WithArgs(instance.DeploymentID, int64(15), int64(24)).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "host_id", "host_instance_name", "deployment_instance_name",
			"runtime_variables", "app_home", "macro_definitions", "macro_values", "path_pattern"}).
			AddRow(instance.DeploymentID, instance.HostID, instance.HostInstanceName, instance.DeploymentInstanceName,
				instance.RuntimeVariables, instance.AppHome, instance.MacroDefinitions, []byte(`{}`), "${APP_HOME}/error.log"))
}

func TestManualLogFormatRequestsPersistEachInstance(t *testing.T) {
	for _, source := range []string{LogFormatSourceInstance, LogFormatSourceWaiver} {
		t.Run(source, func(t *testing.T) {
			mockPointer, _, service := newLogFormatFixture(t, sql.NullString{})
			mock := *mockPointer
			verifier := &fakeLogFormatVerifier{}
			service.SetLogFormatVerifier(verifier)
			mock.ExpectQuery("FROM assets_application_service_deployment").
				WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(int64(20)).AddRow(int64(21)))
			mock.ExpectQuery("SELECT log_collection_enabled, log_retention_tier_id").
				WillReturnRows(sqlmock.NewRows([]string{"log_collection_enabled", "log_retention_tier_id"}).AddRow(true, int64(4)))
			for _, deploymentID := range []int64{20, 21} {
				instance := logFormatInstanceFixture()
				instance.DeploymentID = deploymentID
				expectLogFormatInstanceContext(mock, instance)
				mock.ExpectExec("INSERT INTO assets_log_format_target").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE assets_log_format_target SET fingerprint").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			status, reason := "pending", ""
			if source == LogFormatSourceWaiver {
				status, reason = "waived", "业务已确认例外"
			}
			mock.ExpectBegin()
			for _, deploymentID := range []int64{20, 21} {
				mock.ExpectExec("UPDATE assets_log_format_target SET status").
					WithArgs(status, sqlmock.AnyArg(), "admin", reason, sqlmock.AnyArg(), int64(15), int64(24), deploymentID, sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			result, err := service.VerifyServiceLogFormat(context.Background(), 15, 24, 0, true, source, "admin", reason)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed || result.FormatState != status || result.Queued != (source == LogFormatSourceInstance) {
				t.Fatalf("排队和豁免不能声称真实格式通过: %+v", result)
			}
			if len(verifier.calls) != 0 {
				t.Fatal("手动提交不得在 HTTP 请求内取样")
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLogInstanceFingerprintTracksRuntimeNotJSONFormatting(t *testing.T) {
	instance := logFormatInstanceFixture()
	base, err := logInstanceFingerprint("rule-1", instance)
	if err != nil {
		t.Fatal(err)
	}
	instance.RuntimeVariables = []byte("{ \"PORT\": 8080 }")
	same, _ := logInstanceFingerprint("rule-1", instance)
	if same != base {
		t.Fatal("仅 JSON 排版不应要求重验")
	}
	instance.RuntimeVariables = []byte(`{"PORT":9090}`)
	changed, _ := logInstanceFingerprint("rule-1", instance)
	if changed == base {
		t.Fatal("实例运行变量变化必须使认证失效")
	}
	instance = logFormatInstanceFixture()
	instance.HostID = 6
	changed, _ = logInstanceFingerprint("rule-1", instance)
	if changed == base {
		t.Fatal("换主机必须使认证失效")
	}
}

func TestLogFormatLeaseAndStaleResults(t *testing.T) {
	for _, mode := range []string{"verified", "lease_lost", "changed_before_probe", "changed_after_probe"} {
		t.Run(mode, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			service := newTestService(t, NewRepository(database))
			verifier := &fakeLogFormatVerifier{report: &LogFormatVerifyReport{
				SampleCount: 50, Targets: []LogFormatVerifyTarget{{DeploymentID: 20, SampleCount: 50}},
			}}
			service.SetLogFormatVerifier(verifier)
			instance := logFormatInstanceFixture()
			fingerprint, _ := logInstanceFingerprint(currentLogFingerprint(), instance)
			target := db.AssetsLogFormatTarget{ServiceID: 15, LogDefinitionID: 24, DeploymentID: 20, Fingerprint: fingerprint}
			before := instance
			if mode == "changed_before_probe" {
				before.RuntimeVariables = []byte(`{"PORT":9090}`)
			}
			expectCurrentLogFormatTarget(mock, before)
			if mode != "changed_before_probe" {
				affected := int64(1)
				if mode == "lease_lost" {
					affected = 0
				}
				mock.ExpectExec("UPDATE assets_log_format_target SET status = 'running'").
					WillReturnResult(sqlmock.NewResult(0, affected))
				if mode != "lease_lost" {
					after := instance
					if mode == "changed_after_probe" {
						after.RuntimeVariables = []byte(`{"PORT":9090}`)
					}
					expectCurrentLogFormatTarget(mock, after)
					if mode == "verified" {
						mock.ExpectExec("UPDATE assets_log_format_target SET status = ").
							WithArgs("verified", sqlmock.AnyArg(), "", "", sqlmock.AnyArg(), sqlmock.AnyArg(),
								int64(15), int64(24), int64(20), fingerprint, sqlmock.AnyArg()).
							WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if err = service.runLogFormatTarget(context.Background(), target); err != nil {
				t.Fatal(err)
			}
			if mode == "lease_lost" || mode == "changed_before_probe" {
				if len(verifier.calls) != 0 {
					t.Fatal("未获得租约或指纹已过期不能抽样")
				}
			} else if len(verifier.calls) != 1 || verifier.calls[0].Source != LogFormatSourceInstance {
				t.Fatal("后台只能执行一次真实抽样")
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFormatInstanceSummaryNeverTreatsPartialOrWaiverAsVerified(t *testing.T) {
	for _, testCase := range []struct {
		states []string
		want   string
	}{
		{nil, "waiting"}, {[]string{"verified", "verified"}, "verified"},
		{[]string{"verified", "pending"}, "pending"}, {[]string{"verified", "failed"}, "failed"},
		{[]string{"verified", "waiting"}, "waiting"}, {[]string{"verified", "waived"}, "waived"},
	} {
		instances := make([]LogFormatInstanceState, len(testCase.states))
		for index, state := range testCase.states {
			instances[index].Status = state
		}
		if got := summarizeFormatInstances(instances); got != testCase.want {
			t.Fatalf("states=%v got=%s want=%s", testCase.states, got, testCase.want)
		}
	}
}
