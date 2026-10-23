# 监控架构：Prometheus、Explore 与 Exporter 纳管

本文统一维护监控代理、目标在线守卫和 [Exporter 安装](#target-install)。
告警摄取与通知见 [告警架构](ALERT_HISTORY_ARCHITECTURE.md)，Filebeat 见 [日志采集架构](LOG_COLLECTION_ARCHITECTURE.md)。

## 入口

- 前端 Explore 页（`fronted/src/views/monitor/explore/index.vue`）使用 codemirror-promql 编辑器，自动补全请求 `<后端>/monitor/targets/prometheus/proxy/api/v1/*`（同域代理，避免跨域/鉴权问题）。补全会用到 `/api/v1/label/__name__/values`、`/api/v1/labels`、`/api/v1/series`、`/api/v1/metadata` 等端点；其中 `/labels`、`/series`、`/label/*/values` 以 POST + form 方式提交。
- Go 侧处理器：`autoadmin/internal/monitor/handler.go` 的 `PrometheusProxy`，路由注册为 `Any`（GET/POST 等均可，`router.go`）。仅放行 `/api/v1/*` 前缀；POST 请求的 form 参数会合并进 query（POST 优先覆盖同名 query 参数）后转发给上游。

## 最终逻辑

- Explore 的即时查询结果由前端表格分页，默认每页 10 条，使用公共分页配置，支持切换页码和每页条数；总条数取当前查询结果数量。每次执行有效查询时回到第一页，并保留用户选择的每页条数；翻页不重复请求 Prometheus。范围查询使用图表展示，不展示分页表格。
- `prometheusGet` 统一请求 Prometheus，把响应解为 `prometheusPayload`：`status`、`warnings`、`error` 为定式字段，`data` 用 `json.RawMessage` 保存原始 JSON，不预设形状。
- 原因：Prometheus 各端点的 `data` 形状不一致——`/query`、`/targets`、`/alerts`、`/rules`、`/status/*` 返回对象，`/label/<name>/values`、`/series` 返回数组。早期版本把 `data` 定为 `map[string]any`，导致 Explore 补全（数组形 data）解码失败，前端收到 `prometheus invalid json response: json: cannot unmarshal array...`，补全失效。
- `PrometheusProxy` 将 `payload.Data`（RawMessage）原样透传，格式无关，任意 `/api/v1/*` 端点均可代理。
- 需要 `data` 为对象的内部 handler（targets/alerts/rules/query/status）通过 `payload.dataMap()` 解为 `map[string]any` 使用；数组形 data 会得到空 map，这些 handler 本身不消费数组形端点。
- 非 2xx 或 `status != success` 时，`prometheusGet` 返回错误（优先取上游 `error` 字段）；代理以 HTTP 200 + `{"status":"error", ...}` 返回，保持与 Prometheus 客户端库的容错兼容。

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
- 批量纳管并安装：`POST /monitor/targets/`（`InstallNow=true`）先 `createMonitorTargetIfAbsent`，再逐个 `dispatchExporterJob`。
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
