# 监控目标（Exporter）安装/卸载

> 适用范围：`autoadmin/internal/monitor/target_install.go`、`target_actions.go`、`fronted/src/views/monitor/index.vue` 的 exporter 纳管与下发逻辑。

## 入口

- 单目标重试/重新下发：`POST /monitor/targets/:id/retry/`（`RetryTarget`），按 `managed_enabled` 决定 install / uninstall，不校验 `install_status`（覆盖失败重试与历史遗留）。
- 批量纳管并安装：`POST /monitor/targets/`（`InstallNow=true`）先 `createMonitorTargetIfAbsent`，再逐个 `dispatchExporterJob`。
- 两者最终都走 `dispatchExporterJob` → `prepareExporterDispatch` 选包 → 写 `automation_execution_job` + `monitor_target_install_history` → 置 target `pending`。

## 选包与平台匹配

- 平台键来自资产采集：`assets_hostsystem`（os_id/os_id_like/os_version_id）与 `assets_hosthardware.architecture`。
- `normalizeExporterPlatform` 归一为 `family(rhel/ubuntu/debian)`、`major`、`arch(amd64/arm64)`；按 `family+major+arch+rpm/deb` 精确匹配，`tar.gz(any)` 作可移植兜底。
- 前置守卫（无 agent/离线/无包/无 playbook/pending 冲突）以 `guardFailure` 返回，原因写入 `target.install_message`，HTTP 仍返回目标对象。

## 缺架构自动补采（关键）

`architecture` 来自资产采集，缺失时不再直接报「请先执行资产采集」：

1. `prepareExporterDispatch` 发现 `arch == ""` 且目标有关联主机时，调用注入的 `handler.refreshHostInfo(ctx, hostID)`。
2. 该回调由 `router` 用 `assetsHandler.RefreshHostInfoByID` 注入（`monitor.Handler.SetHostInfoRefresher`），避免 monitor 反向依赖 assets 的采集实现。
3. `RefreshHostInfoByID` 同步向主机 agent 下发 `get_host_info` 并经 `persistHostInfo` 落库（system/hardware/runtime 等）。
4. 采集成功后重新 `loadMonitorTargetRow` 读取最新平台字段，继续选包下发。
5. 只有补采失败或仍未拿到架构时，才回退为 failed 并提示「主机架构信息缺失且自动采集未获取到，请确认 agent 在线后重试」。

## 与「运行记录中心」的关系

一次安装/卸载会同时写两张表，它们描述同一次操作：

- `automation_execution_job`（`source='monitor_target'`）：通用自动化作业，逐主机 stdout/stderr 落在 `automation_execution_host_log`，可取消。运行记录中心是**单页无 tab**，只展示自动化任务运行记录（含 `source` 过滤，可筛出监控安装作业）。
- `monitor_target_install_history`：面向纳管目标的安装历史，行内记录 `automation_job_id_snapshot` 指向上面的作业（`automation_execution_job.id`）。它不再单独占运行记录中心的 tab；纳管目标的「查看日志」取该目标最新一条历史的 `automation_job_id_snapshot`，直接跳到运行记录中心对应作业的日志抽屉。

两表不合并；`automation_execution_job.source` 取值 `manual`/`agent_install`/`monitor_target`，作业列表按来源过滤。历史行关联列由派发时写入（`target_install.go`/`log_target_actions.go`），迁移 `000030` 对存量行按 `create_time` + 作业名快照回填（`source` 对存量作业同样回填）。

## 失败语义

- 业务失败原因写 `target.install_status=failed` + `install_message`，派发接口不整体失败。
- playbook 异步执行（goroutine，30 分钟兜底超时），完成后回写 target 与历史的 success/failed；stale pending（>31 分钟）自动置 failed 放行。
- 自动补采是同步调用，会延长单次 retry/批量安装请求耗时（agent 采集超时 15s）。
