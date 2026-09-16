# 计划：双数据库支持（MySQL + PostgreSQL）与内联 SQL 迁移到 sqlc

- **建立日期**：2026-09-16
- **状态**：P0 已完成；P1 除 P1-7/P1-9 外已完成（P1-2 经实测**不需要执行**，见下；P1-10 的 sqlc 侧已全部修完）；P2-1（baseline 试点）已完成，P2 余下模块未开始；P3 未开始
- **约定依据**：[docs/architecture/SQL_DESIGN.md](../architecture/SQL_DESIGN.md)（选型原则、索引与性能优先级、字段映射、方言可移植规则、目录结构与派生规则）
- **相关**：[BUG_SQLC_NULLABLE_FILTER.md](BUG_SQLC_NULLABLE_FILTER.md)（同名族正确性 bug）

目标：让数据访问层同时支持 MySQL 与 PostgreSQL，并把约 359 条内联 SQL 收敛到 sqlc，消除"手写 SQL 三个事实手工对齐"造成的反复出错。

> **本文件的数字多为计划期估计，执行后已按实测更正**：P1-1 的数量原是按 Go 内联 SQL 统计的
> （误标成 `db/queries/mysql`）、P1-2 的阻塞前提经实测不成立。最终结论以 SQL_DESIGN §4.2/§4.6 为准。

---

## P0 立即可做（阻塞后续验证）——已完成 2026-09-16

| # | 事项 | 验收 | 结果 |
|---|---|---|---|
| P0-1 | **执行迁移 `000022_host_identity_indexes`** —— `assets_host.instance_name` 加 UNIQUE KEY、`ip` 加索引 | 执行后 `assets_host` 出现 `assets_host_instance_name_uniq` 与 `assets_host_ip_idx`；握手落库的 `UPDATE ... WHERE instance_name=?` 走索引而不再全表扫 | ✅ `schema_migrations` = 22（dirty=0）；两个索引就位；握手 UPDATE 的 `EXPLAIN` 从全表扫变为 `type=range` 走唯一索引，按 ip 查询 `Using index`；唯一约束实测拒绝重复值（1062）。执行前确认 19 行数据无重复/NULL/空串 |
| P0-2 | **重启后端三个进程**（api/scheduler/worker）。`MIGRATION_SOURCE_URL` 已改为 `file://db/migrations/mysql`，旧路径已不存在 | 进程重启后迁移可正常执行 | ✅ 已重建（源码比旧二进制新）并重启，三个进程 `MIGRATION_SOURCE_URL=file://db/migrations/mysql`；agent 会话重连、`/assets/hosts/` 正常 200 |

---

## P1 PostgreSQL 查询集补齐 —— 2026-09-16 执行结果

PG schema 已就绪且两侧生成的 model 完全一致（79 表 × 全部字段类型）。
**查询集已补齐：228 条查询全部派生、两侧生成并编译通过。**

