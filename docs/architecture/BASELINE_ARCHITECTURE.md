# 安全中心 / 基线扫描架构（Go 后端 autoadmin）

> 状态：一期已上线（基线管理 + 扫描执行 + 符合率报告）。二期规划：豁免登记、
> 与上次对比、CSV 导出；后续可扩展 CVE/补丁等扫描类型（`security_scan.scan_type` 预留）。

## 定位与边界

- 基线扫描 = **OS 配置合规核查**：按基线标准（等保 2.0 / CIS / 自定义加固项）对主机
  逐项核查，产出符合率与不符合清单。只针对**操作系统**，与应用无关；
- **执行内核复用巡检**：条目即 OPA 检查项，下发通道
  （check_plan → Agent）、挂载目标解析（项目/项目×环境主机集合）、批量落库全部复用；
  OPA 策略条目的采集命令在扫描时以目标主机上下文（host_ip/host_name）展开变量。
- **不与巡检混表**：结果独立（`baseline_scan_result`），合规聚合（符合率/章节）是
  基线特有语义；
- 不联动告警（与巡检同纪律）；不做趋势图表（二期做"与上次对比"）。
- 新扫描同步维护独立[安全问题台账](SECURITY_ISSUES.md)，不回填旧报告。
  问题以主机与稳定条目 ID 唯一归并，明细与历史保存 `rule_version` 内容版本；
  报告、问题、历史、目标终态同事务写入，跨版本通过明确区分于原规则复查通过。
  检查错误、缺失结果及空通过断言记录 error，不映射为不符合或有效通过，
  不创建违规问题、不关闭已有问题。详情单独展示执行异常。

## RHEL7 参考策略的严格判定

参考定义与采集/OPA 回归位于 `dj_agent/internal/executor/rhel7_baseline_policies_test.go`
和 `rhel7_baseline_test.go`。设置 `RHEL7_BASELINE_EXPORT` 为尚不存在的输出路径并运行
`make -C dj_agent test` 可导出定义，以供经过审核的数据库策略更新；
不在进程启动时覆盖用户配置，也不因升级自动放宽标准。编辑保持原条目 ID，
新扫描读取新配置；历史报告与证据不重算。仅更新策略 JSON，无 schema 迁移。

- **口令有效期**：按精确键读取 `PASS_MAX_DAYS`，支持 Tab、空格及注释；缺失、非数字、
  超出 `0..90` 均不符合。条件规则显式默认 `false`，不能让失败分支丢失断言。
  同时检查本地 passwd/shadow 中可登录账号，包括低 UID 账号和 root；仅排除 shell 为
  `false`/`nologin` 的账号。锁定口令不要求过期，空口令、无上限及超期配置不符合。
  只输出账号名、口令可用性和最长有效期，不输出、落盘复制或持久化 shadow 哈希。
  这是最长有效期配置核查，不是“当前口令已经过期多久”的核查；不证明远程 LDAP/AD 账号策略合规。
- **口令长度**：从 passwd/sshd 各自的 password 入口检查，可直接使用本地模块栈，
  或逐层沿唯一的 include/substack 委托查找实际模块栈，不要求固定配置文件名。
  Debian/Ubuntu 样式的 `@include` 会检查引用文件，只在最终解析出唯一的 password 栈时继续。
  RHEL 的 `passwd` 入口允许 `system-auth` 子栈之后接可选的 GNOME Keyring `use_authtok`
  和 `postlogin` 子栈，但须确认 `postlogin` 未引入任何 password 处理。
  首个 password 模块须为强制的 pam_pwquality，
  root 不豁免且 enforcing 启用，后续单个 pam_unix 必须 `use_authtok`，不允许再次提示一个未校验的新口令。
  模块参数覆盖 `pwquality.conf`；按 minlen、正负字符积分及 minclass 计算长度保证下界，
  下界须至少 12。缺少显式 minlen/强制模块等已知不合规条件返回不符合。
  对已知的 Debian 栈（`pam_unix` 跳过 `pam_deny` 后进入 `pam_permit`，无质量模块），
  即使末尾有可选的 GNOME Keyring 模块，也判定为不符合；不推断其他跳转写法。混合委托、循环引用、其他扩展控制语法、未知 password 模块、
  配置片段及不能证明的入口/顺序均返回**无法验证**。参考策略仅覆盖可解析的本地 PAM 链，不代表所有 PAM 服务、
  远程身份源或直接修改口令数据库的路径均已验证。
