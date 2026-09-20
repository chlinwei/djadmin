package assets

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 多服务日志状态汇总（日志中心层级视图的数据源）的判据：
//  1. 逐服务的分桶（未认证/需复验/已验证/逐条关闭/未挂规则）必须与页面同口径 —— 同一份
//     ListServiceTemplateLogs，不允许在这里另算一遍指纹；
//  2. 合计由服务端给（指标条与清单不能各算一套）；
//  3. 配置态算不出来时 `pending` 为 null + `pending_error` 有话说，**绝不显示成 0**。

type fakePendingEvaluator struct {
	result   map[int64]ServiceLogPendingCounts
	err      error
	received []int64
	calls    int
}

func (fake *fakePendingEvaluator) ServiceLogPendingHosts(_ context.Context, serviceIDs []int64) (map[int64]ServiceLogPendingCounts, error) {
	fake.calls++
	fake.received = append([]int64{}, serviceIDs...)
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.result, nil
}

// statusLogEntry 一条日志定义在状态汇总里的关键列。
type statusLogEntry struct {
	logDefinition     int64
	ruleID            sql.NullInt64
	storedFingerprint sql.NullString
	collectionEnabled *bool
}

// statusLogRows 按 ListServiceTemplateLogs 的列序造行。
// 列序错了会被 Scan 报出来（不是静默错值），所以这里自己拼列是安全的。
func statusLogRows(entries []statusLogEntry) *sqlmock.Rows {
	rows := sqlmock.NewRows(serviceTemplateLogColumns())
	for _, entry := range entries {
		ruleID := entry.ruleID
		if !ruleID.Valid {
			ruleID = sql.NullInt64{}
		}
		ruleUpdate := testRuleUpdatedAt
		ruleName := "nginx 规则"
		if !ruleID.Valid {
			// 未挂规则：规则名/更新时间在 SQL 里是 LEFT JOIN 出来的空值。
			ruleUpdate = testRuleUpdatedAt
			ruleName = ""
		}
		rows.AddRow(
			entry.logDefinition, "error.log", "${APP_HOME}/nginx/logs/error.log", ruleID,
			ruleName, ruleUpdate, int64(3), sql.NullInt64{}, entry.collectionEnabled, sql.NullInt64{},
			sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
			sql.NullTime{}, entry.storedFingerprint, sql.NullString{}, sql.NullString{},
			"nginx", "yilake", sql.NullString{String: "poc", Valid: true}, "tib", []byte("{}"),
			[]byte(`[{"name":"APP_HOME","value":"/home/esb/tomcat"}]`), "/home/esb/tomcat", "std",
		)
	}
	return rows
}

// statusRowFingerprint 某条日志定义在当前输入下的指纹（用来构造"已验证"）。
func statusRowFingerprint(id int64, ruleID sql.NullInt64) string {
	return logFormatFingerprintOf(logFormatFingerprintInput{
		LogDefinition: id, LogName: "error.log", PathPattern: "${APP_HOME}/nginx/logs/error.log",
		RuleID: ruleID.Int64, RuleUpdatedAt: testRuleUpdatedAt, MacroValues: "{}", ApplicationVersion: 3,
	})
}

