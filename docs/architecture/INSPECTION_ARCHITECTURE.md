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
  `schema_validate` / `goss`（`scope` 列已删除；应用上下文变量按 `category` 校验：
  仅应用类型组可用）；goss 的 YAML 在保存时用 **goss 官方 JSON Schema**
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
- **进程级路径缓存**：`ensureGossBinary` 每次要读内嵌二进制 + 两次 SHA256 + 读盘比对，
  是逐检查项调用的纯开销；首次校验成功后候选路径缓存在进程内
  （`cachedGossBinary`），执行被拒（文件被删/权限变化）时整体失效并重新校验。
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
| `run_user` | 执行用户，留空默认 root；goss 以该用户身份运行（进程/文件/端口按该用户视角采集）。切换方式为 `su -l <run_user> -c '<goss 命令>'` 登录 shell：加载目标用户完整登录环境（HOME/SHELL/PATH 及 `~/.bash_profile` 中的应用变量），goss 及其 spec 内 command 资源均继承该环境，与用户手工登录后执行一致；所有参数经单引号转义后拼入。`run_user` 等于 agent 自身用户时直接执行，不走 su |
| `vars` | 透传 `--vars-inline`，YAML 内可用 Go template 引用 |
| `environment` | 白名单环境变量注入（APP_HOME/RUN_USER/INSTANCE_NAME/HOST_IP/HOST_NAME 等），goss 进程及其 command 资源均可见 |

### 结果映射

goss `validate --format json` 的 summary 映射为单条检查结果：

- `expected` = 检查项配置的期望（套件本身），`actual` =
  `{test_count, failed_count, skipped_count, failures[], details[], run_user}`；
  其中 `details` 为 goss 每条子测试的全量明细（含通过项），前端执行结果弹窗在检查项行上
  展开即可逐条查看 pass/fail：
  - `resource`/`property`：资源标识（`类型: ID`）与断言属性（listening/exit-status/stdout…）；
  - `title`：spec 里资源声明的 `title`（人类可读说明），前端明细"检查项"列优先展示，
    未设置时回退 resource；
  - `successful`/`message`：断言结果与 goss 原始 summary-line（前端作为结果标签的 tooltip）；
  - `expected`/`actual`：期望值与实际值（goss JSON `matcher-result.expected/actual`）；
    命令输出类实际值 goss 无法序列化（`{}`），前端显示 `-`；
  - 前端对 类型/检查点/期望 做词表与语义化格式（如 exit-status → "退出码等于 0"、
    stdout → "输出包含 x"），未覆盖项回退原文，纯显示层转换；
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

## 检查参数化（最终逻辑）

> 迁移 `000006`：inspection_group 加 `params`（形参声明）、inspection_task_group 加
> `param_values`（实参赋值）。

**一套检查适配多个实例**：Oracle 巡检组声明检查参数 `ORACLE_SID`，检查项里写
`ora_pmon_${ORACLE_SID}`；任务绑定时赋值 `ORACLE_SID = 引用宏 ORACLE_SID`，每个
逻辑服务（macro_values 各自的值）执行时自动展开成各自的 SID。

### 三层模型

| 层 | 角色 | 存储 |
|---|---|---|
| 巡检组 | **形参声明**（name/description/required/default） | `inspection_group.params` |
| 任务绑定 | **实参赋值**：`{"value":"85"}` 固定值 或 `{"ref":"RUN_USER"}` 引用 | `inspection_task_group.param_values` |
| 逻辑服务 | **值源**：`macro_values`（按服务实例化）+ 内置变量（部署模板 run_user/app_home 等） | assets 侧已有 |

### 解析规则（`resolveCheckParams`，编译检查计划时逐实例计算）

- **字面量**：直接取值，所有目标相同；
- **引用**：先查内置变量（APP_HOME/RUN_USER/INSTANCE_NAME/APPLICATION_VERSION/
  HOST_IP/HOST_NAME/SERVICE_NAME），再查该实例 `macro_values`；都取不到 → 记入缺失；
- **缺失处理**：该目标直接失败，计划级 error 列出缺失参数（"巡检参数缺失: ..."），
  绝不拿未展开的 `${XX}` 字面量静默执行；
- 展开优先级：**检查参数 > 内置变量**（参数可覆盖同名内置，但声明时禁止与内置
  重名，双保险）；
- `required` 参数任务保存时必须赋值（`missingRequiredParams` 校验），否则拒绝保存。

### 约束