- **SSH**：由 `sshd -T -f /etc/ssh/sshd_config` 原生解析默认磁盘配置，保留其关键字大小写、
  空白、注释、重复配置首值及默认值语义，避免自己重写 SSH 解析器。
  全局 `Include` 支持标准 `sshd_config.d/*.conf`；片段可进一步引入 Red Hat 的
  `/etc/crypto-policies/back-ends/opensshserver.config`，但被引入的文件必须可读，且无条件块和进一步引入。
  主配置含 `Match` 时，仅在当前待检查字段未被条件覆盖时采用全局值。
  其他引入路径、条件内引入、片段内条件块或待检查字段被覆盖均返回无法验证，
  不能只跑默认 `-T` 后宣称所有用户/来源均安全。本项不证明当前运行进程已重载磁盘配置，
  也不证明以其他 `-f`/`-o` 参数运行的 sshd 实例合规。
- **文件权限**：使用 `stat -L` 观测目标的模式及 UID/GID，要求 root:root。
  shadow 只接受 `000/0400`（含 stat 的 `0/400` 表示），passwd 为 `0644`，
  sshd_config 为 `0600`，不以 `0600` 放宽 shadow 标准。
  检查模式包含本地 POSIX ACL mask 的权限约束，不代表检查了所有 ACL/远程文件系统语义。
- **内核参数**：观测当前值；ICMP 重定向按协议拆分为两个独立条目：
  IPv4 检查 `accept_redirects` 与 `send_redirects`，IPv6 仅检查 `accept_redirects`，
  不要求不存在的 IPv6 `send_redirects`。两项分别覆盖对应协议的 `all`、`default`
  和全部当前接口，所有值必须为 `0`；IPv6 的 `1/2` 均为不符合。
  协议参数目录不可用、缺少必要观测、读取竞态或数据损坏返回无法验证，不跳过接口或默认通过。
  原 ICMP 条目 ID 12 保留给 IPv4，IPv6 使用独立条目 ID 25；新扫描按两项分别维护问题，
  不改写历史扫描与证据。当前内核值通过不代表配置已持久化到重启后，
  仅写入 `/etc/sysctl.d/` 也不代表当前内核值已经生效。
- **服务状态**：通过 `systemctl show` 一次取得 LoadState、ActiveState、UnitFileState，
  要求 `loaded/active/enabled`；仅运行中或仅 runtime 启用不算持久启用。
  明确的 `not-found` 为不符合，D-Bus、权限、命令、属性不完整等采集错误返回无法验证，
  不再使用 `|| true` 吞掉错误。不证明审计规则、远程日志投递或 NTP 实际同步正常。

扫描详情对 error 优先显示异常原因，不显示“违规 0 项”或部分成功断言作为结论；
条目列表仅展示检查摘要和最多三行修复建议，不在窄列内逐条堆叠断言或 JSON。
表格使用固定布局与按可见列宽计算的横向滚动，展开行展示完整消息、修复建议，
并以独立断言表对照检查项、预期、实际与结论；对象证据格式化为 JSON，可在限定高度内滚动。
执行异常不展示部分断言的通过/不符合结论。完整预期与实际证据保留在展开行的“原始证据”折叠区，
不截断、不改写扫描数据。基线编辑器明确空断言不能验证通过；不改变巡检域已有空集契约。
采集仅观察，不运行 chmod、chage、sysctl 写入或服务启停/重载命令。
回归使用隔离文件和 systemd 替身；SSH 另用临时测试主机密钥验证原生 `sshd -T`，
不启动监听、不读取已有主机私钥。

## 领域模型