| # | 事项 | 验收 | 结果 |
|---|---|---|---|
| P1-1 | 按 §4.2 清理 `db/queries/mysql` 的方言构造 | 不再出现禁止清单构造 | ✅ 部分执行。**原计划的数量（113 CAST / 34 反引号 / 35 NOW / 20 JSON）实际是 Go 内联 SQL 的统计，`db/queries/mysql` 里的真实数量是 CAST 22、反引号 17 行、`NOW(6)` 16、JSON 8、`GROUP_CONCAT` 4。** 处理方式：`NOW(6)`→应用层传时间（16 处，baseline 的 10 条查询，无调用方受影响）、`JSON_OBJECT()`→`'{}'`（1 处）改在源里；CAST / 反引号 / `GROUP_CONCAT` / `JSON_ARRAYAGG` / `JSON_UNQUOTE(JSON_EXTRACT)` 改由派生 override 表处理（CAST 直接删会**生成失败**，见 SQL_DESIGN §4.2 更正） |
| P1-2 | 分页查询改为只用位置参数 `?` | 55 条分页查询全部为纯位置参数 | ❌ **实测推翻，不执行**。sqlc 会把命名参数编号排在显式 `$n` 之后，混用不撞号；改成纯位置参数反而让 `(? IS NULL OR col = ?)` 的第一个 `?` 在 PG 里类型不可推断（运行时 `could not determine data type of parameter $1`），且会让 PG 侧丢掉参数名。详见 SQL_DESIGN §4.6 |
| P1-3 | 写派生脚本生成 `db/queries/postgres/` | 脚本可重复运行 | ✅ 实现于 `internal/platform/database/derive`（规则 + override 表）；228 条查询全部派生，MySQL/PG 两侧各 228 个方法 |
| P1-4 | 派生一致性测试 | 手改一个字符即失败 | ✅ `derive.TestDerivedQueriesMatchRepository` + `TestDeriveIsDeterministic`；`make derive` 为唯一生成入口，并删除孤儿产物 |
| P1-5 | 合并 `sqlc.postgres.yaml` 进 `sqlc.yaml` | 两个产物均生成且编译通过 | ✅ 已合并并删除旧配置；`make generate` 一次产出两侧，实测幂等（连续两次零差异）；`generate-postgres` target 已移除 |
| P1-6 | 决定并落地方言切换方式 | 能构建出连 PG 的产物 | ✅ **已完成**（选定：固定路径门面 + 构建标签）。应用代码只 import `internal/platform/database/generated`，默认构建走 MySQL 产物、`-tags postgres` 走 PG 产物并连 pgx；50 个 import 该包的文件零改动。门面由 `make facade` 生成（`derive/facade.go`），分歧的少量查询在手写的 `dialect_postgres_adapters.go` 里适配。验证：两个 tag 下 `build`/`vet`/`test` 均 20 包全绿；PG 变体实际连本机 PostgreSQL 跑通（按 code 搜索、按 name 搜索、不过滤、主机列表结果正确）。**注意**：内联 SQL（P2）在 PG 变体下仍是 MySQL 写法，所以 PG 变体目前只有 sqlc 查询部分可用；见 SQL_DESIGN §4.8 |
| P1-7 | 建 `db/migrations/postgres/` 平行迁移，版本号与 mysql 侧对齐；**同时给 `migrate` 角色补 PG 驱动**（现在只注册了 `migrate/v4/database/mysql`，`-tags postgres` 下无法驱动 PG） | 版本序列一一对应 | 🟡 **仍待做，但阻塞已解除**。已完成 P1-7 的前置：schema 侧能用真 PG 建库（见 P1-9 结果），环境里也有可用的 PostgreSQL（/tmp 单跑的 14.24，端口 55432）。注意验收只能是结构对齐 + PG 语法校验：schema 是"折叠后当前状态"，而迁移是增量（且含菜单数据 INSERT），没有"迁移前的 PG 基线"，所以无法用"空库灌迁移"来验证 |
| P1-8 | `translate` 补 PG 错误码分支 | PG 下唯一约束冲突返回业务错误而非 500 | ✅ 已补 `23505`/`23503`（用 `interface{ SQLState() string }` 断言，不绑定具体 PG 驱动）；`23503` 无法像 MySQL 的 1451/1452 那样按错误号区分，改看报文，退路是 `ErrInvalidRelation`。见 SQL_DESIGN §2.4 |
| P1-9 | PG 专属索引与计划优化（partial index / INCLUDE / BRIN / pg_trgm） | 每条附 EXPLAIN 前后对比 | 🟡 **部分完成**。已在 /tmp 起的 PostgreSQL 14 上：① 用真库装载 `db/schema/postgres`，发现并修掉 3 类 schema 缺陷（约束名全库重名、`position` 默认值类型、跨文件外键装载顺序），现在 79 表/71 外键零错误建库；② 对全部 228 条派生查询做 `PREPARE` 校验，**从 46 条失败到 228/228 通过**——修法是把 `sqlc.narg(x) IS NULL` 从 OR 链首位移到末尾（PG 用参数首次出现定类型）。**仍待做**：灌入真实规模数据后逐条 `EXPLAIN`，再决定 partial index / INCLUDE / BRIN / pg_trgm |
| P1-10 | **修 `:execresult` + `LastInsertId()` 的 PG 不可用**（P2-1 试点的真库冒烟新发现） | 资产/巡检/监控/自动化的新建接口在 `-tags postgres` 下能真正落库 | ✅ **sqlc 侧已全部修完（2026-09-16）**，内联侧随各包迁移处理。pgx 的 `database/sql` 适配层不实现 `LastInsertId()`（恒返回 `LastInsertId is not supported by this driver`），所以「INSERT 后取自增主键」在 PG 侧只能 `RETURNING`。做法：① 派生脚本对**所有 INSERT**统一改写——baseline 的 4 个 `:execlastid` 与 14 个 `:execresult`（assets 9、user 2、menu/role/sys_config 各 1）都变成 PG 的 `:one` + `RETURNING id`（UPDATE/DELETE 的 `:execresult` 不动）；② 门面里加 `insertResult`（手写适配，见 §4.8 表）把返回的 id 包成 `sql.Result`，调用点零改动；③ 漏写适配会在 PG 编译期报错（生成的方法返回 int32/int64，没有 `LastInsertId`），不是运行时静默。**验收**：新真库冒烟 `internal/platform/database/insert_smoke_test.go`（`DB_SMOKE_DSN`）把 14 条插入在真 MySQL 与真 PG 上各跑一遍，`LastInsertId` 均返回真实主键、`RowsAffected` 为 1。**剩余**：assets/monitor/inspection/automation/identity 的**内联** INSERT 仍靠 `ExecContext` + `LastInsertId()`，那条路在 PG 下本来就因为 `?` 占位符不可用，随各包迁 sqlc（P2-2/P2-3）一并消除 |

