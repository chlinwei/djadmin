# dj-agent 通信架构文档

## 1. 整体架构

```
dj-agent (Go)
  └── gRPC 双向长连接 → Django 主进程 :9001
      ├── 在线状态
      ├── 自动化任务与结果
      ├── 主机信息采集
      ├── WebSSH 终端
      └── 文件管理与传输
```

---

## 2. gRPC 通道

dj-agent 主动连接系统参数 `sys.assets.agent.grpc_advertise_addr` 指向的地址。远程
Agent 使用该参数原值，本机 Agent 使用 `127.0.0.1` 和相同端口。backend 不需要
主动访问目标主机。

同一条双向流按 `request_id` 多路复用。Django 通过 `AgentChannelClient` 下发命令，
Agent 在同一连接返回响应、输出和数据块。连接中断后 Agent 自动重连。

---

## 3. 在线状态

`AgentSessionRegistry` 是在线状态的唯一依据：

- Session 建立时写入 `Host.agent_online=True` 和 `agent_online_time`。
- Session 断开且没有同 Agent 的新 Session 时写入 `Host.agent_online=False`。
- 不按历史时间戳做心跳超时，避免覆盖仍然存活的 gRPC Session。

主机信息由 backend 按需通过 gRPC 调用 Agent 的 `get_host_info` 获取。

---

## 4. 后端进程说明

| 进程 | 启动命令 | 职责 |
|---|---|---|
| Django 主进程 | `python manage.py runserver` | REST API、WebSocket、Agent gRPC Server |
| Celery Worker | `python manage.py runceleryworker` | 后台与定时任务执行 |
| Celery Beat | `python manage.py runcelerybeat` | 定时任务调度 |

RabbitMQ 只作为 Celery Broker 使用，不参与 dj-agent 通信。
