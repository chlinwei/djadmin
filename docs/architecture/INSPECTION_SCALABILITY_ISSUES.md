# 巡检中心规模化待提升项（200~500 台虚拟机场景）

> 面向目标规模：200~500 台受管虚拟机、每组任务 10~30 个检查项。
> 代码基准：Go 版 autoadmin（`autoadmin/internal/inspection/`）+ dj_agent（`dj_agent/internal/executor/`）。
> 本文档是问题清单与改造方向，供后续逐项解决；解决后请同步更新对应章节状态。

## 优先级总览

| 优先级 | 问题 | 状态 |
|---|---|---|
| P0 | 定时调度未实现 | ✅ 已解决（2026-09，API 进程内 dispatcher，见下文） |
| P0 | 结果逐条落库、无批量无事务 | ✅ 已解决（100 行/批 multi-value INSERT） |
| P0 | 无全局并发控制 | ✅ 已解决（全局 200 槽信号量 + 内存取消标记） |
| P1 | 单目标检查项串行执行 | ✅ 关闭（性能优化已做：goss 二进制释放缓存；检查项合并按产品决策不做） |
| P1 | 结果存储冗余且无清理策略 | ✅ 已解决（保留期清理；raw_result 冗余仍待评估） |
| P1 | 详情接口 N+1 且无分页 | ✅ 已解决（N+1 已消除；targets 仍全量返回，分页待评估） |
| P1 | 无告警联动 | 🚫 产品决策不做（巡检是低频核对，告警归 monitor；见第三批改造记录） |
| P2 | 无分批/错峰下发 | ⬜ 待处理（全局信号量已缓解瞬时尖刺） |
| P2 | 离线目标语义不明确 | ✅ 已解决（离线目标置 skipped，不再混入失败） |
| P2 | 取消机制只标记不中断 | ⬜ 待处理（内存取消标记已落地，Agent 侧中断需改协议） |

最终执行语义（调度、并发、取消、落库、清理）详见
`docs/architecture/INSPECTION_ARCHITECTURE.md` 的"执行流程（最终逻辑）"章节。

---

## P0-1 定时调度未实现（`next_run_time` 是死数据）

**现状**：`inspection_task` 表有 `cron_expression` / `next_run_time` 列，保存任务时用 robfig/cron 计算了 next_run_time（`autoadmin/internal/api/handler/inspection/tasks.go:255-259`），但全代码库没有任何轮询 `next_run_time` 或创建 `trigger_type='scheduled'` 执行的代码。`internal/scheduler`（gocron）目前只服务两个日志清理任务（`scheduler/worker.go:14-17`）。当前唯一入口是手动 `POST /inspection/tasks/{id}/run`（`runtime.go:50`）。

**改造方向**：
- 在 `internal/scheduler` 中注册巡检 dispatcher：周期扫描到期的 `inspection_task`（`next_run_time <= now()` 且启用），调用现有 `RunTask`，并更新 next_run_time。
- 注意与手动触发的互斥/合并语义（同一任务上一个执行未结束时是否跳过本次调度）。
- 旧 Django 侧靠 Celery Beat 定时（见 `docs/architecture/DJ_AGENT_ARCHITECTURE.md`），Go 侧实现后需在文档标注语义对齐。

## P0-2 结果落库逐条 INSERT、无事务、无批量

**现状**：
- `inspection_result` 明细在 for 循环里逐条 `db.Exec`（`autoadmin/internal/inspection/runtime.go:340` 附近），每条独立提交；
- target_execution 也是循环单条 INSERT（`runtime.go:219-232`）。

**量化影响**：500 目标 × 20 检查项 = 1 万条结果，逐条提交使落库成为执行尾延迟主体。

**改造方向**：multi-values 批量 INSERT / COPY，按 target 或整批包事务；`inspection_execution` 的 4 条统计查询（`runtime.go:262-269`）也应放入同一事务。

## P0-3 并发控制只有单执行内 100 上限，无全局控制

**现状**：`execute` 用 `chan struct{}` 信号量限流，上限硬编码 100（`runtime.go:243-249`）。多个 execution 同时运行时无总量限制（2 个 500 台任务同时触发 = 2×100 并发）。另外每个目标执行前同步 `SELECT status FROM inspection_execution` 两次（`runtime.go:300,330`），是热点行。

**改造方向**：进程级全局信号量（所有 execution 共享）；取消状态改为内存标记（执行器持有 cancel 标志），消除逐目标状态查询。

## P1-4 单目标检查项串行执行

