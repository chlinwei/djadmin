package logcollect

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"autoadmin/internal/job"
	"autoadmin/internal/messaging/rabbitmq"
	generated "autoadmin/internal/platform/database/generated"

	"github.com/google/uuid"
)

// 日志采集批量动作的执行器（计划 LOG_COLLECTION_LIFECYCLE §8 Phase 2）。
//
// 批量下发/批量安装曾经在**单个 HTTP 请求内串行**遍历目标：1000 台时该请求必然超时，
// 而且请求一断就在链路上留下部分下发的中间态；安装/重试更糟——每台起一个 `go func()`
// 在 API 进程内联跑 ansible，没有并发上限。
//
// 现在的形态是「入队 + 有界并发 + 进度可查 + 可续跑」：
//
//   - 入队：API 只写 monitor_log_batch_job / _item 两张表，再向 rabbitmq.LogCollectRoute 投一条消息；
//   - 有界并发：一条消息内用信号量按动作各自的并发上限跑（安装 20 / 下发 5，见 §9 第 9 条：
//     下发并发放小，避免 1000 台同时重启 Filebeat）；
//   - 进度可查：每台的结果落在 item 行上，头表计数从 item 表重算，前端轮询作业详情接口；
//   - 可续跑：一条消息只跑 Budget 时长（分片），到点投一条续跑消息后返回（ack 掉当前消息）；
//     进程被杀时消息未 ack、重启后被重投，只处理仍是 pending 的 item，已成功的不会重跑。
//
// 为什么由 **api 角色**消费而不是 worker 角色：这类作业要通过 agent gRPC 会话在主机上执行，
// 而 agent 会话是 api 进程内的 Gateway 会话表（agents 连的是 api 的 gRPC 端口），
// 在 worker 进程里 IsOnline 恒为 false，作业会全部失败。详见 rabbitmq.LogCollectRoute。

// 批量作业的动作。取值即 monitor_log_batch_job.action。
const (
	// LogBatchActionApply 下发采集配置（渲染片段 + output，指纹一致则跳过）。
	LogBatchActionApply = "apply"
	// LogBatchActionInstall 安装/重新安装 Filebeat（离线 ansible playbook）。
	LogBatchActionInstall = "install"
)

// LogBatchKind 是队列消息的 kind。
const LogBatchKind = "log_collect_batch"

// 作业与 item 的状态：头表 pending → running → success|partial|failed；
// item pending → running → success|failed。
const (
	LogBatchStatusPending = "pending"
	LogBatchStatusRunning = "running"
	LogBatchStatusSuccess = "success"
	LogBatchStatusPartial = "partial"
	LogBatchStatusFailed  = "failed"

	logBatchItemPending = "pending"
	logBatchItemRunning = "running"
	logBatchItemSuccess = "success"
	logBatchItemFailed  = "failed"
)

// LogBatchOptions 批量执行器的规模参数。
type LogBatchOptions struct {
	// ConsumerName 是 RabbitMQ 消费者名（同名消费者在管理界面里归为一组）。
	ConsumerName string
	// Prefetch 是队列侧的未确认上限，也是"同时在跑的批量作业数"。
	Prefetch int
	// InstallConcurrency / ApplyConcurrency 是单个作业内的并发上限，按动作分开配置：
	// 安装是离线包分发（磁盘/带宽型），可以高一些；下发会重启 Filebeat，必须低。
	InstallConcurrency int
	ApplyConcurrency   int
	// Budget 是单条消息的时间预算。必须明显小于 RabbitMQ 的 consumer_timeout（默认 30 分钟，
	// 超时会被服务端强制关闭 channel 并重投消息）：到点就投续跑消息并让出。
	Budget time.Duration
}

// DefaultLogBatchOptions 是缺省规模参数（由配置覆盖）。
func DefaultLogBatchOptions() LogBatchOptions {
	return LogBatchOptions{
		ConsumerName:       "autoadmin-logcollect",
		Prefetch:           2,
		InstallConcurrency: 20,
		ApplyConcurrency:   5,
		Budget:             15 * time.Minute,
	}
}

// logBatchPublisher 只用到"向采集队列投递一条消息"这一点能力，
// 批量执行器不依赖 rabbitmq 客户端的完整接口，便于测试注入。
type logBatchPublisher interface {
	PublishLogCollect(ctx context.Context, message job.Message) error
}