| 表 | 内容 |
|---|---|
| `baseline` | 基线标准（name 唯一 / version / description / enabled） |
| `baseline_category` | 类目：策略的一级分组实体（迁移 000011 起），name + sort（上移下移交换 sort），FK→baseline ON DELETE CASCADE |
| `baseline_item` | 策略：`category_id` FK→baseline_category、`config`（OPA 策略，与 inspection_check 同构）、`severity`（high/medium/low）、sort；`chapter`/`executor` 列已删除（迁移 000009/000011） |
| `security_scan` | 统一扫描任务：`scan_type`（`baseline`，预留 `cve` 等）+ 范围（mount_type=host/project/environment；host 模式 project_id/environment_id 为 NULL）+ status + summary |
| `security_scan_target` | 每主机执行状态：passed/failed/compliance_rate/error_message |
| `baseline_scan_result` | 策略结果明细：scan × host × item，`chapter` 为**快照字段**（扫描时以类目名填充，类目后续改名不影响历史结果）+ expected/actual/message/remediation。expected=agent OPA 检查返回的 `{query, policy}`；actual=`{violation_count, violations, inputs, details, run_user}`；remediation=修复建议快照（文案本体存 `baseline_item.config.remediation`，策略编辑弹窗录入，迁移 000012 加列）。历史上 expected 曾未采集恒为 null，修复后新扫描正常写入 |

注意：`security_scan_target.error_message` NOT NULL 无默认——INSERT 必须显式给 `''`。

## 数据访问（sqlc，2026-09-16 起）

发起扫描有两个独立分支，默认“按主机”（`mount_type=host`），直接从全部资产主机中多选，
主机选择器留空表示全部主机，不要求项目或环境，接口拒绝混入项目/环境参数。
“按项目”分支选择项目及可选环境，扫描解析出的全部主机，不展示主机多选。
`GET /sys/security/baseline/scan-hosts/` 通过查询参数传递范围，使用与执行相同的
范围参数返回 `hosts: [{id,name,ip,online}]`，受 `baseline:scan` 权限保护，不创建扫描。
切换扫描方式或项目/环境立即清空主机选择并重新加载；加载失败或范围为空禁止提交。
按主机分支不依赖项目/环境资产选项加载，不限定项目或环境；
详情中的“主机环境”仍读取各主机当前所属环境，与扫描范围选择方式无关。
扫描请求的 `host_ids` 缺省或空数组表示全部，非空则仅扫描这些主机并去重；
提交时在创建事务内重新解析范围，任一 ID 不属于当前范围（含已删除主机）均明确拒绝，不扩大为全部。
离线主机仍可选择，沿用执行阶段的离线错误语义。目标记录与范围快照只保存实际扫描主机，
不新增表、列或外键。

扫描详情 `GET /sys/security/scans/:id/` 查询时，通过扫描目标的主机 ID 左关联
`assets_host.environment_id` 与环境名称；唯一环境来源是主机自身的“所属环境”，
不采用应用服务环境、扫描选定环境或历史快照中的环境值。按主机、项目、项目×环境扫描，
以及已有和新建扫描均使用同一查询路径。主机当前环境修改或环境改名后，刷新详情即更新展示，
不回写报告、扫描范围快照或安全问题历史，也不改变实际扫描目标、检查结果和符合率。

接口保留 `environments: [{id,name}]` 数组形状，每台主机只有零个或一个当前环境；
主机表与条目表共用同次目标查询得到的映射，概要环境清单按实际扫描主机去重排序。
主机符合率和条目明细工具栏均提供环境筛选，共用同一筛选状态并双向同步；
选择具体环境、未设置环境或清除条件时同时过滤两张表，重置分页和失效主机条件；
刷新后也清除已不存在的环境筛选。
主机未设置环境或原主机已删除时保留扫描目标和结果，显示并可筛选“未设置环境”；
环境 ID 存在但名称不可用时展示 ID 和“已删除或不可用”。关联查询失败明确返回错误，
不能当作所有主机都未设置环境。`environment_source=current_assets` 表示当前主机资产归属，
页面显式提示“非扫描时快照”，与扫描范围的项目/环境条件区分。

迁移 `000058_scan_scope_snapshot` 在 `security_scan` 增加可空 JSON（PG 为 jsonb）`scope_snapshot`，
不新增外键，不修改历史结果。新扫描创建时，在同一事务内保存项目名称、环境 ID/名称清单、
按主机 ID 索引的环境集合；只采集解析出的目标，服务和成员启用条件与扫描范围解析一致。
快照保留扫描创建时的服务范围上下文，项目名称有快照时仍读取快照，无快照时使用当前项目名称；
主机环境展示不读取其中的环境清单或主机环境集合，也不再按当前服务关系补充历史环境。
扫描记录上的 `project_id/environment_id` 仍表示发起时的范围条件，不冒充主机当前归属。
快照损坏仍明确报错。此查询调整不新增迁移、不修改已有快照；
迁移 down 会删除范围快照，回退前应备份该列。

