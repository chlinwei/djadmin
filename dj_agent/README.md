# dj-agent - djadmin 远程执行代理

## 📝 概述

**dj-agent** 是用Go语言编写的轻量级远程执行代理，作为djadmin系统的执行引擎，负责在目标主机上执行Ansible Playbook、Shell脚本、自定义任务等。

- **语言**: Go 1.21+
- **通信**: gRPC over SSH/TLS
- **用途**: 自动化任务执行、主机信息采集、监控软件部署等
- **部署**: 单二进制文件，无需依赖

---

## 🎯 核心功能

| 功能 | 描述 |
|------|------|
| **Ansible Playbook执行** | 在远程主机上执行Playbook，支持变量注入 |
| **Shell脚本执行** | 执行通用Shell脚本，参数替换 |
| **主机信息采集** | 收集CPU、内存、磁盘、OS版本等信息 |
| **gRPC通信** | 与djadmin后端通过gRPC进行双向通信 |
| **日志流式传输** | 实时上报执行日志和进度 |
| **错误处理** | 完整的连接、认证、超时处理机制 |

---

## 📂 项目结构

```
dj_agent/
├── README.md                         # 本文件
├── go.mod                            # Go模块定义
├── DEVELOPMENT_PLAN.md               # 开发路线图
├── TASK_RESULT_SCHEMA_V1.md          # 任务结果数据结构
├── bin/
│   └── dj-agent                      # 编译后的二进制文件（Linux/macOS/Windows）
├── cmd/
│   └── agent/
│       ├── main.go                   # 入口点
│       └── ...
├── internal/
│   ├── app/                          # 应用层逻辑
│   ├── config/                       # 配置加载
│   ├── executor/                     # 任务执行器
│   │   ├── automation_ssh_key.go     # SSH密钥管理
│   │   ├── generic_exporter.go       # 监控导出器
│   │   └── ...
│   ├── grpcfile/                     # gRPC文件定义
│   ├── logger/                       # 日志系统
│   ├── protocol/                     # 通信协议
│   └── ...
├── deploy/
│   └── dj-agent.service.example      # systemd服务文件模板
└── proto/
    └── agent_channel.proto           # gRPC接口定义
```

---

## 🚀 快速开始

### Ansible 安装

平台批量纳管主机时使用 `../backend/djadmin/assets/agent_install.yml`。Playbook
当前支持 Linux amd64，会上传 `bin/dj-agent` 和 `deploy/install.sh`，生成 systemd
配置并执行 `systemctl enable --now dj-agent`。开发阶段 gRPC 握手只使用 `DJ_AGENT_ID`，
不需要 Token；后续切换 mTLS 时再替换认证配置。

### 编译

**唯一认可的构建入口是 `Makefile`**，已内置 `export CGO_ENABLED := 0`，禁止裸跑 `go build` / `go test` / `go vet`：

```bash
cd dj_agent
make build          # 构建到 bin/dj-agent（默认 GOOS=linux GOARCH=amd64）
make test           # go test ./...
make vet            # go vet ./...
make all            # vet + test + build
```

交叉编译只调 `GOOS` / `GOARCH` 变量，不要在命令里重新打开 cgo：

```bash
make build GOARCH=arm64
```

**为什么强制 `CGO_ENABLED=0`**：生成纯 Go 静态二进制，避免运行时依赖构建机的 glibc 版本，否则分发到较旧发行版会直接起不来。

`cmd/agent/cgo_guard.go` 是 `//go:build cgo` 构建守卫：一旦在 `CGO_ENABLED=1` 下编译会直接报错失败，禁止删除或绕过。

### 配置

创建 `config.yaml`:
```yaml
server:
  host: 0.0.0.0
  port: 50051
  tls_enabled: false  # 生产环境建议启用

logging:
  level: info
  format: json
```

### 运行

```bash
./bin/dj-agent --config config.yaml
```

### 作为系统服务运行（Linux）

```bash
# 复制服务文件
sudo cp deploy/dj-agent.service.example /usr/lib/systemd/system/dj-agent.service

# 编辑配置
sudo systemctl edit dj-agent

# 启动
sudo systemctl start dj-agent
sudo systemctl enable dj-agent

# 查看日志
sudo journalctl -u dj-agent -f
```