// 单个分片内一次取回的待处理项条数：并发上限的若干倍——既不频繁查库，
// 也不至于把一整批（可能上千台）读进内存。
const logBatchItemBatchFactor = 4

// 心跳与失联判定：执行中的作业每 30 秒刷新头表 update_time，
// 因此超过 5 分钟没有刷新就能断定执行者已经不在了。
const (
	logBatchHeartbeatInterval = 30 * time.Second
	logBatchStaleAfter        = 5 * time.Minute
	logBatchReapInterval      = time.Minute
)

// LogBatchRunner 消费采集队列并执行批量作业。
type LogBatchRunner struct {
	handler   *Handler
	publisher logBatchPublisher
	options   LogBatchOptions
	// 同一个作业的分片串行执行。**必须有这把锁**：分片续跑是"处理完一个分片后自己再投一条
	// 消息"，而 prefetch>1 时那条消息可能被另一个消费 goroutine 立刻取到——没有锁就会有两个
	// 分片并发从 item 表里挑 pending 项，同一台被跑两遍。
	// （跨进程的重复执行不在防护范围内：本平台按单实例部署 api 角色。）
	locks sync.Map
}

// jobLock 取该作业的进程内执行锁。
func (runner *LogBatchRunner) jobLock(jobID int64) *sync.Mutex {
	value, _ := runner.locks.LoadOrStore(jobID, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func NewLogBatchRunner(handler *Handler, publisher logBatchPublisher, options LogBatchOptions) *LogBatchRunner {
	return &LogBatchRunner{handler: handler, publisher: publisher, options: options}
}

// Consume 消费采集队列（阻塞到 ctx 结束）。由 api 角色启动时拉起。
func (runner *LogBatchRunner) Consume(ctx context.Context, client *rabbitmq.Client) error {
	prefetch := runner.options.Prefetch
	if prefetch < 1 {
		prefetch = 1
	}
	return client.ConsumeVia(ctx, rabbitmq.LogCollectRoute, runner.options.ConsumerName, prefetch, runner)
}

// Handle 执行一条批量作业消息。
func (runner *LogBatchRunner) Handle(ctx context.Context, message job.Message) error {
	if message.Kind != LogBatchKind {
		return fmt.Errorf("unsupported log batch message kind %q", message.Kind)
	}
	if message.ResourceID < 1 {
		return fmt.Errorf("log batch job id is required")
	}
	return runner.run(ctx, message.ResourceID)
}

// ScheduleLogBatch 向采集队列投递一条批量作业消息（创建作业与续跑都走这里）。
func ScheduleLogBatch(ctx context.Context, publisher logBatchPublisher, jobID int64) error {
	return publisher.PublishLogCollect(ctx, job.Message{
		SchemaVersion: job.SchemaVersion,
		ExecutionID:   uuid.NewString(),
		Kind:          LogBatchKind,
		ResourceID:    jobID,
		TriggeredAt:   time.Now().UTC(),
	})
}

// run 执行一个批量作业的一个分片。
//
// 返回 nil 表示本条消息"已完成它的分片"——要么整批跑完并落了终态，要么时间预算到点、
// 续跑消息已投出。返回非 nil 会让消息进死信队列，所以只在确实无法推进时报错。
func (runner *LogBatchRunner) run(ctx context.Context, jobID int64) error {
	// 同一作业的分片串行（见 LogBatchRunner.locks 的说明）。
	lock := runner.jobLock(jobID)
	lock.Lock()
	defer lock.Unlock()

	queries := generated.New(runner.handler.db)
	now := time.Now().UTC()
	// 先读状态，据此区分两种进入方式（读在这里是安全的：同一个作业的分片在进程内串行执行，
	// 见 LogBatchRunner.locks）：
	//   - pending：首次执行（或失联对账重投后的重来）→ 用条件更新认领，抢不到就 ack；
	//   - running：本作业上一个分片投出的续跑消息 → 继续推进（这一条不能靠 UPDATE 的行数判断，
	//     MySQL 的 UPDATE 返回"改变的行数"，把 running 再写成 running 会得到 0 行）；
	//   - 终态：消息重投 → 直接 ack。
	overview, err := queries.GetLogBatchJob(ctx, jobID)
	if err != nil {
		return err
	}
	switch overview.Status {
	case LogBatchStatusSuccess, LogBatchStatusPartial, LogBatchStatusFailed:
		return nil
	case LogBatchStatusPending:
		claimed, claimErr := queries.ClaimLogBatchJob(ctx, generated.ClaimLogBatchJobParams{
			StartedAt: sql.NullTime{Time: now, Valid: true}, Message: "", UpdateTime: now, ID: jobID,
		})
		if claimErr != nil {
			return claimErr
		}
		if claimed == 0 {
			// 被别的分片抢先认领了。ack 掉本条。
			return nil
		}
	}
	concurrency := runner.concurrency(overview.Action)
	if concurrency < 1 {
		concurrency = 1
	}

	// 心跳：执行期间持续刷新头表 update_time，让失联对账能区分
	// "作业还在跑，只是这一台很慢"与"执行者已经没了"。
	heartbeatContext, stopHeartbeat := context.WithCancel(context.Background())
	var heartbeat sync.WaitGroup
	heartbeat.Add(1)
	go func() {
		defer heartbeat.Done()
		ticker := time.NewTicker(logBatchHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatContext.Done():
				return
			case <-ticker.C:
				_ = queries.UpdateLogBatchJobHeartbeat(heartbeatContext, generated.UpdateLogBatchJobHeartbeatParams{
					Message: "", UpdateTime: time.Now().UTC(), ID: jobID,
				})
			}
		}
	}()
	defer func() {
		stopHeartbeat()
		heartbeat.Wait()
	}()

	deadline := time.Now().Add(runner.options.Budget)
	chunkSize := int32(concurrency * logBatchItemBatchFactor)
	for {
		items, listErr := queries.ListPendingLogBatchJobItems(ctx, generated.ListPendingLogBatchJobItemsParams{
			BatchJobID: jobID, Limit: chunkSize,
		})
		if listErr != nil {
			return listErr
		}
		if len(items) == 0 {
			return runner.settle(ctx, queries, jobID)
		}
		runner.processItems(ctx, queries, jobID, overview.Action, concurrency, items)
		if time.Now().After(deadline) {
			break
		}
	}
	// 时间预算到点且还有待处理项：把剩余工作交给下一条消息（本条 ack 掉，
	// 避免一条消息挂满 consumer_timeout 被服务端强断）。
	if err = ScheduleLogBatch(ctx, runner.publisher, jobID); err != nil {
		// 投递失败就让它进死信队列：失联对账会按"running 且心跳过期"重投。
		return fmt.Errorf("schedule next log batch chunk: %w", err)
	}
	return nil
}

// concurrency 取该动作的并发上限（§9 第 9 条：安装与下发分开配置）。
func (runner *LogBatchRunner) concurrency(action string) int {
	switch action {
	case LogBatchActionApply:
		return runner.options.ApplyConcurrency
	case LogBatchActionInstall:
		return runner.options.InstallConcurrency
	default:
		return 1
	}
}

// processItems 以该动作的并发上限处理一批待处理项（同批内并发、批与批之间串行）。
func (runner *LogBatchRunner) processItems(ctx context.Context, queries *generated.Queries, jobID int64, action string, concurrency int, items []generated.ListPendingLogBatchJobItemsRow) {
	permits := make(chan struct{}, concurrency)
	var running sync.WaitGroup
	for index := range items {
		item := items[index]
		running.Add(1)
		permits <- struct{}{}
		go func() {
			defer running.Done()
			defer func() { <-permits }()
			runner.runItem(ctx, queries, jobID, action, item)
		}()
	}
	running.Wait()
}

// runItem 执行单台主机的一项，并把结果落库。
func (runner *LogBatchRunner) runItem(ctx context.Context, queries *generated.Queries, jobID int64, action string, item generated.ListPendingLogBatchJobItemsRow) {
	startedAt := time.Now().UTC()
	if err := queries.MarkLogBatchJobItemRunning(ctx, generated.MarkLogBatchJobItemRunningParams{
		StartedAt: sql.NullTime{Time: startedAt, Valid: true}, UpdateTime: startedAt, ID: item.ID,
	}); err != nil {
		slog.Error("mark log batch item running", "item_id", item.ID, "error", err)
		return
	}
	executionErr := runner.execute(ctx, action, item)
	finishedAt := time.Now().UTC()
	status, message := logBatchItemSuccess, ""
	if executionErr != nil {
		status, message = logBatchItemFailed, executionErr.Error()
	}
	if err := queries.FinishLogBatchJobItem(ctx, generated.FinishLogBatchJobItemParams{
		Status: status, Message: message, FinishedAt: sql.NullTime{Time: finishedAt, Valid: true}, UpdateTime: finishedAt, ID: item.ID,
	}); err != nil {
		slog.Error("finish log batch item", "item_id", item.ID, "error", err)
	}
	// 计数从 item 表重算：并发下应用层累加会漂移。
	if err := queries.RefreshLogBatchJobProgress(ctx, generated.RefreshLogBatchJobProgressParams{
		UpdateTime: finishedAt, ID: jobID,
	}); err != nil {
		slog.Error("refresh log batch progress", "job_id", jobID, "error", err)
	}
}

// execute 执行单台主机的动作。返回 nil 表示这一台成功。
func (runner *LogBatchRunner) execute(ctx context.Context, action string, item generated.ListPendingLogBatchJobItemsRow) error {
	// 每台重新读一次目标行：批量作业可能跑十几分钟，期间目标的状态（已装/禁用/主机名）会变，
	// 用创建作业时的快照会让下发判据失真。这是 worker 侧的逐台查询，不影响请求路径。
	row, err := loadLogTarget(ctx, runner.handler.db, item.TargetID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("日志采集目标已不存在")
	}
	if err != nil {
		return err
	}
	switch action {
	case LogBatchActionApply:
		// 指纹一致时 applyLogTargetConfigRow 返回 skipped，同样是"这一台已经是最新"。
		if _, err = runner.handler.applyLogTargetConfigRow(ctx, row); err != nil {
			return err
		}
		return nil
	case LogBatchActionInstall:
		dispatch, prepareErr := runner.handler.prepareLogTargetInstall(ctx, row)
		if prepareErr != nil {
			return prepareErr
		}
		finalStatus, message := runner.handler.runLogTargetInstall(ctx, dispatch)
		if finalStatus != dispatch.desiredStatus {
			if message == "" {
				message = "Filebeat 任务执行失败"
			}
			return fmt.Errorf("%s", message)
		}
		return nil
	default:
		return fmt.Errorf("不支持的批量动作 %q", action)
	}
}

// settle 判断"这一批是否真的跑完了"：只有每个 item 都有了结果才落终态。
//
// 没有 pending 项但仍有 running 项，只有一个来源：上一个分片的执行者中途消失（进程被杀），
// 那些项既不是 pending（挑不到）也不是 success/failed（计数不满）。此时**不收尾**，直接让出，
// 由失联对账把作业回落为 pending、把这些项落回 pending 后重新入队——既不会把半途而废的一批
// 报成完成，也不会在这里空转重投（那会变成热循环）。
func (runner *LogBatchRunner) settle(ctx context.Context, queries *generated.Queries, jobID int64) error {
	overview, err := queries.GetLogBatchJob(ctx, jobID)
	if err != nil {
		return err
	}
	if overview.SuccessCount+overview.FailedCount < overview.TotalCount {
		slog.Warn("log batch job has unfinished items, waiting for the stale reaper",
			"job_id", jobID, "total", overview.TotalCount,
			"success", overview.SuccessCount, "failed", overview.FailedCount)
		return nil
	}
	return runner.finish(ctx, queries, overview)
}

// finish 计算终态并收尾。
func (runner *LogBatchRunner) finish(ctx context.Context, queries *generated.Queries, overview generated.GetLogBatchJobRow) error {
	jobID := overview.ID
	status := LogBatchStatusSuccess
	message := fmt.Sprintf("全部 %d 台处理完成", overview.TotalCount)
	switch {
	case overview.FailedCount > 0 && overview.SuccessCount > 0:
		status = LogBatchStatusPartial
		message = fmt.Sprintf("成功 %d 台，失败 %d 台", overview.SuccessCount, overview.FailedCount)
	case overview.FailedCount > 0:
		status = LogBatchStatusFailed
		message = fmt.Sprintf("全部 %d 台处理失败", overview.FailedCount)
	}
	now := time.Now().UTC()
	_, err := queries.FinishLogBatchJob(ctx, generated.FinishLogBatchJobParams{
		Status: status, Message: message, FinishedAt: sql.NullTime{Time: now, Valid: true},
		UpdateTime: now, ID: jobID,
	})
	return err
}

// StartReaper 启动失联批量作业对账：执行进程消失后作业会永久停在 running，
// 这里按"running 且头表心跳过期"判定停摆，回落为 pending 并重投消息。
//
// 单实例部署（api 进程内唯一 goroutine），随进程退出终止。
func (runner *LogBatchRunner) StartReaper(ctx context.Context) {
	if runner.publisher == nil {
		slog.Warn("log batch reaper disabled: no queue publisher")
		return
	}
	go func() {
		ticker := time.NewTicker(logBatchReapInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runner.reap(ctx)
			}
		}
	}()
}

