# 定时任务（scheduler）架构

面向「系统管理 → 定时任务」页：一条定时任务 = 库里的一行（编码、cron、启用状态、最近结果）
+ 一个 **Go handler**。本文记录它现在的最终逻辑，特别是 **handler 与任务编码的对应关系**——
"页面上的任务到底会不会执行"取决于它，而不是取决于"启用"开关。

## 进程与数据流

| 角色 | 启动 | 做什么 |
|---|---|---|
| `autoadmin scheduler` | `./bin/autoadmin scheduler` | 根据数据库持久化计划生成执行请求，在有界执行池内直接执行六类数据库清理，不依赖 RabbitMQ |
| `autoadmin worker` | `./bin/autoadmin worker` | 保留自动化失联作业对账，不再消费定时任务消息；无需 RabbitMQ |
| api 进程 | `./bin/autoadmin api` | 任务列表/详情/启停/编辑；`POST /sys/scheduler/tasks/:id/run_now/` 创建数据库待执行请求，不在 HTTP 请求内清理 |

```text
API 保存计划 / 手动触发
        ↓
数据库计划游标 + 待执行记录
        ↓ 多 scheduler 通过数据库锁认领
有界执行池 → 数据库清理 + 完成记录 + 原有执行日志
```

- **robfig/cron 只计算计划**：统一使用五字段标准表达式，不接受秒字段或 `@every`；
  不再给每个实例注册一套独立的内存计时器。`schedule_timezone` 持久化 IANA 时区，
  统一初始化为 **UTC**，不依赖进程本地时区；时间戳统一 UTC，页面按用户时区转换。
  `nextRunAt` 显式接收表达式、时区及基准时刻；仓储层先判断任务和总开关是否启用，
  再决定是否计算游标，不通过隐含“当前时刻”的包装器计算计划。
- **下次运行时间是持久化游标**：接口读取数据库真实计划，不再根据“现在”算一个未来时刻遮住积压。
  游标逾期代表等待处理，不能据此判断历史记录出错。停用、总开关关闭、未初始化或不支持任务可为空；
  `schedule_timezone` 用于解释计划钟点，前端再按用户时区显示。
- **手动触发也是持久化请求**：接口返回 `status=submitted` 代表已入库，不代表执行成功。
  `is_pending` 包括等待和事务内执行中（运行中标记未单独提交）；
  页面显示“等待执行”，不能据此精确区分排队与正在清理，不能用上一轮的成功结果结束轮询。
  `execution_id`、`execution_status` 描述最近一轮；取消时必须识别 `cancelled`，不误报上一轮成功。
  立即执行的等待、成功、失败、取消和超时提示均包含点击时的任务名称，便于区分不同任务，
  不受后续列表刷新或任务改名影响。
- **扫描与执行分离**：慢清理不占据调度循环；并发有界。多个 scheduler 共享同一数据库，
  依靠数据库认领和事务协调，不能只靠进程内互斥。
  扫描独立每 5 秒运行，两个固定执行 worker 通过无缓冲分发器获取任务，不无限创建 goroutine；
  扫描/读取预算 5 秒，单次清理预算 5 分钟。退出时取消并等待这些循环和执行事务结束。

### 配置、补偿与恢复语义

- 停机期间错过多轮自动计划，恢复时**合并补执行一次**，随后将计划推进至未来；
  不逐轮重放，也不将停机当作用户主动停用。
- 修改执行相关配置或停用时取消尚未开始的旧计划，已运行清理允许完成。
  再次启用从启用时刻计算未来计划，不补停用期间；未变配置不能无故重置调度游标。
- `sys.scheduler.enabled` 控制自动计划；用户对启用任务的显式“立即执行”仍可用。
  关闭自动调度取消自动等待项，不取消手动等待项，也不强杀正在执行的清理。
  专用调度接口及系统参数按 ID/按 key 编辑、重置默认值，统一通过同一事务更新总开关。
  不支持绕过应用直接改数据库开关；直接改值不会执行取消/游标重置。
