package logcollect

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/job"

	"github.com/DATA-DOG/go-sqlmock"
)

// 批量作业执行器的回归用例：终态判定、分片续跑、失联对账。
//
// 这些用例覆盖的是"能跑完、跑不完、跑挂了"三种路径的分支选择，不覆盖真实主机上的下发/安装
// （那部分在 log_config_render_test.go 与真库冒烟里）。执行器的每台执行结果都从
// monitor_log_batch_job_item 落库，所以这里的断言点就是"哪一条 SQL 被写了、写了什么状态"。

// fakeLogBatchPublisher 记录执行器投出的续跑消息。
type fakeLogBatchPublisher struct {
	messages []job.Message
	err      error
}

func (publisher *fakeLogBatchPublisher) PublishLogCollect(_ context.Context, message job.Message) error {
	publisher.messages = append(publisher.messages, message)
	return publisher.err
}

// 用"到 WHERE/表名为止"的片段做匹配：占位符形态两侧不同（`?` / `$1`），断言不该依赖它。
var (
	batchClaimSQL     = regexp.QuoteMeta("UPDATE monitor_log_batch_job\nSET status='running'")
	batchResetSQL     = regexp.QuoteMeta("UPDATE monitor_log_batch_job_item\nSET status='pending'")
	batchGetSQL       = regexp.QuoteMeta("FROM monitor_log_batch_job WHERE id=")
	batchListSQL      = regexp.QuoteMeta("FROM monitor_log_batch_job_item\nWHERE batch_job_id=")
	batchItemRunSQL   = regexp.QuoteMeta("SET status='running', started_at=COALESCE")
	batchItemDoneSQL  = regexp.QuoteMeta("UPDATE monitor_log_batch_job_item\nSET status=")
	batchRefreshSQL   = regexp.QuoteMeta("SET success_count=(SELECT COUNT(*)")
	batchFinishSQL    = regexp.QuoteMeta("UPDATE monitor_log_batch_job\nSET status=")
	batchTargetSQL    = regexp.QuoteMeta("FROM monitor_log_collection_target l\nJOIN assets_host h")
	batchStaleListSQL = regexp.QuoteMeta("SELECT id, action FROM monitor_log_batch_job")
	batchRequeueSQL   = regexp.QuoteMeta("UPDATE monitor_log_batch_job SET status='pending'")
)

func batchJobRows(status string, total, success, failed int32) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "action", "status", "total_count", "success_count", "failed_count",
		"concurrency", "message", "requested_username", "started_at", "finished_at",
		"create_time", "update_time",
	}).AddRow(int64(1), LogBatchActionInstall, status, total, success, failed,
		int32(4), "", "admin", time.Now(), nil, time.Now(), time.Now())
}

func batchPendingItemRows(ids ...int64) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "target_id", "host_id", "host_name", "host_ip", "status", "message", "started_at", "finished_at"})
	for _, id := range ids {
		rows.AddRow(id, id*10, id*100, "host-1", "10.0.0.1", logBatchItemPending, "", nil, nil)
	}
	return rows
}

func newLogBatchRunner(t *testing.T, database *sql.DB, publisher *fakeLogBatchPublisher, budget time.Duration) *LogBatchRunner {
	t.Helper()
	options := DefaultLogBatchOptions()
	options.InstallConcurrency = 1
	options.ApplyConcurrency = 1
	options.Budget = budget
	return NewLogBatchRunner(&Handler{db: database}, publisher, options)
}

// 整批都失败（目标在作业执行期间被删）时：每台落 failed + 原因，作业终态是 failed，
// 且**不投续跑消息**（已经没有待处理项了）。
func TestLogBatchRunnerFinalizesFailedChunk(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.MatchExpectationsInOrder(false)
	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, database, publisher, time.Hour)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusPending, 2, 0, 0))
	mock.ExpectExec(batchClaimSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows(1, 2))
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows())

	// 两台主机都已经不在纳管里：逐台失败，但作业照常收尾。
	for item := 0; item < 2; item++ {
		mock.ExpectExec(batchItemRunSQL).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(batchTargetSQL).WillReturnError(sql.ErrNoRows)
		// 这一台必须落 failed 并带上原因（前端逐台展示失败原因）。
		mock.ExpectExec(batchItemDoneSQL).
			WithArgs(logBatchItemFailed, "日志采集目标已不存在", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(item+1)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(batchRefreshSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusRunning, 2, 0, 2))
	// 全部失败 → 终态 failed。
	mock.ExpectExec(batchFinishSQL).
		WithArgs(LogBatchStatusFailed, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 1,
	}); err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("没有待处理项时不应投续跑消息，得到 %d 条", len(publisher.messages))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 部分成功 → 终态是 partial（不是 success，也不是 failed）：前端要能一眼看出"有台没成功"。
func TestLogBatchRunnerFinalizesPartial(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, database, publisher, time.Hour)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusPending, 3, 2, 1))
	mock.ExpectExec(batchClaimSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows())
	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusRunning, 3, 2, 1))
	mock.ExpectExec(batchFinishSQL).
		WithArgs(LogBatchStatusPartial, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 1,
	}); err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 没有 pending 项、但计数不满（存在 running 项，来自上个分片被打断的执行者）：既不能收尾
// （会把半途而废的一批报成完成），也不能在这里重投（会变成热循环）——让出，等失联对账收敛。
func TestLogBatchRunnerSettleWaitsForStaleReaper(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, database, publisher, time.Hour)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusPending, 3, 1, 1))
	mock.ExpectExec(batchClaimSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows())
	// 3 台里只有 2 台出了结果 → 不收尾
	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusRunning, 3, 1, 1))

	if err := runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 1,
	}); err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("不该在等待对账时重投消息，得到 %d 条", len(publisher.messages))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 时间预算到点但还有待处理项：投一条续跑消息并正常返回（让当前消息被 ack），