func (runner *LogBatchRunner) reap(ctx context.Context) {
	queries := generated.New(runner.handler.db)
	staleBefore := time.Now().UTC().Add(-logBatchStaleAfter)
	rows, err := queries.ListStaleLogBatchJobs(ctx, staleBefore)
	if err != nil {
		slog.Error("list stale log batch jobs", "error", err)
		return
	}
	for _, row := range rows {
		now := time.Now().UTC()
		requeued, err := queries.RequeueLogBatchJob(ctx, generated.RequeueLogBatchJobParams{UpdateTime: now, ID: row.ID})
		if err != nil || requeued == 0 {
			continue
		}
		if _, err = queries.ResetStaleLogBatchJobItems(ctx, generated.ResetStaleLogBatchJobItemsParams{
			UpdateTime: now, BatchJobID: row.ID,
		}); err != nil {
			slog.Error("reset stale log batch items", "job_id", row.ID, "error", err)
			continue
		}
		if err = ScheduleLogBatch(ctx, runner.publisher, row.ID); err != nil {
			slog.Error("reschedule stale log batch job", "job_id", row.ID, "error", err)
			continue
		}
		slog.Warn("requeued stale log batch job", "job_id", row.ID, "action", row.Action)
	}
}

// ---- 接口视图 ----

