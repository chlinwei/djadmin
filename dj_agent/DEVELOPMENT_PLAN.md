# dj_agent 技术栈与功能规划

更新时间：2026-07-17

## 1. 当前现状（代码真实状态）

- 已有 Go Module：`github.com/chlinwei/djadmin/dj_agent`
- Go 版本：`1.26.5`
- 当前仅有启动入口：`cmd/agent/main.go`
- 入口当前功能：打印 `dj_agent started`
- 目录已预留但未实现：
  - `internal/config`
  - `internal/executor`
  - `internal/logger`
  - `internal/mq`
  - `internal/protocol`
  - `internal/registry`
  - `internal/reporter`

## 2. 建设目标

将 dj_agent 建成可部署的“任务执行代理”，具备如下能力：

- 接收任务（初期可轮询，后续 MQ 推送）
- 执行脚本/命令（支持超时、取消、并发控制）
- 采集执行结果（stdout/stderr/退出码/耗时）
- 上报状态与结果（开始、运行中、成功、失败、取消）
- 可靠性保障（重试、幂等、心跳、崩溃恢复）
- 可观测性（结构化日志、指标、健康检查）

## 3. 技术栈规划（建议基线）

以下为建议技术栈，按“先最小可用，再增强”原则推进。

### 3.1 语言与构建

- 语言：Go
- 模块管理：Go Modules
- 构建：`go build ./...`
- 代码质量：`go test ./...`、`go vet ./...`
- 格式化：`gofmt -w`（可后续引入 `golangci-lint`）

### 3.2 配置管理

- 基线方案：标准库 + YAML/ENV（优先简单）
- 推荐增强：`viper`（支持 env + file + default）
- 配置拆分：
  - `agent`：实例标识、并发、心跳间隔
  - `backend`：上报地址、认证信息
  - `mq`：broker 地址、队列名、prefetch
  - `executor`：默认超时、shell、工作目录、白名单

### 3.3 日志与可观测

- 基线方案：标准库 `log/slog`（JSON 输出）
- 推荐增强：`zap`
- 日志要求：
  - 必含字段：`trace_id`、`job_id`、`agent_id`、`stage`、`cost_ms`
  - 禁止日志泄露敏感信息（token、密码）
- 指标（P2 起）：可选 Prometheus 暴露 `/metrics`

### 3.4 通信协议

- P0/P1：HTTP + JSON（开发效率最高）
- P2：MQ（RabbitMQ）作为任务分发主通道
- 协议定义位置：`internal/protocol`
- 约束：
  - 所有消息有 `version`
  - 任务有 `job_id`（幂等关键）
  - 状态枚举统一：`queued/running/success/failed/canceled/timeout`

### 3.5 任务执行引擎

- 基线：`os/exec` + `context.WithTimeout`
- 能力：
  - 超时强杀
  - 实时采集 stdout/stderr
  - 退出码捕获
  - 并发 worker 池
- 安全控制：
  - 命令白名单/黑名单
  - 目录隔离（工作目录）
  - 禁止高危命令（可配置）

### 3.6 消息队列（P2 启用）

- Broker：RabbitMQ（与主项目调度生态一致）
- Go 客户端：推荐 `amqp091-go`
- 关键点：
  - 手动 ack
  - 失败重试 + 死信队列
  - 消费幂等（重复消息不重复执行）

### 3.7 注册与发现

- P1：启动时注册 + 周期心跳（HTTP 上报）
- P3：支持动态标签与能力上报（如 shell/script/python）
- 建议上报字段：
  - `agent_id`、`hostname`、`ip`
  - `version`、`os`、`arch`
  - `capacity`、`running_jobs`

## 4. 功能规划（分阶段）

## Phase P0：可运行骨架（1-2 天）

目标：程序可启动、可配置、可优雅退出。

交付：

- `internal/config`：加载配置（env + file）
- `internal/logger`：统一日志初始化
- `cmd/agent/main.go`：启动流程（init config -> init logger -> run）
- 优雅退出：接收 `SIGINT/SIGTERM`

验收标准：

- 启动时打印结构化启动信息
- 关闭信号可在 5 秒内优雅退出
- `go test ./...` 可通过（至少含 smoke test）

## Phase P1：HTTP 拉取/上报闭环（3-5 天）

目标：形成“取任务 -> 执行 -> 上报”最小闭环。

交付：

- `internal/protocol`：任务/结果结构定义
- `internal/executor`：命令执行器（含超时）
- `internal/reporter`：HTTP 上报客户端
- `internal/registry`：注册 + 心跳
- 轮询器：定期从后端拉取待执行任务

验收标准：

- 单任务完整流转成功
- 失败任务可上报错误信息和退出码
- 同一 `job_id` 不会重复执行（本地去重缓存可接受）

## Phase P2：MQ 消费与可靠性（5-8 天）

目标：切换到 RabbitMQ 消费，提升吞吐与可靠性。

交付：

- `internal/mq`：连接、消费、ack/nack 封装
- 失败重试策略：指数退避（上限可配）
- 死信队列对接
- 并发 worker 池

验收标准：

- 支持并发消费（如 `N=5`）
- 单个任务失败不影响其他任务
- 崩溃后重启可继续消费未 ack 消息

## Phase P3：可观测与运维能力（3-5 天）

目标：生产可运维。

