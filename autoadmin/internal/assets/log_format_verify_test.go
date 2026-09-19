package assets

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

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
		"processing_rule_update_time", "application_version_id", "retention_tier_id",
		"override_collection_enabled", "collection_filter_rule_id", "collection_exclude_filter_rule_id",
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
		RuleID: 7, RuleUpdatedAt: testRuleUpdatedAt, MacroValues: "{}", ApplicationVersion: 3,
	})
}

func addServiceTemplateLogRow(rows *sqlmock.Rows, stored sql.NullString) *sqlmock.Rows {
	return rows.AddRow(
		int64(24), "error.log", "${APP_HOME}/nginx/logs/error.log", sql.NullInt64{Int64: 7, Valid: true},
		"nginx 规则", testRuleUpdatedAt, int64(3), sql.NullInt64{}, nil, sql.NullInt64{},
		// 服务级 exclude 覆盖 + 模板级两个方向的默认值：本例都不配。
		sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
		sql.NullTime{}, stored, sql.NullString{}, sql.NullString{},
		"nginx", "yilake", sql.NullString{String: "poc", Valid: true}, "tib", []byte("{}"),
		[]byte(`[{"name":"APP_HOME","value":"/home/esb/tomcat"}]`), "/home/esb/tomcat", "std",
	)
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
	missing map[string][]string
	failOn  map[string]error
	calls   []LogFormatVerifyRequest
}

func (fake *fakeLogFormatVerifier) VerifyLogFormat(_ context.Context, request LogFormatVerifyRequest) ([]string, error) {
	fake.calls = append(fake.calls, request)
	if err, exists := fake.failOn[request.Source]; exists {
		return nil, err
	}
	return fake.missing[request.Source], nil
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

func TestVerifyServiceLogFormatWritesUpsertOnPass(t *testing.T) {
	mock, _, service := newLogFormatFixture(t, sql.NullString{})
	verifier := &fakeLogFormatVerifier{}
	service.SetLogFormatVerifier(verifier)

	// 通过时写库：必须是 upsert（覆盖行不存在时 INSERT 也要落下认证结果）。
	(*mock).ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(24), int64(15), sqlmock.AnyArg(),
			currentLogFingerprint(), LogFormatSourceInstance, "zhangsan").
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := service.VerifyServiceLogFormat(context.Background(), 15, 24, 20, LogFormatSourceInstance, "zhangsan")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !result.Passed || result.FormatState != formatStateVerified {
		t.Fatalf("result = %+v, want passed + verified", result)
	}
	if result.FormatVerifiedBy != "zhangsan" || result.FormatVerifiedSource != LogFormatSourceInstance {
		t.Fatalf("认证要记录操作人与依据，得到 %+v", result)
	}
	if len(verifier.calls) != 1 || verifier.calls[0].DeploymentID != 20 {
		t.Fatalf("执行器应收到部署实例 id，得到 %+v", verifier.calls)
	}
	if err = (*mock).ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestVerifyServiceLogFormatKeepsUnverifiedWhenFieldsMissing(t *testing.T) {
	mock, _, service := newLogFormatFixture(t, sql.NullString{})
	service.SetLogFormatVerifier(&fakeLogFormatVerifier{missing: map[string][]string{
		LogFormatSourceInstance: {"log_level", "error_fingerprint"},
	}})

	// 不通过时**不写库**：没有 Exec 预期，多写一次会被 ExpectationsWereMet 抓到。
	result, err := service.VerifyServiceLogFormat(context.Background(), 15, 24, 20, LogFormatSourceInstance, "zhangsan")
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

func TestVerifyServiceLogFormatWaiverRecordsFingerprintWithoutProbe(t *testing.T) {
	mock, _, service := newLogFormatFixture(t, sql.NullString{})
	// 豁免是"人工确认"，不该去碰 agent/ES；这里注入一个会报错的执行器来证明它没被调用。
	verifier := &fakeLogFormatVerifier{failOn: map[string]error{LogFormatSourceInstance: errors.New("不该被调用")}}
	service.SetLogFormatVerifier(verifier)

	(*mock).ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(24), int64(15), sqlmock.AnyArg(),
			currentLogFingerprint(), LogFormatSourceWaiver, "admin").
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := service.VerifyServiceLogFormat(context.Background(), 15, 24, 0, LogFormatSourceWaiver, "admin")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !result.Passed || result.FormatVerifiedSource != LogFormatSourceWaiver {
		t.Fatalf("豁免也要记指纹与依据，得到 %+v", result)
	}
	if len(verifier.calls) != 0 {
		t.Fatalf("豁免不应调用执行器，实际调用 %d 次", len(verifier.calls))
	}
	if err = (*mock).ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
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
	if _, err = service.VerifyServiceLogFormat(context.Background(), 15, 24, 0, LogFormatSourceInstance, "admin"); !errors.Is(err, ErrLogFormatDeploymentRequired) {
		t.Fatalf("instance 依据缺 deployment_id 应报 ErrLogFormatDeploymentRequired，得到 %v", err)
	}
	if _, err = service.VerifyServiceLogFormat(context.Background(), 15, 24, 20, "bogus", "admin"); !errors.Is(err, ErrLogFormatSourceInvalid) {
		t.Fatalf("非法依据应报 ErrLogFormatSourceInvalid，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生任何查询: %v", err)
	}
}

// 自动认证：实例取不到样例时退到规则样例；两条都失败就保持未认证（best-effort，不报错）。
func TestAutoVerifyFallsBackToRuleSampleLog(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	service.SetLogFormatVerifier(&fakeLogFormatVerifier{
		failOn: map[string]error{LogFormatSourceInstance: errors.New("agent offline")},
	})

	// 1) 列服务日志 → 2) 列实例 → 3) instance 认证（失败）→ 4) sample_log 认证（成功）→ 5) 写入
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WillReturnRows(addServiceTemplateLogRow(sqlmock.NewRows(serviceTemplateLogColumns()), sql.NullString{}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service_deployment")).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(int64(20)))
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WillReturnRows(addServiceTemplateLogRow(sqlmock.NewRows(serviceTemplateLogColumns()), sql.NullString{}))
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WillReturnRows(addServiceTemplateLogRow(sqlmock.NewRows(serviceTemplateLogColumns()), sql.NullString{}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(24), int64(15), sqlmock.AnyArg(),
			currentLogFingerprint(), LogFormatSourceSampleLog, "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	service.autoVerifyServiceLogFormats(context.Background(), 15)
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 已认证的行不该被自动认证重复打扰（认证是幂等的，重复触发只该在读库时短路）。
func TestAutoVerifySkipsAlreadyVerifiedRows(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	verifier := &fakeLogFormatVerifier{}
	service.SetLogFormatVerifier(verifier)

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WillReturnRows(addServiceTemplateLogRow(sqlmock.NewRows(serviceTemplateLogColumns()),
			sql.NullString{String: currentLogFingerprint(), Valid: true}))

	service.autoVerifyServiceLogFormats(context.Background(), 15)
	if len(verifier.calls) != 0 {
		t.Fatalf("已 verified 的行不应再触发认证，实际调用 %d 次", len(verifier.calls))
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
