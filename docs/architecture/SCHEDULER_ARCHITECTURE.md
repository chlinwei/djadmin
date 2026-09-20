# 定时任务（scheduler）架构

面向「系统管理 → 定时任务」页：一条定时任务 = 库里的一行（编码、cron、启用状态、最近结果）
+ 一个 **Go handler**。本文记录它现在的最终逻辑，特别是 **handler 与任务编码的对应关系**——
"页面上的任务到底会不会执行"取决于它，而不是取决于"启用"开关。

## 进程与数据流

| 角色 | 启动 | 做什么 |
|---|---|---|
| `autoadmin scheduler` | `./bin/autoadmin scheduler` | 读 `scheduler_scheduledtask` 里**启用且已实现**的任务，用 `gocron` 按 `cron_expression` 注册；到点向队列发布一条 `scheduled_task` 消息（`internal/scheduler/manager.go`） |
| `autoadmin worker` | `./bin/autoadmin worker` | 消费 `scheduled_task`：原子认领（`Claim`，`is_running` 置位）→ 取任务 → 按 `code` 找 handler → 执行 → 写 `scheduler_scheduledtasklog` 与 `last_status/last_message`（`internal/scheduler/worker.go`） |
| api 进程 | `./bin/autoadmin api` | 任务列表/详情/启停/编辑/立即执行（`POST /sys/scheduler/tasks/:id/run_now/` 走的也是"发布一条消息"这条路） |

- **cron 与时区**（2026-09-20 改）：`cron_expression` 按 **`ScheduleLocation`** 解释——
  它是**唯一一处**时区定义：scheduler 进程建 gocron 时 `WithLocation(ScheduleLocation)`，
  应用层算"下次运行时间"时也用同一个值。执行与展示必须同源，否则会出现"界面显示 17:00、
  实际 9:00 触发"（两边都自认有道理，用户只能猜）。要改成固定时区（例如统一按 Asia/Shanghai）
  只改这一个变量。
- **「下次运行时间」是实时算的，不读库里的快照**：`scheduler_scheduledtask.next_run_time` 只在
  保存/启停时写过一次，真正的触发由进程内 gocron 自己算，所以那列**不会推进**、时间一久就成
  过去的时间（现场："下次运行时间比当前时间早"）。现在列表/详情的 `next_run_time` 由
  `displayNextRunTime` 现场计算（`internal/scheduler/service.go`，在 `withTaskSupport` 里统一覆盖），
  库里那列退化为"保存时的快照"，接口不采信。
- **空值的三种语义**（界面都给了悬停说明，不是"数据没取到"）：任务**停用**、**实现未迁移**
  （调度器根本不注册它，见下一节）、**没有可用的 cron 表达式**。所以"有下次运行时间"本身就等于
  "这个任务真的会被触发"——这比只看 `enabled` 开关更准。DTO 同时带 `schedule_timezone`，
  界面据此说明 cron 的钟点是哪个时区的钟点（显示的时间是换算到用户时区后的同一时刻）。
- **"立即执行"与定时触发是同一条路径**：都是发布消息、都由 worker 执行、都写执行日志——
  所以"手动能跑、定时不跑"这类分歧不存在。
- **认领防重复**：worker 先 `Claim`（只在未运行时成功），多副本或消息重投不会重复执行。

## 任务编码 ↔ handler（**唯一的"会不会执行"判据**）

`internal/scheduler/worker.go` 的 `supportedTaskCodes` 是唯一事实来源，现在有六项：

| 编码 | 名称 | handler | 保留期配置键（默认） | 清理对象与语义 |
|---|---|---|---|---|
| `cleanup_login_audit_logs` | 登录日志清理 | `cleanupLoginAudits` | `sys.audit.login_logs.retention_days`（90） | `audit_login_log`，按 `login_time` |
| `cleanup_operation_audit_logs` | 操作日志清理 | `cleanupOperationAudits` | `sys.audit.operation_logs.retention_days`（90） | `audit_operation_log`，按 `created_at` |
| `cleanup_webssh_session_logs` | WebSSH 会话日志清理 | `cleanupWebSSHSessionLogs` | `sys.audit.webssh.retention_days`（30） | `assets_webssh_session_log`，按 `start_time`（`end_time` 为空的残留也要能清掉） |
| `cleanup_ansible_execution_logs` | 自动化执行日志清理 | `cleanupAutomationExecutionLogs` | `sys.automation.logs.retention_days`（30） | 主机明细 → 字节块 → 作业行，**先子后父**；只清 `end_time` 已过保留期的作业（运行中的绝不碰） |
| `cleanup_monitor_install_histories` | 监控安装历史清理 | `cleanupMonitorInstallHistories` | `sys.monitor.install_history.retention_days`（180） | `monitor_target_install_history`，**每个纳管目标至少保留最新一条**（清完还要能回答"这台机器最近一次装/卸是什么结果"） |
| `cleanup_alert_histories` | 历史告警清理 | `cleanupAlertHistories` | `sys.monitor.alert_history.retention_days`（90） | 投递记录 → 通知事件 → 告警行，**先子后父**；只清 `state='resolved'`（仍在 firing 的是"当前状态"）；年龄取 `COALESCE(resolved_at, started_at)` |

