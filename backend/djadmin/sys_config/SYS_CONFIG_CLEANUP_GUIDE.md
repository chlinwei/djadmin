# SysConfig 清理清单（当前仓库）

本文用于区分：
- 已确认可清理（无运行时引用）
- 已有迁移处理（历史遗留）
- 仍在使用（禁止清理）

## 1. 已确认可清理

### 1.1 `sys.scheduler.ssh_connect_timeout`

结论：当前无运行时使用。

证据：
- 后端无任何读取/校验/默认初始化。
- 前端仅历史常量定义，且已删除该常量。

处理建议：
- 若数据库仍有该 key，可直接删除。
- 删除后无需代码迁移，风险低。

## 2. 已有迁移清理（历史遗留）

以下 key 已在迁移中处理，通常不应在新环境出现：

- `sys.assets.collect.auth_failure_threshold`
  - 迁移：`sys_config/migrations/0006_remove_collect_auth_failure_threshold.py`
- `sys.assets.collect.auth_lock_minutes`
  - 迁移：`sys_config/migrations/0007_remove_collect_auth_lock_minutes.py`
- `sys.assets.collect.interval_seconds`
  - 新增迁移：`sys_config/migrations/0008_add_agent_collect_interval_config.py`
  - 删除迁移：`sys_config/migrations/0013_remove_agent_collect_interval_config.py`
- `sys.monitor.prometheus.alert_rules_yaml`
  - 迁移：`monitor/migrations/0025_remove_legacy_alert_rules_yaml_config.py`
- `sys.monitor.prometheus.alert_rules_file_path`
- `sys.monitor.prometheus.reload_timeout_seconds`
- `sys.monitor.prometheus.deploy_skip_promtool`
  - 迁移：`monitor/migrations/0026_remove_alert_rule_models.py`
- `monitor.prometheus.promtool_path`
  - 迁移：`monitor/migrations/0027_remove_promtool_path_config.py`

处理建议：
- 对老库可做一次巡检，若仍残留可补删。

## 3. 兼容迁移中（谨慎处理）

### 3.1 `sys.monitor.prometheus.base_url`

结论：属于旧 key，当前通过兼容逻辑自动迁移到新 key。

说明：
- 新 key：`monitor.prometheus.base_url`
- 兼容点：`monitor/prometheus_api.py` 中 `PROMETHEUS_BASE_URL_LEGACY_KEY`

处理建议：
- 不建议直接删兼容代码，除非确认所有环境都已迁移完成。

## 4. 仍在使用（禁止清理）

以下 key 已确认有运行时用途：

- `sys.assets.hostgroup.max_tree_depth`
- `sys.assets.host.manage.refresh_interval_seconds`
- `sys.assets.host.detail.collect_dispatch_interval_seconds`
- `sys.automation.logs.refresh_interval_seconds`
- `sys.automation.websocket.job_log_poll_interval_seconds`
- `sys.automation.websocket.workflow_run_poll_interval_seconds`
- `sys.menu.max_tree_depth`
- `sys.scheduler.log_retention_days`
- `sys.scheduler.log_max_rows_per_task`
- `sys.scheduler.enabled`
- `sys.automation.logs.retention_days`
- `sys.audit.login_logs.retention_days`
- `sys.audit.operation_logs.retention_days`
- `sys.monitor.install_history.retention_days`
- `sys.monitor.alert_history.retention_days`
- `sys.audit.webssh.retention_days`
- `sys.audit.webssh.max_content_mb`
- `sys.webssh.idle_timeout_minutes`

## 5. 推荐清理顺序

1. 先删除 `sys.scheduler.ssh_connect_timeout` 的数据库残留（若存在）。
2. 对照第 2 节做一次历史遗留巡检并补删。
3. 保留第 3 节兼容 key，待全环境验证完成后再计划下线。
4. 第 4 节 key 不应删除。