---

## 🔌 gRPC接口

Agent通过gRPC暴露以下主要接口：

| 方法 | 功能 |
|------|------|
| `ExecuteTask` | 执行任务（Playbook/Script） |
| `CollectHostInfo` | 采集主机信息 |
| `GetTaskStatus` | 获取任务执行状态 |
| `CancelTask` | 取消正在执行的任务 |
| `StreamTaskLog` | 流式获取任务日志 |

详见 `proto/agent_channel.proto` 和 `TASK_RESULT_SCHEMA_V1.md`

---

## 🔗 与djadmin的集成

djadmin后端通过SSH隧道连接到Agent：

```
djadmin Backend → SSH → dj-agent gRPC
      ↓
  [Ansible Playbook]
  [Shell Script]
  [Custom Task]
      ↓
  Target Host
```

集成API文档见 `../backend/djadmin/assets/AGENT_JOB_API.md`

---

## 📊 执行流程

```
User Request (djadmin Web UI)
    ↓
Django API (automation/views.py)
    ↓
Celery Task (automation/tasks.py) - 本地后台线程执行
    ↓
gRPC Call to dj-agent
    ↓
dj-agent Executor (Go)
    ├─ SSH to Target Host
    ├─ Execute Playbook/Script
    ├─ Stream Logs Back
    └─ Return Result
    ↓
Update Execution Log (Django Model)
    ↓
WebSocket Notify to Frontend
```

---

## 🛠️ 开发指南

### 添加新的执行器

1. 在 `internal/executor/` 下创建新文件
2. 实现 `Executor` 接口
3. 在 `internal/executor/executor.go` 中注册
4. 更新gRPC proto定义

### 调试

```bash
# 启用debug日志
./bin/dj-agent --config config.yaml --log-level debug

# 连接到本地Agent
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 agent.Agent.ExecuteTask
```

---

## 📦 依赖

- `google.golang.org/grpc` - gRPC框架
- `golang.org/x/crypto` - SSH支持
- 其他见 `go.mod`

---

## 🔐 安全考虑

1. **SSH密钥**: 支持基于key的认证（推荐生产环境）
2. **gRPC TLS**: 支持TLS加密通信
3. **权限隔离**: Agent 进程以 root 运行，执行任务时直接 `setuid`/`setgid` 降权到目标用户（见下）
4. **日志敏感信息**: 自动脱敏密码、API密钥等

### 任务执行身份（降权机制）

涉及 `run_user` 的场景（巡检 shell 检查项、应用控制命令、用户级 systemd）统一走
`applicationRunUserCommand`（`internal/executor/application_control.go`）：

- 命令始终以 `/bin/bash -lc` 启动，login shell 会加载目标用户 profile，`JAVA_HOME` 等用户级环境变量照常生效。
- Agent 以 root 运行时，通过 `SysProcAttr.Credential`（uid/gid + 附加组）直接降权，**不走 `sudo`**。
- 不用 `sudo` 的原因：大量发行版的 sudoers 带 `Defaults requiretty`，Agent 无 tty，会被直接拒绝
  （`sudo: sorry, you must have a tty to run sudo`）。
- Agent 以非 root 运行时，只允许 `run_user` 等于自身，否则直接报错，禁止静默以错误身份执行。
- 自动化任务的 `run_as_user` / `run_as_group` 同样是 setuid/setgid 降权，不使用 ansible become。

---

## 📖 相关文档

- [DJ_AGENT_ARCHITECTURE.md](../docs/architecture/DJ_AGENT_ARCHITECTURE.md) - 架构设计详解
- [DEVELOPMENT_PLAN.md](./DEVELOPMENT_PLAN.md) - 开发计划和roadmap
- [TASK_RESULT_SCHEMA_V1.md](./TASK_RESULT_SCHEMA_V1.md) - 任务结果数据结构
- [AGENT_JOB_API.md](../backend/djadmin/assets/AGENT_JOB_API.md) - 与后端的集成接口

---

## 🤝 贡献

欢迎提交Issue和PR改进dj-agent的功能、性能和稳定性。

---

**最后更新**: 2026-08-17  
**版本**: v1.0  
**维护者**: djadmin Team