func TestLogStatusSummaryBucketsEveryLogDefinition(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	disabled := false
	verified := statusRowFingerprint(24, sql.NullInt64{Int64: 7, Valid: true})
	service.SetServiceLogPendingEvaluator(&fakePendingEvaluator{result: map[int64]ServiceLogPendingCounts{
		15: {Hosts: 3, Managed: 2, Unmanaged: 1, Synced: 1, Drift: 1},
	}})

	// 服务 15：一条已验证、一条需复验（指纹变了）、一条未认证且被关掉采集、一条没挂规则。
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(statusLogRows([]statusLogEntry{
			{logDefinition: 24, ruleID: sql.NullInt64{Int64: 7, Valid: true}, storedFingerprint: sql.NullString{String: verified, Valid: true}},
			{logDefinition: 25, ruleID: sql.NullInt64{Int64: 7, Valid: true}, storedFingerprint: sql.NullString{String: "stale", Valid: true}},
			{logDefinition: 26, ruleID: sql.NullInt64{Int64: 7, Valid: true}, collectionEnabled: &disabled},
			{logDefinition: 27},
		}))

	summary, err := service.LogStatusSummary(context.Background(), []int64{15})
	if err != nil {
		t.Fatalf("log status summary: %v", err)
	}
	if len(summary.Items) != 1 {
		t.Fatalf("items = %d，want 1", len(summary.Items))
	}
	item := summary.Items[0]
	if item.Logs != 4 || item.Verified != 1 || item.NeedsRecheck != 1 || item.Unverified != 2 {
		t.Fatalf("日志定义分桶不对：%+v", item)
	}
	// 关掉采集的那条 = 1；没挂规则的那条 = 1（它同时算"未认证"，两件事互不掩盖）。
	if item.DisabledLogs != 1 || item.NoRule != 1 {
		t.Fatalf("关闭条数/未挂规则条数不对：%+v", item)
	}
	if item.Pending == nil || item.Pending.Managed != 2 || item.Pending.Drift != 1 || item.Pending.Unmanaged != 1 {
		t.Fatalf("配置态未透传：%+v", item.Pending)
	}
	// 合计由服务端算：指标条与清单必须是同一组数字。
	totals := summary.Totals
	if totals.Services != 1 || totals.Logs != 4 || totals.Verified != 1 || totals.NeedsRecheck != 1 ||
		totals.Unverified != 2 || totals.DisabledLogs != 1 || totals.NoRule != 1 {
		t.Fatalf("合计不对：%+v", totals)
	}
	if totals.Managed != 2 || totals.Unmanaged != 1 || totals.Synced != 1 || totals.Drift != 1 {
		t.Fatalf("配置态合计不对：%+v", totals)
	}
	if summary.PendingError != "" {
		t.Fatalf("评估成功时不该有 pending_error：%q", summary.PendingError)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 配置态算不出来（没有启用的默认集群等）：如实报错，pending 为 null、合计里那几个数是 0
// ——界面据此显示"-"，而不是把"没算出来"显示成"都已同步"。
func TestLogStatusSummaryReportsPendingFailureInsteadOfZero(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	service.SetServiceLogPendingEvaluator(&fakePendingEvaluator{err: errors.New("没有已启用的默认 Elasticsearch 集群")})

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(statusLogRows([]statusLogEntry{{logDefinition: 24, ruleID: sql.NullInt64{Int64: 7, Valid: true}}}))

	summary, err := service.LogStatusSummary(context.Background(), []int64{15})
	if err != nil {
		t.Fatalf("评估失败不该让整个汇总失败（日志状态本身仍然有用）：%v", err)
	}
	if summary.PendingError == "" {
		t.Fatal("评估失败必须在 pending_error 里说明")
	}
	if summary.Items[0].Pending != nil {
		t.Fatalf("评估失败时 pending 应为 null，得到 %+v", summary.Items[0].Pending)
	}
	if summary.Totals.Managed != 0 || summary.Totals.Synced != 0 {
		t.Fatalf("评估失败时不该有配置态合计：%+v", summary.Totals)
	}
	// 日志状态照常给出。
	if summary.Items[0].Logs != 1 || summary.Totals.Logs != 1 {
		t.Fatalf("日志状态应照常返回：%+v", summary.Items[0])
	}
}

// 未注入评估器（单测/最小部署）：接口照常工作，pending 为 null 且**不报错**
//（这不是故障，是没接数据面）。
func TestLogStatusSummaryWithoutPendingEvaluator(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(statusLogRows([]statusLogEntry{{logDefinition: 24, ruleID: sql.NullInt64{Int64: 7, Valid: true}}}))

	summary, err := service.LogStatusSummary(context.Background(), []int64{15})
	if err != nil {
		t.Fatalf("log status summary: %v", err)
	}
	if summary.PendingError != "" || summary.Items[0].Pending != nil {
		t.Fatalf("未注入评估器时应静默省略配置态：%+v / %q", summary.Items[0].Pending, summary.PendingError)
	}
}

// 服务 id 去重、排序、只留正数；空集合与超过上限在读库之前就拒绝。
func TestLogStatusSummaryRejectsInvalidInput(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	if _, err = service.LogStatusSummary(context.Background(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("空列表应报 ErrInvalid，得到 %v", err)
	}
	if _, err = service.LogStatusSummary(context.Background(), []int64{0, -3}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("全是非法 id 应报 ErrInvalid，得到 %v", err)
	}
	oversized := make([]int64, MaxLogStatusSummaryServices+1)
	for index := range oversized {
		oversized[index] = int64(index + 1)
	}
	if _, err = service.LogStatusSummary(context.Background(), oversized); !errors.Is(err, ErrInvalid) {
		t.Fatalf("超过上限应报 ErrInvalid，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生任何查询: %v", err)
	}
}

// 同一批入参（顺序不同、带重复）结果必须一致：前端可能把同一个服务在两个维度上各带一次。
func TestLogStatusSummaryNormalizesAndDedupesIDs(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	fake := &fakePendingEvaluator{result: map[int64]ServiceLogPendingCounts{}}
	service.SetServiceLogPendingEvaluator(fake)

	// 收敛后按服务 id 升序读，与入参顺序无关。
	for _, serviceID := range []int64{15, 16} {
		mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
			WithArgs(serviceID).
			WillReturnRows(statusLogRows(nil))
	}
	summary, err := service.LogStatusSummary(context.Background(), []int64{16, 15, 16, 15, 0})
	if err != nil {
		t.Fatalf("log status summary: %v", err)
	}
	if len(summary.Items) != 2 {
		t.Fatalf("去重后应有两个服务，得到 %d", len(summary.Items))
	}
	if summary.Items[0].ServiceID != 15 || summary.Items[1].ServiceID != 16 {
		t.Fatalf("应按服务 id 升序稳定输出：%+v", summary.Items)
	}
	if fake.received[0] != 15 || fake.received[1] != 16 {
		t.Fatalf("评估器也应收敛去重后的 id：%+v", fake.received)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestSummarizeLogStatusItemsSkipsPendingWhenUnavailable(t *testing.T) {
	counts := ServiceLogPendingCounts{Hosts: 2, Managed: 2, Synced: 1, Drift: 1}
	items := []ServiceLogStatusItem{
		{ServiceID: 15, Logs: 2, Pending: &counts},
		{ServiceID: 16, Logs: 3},
	}
	withPending := summarizeLogStatusItems(items, true)
	if withPending.Logs != 5 || withPending.Managed != 2 || withPending.Synced != 1 || withPending.Drift != 1 {
		t.Fatalf("合计不对：%+v", withPending)
	}
	// 评估不可用时：日志类合计照旧，配置态合计一律 0（前端据 pending_error 显示"-"）。
	withoutPending := summarizeLogStatusItems(items, false)
	if withoutPending.Logs != 5 {
		t.Fatalf("日志合计不该受配置态影响：%+v", withoutPending)
	}
	if withoutPending.Managed != 0 || withoutPending.Hosts != 0 {
		t.Fatalf("配置态不可用时不该累加：%+v", withoutPending)
	}
}

// handler 契约：请求体 `{service_ids:[...]}`，响应 data 里是 items/totals/pending_error。
// 空列表是请求错误（不进服务层）。
func TestLogStatusSummaryHandlerContract(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	handler := NewHandler(service, nil, "")

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(statusLogRows([]statusLogEntry{{logDefinition: 24, ruleID: sql.NullInt64{Int64: 7, Valid: true}}}))

	body, _ := json.Marshal(map[string]any{"service_ids": []int64{15}})
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/assets/application-services/log-status-summary/", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	handler.LogStatusSummary(context)

	var payload struct {
		Data ServiceLogStatusSummary `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应体: %v（body=%s）", err, recorder.Body.String())
	}
	if len(payload.Data.Items) != 1 || payload.Data.Items[0].ServiceID != 15 {
		t.Fatalf("响应 items 不对：%s", recorder.Body.String())
	}
	if payload.Data.Totals.Services != 1 || payload.Data.Totals.Logs != 1 {
		t.Fatalf("响应 totals 不对：%s", recorder.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}

	// 空 service_ids：错误信封（data=null），不读库。
	recorder = httptest.NewRecorder()
	context, _ = gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/assets/application-services/log-status-summary/",
		bytes.NewReader([]byte(`{"service_ids":[]}`)))
	context.Request.Header.Set("Content-Type", "application/json")
	handler.LogStatusSummary(context)

	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析错误信封: %v（body=%s）", err, recorder.Body.String())
	}
	if envelope.Code == 200 || string(envelope.Data) != "null" {
		t.Fatalf("空列表应是错误响应，得到 %s", recorder.Body.String())
	}
}