// logBatchJobDTO 是批量作业的对外视图（计数带 pending 便于前端直接渲染进度）。
func logBatchJobDTO(row generated.GetLogBatchJobRow) map[string]any {
	item := logBatchJobMap(row)
	item["is_running"] = row.Status == LogBatchStatusPending || row.Status == LogBatchStatusRunning
	return item
}

func logBatchJobTaskDTO(row generated.GetActiveLogBatchJobByActionRow) map[string]any {
	return map[string]any{
		"id": row.ID, "action": row.Action, "status": row.Status,
		"total_count": row.TotalCount, "success_count": row.SuccessCount,
		"failed_count": row.FailedCount, "pending_count": pendingCount(row.TotalCount, row.SuccessCount, row.FailedCount),
		"concurrency": row.Concurrency, "message": row.Message,
		"requested_username": row.RequestedUsername,
		"started_at":         nullTimeValue(row.StartedAt), "finished_at": nullTimeValue(row.FinishedAt),
		"create_time": row.CreateTime, "update_time": row.UpdateTime,
		"is_running": true,
	}
}

func logBatchJobMap(row generated.GetLogBatchJobRow) map[string]any {
	return map[string]any{
		"id": row.ID, "action": row.Action, "status": row.Status,
		"total_count": row.TotalCount, "success_count": row.SuccessCount,
		"failed_count": row.FailedCount, "pending_count": pendingCount(row.TotalCount, row.SuccessCount, row.FailedCount),
		"concurrency": row.Concurrency, "message": row.Message,
		"requested_username": row.RequestedUsername,
		"started_at":         nullTimeValue(row.StartedAt), "finished_at": nullTimeValue(row.FinishedAt),
		"create_time": row.CreateTime, "update_time": row.UpdateTime,
	}
}

func logBatchItemDTO(row generated.ListLogBatchJobItemsRow) map[string]any {
	return map[string]any{
		"id": row.ID, "target_id": row.TargetID, "host_id": row.HostID,
		"host_name": row.HostName, "host_ip": row.HostIp,
		"status": row.Status, "message": row.Message,
		"started_at": nullTimeValue(row.StartedAt), "finished_at": nullTimeValue(row.FinishedAt),
	}
}

// pendingCount 是"还没出结果的台数"（含正在跑的那些），仅用于展示。
func pendingCount(total, success, failed int32) int32 {
	pending := total - success - failed
	if pending < 0 {
		return 0
	}
	return pending
}

func nullTimeValue(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