**现状**：Agent 端一个目标的所有检查项在一次 gRPC 调用 `check_application_baseline` 内 for 循环顺序执行（`dj_agent/internal/executor/application_baseline.go:99-111`），单目标耗时 = 各检查项之和；且 goss 二进制每次执行都做释放/SHA256 校验（`dj_agent/internal/executor/goss.go:59-117`）。

**改造方向**：同 `run_user` 的检查项合并为一个 goss spec 一次进程调用（goss 原生支持一个 spec 跑全部检查），或 Agent 端按检查项并行。goss 二进制释放校验结果可缓存（释放成功后记录状态，仅版本变更时重新校验）。

## P1-5 结果存储冗余且无清理策略

**现状**：target 级 `raw_result` 存全量结果 JSON 副本（`runtime.go:346`），与 `inspection_result` 明细重复，数据量翻倍；`message` 为 longtext；无任何过期清理。

**量化影响**：500 台 × 每天巡检一次，一年明细表达千万行级。

**改造方向**：raw_result 只存摘要（或去掉，明细表已有全量）；加结果/执行记录保留期清理任务（可挂进现有 `internal/scheduler`，与日志清理任务同模式）。

## P1-6 详情接口 N+1 且无分页

**现状**：`GetExecution` 对每个 target 单独查一次 `ListInspectionResultsByTarget`（`autoadmin/internal/api/handler/inspection/executions.go:153-154`），且 targets/results 全量返回不分页。

**量化影响**：500 target × 20 result 的详情请求 = 数 MB 响应 + 上千次查询。

**改造方向**：结果按 target 懒加载分页（详情页先返回 targets 列表，点开单 target 再查明细），或一次 IN 查询按 target 分组。

## P1-7 无告警联动

**现状**：summary 只是单次执行的计数聚合（`runtime.go:273`），inspection 包内无 alert 相关实现；无"连续 N 次 critical"类规则。

**改造方向**：执行结束后按规则评估（如连续 N 次失败、critical 即告警）并接入通知渠道。

## P2-8 无分批/错峰下发

**现状**：一次性对全部目标并发下发，每台 Agent fork goss 进程 + 写临时文件。500 台同时执行会造成全机房瞬时 CPU 尖刺，且统一超时兜底（任务超时 + 45s，`runtime.go:317`）可能导致批量超时。

**改造方向**：任务级配置分批大小与批次间隔（与 P0-3 的全局信号量配合）。

## P2-9 离线目标语义不明确

**现状**：`resolveTargets` 中 `service_once` 只选在线实例（`runtime.go:123`），但 `per_host` 目标离线时最终状态语义需确认——应明确为"未执行/跳过"，而不是混入失败污染统计。

**改造方向**：为 target_execution 增加 skipped/offline 终态，summary 单独计数。

## P2-10 取消机制只标记不中断

**现状**：取消仅写 DB 标记（`executions.go:262` 注释明确说明 Agent 协议无取消帧），靠状态检查丢弃迟到结果。误触发后 Agent 会继续执行完所有 goss 检查。

**改造方向**：gRPC 协议增加取消消息（复用现有长连接下行通道），Agent 收到后杀掉 goss 进程并提前返回。

---

## 建议实施顺序

1. ~~**P0-1 定时调度**~~ ✅ 已完成；
2. ~~**P0-2 批量落库 + P0-3 全局并发控制**~~ ✅ 已完成；
3. ~~**P1-5 清理策略 + P1-6 详情分页**~~ 清理策略 ✅ 已完成；N+1 ✅ 已消除；targets 全量返回 / raw_result 冗余留待评估；
4. P2 各项按需排期（P1-4 已关闭：合并不做，缓存优化已落地）。

## 已完成改造记录（2026-09）

涉及文件：`autoadmin/internal/inspection/{runtime.go,handler.go,executions.go,scheduler.go}`、
`autoadmin/internal/api/router/router.go`。

**P0-1 定时调度**：新增 `internal/inspection/scheduler.go`。API 进程启动时
（`router.go` 创建 inspectionHandler 后）调用 `StartScheduler()` 启动后台循环：
每 30s 扫描 `next_run_time<=UTC_TIMESTAMP(6)` 的启用任务，先以原子 UPDATE 推进
`next_run_time` 完成认领（RowsAffected==1 才触发，防多副本/重叠 tick 重复触发），
再走与手动触发完全相同的 `startRun(..., "scheduled", ...)` 路径。
选择 API 进程而非独立 scheduler 进程的原因：执行必须经过进程内 Agent gateway。
每 24h 同循环内执行保留期清理（见 P1-5）。

**P0-2 批量落库**：`runtime.go` 新增 `insertResults`，`inspection_result` 按 100 行/批
multi-value INSERT 写入；批量写入失败时目标置 failed 并在 error_message 记录原因
（结果缺失不再静默）。500 目标 × 20 检查项从约 1 万次独立提交降到约 100 批。

