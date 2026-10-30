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
按主机分支不依赖项目/环境资产选项加载；详情明确显示“按主机、不限定项目、不限定环境”，
该分支不推断项目和环境归属，逐主机环境集合为空，不伪造为某一项目的全环境。
扫描请求的 `host_ids` 缺省或空数组表示全部，非空则仅扫描这些主机并去重；
提交时在创建事务内重新解析范围，任一 ID 不属于当前范围（含已删除主机）均明确拒绝，不扩大为全部。
离线主机仍可选择，沿用执行阶段的离线错误语义。目标记录与范围快照只保存实际扫描主机，
不新增表、列或外键。

扫描详情概要包含项目和具体环境名称列表，不以“全部环境”代替环境清单：
按项目扫描列出实际扫描主机在该项目内涉及的全部服务环境，按项目×环境扫描只列出选定环境。
一台主机可以归属多个环境，操作系统检查仍只执行一次；主机表和条目表返回 `environments: [{id,name}]`，
共用一个环境筛选框，按成员关系过滤而不复制结果行。两表增加环境列；筛选重置分页，
条目主机选项随环境收窄，移除失效主机条件。没有可确认归属的目标显示并可筛选“未记录环境”。

迁移 `000058_scan_scope_snapshot` 在 `security_scan` 增加可空 JSON（PG 为 jsonb）`scope_snapshot`，
不新增外键，不修改历史结果。新扫描创建时，在同一事务内保存项目名称、环境 ID/名称清单、
按主机 ID 索引的环境集合；只采集解析出的目标，服务和成员启用条件与扫描范围解析一致。
读取使用快照（`environment_source=snapshot`），后续项目/环境改名、删除或成员关系变化不影响展示。
快照损坏明确报错，不悄悄改查当前资产。

用户明确要求已有扫描也能展示/筛选环境：仅 `scope_snapshot IS NULL` 的记录按当前项目内
服务—成员—主机关联补充（`environment_source=current_assets`），页面显式提示“当前资产关联，非扫描时快照”。
固定环境扫描保留记录中的环境 ID；已删除名称显示原 ID 和“已删除或不可用”。
历史补充不回写为伪快照；该分支只服务迁移前记录，待历史记录按留存策略全部清理后可移除，
不是供新扫描缺少快照时使用的成功兜底。迁移 down 会删除范围快照，回退前应备份该列。

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
   去重）→ 事务写 `security_scan` + 每主机 `security_scan_target`（error_message 显式 ''）
   → 异步 `runScan`；
2. **执行**（`runScan`）：20 并发逐主机；每主机把全部条目编译为**一次 OPA 检查下发**
   （单次 check_plan 下发，10 分钟超时，条目 key `baseline:{scan}:{index}`）；
   Agent 离线 → 目标 skipped（含下发瞬间会话断开的竞态：`gateway.Execute` 返回
   `ErrAgentOffline` 时同样置 skipped，不把离线误报成"存在不符合"）；
3. **落库**：条目结果逐条 INSERT（violations 空即 pass，否则 fail）；
   主机聚合 passed/failed/compliance_rate（decimal 列，写文本）；
4. **收尾**（`finishScan`）：聚合 summary（total/success/failed/skipped）并置终态
   （failed>0 → failed；全 skipped → skipped）。
5. **取消**（`POST /sys/security/scans/:id/cancel/`，对齐巡检 CancelExecution）：仅
   pending/running 可取消；事务内置 `status='canceled'` + end_time + summary 标记
   `canceled:true`，未开始的 target 同步置 canceled；内存镜像（sync.Map）让扫描
   goroutine 跳过剩余主机、丢弃已下发言语为在响应、不聚合覆盖终态。Agent 协议无
   cancel 帧，已下发到主机上的执行无法远程终止（与 automation CancelJob 同限制）。

## 结果语义

- 条目：bound_scope 断言集合为空 → **pass**；非空 → **fail**（violation 即"不符合"）；
- 主机符合率 = passed / (passed+failed) × 100；
- 扫描状态：任一主机 failed → `failed`；全部 skipped → `skipped`；否则 `success`。

## 二期预留

- 豁免：`baseline_exception`（主机 + 条目 + 理由 + 有效期），报告单列不计入不符合；
- 对比：本次 vs 上次扫描（新增不符合 / 修复项）；
- CVE 等新类型：新增扫描器实现 + 类型结果表，`security_scan`（scan_type）与
  菜单/记录中心直接复用。