**注意**：P1-9 与 §0 优先级一致——不为可移植牺牲性能，该用方言能力就用。同理 MySQL 侧的 `(? IS NULL OR col = ?)` 在 MySQL 上实测不毁索引，**不要以可移植为理由现在就去改**。

### 两侧签名差异（P1-6 的前置输入）

派生保证 SQL 语义与**字段名**一致，但 Go 类型有约 26% 的查询不同（sqlc 两个引擎的差异，非派生可消除）：

- **58 个 Params 结构体**：`sqlc.narg` 可选过滤在 MySQL 是 `sql.NullX`、PG 是 `interface{}`；
- **10 个 Params 只在 MySQL 侧存在**：同一查询里重复的 `sqlc.arg(x)` 被 MySQL 拆成 `Pattern`/`Pattern_2`/…，PG 合并为一个 `$n`；
- 其余 205 个共有结构体逐字段一致。

P1-6 选型时必须考虑这 68 个查询：要么加一层按方言的适配，要么按方言分叉这批调用点。


---

## P2 内联 SQL 迁移到 sqlc（359 条；建表阻塞已解除）

14 张缺失的业务表已补进 `db/schema/mysql`（65 → 79 张），约 80 条内联 SQL 的阻塞已解除。

| # | 事项 | 说明 |
|---|---|---|
| P2-1 | **试点 `baseline`（47 条）** | ✅ **已完成 2026-09-16**。① 先接上已有定义：原先闲置的 30 条定义里，除 `UpdateBaselineScanTarget`（与 `FinishBaselineScanTarget` 重复且含 `scan_id = scan_id` 空操作，已删）外全部接线，22 个无调用方的方法全部有了调用点；② 补缺口：扫描列表/详情/取消、级联删除 4 条、类目占用检查、目标状态置位、按类型计数共 15 条新定义；③ **`internal/baseline` 的内联 SQL 已清零**（`grep` 无 SELECT/INSERT/UPDATE/DELETE 字面量），`baseline.go`/`scan.go` 只调 `db.New(...)`；④ 方言让步：`JSON_OBJECT` 拼响应→应用层组装、`JSON_MERGE_PATCH` 合并 summary→同事务 `FOR UPDATE` 读回+应用层合并、`FIELD()`→`CASE`、`SUM(布尔)`→`COUNT(CASE WHEN …)`、类目归属的可变长 `IN (?,?)`→取回集合在应用层比对；⑤ 测试：sqlmock 断言改为方言无关片段 + 按 tag 编译的 `expectCreateReturnsID`（屏蔽 Exec/Query 差异），并补了列表/详情的响应组装用例。**验收证据**：两个 tag 下 `go build`/`go vet`/`go test ./...` 全绿；列数守卫（`TestInlineSQLColumnArityMatchesScan`）仍通过（baseline 已不再贡献待核对的内联 SELECT）；真库冒烟——同一段流程（建基线→类目→策略→扫描→目标→明细→取消→级联删除，44 步）在 **MySQL 9.1 与 PostgreSQL 14 上各 0 失败**；PG 侧 43 条派生查询全部 `PREPARE` 通过；`finishScan` 聚合与明细排序的 `EXPLAIN` 与迁移前逐项一致，排序实测 3 个扫描各 98 行顺序一致 |
| P2-2 | `inspection`（33 条）、`automation`（36 条）、`identity`（14 条） | 体量中等，参照 P2-1 的模式；**顺手做掉 P1-10 的 `:execresult` 适配**（这几个包的插入都靠 `LastInsertId()`）。**`identity` 已完成 2026-09-16**：内联 SQL 清零（用户组 CRUD + 成员整表替换、用户中心的告警媒介绑定共 14 条 → 11 条新定义，另有 2 条复用 `GetUserByID`/`GetAlertMediaTyped`）；顺带修掉一处方言 bug——`isDuplicateEntry` 只认 MySQL 1062，PG 下重名用户组会 500，现在同时认 SQLSTATE 23505（`errors.As` + `interface{ SQLSTATE() string }`，与 assets 的 `translate` 同一手法）；新增的真库冒烟 `internal/identity/smoke_test.go`（`IDENTITY_SMOKE_DSN`）在真 MySQL 与真 PG 上跑通建组/重名文案/成员归组/改名/批量删除/绑定校验。**`inspection`、`automation` 待做** |
| P2-3 | `assets`（91 条）、`monitor`（138 条） | 体量最大，最后做；同样包含 P1-10 的改造 |
| P2-4 | 明确保留为内联的例外 | 可变长 `IN (?,?,?)` 12 条（首选改成"取回集合在应用层比对"，baseline 已验证可行）、动态行数的多行 INSERT（baseline 的 100 行/批结果落库改为逐条 sqlc INSERT，代价见 P2-1 结论）、一次性运维/DDL 语句 |