- 数据库访问、配置校验或清理失败必须明确记录；不能把读取失败当作空任务或成功结果。
- 清理是数据库内操作，成功的清理与执行终态/日志在同一事务提交；
  进程崩溃会回滚未提交的清理，不引入跨系统投递及 outbox。
  恢复路径通过数据库锁协调，不能仅凭内存中的“运行中”标记判断是否可以重跑。
- 任务行和执行记录在整个清理事务中持锁，其他 scheduler 跳过已锁任务，不存在租约到期后
  旧执行器仍在删除的并发窗口。任务编辑会等待该任务执行事务结束；
  总开关变更按顺序锁定全部任务，可能等待运行中的清理并暂时阻塞其他任务配置操作，
  不是无等待的开关。事务失败或进程退出保留 pending 请求，供后续恢复。
- handler 返回错误时回滚清理到保存点，再提交 `failed` 结果和失败日志；
  业务失败不自动重试同一轮，下一自然计划仍可执行。日志/终态/事务提交失败则整体回滚，
  后续周期重新认领，避免清理已经提交而成功记录丢失。
- 本机制只覆盖六类数据库清理；若以后添加外部副作用任务，必须重新设计幂等与恢复，
  不能套用“数据库事务可回滚”的前提。

### 迁移与部署

- MySQL/PostgreSQL 两侧新增持久化调度迁移；必须先执行迁移再启动新代码。
- 迁移 `000053_durable_scheduler` 为任务表增加 `schedule_timezone`，创建
  `scheduler_execution`（UUID 主键、任务外键、`automatic/manual` 来源、
  `pending/succeeded/failed/cancelled` 状态及计划/创建/完成时间），建立等待项和最近执行索引。
  同一任务的生成、取消、认领通过任务行锁串行化，不另加 outbox 或调度版本表。
- 原 `next_run_time` 是不可信的保存时快照，首次切换初始化未来计划，不追补升级前的历史。
  迁移清空旧游标及 `is_running`；后续正常重启不清空可信游标。
- 切换顺序：停止旧 scheduler、暂停手动触发 → 等待旧 worker 完成并排空
  `autoadmin.job.execute` → 停止旧 worker → 执行迁移 → 部署新 API/scheduler/worker。
  禁止新旧 scheduler、旧 MQ worker 与新直接执行器混跑。
- 新代码不再声明、发布或消费旧定时任务队列，不自动删除 RabbitMQ 中已有队列或消息。
  日志采集的 `autoadmin.logcollect.execute` 保持不变，api 仍需要 RabbitMQ；
  scheduler/worker 不再要求 `RABBITMQ_URL`，worker 保留自动化失联作业对账职责。

非当前档位日志流的自动回收属于 `api` 角色的进程内维护循环（启动检查，之后每 10 分钟一次），
不新增 `scheduler_scheduledtask` 任务，也不由 worker 消费。原因是回收必须检查 API 持有的实时
agent 会话与配置下发状态。策略与失败语义见 [日志采集架构](LOG_COLLECTION_ARCHITECTURE.md#非当前档位数据流的自动回收)。

### 回归验证

```bash
cd autoadmin
make derive
make generate
make facade
make vet
make test
make build
# 隔离 MySQL 实例，账号需 CREATE/DROP DATABASE 权限；测试创建并清理专用临时数据库：
# SCHEDULER_SMOKE_DSN='<test-dsn>' make test PKG=./internal/scheduler TESTFLAGS='-run TestDurableSchedulerMySQLSmoke -count=1 -v'

cd ../fronted
npm run test:run -- src/views/sys/scheduler/index.spec.js
npm run build
```

上线验证应在隔离数据库中覆盖：两个 scheduler 同时认领同一轮、清理中断后回滚恢复、
清理与终态同成同败、错过多轮合并一次、修改/停用取消等待项、重新启用不补停用期间。
仅单元测试通过不等于已完成真库并发与故障注入验收；不要在生产数据库直接构造这些故障。

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

保留期一律**读 sys_config 的同名键**；读取失败、缺失或数值非法时明确失败，不以默认保留期
静默执行删除。删除语句都在
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

1. **定时调度跳过**：scheduler 生成执行请求前检查 `IsSupportedTaskCode`，不支持的任务不生成自动执行，
   列表不展示下次计划；
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