扫描记录页进入时读取当前分页；本页存在 `pending` / `running` 扫描时，在请求结束后
间隔 3 秒刷新状态和汇总，全部进入终态后停止轮询。切换至基线标准、离开缓存页面或卸载时停止；
从详情或其他页面返回扫描记录时立即重新读取。翻页和手动刷新仍保留当前分页，
较旧的异步响应不能覆盖新页结果；请求失败明确提示，不将失败当作扫描已完成。

本域**没有任何内联 SQL**：全部语句在 `db/queries/mysql/baseline.sql`（唯一人工维护源），
PostgreSQL 一套由 `make derive` 从它派生（`db/queries/postgres/baseline.sql`，禁止手改），
两侧由 `make generate` 各生成一份（见 [SQL_DESIGN.md](SQL_DESIGN.md) §4.6）。
迁移落地时确定的三条实现约定：

| 约定 | 原因 |
|---|---|
| **响应体在应用层组装**，查询只取列 | 原来 `GetScan` 用 `JSON_OBJECT` 在库里拼响应、`CancelScan` 用 `JSON_MERGE_PATCH` 合并 summary，都是 MySQL 专有函数；PG 侧改为应用层拼装（取消时在同一事务内 `SELECT … FOR UPDATE` 读回 summary 再合并） |
| **可变长 `IN (?,?)` 改为"取回集合在应用层比对"** | `PATCH /{id}/` 的类目归属校验原先按提交的类目个数拼占位符，是 sqlc 表达不了的形状；现在取回该基线的全部类目（`ListBaselineCategories`）在 Go 侧判包含 |
| **排序/聚合不用方言函数** | 明细排序用 `CASE r.status WHEN 'fail' THEN 1 WHEN 'pass' THEN 2 ELSE 0 END`（等价于 MySQL 的 `FIELD()`）；终态计数用 `COUNT(CASE WHEN … THEN 1 END)`（不写 `SUM(布尔)`，PG 不接受） |

两处方言差异由派生层消化，调用点不分叉：

- **自增主键**：MySQL 源写 `:execlastid`（`LastInsertId()`），PG 产物派生为 `:one` + `RETURNING id`
  —— pgx 的 `database/sql` 适配层不实现 `LastInsertId`（恒返回 `LastInsertId is not supported by this driver`），
  只能用 RETURNING。两侧生成的方法签名都是 `(int64, error)`。
- **`compliance_rate` 是 `decimal(5,2)`**：sqlc 两侧都把它生成为 `string`，落库时写
  `strconv.FormatFloat(v,'f',2,64)`、读出时解析回 `float64` 再进响应（对外契约仍是数值）。

已验证：MySQL 9.1 与 PostgreSQL 14 各跑通全流程（建基线→类目→策略→扫描→目标→明细→取消→级联删除，
事务内 44 步零失败）；PG 侧 43 条派生查询全部 `PREPARE` 通过；终态聚合与明细排序的
执行计划与迁移前逐项一致（`type=ref` + 同一索引；排序实测 3 个扫描各 98 行顺序一致）。

这段流程已固化为可重跑的 opt-in 测试 `internal/baseline/smoke_test.go`
（`BASELINE_SMOKE_DSN` 未设置即跳过，全程一个事务、结束回滚，不留数据）：

```bash
# MySQL（默认构建）
BASELINE_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' \
  go test ./internal/baseline/ -run Smoke -v
# PostgreSQL
BASELINE_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' \
  go test -tags postgres ./internal/baseline/ -run Smoke -v
```

它存在的理由：sqlmock 用例在两个 tag 下全绿也不代表 PG 可用——`:execlastid`
（pgx 不实现 `LastInsertId`）与 `decimal` 列的驱动行为都是它抓出来的。

## 接口（/sys/security/baseline）

基线、详情与扫描记录查看使用 `baseline:view`；标准、类目和策略写入使用
`baseline:manage`，发起/取消扫描使用 `baseline:scan`。只读角色不授予后两者。