**验收（每个包）**：`go build ./... && go test ./...` 全绿（两个 tag）；列数守卫测试仍通过；该包内联 SQL 条数下降并更新本文档；**真库冒烟：MySQL + PG 各跑一遍该包的写路径**（现成两个：`internal/baseline/smoke_test.go` 与 `internal/platform/database/insert_smoke_test.go`，都靠 `*_SMOKE_DSN` 触发、事务内回滚；后续包照此各留一个）——baseline 试点的经验是：mock 测试全绿不等于 PG 可用，`LastInsertId` 与 `decimal` 列的差异就是这么暴露的。

---

## P3 机制与守卫

| # | 事项 | 说明 |
|---|---|---|
| P3-1 | 派生一致性测试 | ✅ 已随 P1-4 完成 |
| P3-2 | 生成物漂移检查 | ⏳ 未做。接通 `make generate` 并加"生成物与 `.sql` 不一致即失败"的检查。`BUG_SQLC_NULLABLE_FILTER.md` 教训 2 指出本仓库生成代码曾手工同步、并已漂移过一次。注意：派生侧（`db/queries/postgres`）的漂移守卫已随 P1-4 落地，缺的是**生成物侧**（`generated/*.go` 与 `db/queries/mysql` + `db/schema` 的一致性） |
| P3-3 | 守卫：分页查询不得混用 `sqlc.arg`/`sqlc.narg` 与位置参数 | ❌ 已作废（P1-2 被推翻：混用无害，且不混用才会出问题） |
| P3-4 | 守卫：`db/schema` 与真实库一致性 | ⏳ 未做。幽灵表案例（见 SQL_DESIGN §6.1）说明 schema 漂移无任何告警；比对 `information_schema` 与 schema 文件，发现"schema 有但库里没有"即失败 |
| P3-5 | `go.mod` 的 tool 指令 pin 到 v1.30.0 | ⏳ 未做，目前靠 `Makefile` 的版本守卫兜住（v1.31.1 会去重参数名、静默产出不一致产物） |

---

## P4 已发现但未处理的零散问题