// 不落终态——否则整批会被误判为已完成。
func TestLogBatchRunnerChunkReschedulesWhenBudgetExhausted(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.MatchExpectationsInOrder(false)
	publisher := &fakeLogBatchPublisher{}
	// 预算为 0：处理完第一个分片后立即判定"该让出了"。
	runner := newLogBatchRunner(t, database, publisher, 0)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusPending, 1, 0, 0))
	mock.ExpectExec(batchClaimSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows(1))
	mock.ExpectExec(batchItemRunSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchTargetSQL).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(batchItemDoneSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(batchRefreshSQL).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 7,
	}); err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("预算耗尽应投 1 条续跑消息，得到 %d 条", len(publisher.messages))
	}
	message := publisher.messages[0]
	if message.Kind != LogBatchKind || message.ResourceID != 7 {
		t.Fatalf("续跑消息应指向同一作业，得到 kind=%q id=%d", message.Kind, message.ResourceID)
	}
	// 未落终态：FinishLogBatchJob 的期望没有注册，ExpectationsWereMet 会因为我们没注册而忽略，
	// 因此这里显式断言"没有 Finish 之外的多余写入"由上面的 mock 顺序保证。
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 作业已经结束（终态）时的重投：直接 ack，不碰任何 item、不重投——这是"跑完的一批不会被跑第二遍"。
func TestLogBatchRunnerDeliveryAfterFinishDoesNoWork(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, database, publisher, time.Hour)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusSuccess, 2, 2, 0))

	if err := runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 1,
	}); err != nil {
		t.Fatalf("终态作业的重投应静默返回，得到：%v", err)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("终态作业不应投续跑消息")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("终态作业不应执行其它写入：%v", err)
	}
}

// 首次认领抢不到（并发投递/对账重投时另一分片已认领）：ack 掉，不推进。
func TestLogBatchRunnerLostClaimDoesNoWork(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, database, publisher, time.Hour)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusPending, 2, 0, 0))
	mock.ExpectExec(batchClaimSQL).WillReturnResult(sqlmock.NewResult(0, 0))

	if err := runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 1,
	}); err != nil {
		t.Fatalf("抢不到执行权应静默返回，得到：%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("抢不到执行权不应推进：%v", err)
	}
}

// 分片续跑：作业已是 running（上一个分片投出的续跑消息），必须能继续推进 ——
// 这一条不能靠 UPDATE 的行数判断（MySQL 把 running 再写成 running 返回 0 行），
// 所以执行器是"先读状态再决定"，这里断言读到 running 时会继续处理待处理项。
func TestLogBatchRunnerResumesRunningJob(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.MatchExpectationsInOrder(false)
	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, database, publisher, time.Hour)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusRunning, 1, 0, 0))
	// 不经过 ClaimLogBatchJob（续跑不认领）
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows(5))
	mock.ExpectExec(batchItemRunSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchTargetSQL).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(batchItemDoneSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(batchRefreshSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows())
	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusRunning, 1, 0, 1))
	mock.ExpectExec(batchFinishSQL).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 1,
	}); err != nil {
		t.Fatalf("续跑应推进，得到：%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 失联对账：running 且心跳过期的作业回落为 pending、item 里的 running 回落为 pending，
// 并重新入队（这就是"进程被杀后作业能接着跑"的路径）。
func TestLogBatchReaperRequeuesStaleJob(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, database, publisher, time.Hour)

	mock.ExpectQuery(batchStaleListSQL).WillReturnRows(
		sqlmock.NewRows([]string{"id", "action"}).AddRow(int64(9), LogBatchActionApply))
	mock.ExpectExec(batchRequeueSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(batchResetSQL).WillReturnResult(sqlmock.NewResult(0, 2))

	runner.reap(context.Background())

	if len(publisher.messages) != 1 || publisher.messages[0].ResourceID != 9 {
		t.Fatalf("失联作业应被重新入队，得到 %+v", publisher.messages)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 非本队列的 kind 必须被拒绝（否则会被 rabbit 客户端判为处理成功而 ack 掉）。
func TestLogBatchRunnerRejectsForeignKind(t *testing.T) {
	runner := &LogBatchRunner{}
	err := runner.Handle(context.Background(), job.Message{Kind: "scheduled_task", ResourceID: 1})
	if err == nil || !strings.Contains(err.Error(), "unsupported log batch message kind") {
		t.Fatalf("外来 kind 应被拒绝，得到：%v", err)
	}
}

// 投递失败必须冒泡，让消息进死信队列（然后由失联对账重投），不能静默当成已让出。
func TestLogBatchRunnerRescheduleFailurePropagates(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	mock.MatchExpectationsInOrder(false)
	publisher := &fakeLogBatchPublisher{err: errors.New("broker down")}
	runner := newLogBatchRunner(t, database, publisher, 0)

	mock.ExpectQuery(batchGetSQL).WillReturnRows(batchJobRows(LogBatchStatusPending, 1, 0, 0))
	mock.ExpectExec(batchClaimSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchListSQL).WillReturnRows(batchPendingItemRows(1))
	mock.ExpectExec(batchItemRunSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(batchTargetSQL).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(batchItemDoneSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(batchRefreshSQL).WillReturnResult(sqlmock.NewResult(0, 1))

	err = runner.Handle(context.Background(), job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "schedule next log batch chunk") {
		t.Fatalf("续跑投递失败应冒泡，得到：%v", err)
	}
}
