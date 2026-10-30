# 自动化作业执行（autoadmin）

描述作业（`automation_execution_job`）从派发到终态的最终逻辑：**在哪跑、怎么超时、失败/失联如何收敛、
输出什么时候可见**。后端唯一实现为 `internal/automation`（执行入口 `executeLocalAnsible`，
收尾 `finishJob`，对账 `stale_job_reaper.go`）。Inventory 与 Playbook 的 CRUD 语义见
[Inventory 与数据访问](#inventory)。

## 状态机

```
pending ──(worker 消费/内联派发)──> running ──┬──> success
                                             ├──> failed      （ansible 退出码非 0 / 超时 / 对账判定失联）
                                             └──> cancelled    （用户取消：仅改状态，不杀进程）
```

- `start_time` 进入 running 时写入；`end_time`/`duration_seconds` 由应用层算（`TIMESTAMPDIFF` 是方言函数）。
- 收尾语句带 `status <> 'cancelled'` 守卫：取消后到达的收尾不再覆盖取消结论。

## 任务删除与历史保留

`POST /sys/automation/tasks/batch-delete/` 逐任务开启事务，先锁定任务父行，
再锁定读关联作业。仅当所有作业为 `success`、`failed` 或 `cancelled` 时，才把历史
`task_id` 置为 NULL 并删除任务定义；快照、逐主机结果和日志均保留。其他状态拒绝删除，
返回失败项的作业 ID/状态。任何数据库步骤失败都会回滚，不会只解除关联却保留任务。

父行锁与作业插入的外键检查互斥：并发提交先完成则被删除检查看到，删除先完成则旧任务的
新作业插入失败，不会在检查后漏入新作业。MySQL 作业检查使用锁定读，避免一致性快照漏看新提交。
不改变表结构和外键，也不级联删除历史；存量有历史的任务适用同一流程，无需数据迁移。
删除后历史按快照名称/作业 ID 查询，原任务 ID 筛选不再命中已解除关联的作业。
取消只代表记录终态，不保证外部进程退出；删除不会取消远端副作用。
前端必须核对批删逐项结果，失败项展示原因，不提示删除成功。

## 派发方式：手动"立即执行"是后台异步的

`POST /sys/automation/tasks/:id/run_now/`（`RunTaskNow`）创建作业后**立刻返回**，
作业由独立 goroutine 在后台执行（`dispatchJobAsync`，外层 6 小时兜底超时），
前端拿作业 id 去运行记录中心看状态与日志。

**为什么不在请求里同步跑**（2026-09-18 作业 #831 的成因）：原先 `run_now` 用
`context.Request.Context()` 同步执行整趟 ansible。多主机作业要跑几分钟，请求一旦中断
（浏览器关闭、网关超时、服务重启），请求的 context 就被取消，连锁反应是：

1. `commandCtx` 被取消 → ansible 的 controller 被 SIGKILL（它的 `--forks` 子进程变成孤儿，
   并卡在"输出管道无人读取"上）；
2. 临时目录**被 defer 清掉了**（所以现场看起来"跑过"）；
3. 但收尾那几条 UPDATE 用的是**同一个已取消的 context** → 写入失败 → **作业永久停在 `running`**。

因此定了两条规则：

- **派发脱离请求 context**：作业一旦派发，请求结束/取消都不再影响它。
- **收尾写入用不可取消的 context**（`persistenceContext` = `context.WithoutCancel`）：
  cancel 只用来中断执行本身，不用来中断结果落库。这两条缺一不可——只做第一条，
  服务重启仍会把正在跑的作业卡在 `running`。

监控域的安装派发（Export/Filebeat）本来就是"脱离请求 + goroutine"，此处与之统一。

## 在哪跑：本地 ansible（不是 agent 侧）

作业由控制端**本地**执行：嵌入式 CPython + ansible-core（`-tags embedansible`，见
[ansiblecmd](DJ_AGENT_ARCHITECTURE.md)）或宿主 PATH 上的 `ansible-playbook`。
一次派发对应**一次** `ansible-playbook` 调用，多主机靠 `--forks min(10, 主机数)` 并行。

- 临时目录 `/tmp/autoadmin-ansible-<随机>/`：`inventory.ini`、`playbook.yml`、`controller_key`、`known_hosts`。
- inventory 用 `ansible_user=root` + 控制端私钥 SSH 直连；`StrictHostKeyChecking=accept-new`。
- **主机别名是 `<主机名>(<IP>)`**（`inventoryHostLabel`）：ansible 的 task 输出与 PLAY RECAP 都用别名
  标识主机，用它才能一眼看出是哪台机器（早先的 `host_<id>` 在日志里无法对应到服务器）。
  别名只保留 INI 安全的字符（空格会把它拆成 inventory 变量），非 ASCII 名（中文）回落 `host-<id>`，
  同名同 IP 的重复记录加 `#<id>` 后缀——**别名必须唯一**，否则 ansible 会把它们并成一台、任务只跑一次。
  别名只由后端生成、不被任何代码解析，因此改它只影响展示。
- 子进程按**独立进程组**运行（`Setpgid`），并在返回前按组回收，见下文"孤儿进程"。

## 模板形态：Playbook 与 Shell 脚本（2026-09-21）

`automation_playbook_template.content_format` 区分两种模板形态，`content` 始终存**原始内容**：

- `playbook`（默认）：标准 YAML 全文，保存/校验沿用「非空 + 合法 YAML + 非空 plays 列表」的结构校验。
- `shell`：裸 bash 脚本。保存/校验走 **ShellCheck**（覆盖 `bash -n` 的语法检查 + 引号/变量等静态分析）：
  - **就绪方式二选一**，解析顺序 `上传优先 → PATH`：
    1. 平台上传：`POST /sys/automation/shellcheck/binary/`（multipart 字段 `file`）把二进制存到
       `<工作目录>/media/shellcheck/shellcheck`（0755，覆盖式），离线内网环境的就绪通道；
    2. 服务器安装：`yum/apt install shellcheck`（PATH 兜底）。
    状态接口 `GET /sys/automation/shellcheck/binary/` 实时探测（`source=uploaded|system` + `--version`）；
    `DELETE` 删除上传的二进制回退到 PATH。都没有时校验返回明确指引文案，不落库。
  - **判定**：ShellCheck `error` 级（语法错误等）拒绝保存；`warning/info/style` 放行，并随响应
    `warnings` 带回前端黄条提示（`apply/validate` 同一口径，前端保存前会调 `playbooks/validate/`）。
- **执行时包装**：shell 形态在 `runAutomationJob` 渲染快照时由 `wrapShellPlaybook` 包装成单任务
  playbook（`hosts: all` / `gather_facts: false` / `ansible.builtin.shell` 原文 + `executable: /bin/bash`），
  再走既有 `executeLocalAnsible`。**身份与变量不额外处理**：inventory 的 ssh 用户默认 root；
  任务的 `run_as_user` 由既有 `--become --become-user` 切换；参数沿用 `env_vars`/`--extra-vars`。
- **快照语义不变**：`automation_execution_job.template_content_snapshot` 存原始内容，
  `template_content_format_snapshot` 存形态；改模板形态/内容不影响在途作业（包装只在执行时发生，幂等）。
- agent 安装专用模板（`category=agent`）等内容由系统维护的模板仍按对应流程校验，不受此影响。

## 超时语义

| 项 | 值 |
|---|---|
| 来源 | 任务上的 `execution_timeout_seconds`（作业行只存 `task_id`，读取时 `COALESCE(...,600)`） |
| 取值域 | 1–14400 秒，写入任务时校验 |
| 缺省 | 600 秒（任务未配置或作业无关联任务） |
| 强制方式 | **执行进程内的 `context.WithTimeout`**，超时后返回退出码 **124**，摘要追加 "Playbook execution timed out." |
| 落库 | 超时 → 作业 `failed`（`finishJob` 按 `failed>0 || code!=0` 判定） |
| 后台派发外层上限 | 6 小时（`jobDispatchTimeout`），仅防 goroutine 意外永久挂住 |

⚠️ **这是进程内超时**：执行进程一旦消失（被 kill、崩溃、worker 重启），它永远不会触发——
所以必须有下面对账兜底。

## 失联作业对账（stale job reaper）

**问题**：执行进程消失后作业会永久停在 `running`，界面上「查看日志」一直显示"等待新输出"，
没有任何东西把它收敛（2026-09-18 作业 #831 即此情形：执行进程消失，留下 8 个孤儿 ansible 进程）。

**判定口径**：`running` 且 `start_time` 超过"作业自身超时 + **2 分钟余量**"。
余量刻意给足——正常路径下进程内超时会先把作业置成 failed，对账只在执行进程确实不在时命中，
不会误伤跑得慢的作业。

- 扫描周期 1 分钟，由 **worker 模式**启动（`StartStaleJobReaper`，单实例部署、进程内唯一 goroutine，
  随进程退出终止；与监控域失联告警对账同一范式）。
- 置失败走 `FailStaleAutomationJob`，带 `status='running'` 守卫——正常收尾可能同时在写，
  没有守卫会把刚成功的作业改写成失败。
- 摘要写明"执行进程已失联、实际输出可能已丢失"，并有 `execution_mode: stale_reaper` 标记，
  便于与正常失败区分。

## 输出与作业日志

作业的可见文本分两段，日志 WebSocket（`GET /ws/automation/jobs/:id/logs/`）按作业状态自动选源：

| 阶段 | 数据来源 | 说明 |
|---|---|---|
| 运行中（非终态） | `automation_execution_job_log` 的**实时块** | 边跑边看；块为空时前端显示"等待新输出" |
| 终态（success/failed/cancelled） | `automation_execution_host_log` 的**按主机结果行** | 每台一行，带退出码与 stdout/stderr |

> **stdout 回调固定为 minimal**（`ansiblecmd.CommandContext` 注入 `ANSIBLE_STDOUT_CALLBACK=minimal`，
> 用户显式设置时不覆盖）：ansible 默认回调只打印每台主机的 `ok/changed` 状态行，**不显示
> shell/command 任务成功时的 stdout**，表现为"脚本明明 echo 了、作业日志里什么都没有"（2026-09-21
> 现场）。minimal 把 stdout/stderr 直接跟在 `host | CHANGED | rc=0 >>` 之后，既有内容又不引入
> `-v` 那种整份任务结果字典的噪声。宿主 PATH 与嵌入 ansible 两个构建变体行为一致。

- **实时块**：`executeLocalAnsible` 用管道读 ansible 的 stdout/stderr，边写内存缓冲（供结束时的按主机行）
  边按块落库——攒够 4KB 或间隔 1.5s 刷一次（`liveLogStreamer`）。写失败只丢弃该块：
  实时输出是"尽力而为"的展示，不影响作业本身。
- **块的清理**：作业收尾时由执行方删除（`DeleteAutomationJobLogChunks`）。终态后日志视图改读按主机
  结果行，留着只会让同一份 ansible 输出重复展示。执行进程没来得及清（失联作业）时由对账补清。
  因此这张表只在"运行中"有数据，没有留存/清理负担。
- **多主机作业的输出仍会重复**：收尾写按主机行时，每台都把同一份 ansible stdout 存了一份，
  日志视图最终会把整份输出按主机重复展示。这是既有格式（便于按主机看退出码与错误），
  未做重构。

## 孤儿进程：为什么必须按进程组管理

只杀 `ansible-playbook` 的 controller 是不够的：`--forks N` 会 fork 出多个工作进程，它们还会拉起 ssh。
只杀 controller 会让这些子进程变成孤儿（PPID=1），并**卡在"输出管道已无人读取"上永不退出**——占着
到生产主机的 SSH 连接。据此定了三条规则（`isolateProcessGroup` + `runInProcessGroup`）：

1. `SysProcAttr.Setpgid` 让 ansible 及其 fork 处在**独立进程组**（组长即 controller，pgid == pid）。
2. `Cancel` 改为按**整组**杀（`kill(-pgid)`），让超时/取消能迅速清场。
3. **返回前无条件再按组补一刀**：覆盖"命令正常结束但留下子进程"这条 Cancel 覆盖不到的路径
   （典型是 ansible 跑完仍有挂住的 ssh）。正常结束、超时、被杀三条路径都收口。

另外设了 `WaitDelay = 10s`：进程被杀后若仍有子进程持有 stdout/stderr 管道，`Wait` 会一直等 I/O
结束而挂住；有兜底期限才能保证 `Run` 一定返回。**但 WaitDelay 到期返回的是 `ErrWaitDelay`**，
若此时进程本身已退出且退出码为 0（ansible 其实成功了），不能因为一个残留子进程把作业判失败——
`runInProcessGroup` 因此按真实退出码判定，把"成功 + 残留子进程"归一为成功。

## 取消语义

`CancelJob` **只更新数据库**（`status='cancelled'`、写 `end_time`/时长、摘要 "Cancelled by user"），
不会去杀已派发的进程。对已经失联的作业，取消是最快的止血手段：DB 状态一变，
日志视图立刻从"等待新输出"变成"已结束（已取消）"。

## 权限点：任务 CRUD 与作业执行是两套（2026-09-21 补）

| 动作 | 接口 | 权限码 |
|---|---|---|
| 任务 增/改/删 | `POST/PATCH/PUT/batch-delete /sys/automation/tasks/*` | `automation:tasks:create` / `:update` / `:delete` |
| 任务查看 | `GET /sys/automation/tasks/*` | `automation:tasks:view` |
| 立即执行 / 预检 | `POST tasks/:id/run_now/` / `GET tasks/:id/precheck/` | `automation:jobs:create` |
| 运行记录 | `/sys/automation/jobs/*` 等 | `automation:jobs:view` / `:cancel` |
| 模板 CRUD | `/sys/automation/playbooks/*` | `automation:playbooks:*` |
| ShellCheck 组件 | `/sys/automation/shellcheck/binary/` | 读 `automation:playbooks:view`、写 `automation:playbooks:update` |

任务预检与 Inventory 范围预检均为只读 GET：`/sys/automation/tasks/:id/precheck/` 与
`/sys/automation/inventories/:id/precheck-limit/`。`host_ids` 查询参数使用逗号分隔的正整数；
未传或为空时使用 Inventory 默认主机集合，`limit` 保持 Ansible 筛选语义，任务未指定时沿用
任务默认 limit。非法 ID 返回请求错误，不能退化为全量；预检不创建作业、不下发执行。
Inventory 预检沿用 `automation:inventory:view` 权限，任务预检沿用执行权限。

迁移 000047 补齐了此前 `sys_menu` 里缺失的 `automation:tasks:create/update/delete` 三条权限点
（只有 `tasks:view`，导致非 admin 保存任务 403「无权限访问」），并把误命名为「任务创建」的
`automation:jobs:create` 行改名为「任务执行」。补授权给已拥有 `tasks:view` 的角色；
权限码随 JWT 签发，存量用户需**重新登录**后生效。

## 任务可选模板范围：只列「通用」（2026-09-21）

任务表单的模板下拉按 `category=general` 拉取（`loadPlaybooks`），只允许选通用模板。
`software_package`（监控软件仓库的安装/卸载）与 `agent`（Agent 安装/更新）是系统托管模板，
由各自域按分类定位并派发，带自己的参数与生命周期；在自动化任务里手动选它们会绕过这些约定。
模板管理页仍可查看/编辑这些分类，只是不进入任务选择列表。

任务列表采用固定表格布局，任务名、Inventory 名和模板名保留现有列宽，
名称单行省略并通过顶部 Tooltip 展示完整值，不允许长名称溢出遮挡相邻的执行超时或状态列。
有编辑权限时任务名仍打开任务编辑；Inventory 和模板名称仍跳转到对应资源，
无编辑权限时任务名显示只读文本，但同样支持省略和完整名称提示。
右侧操作列固定并保留明确宽度，表格维持横向滚动；名称提示不改变操作列按钮或权限。
模板列显示固定 16px 的类型图标加纯模板名称：Playbook 为代码文档图标，Shell 为终端图标，
图标不参与省略，剩余宽度用于名称；顶部 Tooltip 展示 `类型 · 完整模板名`。
类型取任务列表接口的 `template_content_format`（`playbook` / `shell`；无关联模板时为 `null`），
随同原有模板 JOIN 一次返回，不按名称前缀猜测，不依赖模板选择器的分页或额外逐条请求。
未识别类型显示问号图标与「未知类型」提示，无模板时显示 `-`，不虚构为 Playbook。
接口现有 `template_name` 内容约定保持不变，列表展示与模板跳转共用名称提取逻辑，
仅移除接口附加的一层 `[Playbook]` 前缀，不修改模板保存名称；无数据库结构变更或迁移。


<a id="inventory"></a>

## Inventory 与数据访问

描述 `automation_inventory` 的 CRUD 语义。后端唯一实现为 Go 版 autoadmin（`internal/automation/runtime.go`）。

### API 语义

- `GET /sys/automation/playbooks/host-options/`：返回统一分页主机选项；
  `instance_name`、`hostname`、`ip` 为 JSON 字符串或 `null`，`group_id` 为数字或 `null`，
  不暴露 Go 数据库可空类型的 `String/Valid` 等内部字段，空字符串与 `null` 保持区别。
  Inventory 范围编辑与只读范围树共用该接口，主机标签显示实例名及 IP；
  无实例名时显示 IP，搜索和资产主机跳转沿用相同名称。此读接口不修改数据库结构。
- `POST /sys/automation/inventories/`：全量创建，`name` 必填；`enabled` 缺省 true，`update_on_launch` 缺省 false，`update_cache_timeout` 缺省 300 且必须非负。
- `PUT/PATCH /sys/automation/inventories/:id/`：**支持部分更新**——未提供的字段保留原值（name/remark/selected_host_ids/enabled/update_on_launch/update_cache_timeout 逐字段合并后整体 UPDATE）。
  - 前端列表"启用状态"开关只发 `{enabled}`，不携带 name，必须走部分更新（此前因全量校验 `name is required` 报错，已修复）。
  - 更新后返回完整资源（`inventoryByID`）。
- `GET`：单条/列表；删除只提供 `POST /sys/automation/inventories/batch-delete/`（body `{"ids":[...]}`），不提供单条 `DELETE`。

### 关键决策

- 部分更新在应用层合并（先 SELECT 现值再整体 UPDATE，`resolveInventoryUpdate` 纯函数，见 `runtime_inventory_test.go`），避免动态拼 SET 子句；`selected_host_ids` 以 JSON 数字数组列存储，合并时经 `decodeJSONInt64Array` 解析原值。
- 列表装饰从 JSON 解码后的数字数组经 `intSlice` 提取主机 ID，再用
  `CountAutomationInventoryHosts` 计算已存在/可用主机数及分组数；健康状态不按数组长度猜测。
- name 语义：显式传空 name 与未传等价，均按"保留原值"处理；仅当现值 name 也为空（脏数据）且未提供 name 时报 400 `name is required`。
- `update_cache_timeout` 在两侧方言里都是 NOT NULL（已对真库核实），所以"存量 NULL 回退 300"这条分支在实现上不可达：默认 300 只在新建或客户端省略该字段时生效，`0` 是合法值且必须原样保留（不能被当成"未设置"覆盖成 300）。

### 数据访问层（sqlc，2026-09-16）

本域（Playbook 模板 / Inventory / 任务 / 作业与主机日志 / 目标主机解析）的**内联 SQL 已清零**：
全部语句定义在 `autoadmin/db/queries/mysql/automation.sql`，Go 侧只调
`internal/platform/database/generated`（方言门面，默认 MySQL、`-tags postgres` 走 PostgreSQL）。
选 sqlc 还是内联、方言可移植规则见 [SQL_DESIGN.md](SQL_DESIGN.md)，这里只记本域的最终逻辑。

- **SQL 归属**：模板 CRUD/列表（含计数与可选过滤）、Inventory CRUD、任务 CRUD 与开关、
  主机选项与主机组树、作业派发/认领/收尾/取消、主机日志写入与读取、控制器 SSH 密钥的
  读改、目标主机快照与聚合计数，共 31 条定义（另有 4 条复用已有定义：
  `GetInventoryTyped`、`GetAutomationPlaybook`、`GetConfigByKey` 等）。
- **列表过滤与排序**：模板列表的搜索/分类过滤用 `sqlc.narg`（NULL 表示不过滤），
  排序由 `sort_key` 参数选择 CASE 表达式（`id`/`name`/`create_time`/`update_time`，
  带 `-` 前缀表示倒序；非法值回落到 `id DESC`）。代价是该表排序不再走索引（表很小，可接受），
  收益是标识符不再由 Go 拼进 SQL。
- **时长统计**：作业的 `duration_seconds` 由应用层计算（取消时先读回 `start_time`，
  为 NULL 则按"现在开始"即时长 0），写入的秒数带小数（列类型是 `double`）。
  迁移前用 MySQL 的 `TIMESTAMPDIFF(MICROSECOND,…)/1000000`，是整数截断且 PG 无法执行。
- **主机解析**：Inventory 的主机集合是可变长的，查询写 `IN (sqlc.slice(host_ids))`，
  派生时 PG 侧改写成 `= ANY($1::bigint[])`（见 SQL_DESIGN §4.2 的更正与计划 P4-7）。
  这条路径此前在 PG 下会因为占位符与参数个数不符而报错，调用点没检查错误时表现为"解析到 0 台主机"。
- **存在性校验**：模板/Inventory 的存在性用取行查询的 `sql.ErrNoRows` 判定（不再 `SELECT COUNT(*)`）。
- **作业来源（`automation_execution_job.source`）**：区分作业由谁派发，取值 `manual`（普通任务，
  `CreateAutomationJob` 显式写）、`agent_install`（Agent 安装/更新，`assets.CreateAgentExecutionJob`）、
  `monitor_target`（exporter/Filebeat 安装，`monitor.CreateMonitorTargetJob`）；列默认 `manual`。
  `CountJobs`/`ListJobsTyped` 支持可选 `source` 过滤（`sqlc.narg`）。运行记录中心是单页无 tab，只展示
  自动化任务运行记录，来源列 + 来源过滤用于区分普通任务 / Agent 安装 / 监控安装；监控安装历史不再单独占 tab，
  纳管目标「查看日志」用历史行的 `automation_job_id_snapshot` 跳到对应作业。
- **验证**：`internal/automation/smoke_test.go`（`AUTOMATION_SMOKE_DSN`）在真 MySQL 与真 PG 上
  跑一遍上述写路径（模板/Inventory/任务 CRUD、作业派发-认领-收尾-取消、主机日志、多值 IN 与聚合计数、
  控制器密钥三条语句）；`scripts` 级的用法写在文件的函数注释里。会改动全局数据的语句
  （控制器密钥替换）只在回滚事务里验证。