- 应用上下文变量校验按分类：general 组检查项禁用 `${APP_HOME}` 等（含参数引用语义）；
- **引用类实参仅 application 组可用**（实例上下文只有应用组挂载提供）；general 组
  参数只能固定值；
- 隐式宏展开已撤销：服务宏**只能**通过"参数声明 + 引用赋值"进入检查计划——依赖
  显式化，缺配在保存/执行两端都有明确报错。

## 组挂载点模型（最终逻辑）

> 迁移 `000003`（挂载点列）、`000004`（service_id）、`000005`（**删除 scope 与
> 静态范围**）：inspection_task_group 有 `mount_type`/`project_id`/`environment_id`/
> `business_system_id`/`instance_mode`/`service_id`。

### 单路径目标解析（000005 起，scope/静态范围已删除）

`inspection_group.scope`、`inspection_task.logical_service_id`/`selected_host_ids`
与 legacy_static 绑定已整体删除（开发阶段决策，不做兼容）。目标只有一条解析路径：
**挂载点在每次执行时动态解析**（`mounts.go` 的 `resolveMounts`），任务不再保存
任何目标信息。

**挂载点由树节点决定，与组类型无关**（组类型只影响解析粒度与变量上下文）：

| 挂载点 | 通用组解析为 | 应用组解析为 |
|---|---|---|
| `project` | 项目下全部实例**主机**（去重） | — |
| `environment` | 项目×环境的实例**主机**（去重） | — |
| `business`（×可选环境）+ instance_mode | 该业务×环境的实例**主机**（去重） | 该业务×环境的**部署实例**（变量各自展开；once 只选一台在线实例） |
| `service` + instance_mode | 该逻辑服务的实例**主机**（去重） | 该逻辑服务的**部署实例** |

- 通用组主机级上下文（禁用应用变量，保存时校验）；应用组每实例展开 `${APP_HOME}` 等；
- `instance_mode`（all/once）仅对应用组有意义，通用组忽略；

- **应用组按逻辑服务粒度解析**（非主机）：同业务 web-v1/web-v2 同机部署是两个独立
  检查分片，变量无歧义；通用组按主机粒度且**每台主机只跑一次**（不随部署实例重复）。
- **执行目标 = 各挂载点主机并集（按主机去重）**；每台主机的检查计划 =
  `通用组检查项 ×1 + 每个适用逻辑服务的应用组检查项 ×1`（`buildCheckPlan` 组装），
  实例身份写进检查项名称（`实例名 · 检查名`），结果按 group_id/group_name 分区。
- **once（主实例）模式**：HA 场景只选一台在线实例（优先 agent 在线，按
  service/deployment 稳定排序）。局限：不保证选中 VIP 主节点——选中的实例记录在
  `service_snapshot.mounts[].selected_instance` 与目标快照 `services[]` 中。
- 挂载点解析为空：该绑定跳过（group_snapshot 的 `mount.skipped` 记录原因），不阻断
  其他绑定；全部为空时执行被拒绝（"巡检挂载点没有解析到任何目标"）。
- **单组模型（000004 起强制）**：一个任务只绑一个巡检组；通用基线和应用巡检分别
  建任务（树上各挂各的，调度/超时/执行历史自然分开）。环境不同 = 任务不同，
  应用相同 = 组复用。
- 保存校验（`validateMountBinding`）：挂载点引用存在性校验（业务/逻辑服务/项目）；
  应用组挂业务/服务必须带实例模式。前端**不再暴露挂载编辑控件**——挂载完全由
  "左侧树选中的节点"推导（服务→service、业务/环境→business×环境、项目→project），
  弹窗只读显示"巡检对象"摘要。

### 快照

- `group_snapshot` 数组元素带 `mount`：`{mount_type, project/environment/business,
  instance_mode, selected_instance, targets, skipped}`；
- `service_snapshot.mounts` = 本次执行全部挂载点解析摘要；
- `target_snapshot[].services[]` = 该主机涉及的逻辑服务实例（service/instance/mode）；
- `target_snapshot[].business` / `service_snapshot.project/business_system/environment`
  业务链路冗余对所有路径生效（含挂载点任务，按主机的部署归属解析）。

### 任务导航树（前端展示语义）

巡检任务页左侧为资产树：`项目 → 业务系统 → 环境 → 逻辑服务`，结构与排序规则对齐
服务树（业务按名称排序、环境按 order 排序且只显示真实存在服务的环境、逻辑服务为
叶子节点），外加两个顶层节点——"全部任务"与"静态范围（存量任务）"。
任务按挂载点归属到树节点（可同时归属多个）：