交付：

- 健康检查接口：`/healthz`
- 指标暴露：`/metrics`（可选）
- 日志字段标准化与 trace 串联
- 配置热更新（可选）

验收标准：

- 可快速定位单个任务全链路日志
- 可观测当前队列积压和执行耗时分布

## Phase P4：安全与治理（持续）

目标：降低执行风险。

交付：

- 命令策略（白名单/黑名单）
- 认证签名校验（agent <-> backend）
- 审计日志（谁在何时执行了什么）

验收标准：

- 未授权任务不可执行
- 高危命令默认拒绝

## 5. 模块职责（代码分工）

- `cmd/agent`
  - 进程入口
  - 生命周期管理
- `internal/config`
  - 配置模型、读取、校验
- `internal/logger`
  - 结构化日志封装
- `internal/protocol`
  - 任务/状态/上报 DTO
- `internal/executor`
  - 任务执行与结果采集
- `internal/mq`
  - MQ 连接与消费语义
- `internal/registry`
  - 注册、心跳、实例元数据
- `internal/reporter`
  - 上报请求封装（重试/超时）

## 6. 推荐目录落地（目标结构）

```text
dj_agent/
  cmd/
    agent/
      main.go
  internal/
    app/
      app.go                # 组装依赖和启动流程
    config/
      config.go
      loader.go
    logger/
      logger.go
    protocol/
      job.go
      report.go
    executor/
      executor.go
      worker_pool.go
    mq/
      consumer.go
      publisher.go
    registry/
      registry.go
    reporter/
      reporter.go
  pkg/
    version/
      version.go
  configs/
    agent.example.yaml
  scripts/
    run_local.sh
  Makefile
```

## 7. 配置示例（建议）

```yaml
agent:
  id: "agent-001"
  heartbeat_interval: "15s"
  max_workers: 3

backend:
  base_url: "http://127.0.0.1:8000"
  token: "${DJ_AGENT_TOKEN}"
  report_timeout: "10s"

mq:
  enabled: false
  url: "amqp://guest:guest@127.0.0.1:5672/"
  queue: "djadmin.jobs"
  prefetch: 10

executor:
  shell: "/bin/bash"
  default_timeout: "10m"
  work_dir: "/tmp/dj_agent"
  deny_commands:
    - "rm -rf /"
    - "mkfs"
```

## 8. 开发顺序（你后续可以按这个来）

1. 先完成 P0：配置、日志、优雅退出。
2. 再完成 P1：HTTP 闭环，确保功能可跑通。
3. 接入 P2：RabbitMQ 消费和重试。
4. 增强 P3：健康检查 + 指标 + tracing。
5. 最后做 P4：安全策略和审计治理。

## 9. 每阶段最少测试要求

- 单元测试：
  - config 解析
  - executor 超时/退出码
  - reporter 重试
- 集成测试：
  - 模拟 backend 回包
  - 模拟 MQ 消息消费
- 可靠性测试：
  - 任务执行中断电/kill 后恢复
  - 网络抖动下的重试行为

## 10. 第一批代码任务清单（可直接开工）

- Task 1：定义 `Config` 结构与加载器（含校验）
- Task 2：定义统一日志初始化函数
- Task 3：实现 `App` 启动器（组装 config/logger）
- Task 4：实现优雅退出（context + signal）
- Task 5：编写 `agent.example.yaml`
- Task 6：补充 P0 单元测试

完成上述 6 个 Task 后，再进入 P1。

## 11. 风险与应对

- 风险：命令执行安全边界不清
  - 应对：P1 就加入 denylist；P4 引入 allowlist
- 风险：消息重复导致重复执行
  - 应对：以 `job_id` 作为幂等键，落本地状态缓存
- 风险：日志难追踪
  - 应对：强制日志字段规范（trace_id/job_id）

## 12. 你后续让我“按规划写代码”时的建议指令模板

你可以直接发下面这种指令，我会按文档顺序落地：

- "按 DEVELOPMENT_PLAN.md 的 P0，先实现 config + logger + 优雅退出。"
- "继续 P1，只做 protocol + executor，先不接 MQ。"
- "把 P2 的 RabbitMQ 消费落地，并补最小可测样例。"

## 13. 当前进度快照（2026-07-17）

当前代码状态（已完成）：

- 已完成 P0 主体骨架。
- 已落地配置模块：internal/config/config.go（环境变量加载 + 参数校验）。
- 已落地日志模块：internal/logger/logger.go（slog JSON 初始化）。
- 已将入口流程拆分为 run 模式，并引入 app 层进行生命周期承接（等待信号 + 优雅退出）。
- 优雅退出路径已验证可工作：收到 SIGINT/SIGTERM 后进入 shutdown 流程。

当前策略（按你的要求）：

- 前期先不补测试，先快速推进功能。
- 单次代码增量尽量小，每次改动控制在 150 行以内，便于理解。

下次继续起点：

- 从 P1 起步，先做 internal/executor 的最小可运行版本。
- 第一目标是支持执行单条 shell 命令，并返回 stdout、stderr、exit code、耗时、是否超时。

下次可直接使用的续写指令：

- 继续第9步，做 executor 最小雏形（不写测试，单次不超过150行）。
- 从当前进度快照继续，先把 executor 接入 app，启动后执行一条示例命令并打印结果。
