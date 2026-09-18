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
| `security_scan` | 统一扫描任务：`scan_type`（`baseline`，预留 `cve` 等）+ 挂载点（mount_type=project/environment + project_id/environment_id）+ status + summary |
| `security_scan_target` | 每主机执行状态：passed/failed/compliance_rate/error_message |
| `baseline_scan_result` | 策略结果明细：scan × host × item，`chapter` 为**快照字段**（扫描时以类目名填充，类目后续改名不影响历史结果）+ expected/actual/message/remediation。expected=agent OPA 检查返回的 `{query, policy}`；actual=`{violation_count, violations, inputs, details, run_user}`；remediation=修复建议快照（文案本体存 `baseline_item.config.remediation`，策略编辑弹窗录入，迁移 000012 加列）。历史上 expected 曾未采集恒为 null，修复后新扫描正常写入 |

注意：`security_scan_target.error_message` NOT NULL 无默认——INSERT 必须显式给 `''`。

## 数据访问（sqlc，2026-09-16 起）

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

## 接口（/sys/security/baseline，权限 baseline:manage / baseline:scan）

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
DELETE /sys/security/baseline/{id}/items/{itemId}/   删除策略（即时生效）
POST   /sys/security/baseline/{id}/scan/  发起扫描 {mount_type, project_id, environment_id}
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
按钮风格与巡检中心对齐：工具栏新增按钮用默认（非 primary）类型；策略表格行内
操作为 small 图标按钮（编辑=primary 实心 pen，删除=`delBtn` danger trash，均带
tooltip）；左侧基线面板为 small 图标按钮组（新增 primary / 编辑 / 删除 delBtn /
发起扫描 ghost）。表格风格与巡检中心对齐：「扫描记录」表用标准服务端分页
（`scanPagination`：current/pageSize/total + showSizeChanger + 共 N 条，`@change`
回读，scroll.x=1000）；「策略」表与扫描详情页两表用客户端分页
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
菜单由迁移 000008 写入：**「安全中心」为顶层目录**（M，parent_id=0，order 70）→
「基线扫描」子页（C，component `security/baseline/index`）+ 两个按钮权限（F，
`baseline:manage` / `baseline:scan`）。注意 `sys_menu.menu_type` 沿用 Django 风格
M/C/F 枚举（不是 0/1/2），location=1、is_expanded 与现有菜单一致，否则会被前端
侧边栏过滤不显示。菜单按角色过滤（`sys_role_menu`）：迁移会授权给所有已拥有
「自动化运维」菜单的角色，其他角色需在角色管理中手动勾选。菜单数据在登录时实时
读取 `sys_menu`（旧后端二进制也能返回），但基线页面接口需要部署包含 baseline
路由的新二进制。

## 扫描执行流（scan.go）

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