- 通用组@项目/环境 → 项目节点；
- 应用组@业务（×环境）→ 该业务下**被覆盖的每个逻辑服务**节点；应用组@逻辑服务 →
  该逻辑服务节点（精确绑定）；
- 新建任务时树上选中的节点自动预填挂载点（应用组：服务节点→精确挂载该服务，
  业务/环境节点→挂业务×环境；通用组：项目/环境节点→对应挂载）。
- legacy_static 绑定 → "静态范围"节点。

右侧列表按选中节点过滤（任务出现在其任一挂载路径上即显示）。归属由前端从任务列表
响应的 `groups[].mount_*` 字段推导，后端不存任务的树锚点。

## 巡检组分类与多组组合（最终逻辑）

> 迁移 `db/migrations/000002_inspection_multigroup`（需执行 `autoadmin migrate`）：
> inspection_group 加 `category`/`application_id`，新增 `inspection_task_group` 关联表
> （存量任务的 group_id 已迁入），inspection_result 加 `group_id`/`group_name`。

### 巡检组分类

- `category`: `general`（通用基线）/ `application`（应用类型）；应用类型组可带"适用应用"
  标签（`application_id` → `assets_application`，跨环境实体，填了才校验存在，可空）。
- 分类是**纯组织属性**：组列表展示分类标签，任务表单按分类分组展示，不强制拦截。

### 单组模型（最终语义）

- **一个任务只绑一个巡检组**（保存强制，后端在查询前拦截多绑定）——通用基线和
  应用巡检分别建任务，各自挂载、各自调度。
- `inspection_task.group_id` 保存该组 ID（单组模型的自然列，非兼容字段）。
- 执行时该组检查项编译进一份检查计划下发；**检查项逐项独立执行、独立结果、独立
  severity**（产品决策：不做合并）。
- 结果按组分区：`inspection_result.group_id`/`group_name` 随每条结果写入。
- `group_snapshot` 为数组（当前恒单元素）：`[{id,name,category,checks,mount}]`。
- 空 enabled 检查项的组在执行时被跳过；全部组无启用检查项才拒绝执行。

### 业务链路快照（汇总报表地基）

`createExecution` 时把项目/业务系统/环境链路冗余进快照（查询失败不阻断执行）：

- 逻辑服务任务：`service_snapshot` 增 `project`/`business_system`/`environment`
  （service → business_system → project，environment 取自 service），每个
  target_snapshot 同样带 `business` 字段；
- 主机组任务：按目标主机经 deployment → service 链路解析每台主机的业务归属，
  写入对应 target_snapshot 的 `business`（一台主机多服务时取任一）；
- 各层级 ID 为空时该层级省略，owner 字段一并冗余（`project.owner`/`business_system.owner`）。

## 执行流程（最终逻辑）

### API 调用链（手动触发）

```
POST /sys/inspection/tasks/{id}/run/          （inspection:tasks:run 权限）
  → router.go: inspectionHandler.RunTask
    → startRun(ctx, taskID, "manual", userID, username)   [runtime.go]
      1. prepareRunTask   装载任务 + 「组→挂载点」绑定（ListInspectionTaskBindings），
                          逐组装载启用检查项
      2. resolveExecutionTargets
                          legacy 绑定 → 旧 resolveTargets（静态范围，全组×全目标）
                          挂载点绑定 → resolveMounts（动态解析，见挂载点模型章节）
      3. createExecution  事务写执行记录 + 快照 + 逐目标 pending 行，刷新 last_run_time
      4. go execute(...)  异步执行，接口立即返回 {execution_id, status:"pending"}
```

查询链：

```
GET  /sys/inspection/executions/               分页列表（status/trigger_type/时间过滤）
GET  /sys/inspection/executions/{id}/          详情：单查询取全部 targets，一次 JOIN 查询
                                               取回该 execution 全部结果按 target 分组
                                               （listExecutionResults，无 N+1）
POST /sys/inspection/executions/{id}/cancel/   事务置 canceled 并 markCanceled(id) 写入
                                               内存取消标记，Agent 迟到响应直接丢弃
```

### 定时调度（最终逻辑）

- 调度器**运行在 API 进程内**（`router.go` 创建 `inspectionHandler` 后调用
  `StartScheduler()`，见 `internal/inspection/scheduler.go`）——巡检执行依赖进程内
  Agent gateway 下发 gRPC，不能放进独立的 `scheduler`/`worker` 进程。
