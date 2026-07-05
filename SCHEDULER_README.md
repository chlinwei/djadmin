w

# 定时任务使用说明（Celery + RabbitMQ）

## 架构说明

当前调度已从 APScheduler 迁移到 Celery：

- **Celery Beat**：按周期分发到期任务
- **Celery Worker**：执行具体任务
- **RabbitMQ**：消息队列 Broker

如果“最近执行时间”一直为空，通常是 Worker 或 Beat 其中一个未运行。
如果任务长期显示 `pending` 且“开始时间”为 `-`，通常是 Worker 未消费（任务只入队未执行）。

## 启动方式

推荐单命令启动（自动拉起 Worker + Beat）：

```bash
cd backend/djadmin
python manage.py runscheduler
```

兼容说明：`python manage.py runsscheduler` 也支持，作为误拼兼容别名，实际等价于 `runscheduler`。

也可用脚本：

- `python start_scheduler.py`
- `backend/start_scheduler.ps1`
- `backend/start_scheduler.sh`

如果你要分别调试两个进程，再使用以下命令：

```bash
python manage.py runceleryworker --loglevel=info --concurrency=2
python manage.py runcelerybeat --loglevel=info
```

## RabbitMQ 配置

默认从 Django settings 读取：

- `CELERY_BROKER_URL`（默认 `amqp://admin:admin123@10.25.66.150:5672//`）
- `CELERY_RESULT_BACKEND`（默认 `rpc://`）

生产环境建议通过环境变量覆盖，不要将密码硬编码在代码中。

## 日志保留配置（系统参数）

- `sys.scheduler.log_retention_days`：按时间清理，保留最近 N 天日志（`0` 表示不按时间清理）
- `sys.scheduler.log_max_rows_per_task`：按数量清理，每个任务最多保留 N 条日志（`0` 表示不按数量清理）
- `sys.scheduler.enabled`：调度总开关（`true` 启用分发，`false` 停止分发）

可在“系统参数”页面直接修改上述 key 对应 value。

## 验证

1. Worker 与 Beat 进程都在运行；
2. 任务列表里“最近执行时间”会周期更新；
3. 前端“立即执行”后，可看到任务日志新增记录。

## 最近行为变更（已生效）

- `/sys/scheduler/tasks/{id}/run-now` 现在会先检查 Worker 在线状态：Worker 不在线时直接返回错误，不再“提交成功但实际不执行”。
- `dispatch_due_tasks` 在 Worker 不可用时，不再继续投递到期任务，
  并会写入失败提示（Worker 不可用，任务未投递），避免静默 pending。

## 常用排查命令

在 `backend/djadmin` 目录执行：

```bash
# 前台看 Worker 日志（建议先用这个确认是否真的在跑）
python manage.py runceleryworker --loglevel=info --concurrency=2

# 另一个终端启动 Beat
python manage.py runcelerybeat --loglevel=info

# 或一条命令同时启动 Worker + Beat
python manage.py runscheduler
```

## 关联服务提醒（WebSSH 文件传输）

调度服务与 transfer 服务是两个独立进程。
若涉及 WebSSH 上传/下载，还需单独启动 transfer：

```bash
cd backend/djadmin
python manage.py runtransfer --host 0.0.0.0 --port 9101
```

说明：`runtransfer` 内部使用 Daphne，并自动设置 `DJANGO_SETTINGS_MODULE=djadmin.transfer_settings`。
