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

同一条双向流按 `request_id` 多路复用。backend 通过网关下发命令，
Agent 在同一连接返回响应、输出和数据块。连接中断后 Agent 自动重连。

会话建立时 Agent 发送的第一帧 `Hello` 携带 `agent_id`、`token`（共享密钥校验）
与 `version`（构建期注入的 `buildinfo.Version`）。校验通过后 backend 回 `HelloAck`。

---

## 3. 在线状态与版本上报

Go 版网关（`autoadmin/internal/agent/gateway.go`）是在线状态的唯一依据，
握手成功即回调 `newAgentHelloRecorder`（`autoadmin/internal/app/app.go`）落库：

- 更新 `assets_host.agent_online=True` 和 `agent_online_time`（按 `agent_id` 绑定）。
- `Hello.version` 非空时同步更新 `assets_hostsystem.agent_version`（仅更新 `update_time`，
  不动 `collected_at`，因为 OS 信息并未重新采集）。
- `agent_id` 未绑定主机或主机尚无 hostsystem 行时跳过并记日志；回调失败只记日志，不阻断会话。
- 不按历史时间戳做心跳超时，避免覆盖仍然存活的 gRPC Session。

因此 agent 安装/更新重启后，主机列表的 Agent 版本即随握手自动刷新，
无需等待按需 `get_host_info` 采集（采集仅用于补全 OS 等完整主机信息）。

---

## 4. 任务执行身份

Agent 进程以 root 运行，执行任务时才降权到目标用户。

涉及 `run_user` 的场景（应用控制命令、用户级 systemd）统一走
`applicationRunUserCommand`（`internal/executor/application_control.go`）：

- 命令固定以 `/bin/bash -lc` 启动，login shell 会加载目标用户 profile，`JAVA_HOME`
  等用户级环境变量照常生效。
- root 场景通过 `SysProcAttr.Credential`（uid/gid + 附加组）直接 setuid/setgid，
  **不经过 `sudo`**。多数发行版 sudoers 带 `Defaults requiretty`，Agent 无 tty 会被
  拒绝（`sudo: sorry, you must have a tty to run sudo`）。
- Agent 以非 root 运行时，只允许 `run_user` 等于自身，否则直接报错，禁止静默以错误
  身份执行。
- 自动化任务的 `run_as_user` / `run_as_group` 同样是 setuid/setgid 降权，不使用
  ansible become。

---

## 5. 构建约束

dj-agent 必须编译为纯静态二进制，否则会动态链接构建机的 glibc，跨发行版分发失败。

- 唯一认可的入口是 `dj_agent/Makefile`（已 `export CGO_ENABLED := 0`）：
  `make build` / `make test` / `make vet` / `make all`。
- 禁止裸跑 `go build` / `go test` / `go vet`。
- `cmd/agent/cgo_guard.go` 是 `//go:build cgo` 构建守卫：`CGO_ENABLED=1` 时直接编译
  失败，禁止删除或绕过。
- 交叉编译只调 `GOOS` / `GOARCH`，例如 `make build GOARCH=arm64`。

---

## 6. 后端进程说明

| 进程 | 启动命令 | 职责 |
|---|---|---|
| Django 主进程 | `python manage.py runserver` | REST API、WebSocket、Agent gRPC Server |
| Celery Worker | `python manage.py runceleryworker` | 后台与定时任务执行 |
| Celery Beat | `python manage.py runcelerybeat` | 定时任务调度 |

RabbitMQ 只作为 Celery Broker 使用，不参与 dj-agent 通信。