基线保存由 `SaveBaseline` 就地绑定可选字段；策略创建、更新及整组替换共用
`baselineItemInput`，策略配置统一经过 `opapolicy.Validate` 校验。

```
GET    /sys/security/baseline/          基线列表（search，含 item_count/scan_count；count 为真实总数，
                                        但行仍是固定首页 20 条——前端不传分页参数，见下方"数据访问"）
GET    /sys/security/baseline/{id}/     详情（categories[] + 全部策略 items[]，item 带 category_id/category；
                                        items 按「类目顺序 → 类目内 sort」排列）
POST   /sys/security/baseline/          新建（不接受 items：类目未建立，策略须建基线后添加）
PATCH  /sys/security/baseline/{id}/     更新（items 提交时整体重建；item.category_id 必须属于该基线）
                                        策略 config 走 opapolicy.Validate 共享校验
                                        （含策略 input.<key> 引用与采集 key 的一致性检查，
                                        引用未采集字段保存即 400，详见巡检中心架构文档）
DELETE /sys/security/baseline/{id}/     删除（事务内显式级联：扫描历史明细/目标/记录 → 策略 → 类目 → 基线；外键未设级联，顺序删除子表）
POST   /sys/security/baseline/{id}/categories/   新增类目 {name}（追加到末尾）
PATCH  /sys/security/baseline/{id}/categories/{categoryId}/   {name} 重命名 | {direction: up/down} 上移下移
DELETE /sys/security/baseline/{id}/categories/{categoryId}/   删除类目（非空 400：提示先移走或删光策略）
POST   /sys/security/baseline/{id}/items/     新增策略（弹窗确认即落库；category_id 属于该基线 + opapolicy 校验）
PATCH  /sys/security/baseline/{id}/items/{itemId}/   更新策略（弹窗确认即落库）
                                        或提交 {direction: up/down} 在当前类目内上下移动
DELETE /sys/security/baseline/{id}/items/{itemId}/   删除策略（即时生效）
GET    /sys/security/baseline/scan-hosts/?mount_type=host  预览主机；项目/环境范围另传 project_id、environment_id
POST   /sys/security/baseline/{id}/scan/  发起扫描 {mount_type, project_id?, environment_id?, host_ids?}
GET    /sys/security/scans/?type=       扫描记录（type 默认 baseline；page/page_size 分页，page_size 默认 10 上限 100，返回 count+results）
GET    /sys/security/scans/{id}/        扫描详情（概要 + 每主机符合率 + 条目明细：全部 pass/fail 条目，含主机、级别、expected（Rego 策略）/actual（违规明细）、消息，fail 排前）
                                        start_time/end_time 为 RFC3339（与列表接口一致，迁移 sqlc 前是
                                        库内 JSON_OBJECT 产出的 "YYYY-MM-DD HH:MM:SS.ffffff" 文本）
```