保留期一律**读 sys_config 的同名键**（与 Django 时代同一个键，升级后行为不变），读不出来时退回默认值：
"读不到就不清理"会让表一直涨、"读不到就全清"会丢数据，两者都是错的。删除语句都在
`db/queries/mysql/scheduler.sql`，有子表的**子表先删**——这些外键都没有级联，顺序反了会直接撞外键
（在真库上实测到过 1451）。

**分两类处理（2026-09-19 逐条对着 sys_config 的配置说明核对后定的）**：能实现的实现，
平台别处已经持续在做的一律**退役任务行**——绝不给同一张表补第二套清理（两套迟早互相打架）。
已退役的三条（迁移 `000042_retire_internal_scheduler_tasks`：删行 + 删它们的执行日志，`down` 可还原行）：

| 退役的任务 | 谁在做 |
|---|---|
| `cleanup_inspection_executions` 巡检执行记录清理 | 巡检自带的清理：`internal/inspection/scheduler.go` 的 `cleanupExpiredExecutions`，每 24h 跑一次，保留期 `sys.inspection.executions.retention_days`（顺带修掉一个键名不匹配：Go 原先读 `inspection.results.retention_days`，那个键根本不存在 → 一直按 180 天兜底） |
| `reconcile_prometheus_alert_history` 历史告警对账 | 告警侧失联对账：`internal/monitor/alert_notification.go` 的 `reconcileStaleAlertsLoop`，**进程内 ticker**（firing 且 `last_seen_at` 超 10 分钟即判恢复），比"每 5 分钟一条定时任务"更及时、也不依赖队列 |
| `cleanup_orphan_temp_credentials` WebSSH 临时凭证清理 | 这套机制在 Go 里已经不存在：没有任何 Go 代码写 `assets_webssh_temp_credential`，`session_pk` 指的 Django session 表也没了；迁移里顺手清掉历史残留（未被主机绑定的凭证一并删） |

> 曾经还有个 `cleanup_alert_histories` 被 `ListScheduledTasks` 里的 `WHERE code <> 'cleanup_alert_histories'`
> **硬编码藏起来**，后果是"历史告警从来没被清理过"这件事没人看得见（2026-09-19 才查出来）。那条过滤已删：
> 要隐藏一个任务就**退役它**（删行 + 写文档），不要在查询里做静默排除。

若再出现没有实现的编码，三处行为一致，不会给人"看着在用"的错觉：

1. **定时调度跳过**：`scheduler` 进程注册任务前检查 `IsSupportedTaskCode`，不匹配的直接不注册
   （`internal/app/app.go`）——不会执行、也不会产生失败日志；
2. **立即执行拒绝**：`RunNow` 返回 400，消息按任务构造（见下）；
3. **列表标注**：任务列表/详情返回 `supported` 与 `support_note`（`withTaskSupport`），
   页面据此把「立即执行」**置灰**、在行内标「未迁移」、页头给一条警告。

**为什么要把这件事显式化**（2026-09-19 现场）：用户点了历史任务的「立即执行」，得到的是
`任务提交失败: 任务 handler 尚未迁移到 Go`——不知道是谁的问题、要不要等、有没有替代；
而列表里这些任务的「最近结果」还显示着 **成功**（Django 时代留下的最后一条日志），
看上去一切正常。所以：`supported=false` 时必须"点不动 + 说清为什么 + 给出替代"，
`UnsupportedTaskNote(name, code)` 负责那句话（含"当前已实现的任务"清单）。

## 新增一条定时任务要做什么

1. 在 `internal/scheduler/worker.go` 实现 `func (worker *Worker) xxx(ctx) (string, error)`
   （返回的字符串会写进执行日志的"输出"）；
2. 注册进 `worker.handlers`（`NewWorker`），**并**登记进 `supportedTaskCodes`（含给用户看的名字）。
   两张表必须一一对应，`TestWorkerHandlersMatchSupportedCodes` 盯着这件事：
   只登记不实现 = 页面放行一个必然失败的执行；只实现不登记 = 明明能跑却被置灰；
3. 库里补任务行（`scheduler_scheduledtask`：code / cron / enabled），或由迁移插入；
4. 需要保留期之类的可配置项时，用 `repository.RetentionDays(ctx, 配置键, 默认值)` 取
   `sys_configs` 的值，不要在代码里写死天数。

任务**不做**的：Go 侧不再有 Celery/beat（历史文档已归档 `docs/archive/SCHEDULER_CELERY_README.md`）；
巡检的定时调度是另一套（跑在 api 进程内，见 INSPECTION_ARCHITECTURE.md 的定时调度一节），
与本页的通用定时任务无关。
