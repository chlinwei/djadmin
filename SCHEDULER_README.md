# 定时任务使用说明

## 问题分析

定时任务列表现在有内容，但最近执行时间为空。这是因为 APScheduler 后台进程还没有启动。

## 解决方案

需要在后台启动 APScheduler 的管理命令来让任务定时执行。

### Windows 用户

在项目根目录打开 PowerShell，运行：

```powershell
.\start_scheduler.ps1
```

或者手动运行：

```bash
cd backend/djadmin
python start_scheduler.py
```

### Linux/Mac 用户

```bash
bash start_scheduler.sh
```

或者手动运行：

```bash
cd backend/djadmin
python start_scheduler.py
```

### 日志保留配置（系统参数）

- `sys.scheduler.log_retention_days`：按时间清理，保留最近 N 天日志（`0` 表示不按时间清理）
- `sys.scheduler.log_max_rows_per_task`：按数量清理，每个任务最多保留 N 条日志（`0` 表示不按数量清理）

可在“系统参数”页面直接修改上述 key 对应的 value，无需通过启动脚本传参。


## 验证

1. 启动 APScheduler 后，定时任务将每 15 分钟执行一次
2. 查看任务列表，应该能看到"最近执行时间"被更新
3. 前端点击手动采集按钮也可以随时触发任务

## 配置

- 任务间隔：15 分钟（在 `scheduler_manager.py` 的 `IntervalTrigger(minutes=15)` 中配置）
- 更改间隔时间：编辑 `scheduler_manager.py` 中的分钟数

## 生产部署建议

在生产环境中，建议使用以下方式启动 APScheduler：

1. **使用 systemd 服务**（Linux）
2. **使用 Windows 任务计划程序**
3. **使用 Docker 容器**
4. **使用 Supervisor 等进程管理工具**