- 每 30s 扫描一次：`SELECT ... FROM inspection_task WHERE enabled=TRUE AND
  cron_expression<>'' AND next_run_time<=UTC_TIMESTAMP(6)`。
- **原子认领**：对每个到期任务执行 `UPDATE inspection_task SET next_run_time=<下一周期>
  WHERE id=? AND next_run_time<=UTC_TIMESTAMP(6)`，`RowsAffected==1` 才触发，多副本部署
  或 tick 重叠不会重复触发。cron 计算与时区均用 UTC（与 `tasks.go` 保存逻辑一致）。
- 触发路径与手动完全一致：`startRun(ctx, taskID, "scheduled", 0, "scheduler")`，
  execution 的 `trigger_type='scheduled'`，`requested_username='scheduler'`。
- 失败语义：调度触发被业务拒绝（任务/组禁用等）记 Warn 日志；基础设施错误记 Error，
  不重试（等下一个 cron 周期）。

### 并发与取消（最终逻辑）

- 任务级并发：`execute` 按 `concurrency`（1~100）信号量并发执行目标（`runtime.go`）。
- **全局并发**：所有 execution 共享一个 200 槽信号量（`maxGlobalConcurrentTargets`），
  多个大任务同时运行时总扇出被钳制，不再放大 DB/gRPC 压力。
- **取消**：`CancelExecution` 事务落库 canceled 后调用 `markCanceled(id)` 写入内存
  `sync.Map`；`executeTarget` 各阶段只查内存标记（原每目标两次 `SELECT status` 热点行
  查询已删除），迟到响应被丢弃。

### 单目标执行（最终逻辑）

1. `executeTarget` 先查内存取消标记，置目标 `running`；
2. 全部检查项编译为 Agent 检查计划（`check_plan`），通过 gRPC
   `check_application_baseline` 下发，超时 = 任务超时 + 45s；
3. **Agent 离线/未注册 → 目标置 `skipped`（"未执行"而非"检查失败"）**，不产生检查结果，
   error_message 记 "Agent 离线，未执行巡检"；全部目标 skipped 时 execution 状态为
   `skipped`（不算失败），summary 单独计 `skipped` 数；
4. 结果按 `inspection:{execution}:{index}` 关联回快照中的检查项取得 severity；
5. **结果批量落库**：`insertResults` 以 100 行/批 multi-value INSERT 写
   `inspection_result`（原逐行 INSERT 是大规模执行的尾延迟瓶颈）；批量写入失败时目标
   置 failed，error_message 追加写入失败原因；
6. 判定：critical 级检查项非 pass/skipped → 目标失败；warning 只计入汇总；全部目标
   完成后聚合 execution 状态：failed>0 → failed；success=0 且有 canceled → canceled；
   success=0 且有 skipped → skipped；success=0 → failed。

### 历史数据保留（最终逻辑）

- 调度器循环内每 24h 执行一次保留期清理：删除 `end_time` 早于保留期且状态非
  pending/running 的 execution 及其 target_execution、inspection_result（先删子表，
  仅 `inspection_result` 有物理外键）。
- 保留天数读 `sys_config` 的 `inspection.results.retention_days`，缺省 180 天；配置
  缺失或非法时用缺省值，不会中断清理。

> **设计决策：巡检不做告警联动。** 巡检是低频核对（内容基本不变，偶尔跑一次确认），
> 持续性异常发现是监控告警（monitor，Prometheus → alertmanager webhook）的职责，
> 两者不混用。巡检结果只在执行记录页查看，失败不外发通知。

### 失败语义

- Agent 返回计划级 error（版本不支持 / 能力不支持 / 结果格式无效）会让该目标失败；
  goss 释放失败同语义；结果批量落库失败同样置目标 failed（error_message 带原因）。

## 与 Django 双实现的差异（重要）

Django 后端（`backend/djadmin/inspection/`，已冻结不再修改）仍保留 `shell` / `http` /
`tcp` 执行器与 `controller` 执行位置，**两者语义不再对齐**：

- Go 后端：`schema_validate` + `goss`，仅 Agent 执行；Agent 端能力同步调整。
- Django 后端：四种旧执行器 + 双执行位置；其下发的 `shell` / `http` / `tcp` 检查计划
  会被新版 dj-agent 以"不支持的能力/执行器"拒绝。
- 数据库 `inspection_check.executor` / `execution_location` 列两侧共用，历史脏数据
  （旧执行器）在 Go 侧保存时被校验拒绝，执行时表现为计划级 error。
