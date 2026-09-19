# dj-agent 通信架构文档

## 1. 整体架构

```
dj-agent (Go)
  └── gRPC 双向长连接 → autoadmin gRPC 网关 :9001
      ├── 在线状态
      ├── 自动化任务与结果
      ├── 主机信息采集
      ├── WebSSH 终端
      └── 文件管理与传输
```

Agent 自身不区分"注册/数据"两条通道：文件传输、WebSSH、任务执行复用同一条
`AgentChannel.Session` 双向流（见 `proto/agent_channel.proto`）。

---

## 2. 主机身份：实例名（instance_name）

**主机只有一个标识：`assets_host.instance_name`（实例名）。** 历史上曾用独立的
`assets_host.agent_id` 承载 agent 身份，该字段已彻底删除（迁移
`000020_drop_host_agent_id`），不再存在"agent_id 与实例名两份标识"。

契约：

- **唯一性**：实例名与 IP 都要求全局唯一且非空，作为主机的双重标识——
  实例名是业务标识（agent 侧来源 `DJ_AGENT_INSTANCE_NAME`），IP 是寻址标识。
  校验在服务层完成（`assets.checkInstanceNameUnique` / `checkHostIPUnique`），
  不依赖 DB 唯一约束，以便存量重复数据平滑收敛。
- **agent 侧**：`DJ_AGENT_INSTANCE_NAME` **必填无默认值**，缺失即启动失败
  （`dj_agent/internal/config`），避免静默用一个无意义的标识。安装时由平台渲染
  playbook 变量 `dj_agent_instance_name` 写入 `/etc/dj-agent/config.env`。
- **网关会话 key**：握手 `Hello.instance_name` 即网关会话的路由 key，
  `Gateway.IsOnline(instanceName)` / `Gateway.Execute(ctx, instanceName, …)`
  以及所有下发路径（应用控制、WebSSH、文件传输、监控/日志下发、巡检、基线扫描、
  自动化）都按该值定位 agent。协议字段号保持为 1（仅改名，未改号），
  因此新旧 agent 在二进制协议上互通。
- **落库匹配**：握手回调按 `instance_name` 匹配 `assets_host` 行；实例名未匹配到
  主机时跳过并记日志，不阻断会话。
- **快照列**：`security_scan_target.instance_name_snapshot`、
  `inspection_target_execution.instance_name_snapshot` 记录执行当时的主机实例名
  （列名曾为 `agent_id` / `agent_id_snapshot`，同一次迁移改名并回填）。

---

## 3. gRPC 通道

dj-agent 主动连接系统参数 `sys.assets.agent.grpc_advertise_addr` 指向的地址。远程
Agent 使用该参数原值，本机 Agent 使用 `127.0.0.1` 和相同端口。autoadmin 不需要
主动访问目标主机。

同一条双向流按 `request_id` 多路复用。autoadmin 通过网关下发命令，
Agent 在同一连接返回响应、输出和数据块。连接中断后 Agent 自动重连。

**keepalive 协商**：Agent 客户端每 30s 发送 keepalive ping（`PermitWithoutStream=true`，
`grpcfile/client.go`）。autoadmin 的 gRPC server 必须放宽 `EnforcementPolicy`（`MinTime=15s`、
`PermitWithoutStream=true`）并配置服务端 `KeepaliveParams`（`app.go`）；否则默认
`MinTime=5m` 会把高频 ping 判为 `too_many_pings` 并发 GOAWAY，表现为 agent 每约 90s
被断开重连一次——两端看起来时而在线时而不在线，基线/巡检下发恰好撞在断开窗口就会误报离线。

会话建立时 Agent 发送的第一帧 `Hello` 携带 `instance_name`（= `DJ_AGENT_INSTANCE_NAME`）、
`token`（共享密钥校验）与 `version`（构建期注入的 `buildinfo.Version`）。
校验通过后 autoadmin 回 `HelloAck`；校验失败直接关闭流，避免未授权 client 冒充 agent。

---

## 4. 在线状态与版本上报

Go 版网关（`autoadmin/internal/agent/gateway.go`）是在线状态的唯一依据，
握手成功即回调 `newAgentHelloRecorder`（`autoadmin/internal/app/app.go`）落库：

- 更新 `assets_host.agent_online=True` 和 `agent_online_time`（按 `instance_name` 匹配主机行）。
- `Hello.version` 非空时同步更新 `assets_hostsystem.agent_version`（仅更新 `update_time`，
  不动 `collected_at`，因为 OS 信息并未重新采集）。
- `instance_name` 未匹配到主机或主机尚无 hostsystem 行时跳过并记日志；回调失败只记日志，不阻断会话。
- 不按历史时间戳做心跳超时，避免覆盖仍然存活的 gRPC Session。

因此 agent 安装/更新重启后，主机列表的 Agent 版本即随握手自动刷新，
无需等待按需 `get_host_info` 采集（采集仅用于补全 OS 等完整主机信息）。

---

## 5. 任务执行身份

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

## 6. 构建约束

dj-agent 必须编译为纯静态二进制，否则会动态链接构建机的 glibc，跨发行版分发失败。

- 唯一认可的入口是 `dj_agent/Makefile`（已 `export CGO_ENABLED := 0`）：
  `make build` / `make test` / `make vet` / `make all`。
- 禁止裸跑 `go build` / `go test` / `go vet`。
- `cmd/agent/cgo_guard.go` 是 `//go:build cgo` 构建守卫：`CGO_ENABLED=1` 时直接编译
  失败，禁止删除或绕过。
- 交叉编译只调 `GOOS` / `GOARCH`，例如 `make build GOARCH=arm64`。

---

## 7. 后端进程说明

| 进程 | 启动命令 | 职责 |
|---|---|---|
| Django 主进程 | `python manage.py runserver` | REST API、WebSocket、Agent gRPC Server |
| Celery Worker | `python manage.py runceleryworker` | 后台与定时任务执行 |
| Celery Beat | `python manage.py runcelerybeat` | 定时任务调度 |

RabbitMQ 只作为 Celery Broker 使用，不参与 dj-agent 通信。
