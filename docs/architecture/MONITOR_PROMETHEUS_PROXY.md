# 监控架构：Prometheus、Explore 与 Exporter 纳管

本文统一维护监控代理、目标在线守卫和 [Exporter 安装](#target-install)。
告警摄取与通知见 [告警架构](ALERT_HISTORY_ARCHITECTURE.md)，Filebeat 见 [日志采集架构](LOG_COLLECTION_ARCHITECTURE.md)。

## 监控目标与软件包操作细节

`targets/batch-create/` 按输入主机顺序返回 `results`（`id`、`host_id` 均为输入主机 ID）。
`detail.created`、`detail.target_id` 表示纳管已落库；指定 `install_now` 时，整项成功只表示安装已下发，
不表示主机安装成功。`detail.install_accepted` 标明受理状态，有作业记录时保留 `detail.job_id`；
下发失败不回滚已建目标，可在该目标上重试安装。
`targets/batch-start-service/`、`targets/batch-stop-service/` 则按目标 ID 返回真实执行结果，
保留主机标签、失败原因及 Agent 作业回执。批量接口以 `count`/`success` 计完整成功项，
部分成功返回业务码 `10001`，全部失败返回同项错误码或 `10002`（多种错误码）。
单目标服务操作与批量逐项使用相同业务码：目标不存在为 `404`，Agent 离线为 `400`。
批量删除只在执行删除时目标已停用且安装状态不为 `pending` 的情况下生效；未删到行不计成功。
编辑目标在同一事务中锁定当前行、合并提交的字段再写回；省略字段保留最新值，`remark: null` 清空备注。
未提交任何可写字段时返回 `400`，不刷新更新时间；没有启用的软件包是配置错误（`400`），
读取软件包或保存目标的内部数据库故障返回 `600`，根因仅写服务端日志。
从 Exporter、Filebeat 或安装历史页取消，均在事务中锁定最新安装历史，只允许取消 `pending` 或 `running` 的当前任务；
历史置 `cancelled`，目标安装状态置 `unknown`，不是断言主机执行失败。任一写入失败则回滚两项。
对旧历史或已结束任务返回 `400`。取消的是管理记录，不证明主机进程已停止；
迟到的作业结果不能改写已取消历史或新任务的目标状态，也不触发 Filebeat 安装后的自动配置下发。
Filebeat 不在 Exporter 列表中维护，去「日志管理 → 日志采集」。
软件仓库中的 Exporter 与 Filebeat 软件包下载使用 API 返回的 `download_url`，
通过公共媒体地址解析工具将 `/media/...` 路径绑定到 `VITE_API_BASE_URL` 指定的后端，
未配置时使用当前主机的 `9000` 端口，而不是前端开发端口。完整 HTTP(S) 下载地址保持原样；
未同步或没有下载地址的软件包不提供可点击的下载链接。查看用户保留软件下载能力，不要求管理或执行权限。
软件仓库 tab 的 Exporter、Filebeat 列表不展示内部 ID 列；ID 仅用于表格行标识和操作请求，
不影响类型筛选、分页、编辑、上传、同步、下载及删除。
软件包与通知策略的批删按 ID 独立处理；仅实际删除的记录计入 `count`，
查询后被其他请求删除的项同样返回 404，不能据前置查询记作成功。
软件包状态采用统一的「启用 / 停用」文字开关，`monitor:update` 可在列表直接切换。
仅更新包的可用状态与更新时间，不改包文件、平台、模板和运行参数，也不触发目标主机安装/卸载；
安装目标的执行型启停仍要求 `monitor:execute`。查看用户只读展示状态。


## 入口

- 前端 Explore 页（`fronted/src/views/monitor/explore/index.vue`）使用 codemirror-promql 编辑器，自动补全请求 `<后端>/monitor/targets/prometheus/proxy/api/v1/*`（同域代理，避免跨域/鉴权问题）。补全会用到 `/api/v1/label/__name__/values`、`/api/v1/labels`、`/api/v1/series`、`/api/v1/metadata` 等端点；SDK 显式配置 `httpMethod: 'GET'`，所有筛选条件放在查询参数中。
- Go 侧处理器：`autoadmin/internal/monitor/handler.go` 的 `PrometheusProxy`，只注册 GET 路由，不接收 POST 查询。仅放行 `/api/v1/*` 前缀，原样保留包括多个 `match[]` 在内的查询参数，以 GET 转发给上游。

## 最终逻辑

- Explore 的即时查询结果由前端表格分页，默认每页 10 条，使用公共分页配置，支持切换页码和每页条数；总条数取当前查询结果数量。每次执行有效查询时回到第一页，并保留用户选择的每页条数；翻页不重复请求 Prometheus。范围查询使用图表展示，不展示分页表格。
- `prometheusGet` 统一请求 Prometheus，把响应解为 `prometheusPayload`：`status`、`warnings`、`error` 为定式字段，`data` 用 `json.RawMessage` 保存原始 JSON，不预设形状。
- 原因：Prometheus 各端点的 `data` 形状不一致——`/query`、`/targets`、`/alerts`、`/rules`、`/status/*` 返回对象，`/label/<name>/values`、`/series` 返回数组。早期版本把 `data` 定为 `map[string]any`，导致 Explore 补全（数组形 data）解码失败，前端收到 `prometheus invalid json response: json: cannot unmarshal array...`，补全失效。
- `PrometheusProxy` 将 `payload.Data`（RawMessage）原样透传，格式无关，任意 `/api/v1/*` 端点均可代理。
- 需要 `data` 为对象的内部 handler（targets/alerts/rules/query/status）通过 `payload.dataMap()` 解为 `map[string]any` 使用；数组形 data 会得到空 map，这些 handler 本身不消费数组形端点。
- 非 2xx 或 `status != success` 时，`prometheusGet` 返回错误（优先取上游 `error` 字段）；代理以 HTTP 200 + `{"status":"error", ...}` 返回，保持与 Prometheus 客户端库的容错兼容。
- 上述例外只限原生代理；管理端的概览、目标、告警、规则、状态及 PromQL 查询读取失败时返回非 200 **业务码**（非法查询为 400，上游不可用为 502），不会伪装成成功的空列表。

## 双实现对齐

Django 侧已废弃（源码已移出版本库）；监控中心相关功能只以 Go 版 autoadmin 为准。


## 纳管目标的 agent 在线依赖（internal/monitor）

项目无应用层心跳，Agent 在线权威是活跃 gRPC 会话（`Gateway.IsOnline`）。纳管目标页所有"经 agent 下发"的操作必须在派发前校验在线，检查矩阵：

| 操作 | 派发函数 | 在线校验 |
|---|---|---|
| Exporter 安装/卸载（retry、批量纳管 install_now） | `dispatchExporterJob` | ✅ 离线→target 标记 failed + install_message |
| Exporter 启动/停止/查状态（单台/批量） | `dispatchTargetServiceControl` | ✅ 400 `host agent is offline` |
| Filebeat 安装/卸载（retry、批量纳管 install_now） | `dispatchLogTargetInstall` | ✅ 同上语义 |
| Filebeat 启停/查状态/下发配置 | `dispatchLogTargetServiceControl` / `ApplyLogTargetConfig` | ✅ |
| 取消任务 / 删除目标 / PATCH 目标字段 | 仅改库，不触 agent | 不需要在线 |

- Exporter 单台状态查询使用 `GET /monitor/targets/:id/check-service-status/`，只执行 `systemctl status` 并返回结果，不写入业务状态；启动、停止仍为 POST。所有纯 GET 查询不记录为变更类操作审计。
- ⚠️ `BatchCreateTargets`（Exporter 批量纳管）必须实现 `install_now=true` 时立即调用 `dispatchExporterJob`（前端固定传 install_now=true，Django 同语义）；此前曾漏实现导致纳管后目标停在未安装状态，勿回退。
- 列表展示的 `host_agent_online` 来自实时 gRPC 会话（`read_resources.go`），不是落库快照。
- 前端所有操作按钮以 `host_agent_online` 置灰（含 Filebeat 需 `agent_installed`），与后端校验双保险。
- TOCTOU 余留：在线校验与 agent 实际执行间极小窗口内掉线，`gateway.Execute` 会以错误返回并落库失败状态，属可接受语义。

## 纳管目标在线守卫测试（internal/monitor/target_online_guard_test.go）

集成回归用例，依赖 `MYSQL_DSN`（未配置自动跳过）；自建测试主机/目标并在结束时清理：

- `TestBatchCreateTargetsInstallNowOfflineGuard`：**核心回归**——`install_now=true` 必须真的触发派发（此前漏实现）；gateway 为 nil（全部离线）时派发被守卫拦截，target 落 `failed` + 可读 `install_message`，单条结果 ok=false 带原因。
- `TestTargetServiceControlRejectsMissingAgent`：主机无 agent 时服务控制拒绝（业务码 400 + 可读原因），不能当成功下发。


<a id="target-install"></a>

## Exporter 纳管与安装

> 适用范围：`autoadmin/internal/monitor/target_install.go`、`target_actions.go`、`fronted/src/views/monitor/index.vue` 的 exporter 纳管与下发逻辑。

### 入口

- 单目标重试/重新下发：`POST /monitor/targets/:id/retry/`（`RetryTarget`），按 `managed_enabled` 决定 install / uninstall，不校验 `install_status`（覆盖失败重试与历史遗留）。
- 批量纳管并安装：`POST /monitor/targets/`（`InstallNow=true`）按主机创建目标，仅对实际新建的目标调用 `dispatchExporterJob`。
- 两者最终都走 `dispatchExporterJob` → `prepareExporterDispatch` 选包 → 写 `automation_execution_job` + `monitor_target_install_history` → 置 target `pending`。

### 选包与平台匹配

- 平台键来自资产采集：`assets_hostsystem`（os_id/os_id_like/os_version_id）与 `assets_hosthardware.architecture`。
- `normalizeExporterPlatform` 归一为 `family(rhel/ubuntu/debian)`、`major`、`arch(amd64/arm64)`；按 `family+major+arch+rpm/deb` 精确匹配，`tar.gz(any)` 作可移植兜底。
- 前置守卫（无 agent/离线/无包/无 playbook/pending 冲突）以 `guardFailure` 返回，原因写入 `target.install_message`，HTTP 仍返回目标对象。

### 缺架构自动补采（关键）

`architecture` 来自资产采集，缺失时不再直接报「请先执行资产采集」：

1. `prepareExporterDispatch` 发现 `arch == ""` 且目标有关联主机时，调用注入的 `handler.refreshHostInfo(ctx, hostID)`。
2. 该回调由 `router` 用 `assetsHandler.RefreshHostInfoByID` 注入（`monitor.Handler.SetHostInfoRefresher`），避免 monitor 反向依赖 assets 的采集实现。
3. `RefreshHostInfoByID` 同步向主机 agent 下发 `get_host_info` 并经 `persistHostInfo` 落库（system/hardware/runtime 等）。
4. 采集成功后重新 `loadMonitorTargetRow` 读取最新平台字段，继续选包下发。
5. 只有补采失败或仍未拿到架构时，才回退为 failed 并提示「主机架构信息缺失且自动采集未获取到，请确认 agent 在线后重试」。

### 与「运行记录中心」的关系

一次安装/卸载会同时写两张表，它们描述同一次操作：

- `automation_execution_job`（`source='monitor_target'`）：通用自动化作业，逐主机 stdout/stderr 落在 `automation_execution_host_log`，可取消。运行记录中心是**单页无 tab**，只展示自动化任务运行记录（含 `source` 过滤，可筛出监控安装作业）。
- `monitor_target_install_history`：面向纳管目标的安装历史，行内记录 `automation_job_id_snapshot` 指向上面的作业（`automation_execution_job.id`）。它不再单独占运行记录中心的 tab；纳管目标的「查看日志」取该目标最新一条历史的 `automation_job_id_snapshot`，直接跳到运行记录中心对应作业的日志抽屉。

两表不合并；`automation_execution_job.source` 取值 `manual`/`agent_install`/`monitor_target`，作业列表按来源过滤。历史行关联列由派发时写入（`target_install.go`/`log_target_actions.go`），迁移 `000030` 对存量行按 `create_time` + 作业名快照回填（`source` 对存量作业同样回填）。

### 失败语义

- 业务失败原因写 `target.install_status=failed` + `install_message`，派发接口不整体失败。
- playbook 异步执行（goroutine，30 分钟兜底超时），完成后回写 target 与历史的 success/failed；stale pending（>31 分钟）自动置 failed 放行。
- 自动补采是同步调用，会延长单次 retry/批量安装请求耗时（agent 采集超时 15s）。
