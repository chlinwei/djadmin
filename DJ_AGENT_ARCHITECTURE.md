# dj-agent 通信架构文档

## 1. 整体架构

```
dj-agent (Go)
  ├── 心跳 / 快照  →  RabbitMQ agent.reports  →  runagentconsumer  →  MySQL
  ├── 任务接收     ←  RabbitMQ agent.tasks    ←  Django 后端下发
  ├── 终端 I/O    ↔  RabbitMQ agent.term.<agent_id>  ↔  Django WebSocket Consumer
  └── 配置拉取    →  HTTP GET /api/agent/configs/by-key/...  →  Django 后端
```

---

## 2. 消息队列

| 队列 | 方向 | 用途 |
|---|---|---|
| `agent.reports` | agent → backend | 心跳、主机快照、任务结果、任务事件 |
| `agent.tasks` | backend → agent | 下发任务（采集、自动化、配置更新等） |
| `agent.term.<agent_id>` | 双向 | WebSSH 终端 I/O |

---

## 3. agent.reports 消息类型

所有消息由 `runagentconsumer` Django 管理命令消费：

```
python manage.py runagentconsumer
```

### 3.1 heartbeat（每 10 秒）

```json
{
  "type": "heartbeat",
  "agent_id": "localhost",
  "ts": "2026-07-19T01:47:40Z"
}
```

**处理逻辑**：更新 `Host.agent_online = True`、`Host.agent_online_time = now()`

**在线判断**：`(now - agent_online_time) <= 30 秒`（3× 心跳间隔，容忍 2 次丢包）

### 3.2 host_snapshot（定时，默认 40 秒）

```json
{
  "type": "host_snapshot",
  "agent_id": "localhost",
  "action": "get_host_info",
  "status": "success",
  "result_data": {
    "agent_version": "dj_agent:v1",
    "hostname": "node-01",
    "os": "linux",
    "arch": "amd64",
    "go_version": "go1.26.5",
    "cpu_count": 8,
    "local_ipv4": ["10.0.0.12"],
    "pid": 12345
  },
  "error": "",
  "ts": "2026-07-19T01:47:40Z"
}
```

**处理逻辑**：
- 成功时：更新 `Host.collect_time`、`collect_status=success`、`host_snapshot`（原始 JSON）
- 失败时：仅更新 `collect_status=failed`、`collect_message`，**不覆盖** `collect_time`
- 写入 `HostSystem`：`os_type`、`hostname`、`agent_version`、`collector_source='agent'`
- 写入 `HostHardware`：`cpu_cores`、`architecture`

### 3.3 job_result

```json
{
  "type": "job_result",
  "job_id": "xxx",
  "agent_id": "localhost",
  "status": "success",
  "exit_code": 0,
  "stdout": "...",
  "stderr": "",
  "result_data": {}
}
```

**处理逻辑**：更新 `AgentJob` 的状态、输出、完成时间

### 3.4 job_event

**处理逻辑**：记录 `AgentJobEvent`（任务执行进度事件）

### 3.5 term_event

**消费方**：Django WebSocket Consumer（`assets/consumers.py`），直接从 `agent.term.<agent_id>` 队列消费，不经过 `runagentconsumer`。

---

## 4. 主机采集间隔动态更新

修改系统配置 `sys.assets.collect.interval_seconds` 时，**自动触发下发，无需重启 agent**。

```
用户在前端修改 sys.assets.collect.interval_seconds
    ↓
SysConfigViewSet.partial_update() （sys_config/views.py）
    ↓
_try_dispatch_agent_collect_interval_update()
    ↓
dispatch_host_report_interval_update()  ←  广播给全部在线 agent
    ↓  为每个 agent_id 创建 AgentJob（action=set_host_report_interval）
RabbitMQ agent.tasks 队列
    ↓
dj-agent 消费任务
    ↓
hostReportIntervalUpdateChan  ←  通道通知主循环
    ↓
hostReportTicker.Reset(newInterval)  ←  动态更新，立即生效
```

**约束**：`30s ≤ interval ≤ 12h`（前后端均校验）

**初始化**：agent 启动时通过 HTTP GET `/api/agent/configs/by-key/sys.assets.collect.interval_seconds` 拉取一次初始值；若请求失败，退回到本地配置 `DJ_AGENT_HOST_REPORT_INTERVAL`（默认 40s）。

---

## 5. 后端进程说明

| 进程 | 启动命令 | 职责 |
|---|---|---|
| Django 主进程 | `python manage.py runserver 0.0.0.0:9000` | REST API + WebSocket |
| agent consumer | `python manage.py runagentconsumer` | 消费 agent.reports 队列 |
| transfer 服务 | `python manage.py runtransfer` | SSH 文件传输 |
| Celery Worker | `python manage.py runceleryworker` | 定时任务执行 |
| Celery Beat | `python manage.py runcelerybeat` | 定时任务调度 |

---

## 6. agent 在线状态判断

在线判断发生在两个位置，逻辑一致（阈值均为 30 秒）：

| 位置 | 触发时机 | 作用 |
|---|---|---|
| `HostManage.get_queryset()` | 每次主机列表 API 请求 | 批量更新超时 agent 的 `agent_online=False` |
| `assets/consumers.py` | WebSSH 建立连接前 | 拒绝离线 agent 的终端请求 |
| `HostSerializer.get_system()` | 序列化每条主机记录 | 读取 DB 中的 `agent_online` 字段返回给前端 |