| # | 问题 | 建议 |
|---|---|---|
| P4-1 | `DJ_AGENT_HOST_REPORT_INTERVAL` 是**死配置**：agent 侧无任何 ticker/scheduler，仅出现在启动日志与运行时状态；`host_report_interval_current_seconds` 恒为 0 | 删除该配置与运行时状态字段；同步 `dj_agent/deploy/*` 与前端展示 |
| P4-2 | 前端 agent-runtime 页有多个恒空字段（`schedulers`/`runtime`/`registered_tasks` 恒为空、两个上报间隔栏无意义）；该页调用的 `queryHostDynamicTasks` 后端无对应路由 | 要么补齐后端，要么删掉页面上的死字段 |
| P4-3 | `assets_agent_job_event` 表已建模，但 Go 侧不使用（仅出现在 `internal/modules/catalog.go` 的表清单里） | 确认是历史遗留则从清单与 schema 中清理 |
| P4-4 | `sys_agent_token.agent_id` 与主机身份概念同名但语义不同（前端标签为"Api ID"） | 是否改名为 `api_id`（涉及 API 契约）待决 |
| P4-5 | 前端 `monitor/alerts/__tests__/UserNotificationChain.spec.js` 有一个既存失败（断言 `该绑定已禁用` 与 `binding.policies` 为 undefined），与 SQL 工作无关 | 单独修 |
| P4-6 | `autoadmin/docs/DEVELOPMENT_PLAN.md` 的 `- [ ] Export the remaining fully migrated Django MySQL schema into db/schema` 未勾选 | 本次已补 14 张表，更新该勾选状态与说明 |
| P4-7 | **`sqlc.slice` 的 PG 产物是坏的**（`db/queries/mysql/inspection.sql` 的 `ListHostBusinessChains`，目前仅此一处）：MySQL 引擎生成 `/*SLICE:host_ids*/?` 标记 + 运行时替换，PG 引擎把标记直接渲染成 `IN ($1)`，而生成代码里的替换仍在找那个 MySQL 标记——于是传入 N>1 个 host_id 时参数个数与占位符不符、查询报错。调用点（`internal/inspection/runtime.go`）是 `if err == nil` 忽略错误，表现为"业务链路快照静默变空"（该字段本身是冗余信息，所以一直没被发现） | 共享查询里**不要用 `sqlc.slice`**。inspection 迁 sqlc（P2-2）时改掉：该查询只用于快照，可改成按单个 host 查（调用点本来就是逐主机填 map），或按 §4.5 分叉方言写法 |

---

## P5 已知陷阱清单（踩过的坑，避免重犯）