前端：`fronted/src/views/security/baseline/index.vue`，「基线标准」tab 为**二层主从布局**
——左侧 `a-menu` 二层结构：基线（sub-menu，徽标=策略总数）→ 类目（menu-item，
选中基线后展开加载）；右侧展示**选中类目**的策略表格（表格按 category_id 过滤并保留
原始下标）。类目管理（新增/重命名/上移下移/删除）在右侧工具栏，删除非空类目由后端
400 拒绝；**策略为单条即时保存**——弹窗「确定」即调用策略单条 POST/PATCH 落库、
删除均走统一删除确认弹窗 `openDeleteConfirm`（`util/deleteConfirm.js`，居中 Modal、
红色确认按钮）后即 DELETE，基线/类目/策略三处一致，不再用 `a-popconfirm`
（无「保存策略」批量按钮；整体覆盖 PATCH items 接口保留但前端不再使用）。
携带 `items` 的 PATCH 将基线及其策略集作为一个事务单元（`[]` 表示清空）。响应保留 `data.id`，
并以该基线 ID 返回一个 `data.results` 项，提交成功 `count=1`；基线不存在返回 404，
校验、替换或提交失败时 `count=0`，不将被重建的策略当作独立成功项。
按钮风格与巡检中心对齐：新增按钮使用 primary + plus-circle 图标 + 文本；策略表格
行内操作为 small 图标按钮（编辑=primary 实心 pen，删除=`delBtn` danger trash，均带
tooltip）。策略工具栏按「类目操作 / 策略操作」分组并自动换行，标题与刷新动作独立；
左侧基线操作区以整行 primary 新增按钮为主，其余编辑/删除/扫描动作紧凑排列。窄屏下
基线列表与策略区纵向排列，工具栏操作组纵向堆叠。
左右面板顶部对齐且各自按内容确定高度，基线菜单不填充策略表格带来的额外高度；
菜单超过高度上限时内部滚动，基线操作区紧随菜单展示。
策略操作列提供上移/下移按钮，按完整类目列表判断边界（跨分页移动也有效），首条禁用
上移、末条禁用下移。移动复用策略 PATCH，事务内锁定该基线的策略排序，定位同类目
相邻项后交换并重新编号该类目的 sort，提交后重新读取策略列表；移动期间禁用移动和
策略编辑/删除按钮，策略编辑弹窗打开时禁用移动，避免排序改变编辑定位下标。
非法方向、跨基线策略、类目边界及数据库失败明确返回错误；失败回滚，不留下半次排序。
排序只影响当前策略列表及后续扫描，历史扫描快照不变，无 schema 或数据迁移要求。
「删除类目」保留完整文字，使用独立的固定宽度，不套用图标删除按钮的 48px 宽度。
策略表和扫描记录表的操作列固定在右侧并明确宽度，两表均支持横向滚动；扫描记录行内
「详情」「取消」动作使用顶部 tooltip。下拉弹层挂载到当前上下文，扫描时间按用户时区
格式化。表格风格与巡检中心对齐：「扫描记录」表用标准服务端分页
（`scanPagination`：current/pageSize/total + showSizeChanger + 共 N 条，`@change`
回读，scroll.x=1100）；「策略」表与扫描详情页两表用客户端分页
（showSizeChanger + showQuickJumper）。单条策略的采集/Rego 在二级弹窗编辑（弹窗内类目只读展示为左侧当前
选中类目）。**「扫描记录」tab 点「详情」不弹窗**，而是 `router.push` 到独立路由页
`/sys/security/baseline/scans/:id`（`views/security/baseline/scanDetail.vue`，路由名
「扫描详情页」，顶部页签打开新 tab）：页面按路由参数拉取详情，展示概要 + 主机符合率 +
条目明细（全部 pass/fail 条目，可按「全部/不符合/通过」过滤；expected/actual
优先按 **assertion 级结构化展示**——每条断言一行「名称 + 应为 X / 实际 Y（✓/✗）」，
数据来自 agent OPA 检查的 `actual.details`，历史数据或无 details 时回退展示策略源码/JSON）。

条目编辑弹窗与巡检组检查项编辑器保持同一套布局语义（80vw 弹窗、`input-grid`
采集行紧凑布局、行末 delBtn 删除）：条目名称/章节/严重级别（high/medium/low，
合规等级语义，与巡检检查项的 critical/warning 不同）+ 说明 + 运行用户
（`config.run_user`，留空默认 root，su -l 降权执行）+ 修复建议
（`config.remediation`，不符合时展示在扫描详情「修复建议」列，随扫描快照落库）+
可用变量提示
（`${HOST_IP}`/`${HOST_NAME}`，仅命令/路径展开）+ 采集命令/文件采集区块 +
Rego 策略（提示与巡检一致：`input.key` 必须与采集 key 一致，保存校验
`opapolicy.Validate` 强制）。
采集命令在独占一行的多行文本框中编辑，随内容增高，达到高度上限后可滚动；
保存时保留命令原有的换行和缩进。文件采集路径仍使用单行输入。
策略编辑和只读详情的代码区域禁用字体连字，`!=` 等运算符按原始字符显示。
**「安全中心」为顶层目录**（M，parent_id=0，order 70）→「基线扫描」子页
（C，component `security/baseline/index`，权限 `baseline:view`）+ 两个按钮权限
（F，`baseline:manage` / `baseline:scan`）。`sys_menu.menu_type` 使用 M/C/F 枚举
（不是 0/1/2），location=1、is_expanded 与现有菜单一致，否则会被前端侧边栏过滤。
菜单按角色过滤（`sys_role_menu`），页面查看、标准管理、扫描执行分别授权。
迁移 000061 将原页面权限调整为查看，并给原管理角色补只读能力，不把仅查看角色提升为管理者。
菜单与 JWT 权限快照在重新登录时加载，API 与前端需要同步部署。

