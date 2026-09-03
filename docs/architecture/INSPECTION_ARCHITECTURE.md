# 巡检中心架构（Go 后端 autoadmin）

## 执行器

巡检检查项支持两种执行器，均**固定在 Agent 端执行**（`execution_location` 概念已删除，列保留但恒写 `agent`）：

1. **`schema_validate`（Schema 校验）**：读目标主机上的文件，按 JSON Schema /
   Schematron / Regexp 规则校验内容。
2. **`goss`（声明式套件）**：一个检查项对应一份 [goss](https://github.com/goss-org/goss)
   YAML 套件，覆盖 file/port/process/service/package/user/command/dns 等十余类资源。
3. **在线校验**：`POST /sys/inspection/goss/validate/`（`inspection:view` 权限）供前端
   编辑器"校验 YAML"按钮即时校验，与保存时的 schema 校验同源（`validateGossSpec`）。

早期版本曾支持 `shell` / `http` / `tcp` 执行器及 `controller`（djadmin 后端本机）
执行位置，已全部删除：

- `autoadmin/internal/inspection/groups.go` 的 `validateCheck` 只放行
  `schema_validate` / `goss`；goss 的 YAML 在保存时用 **goss 官方 JSON Schema**
  （`internal/inspection/gossschema/goss-schema.yaml`，上游 docs/schema.yaml）预校验。
- `runtime.go` 的 `compileAgentCheck` 编译两种执行器；controller 执行路径已删除。
- dj-agent 的 `application_baseline.go` 只认 `goss:v1` / `schema_validate:v1` /
  `schema_validate:inline:v1` 能力；`checkShell` / `checkHTTP` / `checkTCP` 已删除。
- 前端 `fronted/src/views/inspection/index.vue` 提供两种执行器的编辑表单。

历史执行记录中旧类型（`shell` / `http` / `tcp`）仅作展示，不再可新建。

## Goss 执行器（最终逻辑）

### 二进制打包与释放

- dj-agent 通过 `go:embed`（`internal/executor/embedded/goss/`）内嵌 goss 官方发布
  二进制，当前版本 **v0.4.10**（`dj_agent/resources/goss/VERSION`，带 SHA256SUMS）。
- `dj_agent/Makefile` 构建前按 `GOARCH` 从 `resources/goss/` 复制对应架构二进制到
  embed 目录（只嵌当前架构，Agent 体积 22MB→37MB）；`goss-verify` 目标做哈希校验。
- Agent 首次执行 goss 检查时把二进制释放到 **Agent 可执行文件同目录**
  （如 `/usr/local/bin/goss/<version>/goss`，Agent 能运行即证明该目录可写可执行），
  失败时回退 `os.TempDir()`。不使用 `/var/lib`（目标机常见 noexec 挂载或受限权限）。
  释放流程：内容 SHA256 比对 → 临时文件写入 → chmod 0755 → 原子 rename；目录整条
  链路按 0755 创建并对旧的 0700 目录逐层放开（goss 以巡检 run_user 降权运行，需要
  穿越并执行）；已存在且哈希一致则补权限后复用。升级 goss = 升级 Agent（版本编译期
  钉死）。执行仍被拒时自动换候选目录重试，错误消息带文件/目录权限位、noexec、
  SELinux 诊断。

### 检查计划字段（check_plan 中 executor=goss）

| 字段 | 语义 |
|---|---|
| `spec` | goss YAML 文本，后端已展开 `${APP_HOME}` 等变量 |
| `run_user` | 执行用户，留空默认 root；goss 以该用户身份运行（进程/文件/端口按该用户视角采集），降权语义与巡检 shell 时代一致（setuid/setgid，不走 sudo） |
| `vars` | 透传 `--vars-inline`，YAML 内可用 Go template 引用 |
| `environment` | 白名单环境变量注入（APP_HOME/RUN_USER/INSTANCE_NAME/HOST_IP/HOST_NAME 等），goss 进程及其 command 资源均可见 |

### 结果映射

goss `validate --format json` 的 summary 映射为单条检查结果：

- `expected` = 检查项配置的期望（套件本身），`actual` =
  `{test_count, failed_count, skipped_count, failures[], run_user}`；
- 有失败 → `fail`，消息含失败条数与逐条 summary-line；YAML 空/二进制释放失败/JSON
  解析失败 → `error`；全部通过 → `pass`；
- 退出码语义：0=通过，1=有失败，3=超时；无 JSON 输出按 error 处理。

## 巡检中心模式（最终语义）

巡检检查**只执行巡检中心下发的检查计划**（`check_plan`，即 `schema_validate` / `goss`
检查项）。早期 baseline 方案的内置检查——应用控制状态（systemd/docker/命令行）、
端口、路径、日志——已整体移除：

- dj-agent 的 `check_application_baseline` action 只解析 `check_plan` 并执行其中的
  检查项，不再执行任何内置基线检查（运行状态、端口监听等如需检查，用 goss 的
  `service` / `port` / `process` / `command` 资源在巡检组里显式声明）。
- autoadmin 下发的参数只有 `check_plan`，不再携带 `control_type` / 控制配置。
- 应用控制（启停/状态查询，`control_application` action）是独立功能，不受影响。
- Django 后端（已冻结）下发的旧格式参数（含 control_type）与新 Agent 兼容：
  多余字段被忽略，只执行 check_plan。

## 执行流程（最终逻辑）

1. **入口**：`POST /inspection/tasks/{id}/run`（手动）或定时调度。`prepareRunTask` 校验
   任务与巡检组启用、检查项非空；`resolveTargets` 按 scope 解析目标
   （`per_host` → 固定主机 ID 列表，其余 → 逻辑服务的启用部署实例）。
2. **落库**：`createExecution` 写执行记录与任务/巡检组/服务/目标四份快照，目标状态
   `pending`，任务刷新 `last_run_time`。
3. **执行**：`execute` 按 `concurrency`（1~100）并发执行目标。每个目标：
   - 所有检查项编译进 Agent 检查计划（`control_type=command` + `check_plan`）；
   - Agent 离线或未注册 → 目标直接失败，计划级 error "Agent 离线"；
   - 在线则通过 gRPC `check_application_baseline` 下发，超时为任务超时 + 45s 余量；
   - 结果按 `inspection:{execution}:{index}` 关联回快照中的检查项取得 severity，
     写入 `inspection_result`（重跑前先清旧结果）。
4. **判定**：critical 级检查项非 pass/skipped → 目标失败；warning 级只计入汇总。
   全部目标完成后按目标状态聚合 execution 状态（failed>0 或 success=0 → failed）。
5. **失败语义**：Agent 返回计划级 error（版本不支持 / 能力不支持 / 结果格式无效）
   会让该目标失败；goss 释放失败同语义。

## 与 Django 双实现的差异（重要）

Django 后端（`backend/djadmin/inspection/`，已冻结不再修改）仍保留 `shell` / `http` /
`tcp` 执行器与 `controller` 执行位置，**两者语义不再对齐**：

- Go 后端：`schema_validate` + `goss`，仅 Agent 执行；Agent 端能力同步调整。
- Django 后端：四种旧执行器 + 双执行位置；其下发的 `shell` / `http` / `tcp` 检查计划
  会被新版 dj-agent 以"不支持的能力/执行器"拒绝。
- 数据库 `inspection_check.executor` / `execution_location` 列两侧共用，历史脏数据
  （旧执行器）在 Go 侧保存时被校验拒绝，执行时表现为计划级 error。