1. **`sqlc.arg(x) IS NULL` 恒假**：非空参数（`int64`）使 `0 IS NULL` 为假 → "不过滤"路径恒 0 行。**可选过滤必须用 `sqlc.narg`**（生成可空参数）；已修的 4 处采用 `= 0 表示不过滤` 约定。详见 [BUG_SQLC_NULLABLE_FILTER.md](BUG_SQLC_NULLABLE_FILTER.md)。
2. **派生时不要手动去重 `$n`**：同名参数去重会让两侧生成的签名差一个字段（实测 MySQL `Params{Column1, Name}` vs PG `Params{Column1}`）。
3. ~~**分页查询不要混用** `sqlc.arg`/`sqlc.narg` 与位置参数 `?`：编号由 sqlc 内部决定，外部无法预测。~~ **已推翻（2026-09-16 实测）**：sqlc 把命名参数编号排在显式 `$n` 之后，混用不撞号；反而**改成纯位置参数才会坏**——`(? IS NULL OR col = ?)` 的第一个 `?` 在 PG 里类型不可推断，运行时报 `could not determine data type of parameter $1`。见 SQL_DESIGN §4.6。
4. **PG 保留字列名**（本仓库有 `order`、`limit`）在 PG 里必须加双引号。**但只能由派生脚本加**：MySQL 源里必须用反引号，双引号在 sqlc 的 mysql 引擎里是字符串字面量（实测 `SELECT id, "order"` 生成的是 string 类型的 `Column3`，能通过生成但列是错的——这类静默错误比报错危险）。仅作列别名时 PG 允许裸写保留字（`AS group`），作列引用则必须加引号。
5. **sqlc 版本必须 v1.30.0**：v1.31.1 会把重复的 `sqlc.arg` 去重，静默产出与仓库不一致的产物并打断调用方。
6. **`.sql` 与生成的 `.go` 文件名不要以 `_` 开头**：Go 会忽略这类文件，导致生成的方法不参与编译（表现为"方法未实现接口"）。
7. **`sqlc generate` 任一 block 失败则整轮不产出**：测试脚手架里把两种方言放在同一目录会导致互相干扰。
8. **改 schema 的完整动作**：改 `db/schema/mysql` → 加迁移（`db/migrations/mysql`，up/down）→ 重新生成 → **同步翻译 `db/schema/postgres` 并重生成 PG 产物** → 跑守卫与全量测试。只改一侧即为半成品。
9. **改查询的完整动作**：只改 `db/queries/mysql` → 重新派生 `db/queries/postgres` → 两侧各自生成。**不要手改 PG 侧**。
10. **`CAST(sqlc.arg(x) AS CHAR)` 是「承重」的，不要当噪声删掉**（2026-09-16 实测）：参数同时与可空列和 `COALESCE(...)` 比较时，删掉 CAST 会让 sqlc 推断出 `sql.NullString`/`string` 两种类型，直接生成失败（`named param Pattern has incompatible types`）。它的作用是把参数类型统一成文本；PG 侧由派生 override 换成 `AS text`。
11. **两侧参数个数会因 `sqlc.arg` 重复而不同**：v1.30.0 下 MySQL 引擎把同一条查询里重复的 `sqlc.arg(x)` 拆成多个参数（`Pattern`、`Pattern_2`…），PG 引擎合并为一个 `$n`。因此这类查询的 `Params` 结构体只在 MySQL 侧存在（实测 10 条），接 PG 调用方时要单独处理。
12. **`make generate` 失败时不要以为"零差异"**：生成失败不产出任何文件，此时对比前后产物会得到"没有变化"的假结论。跑完务必看 exit code（本次就因此误判过一次 CAST 的影响）。
13. **`sqlc.narg(x) IS NULL` 必须写在 OR 链末尾**（2026-09-16 真 PG 实测）：PG 解析期用参数的**首次出现**定类型，`($1 IS NULL OR col = $1)` 会让 PG 直接报 `could not determine data type of parameter $1`——是解析失败，不是计划退化。228 条派生查询里 46 条中招。改成 `(col = $1 OR $1 IS NULL)` 即修复，MySQL 侧执行计划实测逐项不变。见 SQL_DESIGN §2.2。
14. **schema 只被 sqlc 解析过 ≠ 能建库**：`db/schema/postgres` 之前"解析通过、模型一致"，但真 PG 装载时暴露 3 类错误（约束名全库重名 `name`×14、`position` 的 `DEFAULT false`、跨文件外键顺序）。**sqlc 不做执行，所以这类错误一直是静默的**——凡改 schema，除了跑 generate，还应该真装载一次。见 SQL_DESIGN §4.7。
15. **PG 的约束名与索引名是全库唯一的**（MySQL 是每表唯一）：直译 MySQL 索引名到 PG 会撞。新表在 PG 侧一律用表名前缀命名。
16. **同一参数在查询里出现多次时，生成物会出现 `Xxx_2`/`Xxx_3` 字段——调用点漏设就是静默失效**（2026-09-16 线上复现的真实 bug）：`CAST(sqlc.arg(pattern) AS CHAR)` 重复 4 次会生成 4 个参数，调用点只设第一个 → `LIKE NULL` → 按编码/负责人/IP/备注搜索恒空。修法是「列统一 `COALESCE(col,'')` + `sqlc.narg`」合并成一个参数。写完查询后**必须检查 `Params` 有没有后缀字段**，见 SQL_DESIGN §2.5 与 [BUG_SQLC_NULLABLE_FILTER.md](BUG_SQLC_NULLABLE_FILTER.md)。
17. **`LastInsertId()` 在 PostgreSQL 上恒失败，`:execlastid` / INSERT 的 `:execresult` 必须靠 `RETURNING` 取主键**（2026-09-16 真库冒烟发现，已由派生脚本统一处理）：pgx 的 `database/sql` 适配层对普通 Exec 返回 `driver.RowsAffected`，其 `LastInsertId()` 固定返回 `LastInsertId is not supported by this driver`——**两侧都能生成、都能编译，只有真跑才炸**（这正是"sqlc 生成通过 ≠ PG 可用"的第三个实例，前两个是 schema 建不出库、`$1 IS NULL` 解析失败）。规则：派生脚本把所有 INSERT 改写成 PG 的 `:one` + `RETURNING id`，门面用 `insertResult` 包回 `sql.Result`；多行 INSERT 在派生期报错。**内联 INSERT（`ExecContext` + `LastInsertId()`）随各自模块迁 sqlc 时消除**。
18. **mock 测试全绿不代表方言可用**：`baseline` 迁移后 sqlmock 用例在两个 tag 下都通过，但 `-tags postgres` 的真库冒烟一次就暴露了 `LastInsertId` 与 decimal 列的驱动行为差异。**每个包迁完都要在真 MySQL + 真 PG 上各跑一遍写路径**（读路径可以只做 `PREPARE`）；顺带注意 `decimal(p,s)` 两侧生成的都是 `string`，需要应用层换算。