**P0-3 全局并发 + 内存取消**：
- `handler.go` 的 Handler 增加全局 200 槽信号量 `globalSlots`，所有 execution 的目标
  goroutine 先拿任务级信号量再拿全局槽位，总扇出被钳制；
- 取消改为内存 `sync.Map`（`markCanceled`/`isCanceled`），`CancelExecution` 落库后写
  标记；`executeTarget` 不再两次 `SELECT status`（热点行查询删除）。

**P1-6 消除 N+1**：`executions.go` 新增 `listExecutionResults`，详情接口一次 JOIN 查询
取回该 execution 全部结果并按 target 分组；原每 target 一次查询（500 目标 = 500+ 次）
降为 2 次查询。targets 列表仍全量返回，前端侧如有需要再做分页。

**P1-5 保留期清理**：调度循环每 24h 清理一次 `end_time` 超过保留期且状态非
pending/running 的执行历史（inspection_result → inspection_target_execution →
inspection_execution，先子后父）。保留天数读 `sys_config` key
`inspection.results.retention_days`（缺省 180）。如需调整保留期：
`INSERT INTO sys_config(...) VALUES ... ('inspection.results.retention_days','180','int',...)`
或 UPDATE 已有行。

**对部署/运维的影响**：
- 巡检定时功能随 API 进程生效，无需额外部署 scheduler/worker 角色；
- 多副本部署 API 时调度器每副本都在运行，但原子认领保证任务不重复触发；
- gRPC Agent 协议未变更，Agent 无需升级。

## 第二批改造记录（2026-09 续）

**P2-9 离线目标 skipped 语义**（`autoadmin/internal/inspection/runtime.go` +
`fronted/src/views/inspection/index.vue`）：
- Agent 离线/未注册的目标从"critical error → failed"改为置 `skipped`
  （"未执行"而非"检查失败"），不产生任何 `inspection_result`，error_message 记
  "Agent 离线，未执行巡检"；
- execution 聚合：summary 新增 `skipped` 计数；全部目标 skipped（没有任何目标真正
  执行）时 execution 状态为 `skipped` 而非 failed；有 canceled 且无成功时状态为
  `canceled`；
- 前端 `statusLabel`/`statusColor` 增加 skipped 映射（"已跳过"），目标折叠面板徽标
  skipped 显示为中性色而非红色。注意：目标级 `passed=FALSE`（布尔无第三态），
  前端已按 status 优先判断。

**P1-4 关闭：检查项保持逐项独立执行**：
- 已做的性能优化：goss 二进制路径进程级缓存（`dj_agent/internal/executor/goss.go`，
  首次校验成功后缓存候选路径，执行被拒时整体失效重新校验）；
- **产品决策：检查项合并不做**。逐项独立执行保证每个检查项一条独立结果、severity
  独立判定、单个 goss 套件失败不影响其他检查项——这个结果粒度是巡检报告的基础，
  合并成单个 spec 换来的进程启动开销节省（低频巡检场景下毫秒级）不值得改变语义。

## 第三批改造记录（2026-09 续 2）

**P1-7 产品决策不做：巡检不与告警关联**：
- 曾实现过"巡检失败邮件告警"（inspection 失败时经 monitor 的 SMTP 媒介外发），后按
  产品决策整体移除（`inspection/alert.go`、`monitor/notifier.go` 已删除，
  `media_send.go` 已还原）：巡检是低频核对，内容基本不变，偶尔跑一次确认即可；
  持续性异常发现和通知是监控告警（monitor，Prometheus → alertmanager）的职责，
  两者不混用。
- 巡检失败的可视性由执行记录页承担（状态/summary/目标 error_message）；
  定时任务是否正常调度看 API 日志（`scheduled inspection dispatched/rejected`）。

**P2-10 评估结论（暂不实施）**：gRPC 取消帧需 protoc v5.29.x + protoc-gen-go
v1.36.x 重新生成两侧 pb（仓库无生成脚本，`proto/agent_channel.proto` 手工维护），
当前内存取消标记已保证"迟到响应丢弃 + 目标及时置 canceled"，Agent 端真正中断
goss 进程收益有限，暂缓。

## 相关文档

- 巡检中心架构：`docs/architecture/INSPECTION_ARCHITECTURE.md`
- 设计构想（未实施：组分类/任务多组/业务维度/动态目标）：`docs/architecture/INSPECTION_DESIGN_IDEAS.md`
- Agent 结果 schema：`dj_agent/TASK_RESULT_SCHEMA_V1.md`