## 扫描执行流（scan.go）

发起扫描弹窗每次打开完整读取分页项目、环境和启用逻辑服务名单。项目通过响应的
`business_systems` 与服务的 `business_system` 关联，环境候选仅列出所选项目启用服务实际使用的环境；
未选择项目或项目没有关联服务时不展示全局环境。切换项目会同步清除不属于新范围的已选环境，
环境仍为可选字段，留空表示该项目全部环境，不自动将唯一环境设为必选。
资产名单加载中或失败时禁止提交，失败显式提示，重开弹窗可重试；迟到的旧加载响应不覆盖新名单。
候选联动只约束页面选择，实际主机仍由下述挂载解析查询校验和去重。

1. **发起**（`StartScan`）：校验基线启用、条目非空 → `resolveHosts` 复用 inspection 域的
   `ListMountProjectHosts` / `ListMountProjectEnvironmentHosts`（项目或项目×环境实例主机
   去重）→ 事务写 `security_scan`、每主机 `security_scan_target` 和下发快照
   → 异步 `runScan`；
2. **执行**（`runScan`）：20 并发逐主机；每主机把全部条目编译为**一次 OPA 检查下发**
   （单次 check_plan 下发，10 分钟超时，条目 key `baseline:{scan}:{index}`）；
   Agent 离线 → 目标 skipped（含下发瞬间会话断开的竞态：`gateway.Execute` 返回
   `ErrAgentOffline` 时同样置 skipped，不把离线误报成"存在不符合"）；
3. **落库**：条目结果逐条 INSERT（非空全量断言均成功才为 pass，明确失败为 fail，
   采集/求值错误、缺失结果或空断言为 error）；
   主机聚合 passed/failed/compliance_rate（decimal 列，写文本）；
4. **收尾**（`finishScan`）：聚合 summary（total/success/failed/skipped）并置终态
   （failed>0 → failed；全 skipped → skipped）。
5. **取消**（`POST /sys/security/scans/:id/cancel/`，对齐巡检 CancelExecution）：仅
   pending/running 可取消；事务内置 `status='canceled'` + end_time + summary 标记
   `canceled:true`，未开始的 target 同步置 canceled；内存镜像（sync.Map）让扫描
   goroutine 跳过剩余主机、丢弃已下发言语为在响应、不聚合覆盖终态。Agent 协议无
   cancel 帧，已下发到主机上的执行无法远程终止（与 automation CancelJob 同限制）。
6. **重启恢复**：单实例 API 在启动时先对账，等待 Agent 重连后用下发快照继续 `pending` 目标；
   已完成结果保留，`running` 目标标记 `interrupted`（「结果未确认」），不自动重发，也不更新安全问题台账。
   无快照的旧未完成扫描不能恢复，未完成目标同样标记中断。部署时先应用 `000068_agent_job_recovery`，
   新旧 API 实例不得重叠运行。
   正常停机时停止领取新目标，在 `SHUTDOWN_TIMEOUT` 内继续接收在途结果；未领取的
   目标不参与停机收尾，留给下次启动恢复。

## 结果语义

- 列表与详情的 GET 响应用 `outcome_summary` 分开统计通过、存在不符合、执行异常和跳过；中断单独标注，
  在汇总中归入执行异常。旧记录同样按存储的目标状态、有效不符合条目数及是否有执行错误分类，
  不解析备注文案。进度字段 `status` 与原 `summary` 保持原有语义。
- 只有有效合规结论才显示主机条目计数和符合率；中断、执行异常（含仅部分验证）及未执行目标显示 `—`。

- 条目：bound_scope 断言集合为空 → **pass**；非空 → **fail**（violation 即"不符合"）；
- 主机符合率 = passed / (passed+failed) × 100；
- 扫描状态：任一主机 failed → `failed`；全部 skipped → `skipped`；否则 `success`。

## 二期预留

- 豁免：`baseline_exception`（主机 + 条目 + 理由 + 有效期），报告单列不计入不符合；
- 对比：本次 vs 上次扫描（新增不符合 / 修复项）；
- CVE 等新类型：新增扫描器实现 + 类型结果表，`security_scan`（scan_type）与
  菜单/记录中心直接复用。
