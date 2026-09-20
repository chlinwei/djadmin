# SQL 设计与方言约定

本文是 autoadmin 数据访问层的**唯一约定文档**：怎么选 sqlc 还是内联 SQL、字段映射怎么避坑、以及为了同时支持 MySQL 与 PostgreSQL 该怎么写 SQL。新增或修改任何 SQL 前请先读本文。

相关文档：文档组织与 API 约定见 [CONVENTIONS.md](CONVENTIONS.md)；主机身份的实例名契约见 [DJ_AGENT_ARCHITECTURE.md](DJ_AGENT_ARCHITECTURE.md) 第 2 节。

---

## 0. 优先级（最高原则，冲突时按此裁决）

1. **正确性**：数据语义（NULL / 唯一性 / 并发）不能为了写法方便而妥协。
2. **性能与索引**：SQL 的执行计划必须按**目标库**优化；该建的索引必须建。
3. **方言可移植性**：**只在"不影响上面两条"的前提下追求。**
4. **代码统一 / 少写一份**：最末位。

**冲突时一律分叉，不要为了一份 SQL 让两个引擎都妥协。** 具体含义：

- 不要为了共用一份查询而放弃某个方言的优化能力（PG 的 partial index / `INCLUDE` 覆盖索引 / BRIN、MySQL 的覆盖索引与 `FORCE INDEX`，在对方那里没有对应物）。
- 不要为了共用一份查询而接受更差的计划（例如把能走索引的等值查询改成走不了索引的写法）。
- **分叉不是失败**：路线的选择（§4.5）本身是"可移植性"与"两份维护成本"的权衡，性能与正确性不参与这个权衡。

---

## 1. 选型原则：sqlc 优先，判据是「形状是否在编译期确定」

**判据不是 SQL 的复杂度，而是「SQL 的文本与形状在编译期是否确定」。**

| 情况 | 选择 | 理由 |
|---|---|---|
| 表已在 `db/schema`，语句形状固定 | **sqlc**（再复杂的 JOIN / 聚合 / 子查询也用它） | 越复杂越该用：列错配风险越大，而 sqlc 由 DDL 推导 |
| 表未建模 | **先把表补进 `db/schema`，再写 sqlc**；只有临时脚本才内联 | 这是欠账，不是设计 |
| 占位符个数随入参变化（可变长 `IN (?,?,?)`） | 内联（或改用 JSON 数组参数） | sqlc 无法生成可变个数占位符 |
| 一次性运维 / DDL / 数据修复 | 内联 | 不属于查询层 |

「简单所以手写」**不成立**，而且是最危险的直觉：历史上一次 `POST /api/agent/install` 500，炸的就是最简单的三列单表查询（SQL 从 4 列改成 3 列而 `rows.Scan` 未同步删参）。简单语句没人会去核对，又绕开了编译器——**越简单越应该交给 sqlc**。

### 1.1 为什么手写 SQL 反复出问题

一条手写 SQL 是**三个独立事实的手工对齐**：

1. DDL 的可空性；
2. SQL 文本里的列（个数、顺序、有没有 `COALESCE`）；
3. Go 侧 `rows.Scan` 目标的个数与类型。

三者任意一个漂移，编译器都看不见。sqlc 把它们塌缩成一个：行结构体**从 DDL 生成**，SQL 是它的声明；改 schema → 重新生成 → 编译器把每个调用点报出来。

### 1.2 四类病症与治法

| 病 | 何时暴露 | sqlc 能防吗 | 治法 |
|---|---|---|---|
| 缺/多字段（列数 ≠ Scan 数） | 运行时 fatal | **能**，结构上不可能 | 迁 sqlc；守卫测试 `TestInlineSQLColumnArityMatchesScan` 兜住剩余内联 |
| NULL 扫进非空类型 | 运行时 fatal | **能**，可空性从 DDL 推导 | 迁 sqlc；内联 SQL 一律扫进 `sql.Null*` |
| `COALESCE(x,'')` 掩盖 NULL | **静默**（NULL 变 `''`/`0`） | 不能，是设计决策 | 见 §3.1 |
| 零值覆盖（"未设置" vs "就是 0"） | **静默** | 不能 | 见 §3.2 |
| 同一参数在查询里多次出现 → sqlc 拆成多个字段，调用点只设第一个 | **静默**（多出的字段不报错，值为 nil） | 能生成但防不住漏设 | 见 §2.5 |

---

## 2. 索引与执行计划（性能）

**凡用于定位的列必须有索引**：出现在 `WHERE` / `JOIN ON` / `ORDER BY` 中的列，以及作为全局标识的列。后者还必须有 **DB 级唯一约束**——只靠服务层的 check-then-insert 存在并发竞态。

### 2.1 已修的实际缺口：主机身份键没有索引

`assets_host` 原先只有 `PRIMARY` / `cloud_account_id` / `group_id` / `environment_id` 四个索引，**`instance_name` 与 `ip` 都没有**。而 `instance_name` 是整个 agent 体系的身份键，导致：

- 每次 agent 握手的落库都执行 `UPDATE assets_host SET agent_online=TRUE … WHERE instance_name=?` → 无索引可用，退化为全表扫描；
- 主机列表的 `instance_name` / `ip` 模糊搜索 → 全表扫描；
- `HostIPExists` / `InstanceNameExists` 的唯一性校验（每次主机增改都跑 `SELECT COUNT(*) … WHERE col=?`）→ 全表扫描。

19 行时无感，但这几个操作在每次握手、每次主机增改时都会执行。**迁移 `000022_host_identity_indexes` 补上：`instance_name` 加 `UNIQUE KEY`（恢复原 `agent_id` 列具备的唯一约束），`ip` 加普通索引**（IP 唯一性仍由服务层保证，保留存量重复数据平滑收敛的余地）。

`instance_name` 必须是 UNIQUE 而不只是 INDEX，因为唯一性是**正确性**问题：实例名重复时，握手那条 UPDATE 会一次命中多行，把两台主机的在线状态一起改掉。唯一冲突经 `translate` 映射为业务错误（MySQL 1062 / PG 23505 → `ErrDuplicate`），不会以 500 暴露。

**已执行并实测（2026-09-16）**：迁移 000022 已应用（`schema_migrations` = 22）；`assets_host` 出现 `assets_host_instance_name_uniq`（UNIQUE）与 `assets_host_ip_idx`；握手那条 `UPDATE ... WHERE instance_name=?` 的 `EXPLAIN` 从全表扫变为 `type=range` 走唯一索引，按 ip 查询走 `assets_host_ip_idx` 且 `Using index`。执行前已确认 19 行数据无重复、无 NULL、无空串。

### 2.2 可选过滤 `(? IS NULL OR col = ?)`：实测结论与一个被推翻的担心

此前担心这个模式会破坏索引使用。**在 MySQL 9.1 上实测（`assets_agent_job` 有 `idx(instance_name,status)` 与 `idx(status,create_time)`），担心不成立**：

| 写法 | 计划 |
|---|---|
| `WHERE instance_name=? AND status=?` | `type=ref`，走 `idx(instance_name,status)` |
| `WHERE (? IS NULL OR instance_name=?) AND (? IS NULL OR status=?)` | **仍为 `type=ref`，仍走该索引** |
| 只提供 status（另一条件传 NULL） | 走 `idx(status,create_time)` |

MySQL 会在优化期对参数做常量折叠，因此 OR-IS-NULL 链不会必然导致全表扫描。

**PostgreSQL 侧：已实测，且后果比"计划退化"严重得多（2026-09-16）**。用真 PostgreSQL 14 对全部 228 条派生查询做 `PREPARE`（= 解析期参数类型推断，与驱动发 OID 0 的路径一致）后确认：

| 派生后的 PG 写法 | PG 结果 |
|---|---|
| `($1 IS NULL OR col = $1)` | ❌ `could not determine data type of parameter $1` |
| `(col = $1 OR $1 IS NULL)` | ✅ 通过 |
| `($1::text IS NULL OR col = $1::text)` | ✅ 通过（但要逐列知道类型） |

**根因**：PG 在解析阶段用参数的**首次出现**定类型；首次出现若是裸的 `$1 IS NULL`（没有任何类型上下文）就直接报错，不会延后到后面有类型的用法去推断。所以这不是"规划器倾向 generic plan"的性能问题，而是**解析期硬失败**——实测 228 条里 46 条（约 20%）中招，上 PG 会直接跑不起来。

**修法（已执行）**：把 `IS NULL` 这一项挪到 OR 链**末尾**——`(col = sqlc.narg(x) OR sqlc.narg(x) IS NULL)`。OR 满足交换律，两方言语义完全等价；首个出现变成带类型的比较，PG 即可推断。已改 170 处，改后 228/228 全部 `PREPARE` 通过。

**对 MySQL 零代价（实测）**：在 `assets_agent_job` 上对"全 NULL / 两条件带值 / 只带 status"三种组合分别 `EXPLAIN` 新旧写法，访问类型、命中索引、行数估计**逐项相同**（旧 `ref`+`idx(instance_name,status)` ↔ 新 `ref`+同一索引）。

**结论**：`sqlc.narg(...) IS NULL OR ...` 这个形态本身可以保留（MySQL 上不影响索引、也不必改成动态拼谓词），但**`IS NULL` 必须写在 OR 链末尾**，不能写在最前面。这条同时适用于 `db/queries/mysql` 与 Go 里的内联 SQL（内联部分迁 sqlc 时按此写）。

### 2.3 模糊搜索 `LIKE '%x%'` 天然无索引可用

前导 `%` 使 B-Tree 索引无法使用，两个方言都一样。这是产品取舍而非实现缺陷：要搜索性能，**PG 用 `pg_trgm` + GIN 索引，MySQL 用 ngram fulltext——两者解法不同，硬凑一份 SQL 就永远用不上**。属于"该分叉、该用方言能力"的典型场景。

**同时是一个正确性陷阱**：MySQL 默认排序规则 `utf8mb4_general_ci` **不区分大小写**，而 PG 的 `LIKE` **区分大小写**。同一份 SQL 跑在两个库上，搜索结果集不同。当前搜索行为依赖 MySQL 的 CI 语义，直接搬 PG 会静默改变结果。

### 2.4 错误码翻译也是方言相关的

`internal/assets/service.go` 的 `translate` 要同时认两种方言的错误标识：MySQL 用数字错误号（1062 唯一冲突 → `ErrDuplicate`、1451 存在引用 → `ErrDeleteProtected`、1452 外键不存在 → `ErrInvalidRelation`），PG 用 SQLSTATE（`23505` → `ErrDuplicate`、`23503` → 外键类）。

**已实现（2026-09-16）**：`translate` 增加 SQLSTATE 分支，通过 `interface{ SQLState() string }` 断言识别（pgx 的 `*pgconn.PgError` 实现了它），因此不依赖具体 PG 驱动、也不为此引依赖。**一条限制**：MySQL 用 1451/1452 区分「被引用不能删」与「关联不存在」，PG 两者都是 `23503`，只能看报文（`update or delete on table` → `ErrDeleteProtected`，否则 `ErrInvalidRelation`）；报文依赖 PG 的 `lc_messages`（默认英文），认不出时退到语义更宽的 `ErrInvalidRelation`，不会 500。


---

### 2.5 同一参数多次出现：`sqlc.arg` 会拆成多个字段，漏设即静默失效

**已修的真实故障（2026-09-16，线上复现）**：`assets` 域的列表/计数查询写成

```sql
-- 反例：不要这样写
WHERE name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR code LIKE CAST(sqlc.arg(pattern) AS CHAR)
   OR owner LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR);
```

sqlc 的 mysql 引擎**不会把它合并成一个参数**：生成 `Pattern`、`Pattern_2`、`Pattern_3`、`Pattern_4` 四个字段，并逐个占位符传参。调用点只设了 `Pattern`，后三个为 nil → `code LIKE NULL` → NULL。**后果是"搜索"只在第一个字段（名称）上生效，按编码 / 负责人 / IP / 备注搜索恒返回空**，而且没有任何报错。受影响的接口包括项目、业务系统、环境、凭据、主机分组、主机、应用、应用版本、集群模型、部署模板的列表页。

真库复现（`assets_host`，按 IP `10.25.66.201` 搜索）：当前传参 0 行，四个参数都传同一个值 1 行。

**修法**：改成「列统一 `COALESCE(col,'')` + `sqlc.narg`」，只保留一个参数：

```sql
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern)
   OR COALESCE(owner, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern);
```

生成 `Pattern sql.NullString` 一个字段，生成的 Go 把它传给每个占位符。**为什么必须是这个组合**（沙箱实测矩阵）：

| 写法 | 结果 |
|---|---|
| `col LIKE sqlc.arg(p)`（列可空性不一致：NOT NULL 列 vs 可空列） | ✗ 生成失败 `named param P has incompatible types: sql.NullString, string` |
| 上面再加 `COALESCE(col,'')` 包列，仍用 `sqlc.arg` | ✗ 同样失败（sqlc 会看穿 COALESCE 到列的原始可空性） |
| 同上，改用 `sqlc.narg` | ✅ 一个 `sql.NullString` 参数 |
| `CAST(x AS CHAR)` 包参数（原写法） | ✅ 能生成，但参数按出现次数拆成 N 个 ← 就是本次的故障形态 |

**两条规则**：
1. 一个参数要在同一查询里出现多次时，**列的可空性必须一致**（用 `COALESCE(col,'')` 统一），并**用 `sqlc.narg`**，不要用 `sqlc.arg`。
2. 新增/修改查询后，**检查生成的 `Params` 结构体里有没有 `Xxx_2` / `Xxx_3` 这类后缀字段**——它们出现就说明调用点需要重复传值，漏传是静默失效。这是 §1.2 里"多字段"病症在参数侧的表现。

### 2.6 PostgreSQL 侧的索引：P1-9 的实测结论（2026-09-16）

方法与局限先说清楚：本仓库真库的规模很小（19 台主机、146 条告警、882 条基线明细），在这个量级上
**任何**索引讨论都没有意义 —— 所以这次试验是"按真库基数放大"的合成数据（50k 主机、200k 告警、
400k 投递记录、800k 基线明细、约 437MB），在 /tmp 单跑的 PostgreSQL 14 上逐条 `EXPLAIN (ANALYZE,
BUFFERS)` 对比。**结论只在"数据涨到几十万行以后"这个前提下成立**；今天就有意义的只有第 1 条
（它修的是结构与计划的对齐，不是速度）。

| 项 | 决定 | 证据 |
|---|---|---|
| **外键列索引**（21 处） | ✅ **补齐**（`db/schema/postgres` + 迁移 000023） | MySQL 建外键时若没有可用索引会**自动创建**一个，**PG 不会** —— 两侧因此会有不同的计划。实测最热的一条（告警主机服务树归属：`deployment → service_deployment → service`）：`assets_application_service_deployment.deployment_id` 没索引时是 Seq Scan（90k 行、Rows Removed 45000），**21.5ms**；补索引后 Bitmap Index Scan，**0.18ms**（约 120×）。这条查询在每次告警派发时都会跑。补齐即让两侧的写入成本与访问路径同源 |
| **失联对账的 partial index**（`monitor_alert_history(last_seen_at) WHERE state='firing' AND source='prometheus'`） | ✅ **补上** | 该语句每 5 分钟跑一次（每个进程）。200k 行、5% firing 的数据下：Seq Scan **35.9ms**（14917 buffers）→ Index Scan **12.1ms**，索引仅 **88kB**。**普通 `last_seen_at` 索引不会被选中**（95% 的行都满足 `< now()`，实测仍是 Seq Scan 38.3ms）——这正是 partial index 的典型场景：索引大小只跟"当前未恢复告警"成正比，与历史总量无关 |
| **`pg_trgm`（模糊搜索）** | ❌ **不采用** | 试了 `gin_trgm_ops` 索引（实例名 `LIKE '%host-1234%'`）：**计划没变**（仍 Seq Scan，12.4ms → 11.3ms）。原因是查询形状 `(instance_name LIKE \$1 OR COALESCE(ip,'') LIKE \$1)` 里的 `COALESCE(ip,'')` 与 OR 分支让 trgm 索引用不上，且 50k 行的 Seq Scan 本身只要 12ms。要在更大规模上用它，得先改查询形状（给 ip 建表达式索引、或拆成 UNION），属于"先有证据再说" |
| **BRIN** | ❌ **不采用** | 只有"写得极多、且列值与物理顺序强相关"的大表才划算。本仓库最大的表是明细/日志类（`baseline_scan_result` 800k、`automation_execution_host_log` 400k），但它们的访问模式都是按外键（scan_id / job_id）取一小组行，**选择性索引已经是最优解**；BRIN 的块级摘要在这种等值/范围过滤上不占优。数据再涨一个量级且出现"按时间范围扫大段"的查询时再评估 |
| **INCLUDE（覆盖索引）** | ❌ **不采用** | 宿主列表（`ListMonitorHosts`）是 JOIN + 多种可选过滤 + `ORDER BY instance_name`，不是"单表按索引取多列"的形状；要让它走 Index Only Scan 得把 JOIN 后的列都塞进 INCLUDE，收益不明而索引会显著变胖。当前 50k 行下各类列表都在十几毫秒内 |

**这次唯一"顺手修掉"的旧缺口**：`monitor_alert_history` 上原本没有任何索引与 `ListStaleFiringAlerts` /
`GetOpenFiringAlertForUpdate` 的过滤条件匹配（后者走 `fingerprint = ? AND state='firing'`，200k 行上是
Seq Scan）。前者由 partial index 覆盖，后者在 `(fingerprint)` 上没有索引 —— **没有补**，因为按指纹查未恢复
告警的调用频率低（每次 webhook 摄取一次）且 `fingerprint` 的取值分散，Seq Scan 的选择性判断与实际耗时
在本次数据规模下都可接受；数据再涨一个量级时它会是第一个要补的。

## 3. 字段映射约定

### 3.1 可空性：不要让 `COALESCE` 兼职表达可空

`COALESCE(x,'')` 会把「没有值」和「值是空串」**永久合并**，之后任何地方都再也分不开。二选一，不要混用：

- 需要区分 → 列保持可空，Go 侧用 `*T` / `sql.Null*` 显式承载；
- 不需要区分 → DDL 上写 `NOT NULL DEFAULT ''`，明确声明这一列没有 NULL 语义。

### 3.2 三态：用显式的 present/value，不要让零值兼职

需要区分「未传」「显式置空」「有值」时，用 `PatchField[T]{Present, Value}`（见 `internal/assets/types.go`，主机 PATCH 已在用），不要用零值表达"未设置"。

### 3.3 布尔：可空布尔跨层一律 `*bool`

- `db/schema` 里布尔列写 `BOOLEAN` / `BOOLEAN DEFAULT NULL`。
- **可空布尔在 Go 侧统一用 `*bool`**，由 `sqlc.yaml` 的 `overrides` 强制；不要逐列零散列举（漏一列就会出现第三种表示）。
- `sql.NullBool` 只允许出现在 repository 内部，**出 JSON 前必须折叠**——它没有 `MarshalJSON`，直接塞进 `gin.H` 会序列化成 `{"Bool":false,"Valid":true}` 对象，前端 `x === true` 恒为 false，静默失效。
- **区分「未知」与「否」**：不要写 `agentInstalled.Valid && agentInstalled.Bool` 把 NULL 折叠成 `false`。用 `*bool` 输出，让前端把 `null` 显示为「未知」而不是「否」。
- MySQL 的 `tinyint(1)` 允许存 0–127。若写入路径可能出现非 0/1 值，`Scan(&bool)` 会直接抛 `couldn't convert N to bool`（运行时 fatal）。必要时加 `CHECK (col IN (0,1))`。

### 3.4 前端

布尔字段一律按 JSON 的 `true`/`false`/`null` 三态处理，`null` 单独展示；不要把 `=== true` 当作唯一判据来承载"未采集"这类语义。

---

## 4. 方言可移植性（MySQL + PostgreSQL）

目标：同一套数据访问代码能跑在 MySQL 和 PostgreSQL 上。以下是实测结论（sqlc v1.30.0，两个 engine 指向同一份查询文件）。

### 4.1 可以直接共用的「可移植子集」

`sqlc.arg` / `sqlc.narg`、`COALESCE`、`LIKE`、`IS NULL`、`COUNT(*)`、`TRUE` / `FALSE`、`ORDER BY`、`GROUP BY`、多表 `JOIN`、子查询。

用 `sqlc.narg` 表达可选过滤即可替代「运行时拼 WHERE」，因此**动态拼接的正当性大幅下降**，不要以"条件可选"为由手写 SQL。

### 4.2 不得进入共用查询的构造

| 禁止 | 原因 | 替代 |
|---|---|---|
| `CAST(x AS CHAR)` | MySQL 里是变长文本，**PostgreSQL 里是 `char(1)`**——`CAST('abcd' AS CHAR)` 会被截成 `'a'`，静默算错 | 派生时把目标类型换成 `text`（见下方更正：不能直接删掉这个 CAST） |
| `::text` / `::boolean` | MySQL 解析器不接受 | 同上，只在派生产物里用 |
| 反引号引用标识符 | PostgreSQL 用双引号 | 派生时转成双引号。**注意不能反过来让 MySQL 源用双引号**（见下方更正） |
| `LIMIT ? OFFSET ?` | 占位符风格不同，两侧解析器互不接受 | 分页查询按方言各写一份（见 §4.4） |
| `IFNULL` | 方言函数 | `COALESCE` |
| `NOW(6)` / `UTC_TIMESTAMP` | 方言函数 | 由应用层传时间参数；或按方言各写一份 |
| `JSON_OBJECT` / `JSON_EXTRACT` / `JSON_UNQUOTE` / `JSON_ARRAYAGG` | 方言 JSON 函数 | 按方言各写一份（派生 override 表），或在应用层组装/解析 JSON |
| `GROUP_CONCAT` | 方言函数 | `string_agg`（PG）/ `GROUP_CONCAT`（MySQL）→ 派生 override 表 |
| `TIMESTAMPDIFF` / `DATE_ADD` / `DATE_FORMAT` / `STR_TO_DATE` / `UNIX_TIMESTAMP` | 方言函数 | 应用层计算，或按方言各写一份 |
| `INSERT IGNORE` | 方言语句 | **写成 `INSERT IGNORE INTO t(…) VALUES(…)` 即可**：派生脚本改写成 PG 的 `INSERT INTO … ON CONFLICT DO NOTHING`（见 §4.6）。**语义差异**：MySQL 的 INSERT IGNORE 还吞掉外键等可忽略错误，PG 的 ON CONFLICT 只处理唯一/排他冲突——新增用法时要确认被吞掉的是哪类错误 |
| `ON DUPLICATE KEY UPDATE` | 方言语句 | **写成 `ON DUPLICATE KEY UPDATE a=VALUES(a)` + 查询上声明一行 `-- conflict: <列名>`**：派生脚本据此改写成 PG 的 `ON CONFLICT (<列名>) DO UPDATE SET a=EXCLUDED.a`（见 §4.6）。MySQL 的 ON DUPLICATE KEY 对"表上任意唯一键"生效、语句本身看不出打在哪个键上，所以冲突目标必须显式声明；缺声明直接报错 |
| `SUM(布尔表达式)` | MySQL 把布尔当 0/1，PG 需要显式转换 | `COUNT(*) FILTER (WHERE ...)`（PG）/ `SUM(CASE WHEN ... THEN 1 ELSE 0 END)`（两方言都可） |
| **裸的** `sqlc.slice(x)` 做可变长 `IN` | 两侧生成物不等价：MySQL 生成 `/*SLICE:x*/?` 标记 + 运行时替换，**PG 把标记直接渲染成 `IN ($1)`，生成代码里的替换找不到标记**，传入 N>1 个值时参数数与占位符不符（实测：inspection 的 `ListHostBusinessChains` 在 PG 下必错，调用点吞了错误所以静默） | **写成 `IN (sqlc.slice(x))` 即可**：派生脚本会把 PG 侧改写成 `x = ANY(sqlc.arg(x)::bigint[])`，两侧签名都是 `[]int64`、都走索引（见下方 2026-09-16 更正与计划 P4-7）。**其它形状的 `sqlc.slice` 在派生期直接报错**，不会静默产出坏 SQL |
| `FOR UPDATE` 语法差异 | 方言差异（`FOR UPDATE OF` / 锁强度） | 谨慎使用，需按方言验证 |

**更正（2026-09-16，实测）**：

- **双引号不是「两方言都接受」的标识符写法。** sqlc 的 mysql 引擎把 `"order"` 解析成**字符串字面量**：`SELECT id, name, "order" FROM t` 能生成，但出来的是一个 string 类型的 `Column3`（不是 `order` 列）。所以标识符引用的方向只能是「MySQL 源用反引号 → 派生时转双引号」，且 `t."group"` 这种限定名在 MySQL 引擎里直接语法错误。PG 保留字列名（本仓库的 `order`）在 PG 侧必须带引号：裸写 `ORDER BY order` 会报 `syntax error at or near "order"`；仅作**列别名**时（`AS group`、`AS groups`）PG 允许裸写。
- **`CAST(... AS CHAR)` 不能直接删。** 早先记录说它「多余，去掉后 LIKE 结果一致」——结果一致是对的，但**去掉会让 sqlc 生成失败**：同一个参数既与可空列比较、又与 `COALESCE(...)` 比较时，会推断出 `sql.NullString` 与 `string` 两种类型，报 `named param Pattern has incompatible types: sql.NullString, string`。这个 CAST 是把参数类型统一成文本的手段，必须保留；派生时只把目标类型换成 `text`。
- **`sqlc.narg` 的可选过滤保留原样即可，不要改成位置参数**（见 §4.6 的更正）。
- **`sqlc.narg(x) IS NULL` 不能写在 OR 链最前面**：PG 在解析期用参数的首次出现定类型，裸 `$1 IS NULL` 出现在最前面会直接报 `could not determine data type of parameter $1`（不是计划退化，是解析失败）。必须写成 `(col = sqlc.narg(x) OR sqlc.narg(x) IS NULL)`，见 §2.2 的实测与修法。
- **UPSERT 也能机械派生，但冲突目标必须由源声明**（2026-09-16 实测，P2-3）：MySQL 源写
  `ON DUPLICATE KEY UPDATE update_time=VALUES(update_time), …`，查询头上写一行 `-- conflict: host_id`，
  派生结果即 `ON CONFLICT (host_id) DO UPDATE SET update_time=EXCLUDED.update_time, …`。
  两侧生成的 Go 签名完全一致（`UpsertHostRuntimeParams` 等），调用点零分叉；真库实测第二次写入
  确实走 UPDATE 分支（`ASSETS_SMOKE_DSN` 冒烟里连写两次断言指纹变化）。**缺 `-- conflict:` 注释
  或残留 `ON DUPLICATE KEY UPDATE` 都会让派生失败**——猜错冲突目标等于改了语义（可能命中另一条唯一键）。
- **可变长 `IN` 有可移植解法了，不必退回"取回集合在应用层比对"**（2026-09-16 实测，P2-2）：MySQL 源写 `WHERE id IN (sqlc.slice(host_ids))`，派生脚本把 PG 侧改写成 `WHERE id = ANY(sqlc.arg(host_ids)::bigint[])` —— **两侧生成的 Go 签名都是 `[]int64`**（`func (q *Queries) ListHostBusinessChains(ctx, hostIds []int64)`），调用点零分叉，两方言都用得上索引。PG 侧 sqlc 会为数组参数生成 `pq.Array(...)` 并 import `github.com/lib/pq`（因此 `go.mod` 需要它，且只在 `-tags postgres` 下编译）；实测 pgx 的 `database/sql` 适配层接受 `pq.Array`（也接受裸 `[]int64`）。这条改写是机械的（`derive.rewriteSlices`），**派生后若仍残留 `sqlc.slice` 会直接失败**，所以 P4-7 那种"生成通过、PG 静默出错"的形态不会复现。
  **一个边界情况**（2026-09-16 P2-3 实测）：**被比较列可空**时，MySQL 侧的切片元素类型取自列的可空性（`assets_agent_job.host_id` 可空 → `[]sql.NullInt64`），而 PG 侧的 `::bigint[]` 固定是 `[]int64` —— 两侧签名不一致，要在门面里补一段换算（丢掉 NULL 元素；集合成员判定里 NULL 永远不可能命中）。列非空时两侧都是 `[]int64`，无需适配。


### 4.3 生成的 Go 类型：默认相同，只有 OR-IS-NULL 链会分歧（实测，含一次更正）

**先说结论：在正确配置下，两个引擎生成的行类型逐字相同。** 前提是 `sqlc.yaml` 里开了 `emit_pointers_for_null_types: true` 与 `emit_json_tags: true`（本仓库已配置）。用一份真实查询（含 JOIN/聚合/可空列/布尔列/`:execlastid`）对两侧产物做整文件 diff，**差异只剩 SQL 字符串里的占位符风格（`?` vs `$n`）**，行结构体（`GetHostRow` / `ListHostsRow` 等）完全一致。

> 更正：本文档早期版本写着"MySQL 出 `sql.NullString`、PG 出 `interface{}`，所以调用方不能共用"——**那是我用最小测试配置（未开 `emit_pointers_for_null_types`）得出的错误结论**。正确配置下不成立。

**唯一的参数 API 分歧，出现在 `sqlc.narg` 用于 OR-IS-NULL 可选过滤链时**：

| 查询形态 | MySQL 调用签名 | PostgreSQL 调用签名 | 调用点可共用 |
|---|---|---|---|
| `col = sqlc.arg(x)` | `name sql.NullString` | `name sql.NullString` | ✅ |
| `col = COALESCE(sqlc.narg(x), col)` | `name sql.NullString` | `name sql.NullString` | ✅ |
| `(sqlc.narg(x) IS NULL OR col = sqlc.narg(x))` | `Params{Name sql.NullString}` | `name interface{}` | ❌ 签名不同 |

也就是说：**"调用方能否共用"取决于查询里是否用了 OR-IS-NULL 可选过滤，与方言无关。** 仓库现有的 200+ 处该模式，若走共用就要为这些查询保留一层适配；若不想适配，就把它们放进方言目录分叉（此时可各按方言写习惯写法）。

**其余实测确认可移植的构造**（两方言都能生成）：

- 布尔字面量 `TRUE` / `FALSE`。
- `sqlc.arg` / `sqlc.narg`（除上述 OR-IS-NULL 情形）、`COALESCE`、`LIKE`、`IS NULL`、`COUNT(*)`、`JOIN`、子查询、`GROUP BY`、`ORDER BY`。
- `COUNT(CASE WHEN … THEN 1 END)` 计数：两侧都是 bigint，且零行返回 0（`SUM(布尔表达式)` 既不可移植、零行还是 NULL）。
- `ORDER BY CASE col WHEN 'x' THEN 1 WHEN 'y' THEN 2 ELSE 0 END`：与 MySQL 的 `FIELD(col,'x','y')` 等价
  （FIELD 未命中返回 0，所以 ELSE 必须是 0）；真库实测排序结果与执行计划都不变。
- `SELECT … FOR UPDATE`：单表 `FOR UPDATE` 两侧都认（`FOR UPDATE OF` / `NOWAIT` 才有差异）。

**`:execlastid` 能生成但 PG 侧运行时不可用（2026-09-16 更正，真库冒烟暴露）**：

PostgreSQL 的 `database/sql` 适配层不实现 `LastInsertId()`——pgx 对普通 Exec 返回
`driver.RowsAffected`，它的 `LastInsertId()` 恒返回 `LastInsertId is not supported by this driver`
（`go/src/database/sql/driver/driver.go`），而这正是 `:execlastid` 生成代码的取主键方式。
这不是配置问题、无法绕过。**规则：MySQL 源继续写 `:execlastid`，PG 产物由派生脚本改成
`:one` + 语句末尾 `RETURNING id`**（两侧生成的方法签名都是 `(int64, error)`，调用点不用分叉）。

**`decimal(p,s)` 两侧都生成为 `string`**：MySQL 驱动把 DECIMAL 当文本返回，PG 的 numeric 同理由
sqlc 映射成 `string`。这类列要在应用层换算（baseline 的 `compliance_rate` 写库用
`strconv.FormatFloat(v,'f',2,64)`、读出解析回 float64 进响应）。

**`:execresult` 的 INSERT 同一问题（2026-09-16 已修）**：assets / user / role / menu / sys_config 的
插入走 `:execresult`（拿到 `sql.Result` 后由调用点调 `LastInsertId()`），在 PG 变体下同样会报上面的错。
现在派生脚本对**所有 INSERT**统一改写：`:execlastid` 与 `:execresult` 都变成 PG 的 `:one` + `RETURNING id`；
返回的 id 由门面里的 `insertResult`（`dialect_postgres_adapters.go`）包成 `sql.Result`，
两侧调用点继续 `result.LastInsertId()` / `result.RowsAffected()` 而不分叉。
UPDATE/DELETE 的 `:execresult` **不改**（它们靠 `RowsAffected()`，PG 侧原样可用）。
多行 `INSERT … VALUES (…),(…)` 在派生期直接报错（RETURNING 只会取到首行 id，不能静默）。

自动改写 + 手写适配的组合是自守卫的：漏写某个适配方法时，调用点编译不过——
PG 生成的方法返回 `int32`/`int64`，没有 `LastInsertId()`。

**实测确认必须分叉或改写**：

- `?` 占位符是 MySQL 专有（可移植写法必须全用 `sqlc.arg` / `sqlc.narg`）。
- 分页（§4.4，硬分叉）。
- `NOW(6)`：PG 的 `now()` 不接受精度参数，报 `function now(unknown) does not exist`。**改为由应用层传入时间参数**即可消除分叉（也更符合 §0 的优先级）。
- 其余方言函数见 §4.2 禁止清单。

**由此，真正的分叉集合很小**：分页查询 + 少数方言函数查询（其余都能靠"应用层消化"或改写消除）。

### 4.4 关键限制二：**分页没有双方言通用写法**（实测）

对同一个 schema 分别用两个 engine 生成，逐种写法验证：

| 写法 | MySQL | PostgreSQL |
|---|---|---|
| `LIMIT ? OFFSET ?`（仓库现状） | ✅ | ❌ |
| `LIMIT $1 OFFSET $2` | ❌ | ✅ |
| `LIMIT sqlc.arg(n) OFFSET sqlc.arg(m)` | ❌ | ✅ |
| `LIMIT CAST(sqlc.arg(n) AS SIGNED) OFFSET …` | ❌ | ✅ |
| 只有 `LIMIT ?` | ✅ | ❌ |
| `LIMIT <字面量> OFFSET sqlc.arg(m)` | ❌ | ✅ |

**零交集。** MySQL 的 LIMIT 只接受 `?`，PostgreSQL 只接受 `$n` / `sqlc.arg`。

**根因（不是 sqlc 的任意限制，而是 MySQL 语法本身）**：sqlc 把每条查询交给各方言的**真实**解析器——MySQL 用 `github.com/pingcap/tidb/pkg/parser`（TiDB 的 MySQL 语法实现），PostgreSQL 用 `github.com/pganalyze/pg_query_go`（PostgreSQL 官方解析器 libpg_query 的绑定）。两方言的 LIMIT 语法不同类：

MySQL（TiDB `parser.y` 的 `LimitOption` 产生式，该规则注释原文为 *"Limit option could be integer or parameter marker."*）：

```
LimitOption:
    LengthNum        ← 只能是整数字面量
|   paramMarker      ← 或者占位符 ?
```

PostgreSQL 的 LIMIT 接受**任意表达式**（`LIMIT $1`、`LIMIT (SELECT ...)` 均合法）。所以 `LIMIT ? OFFSET ?` 在语法层面**只属于 MySQL**——只用 MySQL 时它看起来天经地义，要支持 PG 才发现不可移植。任何同时解析两方言的工具都会撞上这面墙；而不做语法解析就没有类型安全，也就失去了 sqlc 的意义。

附带观察：两个引擎的报错文案风格也不同（MySQL 侧 `syntax error near "..."`，PG 侧 `syntax error at or near "OFFSET"`），正好印证走的是各自的真实语法。

**可移植的单份分页写法（实测两侧都通过）：把占位符挪出 LIMIT/OFFSET 子句，改成 keyset 游标。**

```sql
-- name: ListJobs :many
SELECT ... FROM assets_agent_job
WHERE host_id = sqlc.arg(host_id) AND id < sqlc.arg(cursor)
ORDER BY id DESC LIMIT 100;
```

`sqlc.arg` 在 WHERE 位置两方言都认，`LIMIT` 用字面量两方言都认 → **一份查询通吃**。代价是改成 keyset（游标）分页，但它顺带解决深翻页性能（`OFFSET 100000` 要扫过前 10 万行）。`assets_agent_job`（已 1 万行）、`assets_agent_job_event`（2.3 万行）、审计日志这类持续增长的列表本来就应该用游标。

**分页选型结论**：

| 方案 | 可移植性 | 适用 |
|---|---|---|
| OFFSET 分页按方言各写一份 | 两份，差别只在 `LIMIT ? OFFSET ?` / `LIMIT $1 OFFSET $2` 一行 | 小管理列表（现状约 45 条） |
| **keyset 游标 + 字面量 LIMIT** | **单份可移植** | 持续增长的列表（agent job / job event / 审计日志） |
| 字面量 LIMIT + 应用层切片 | 单份 | 不推荐（语义与性能都差） |

**另一条已知 sqlc 限制（这条确实是它自身的问题）**：无法在 WHERE 中引用 FROM 子查询的别名。用 `ROW_NUMBER() OVER (...)` 套一层再在外层 WHERE 过滤的窗口函数分页写法，两个引擎都报：

```
table alias "x" does not exist
```

因此窗口函数分页在 sqlc 下不可用，不要走这条思路。

仓库现有约 45 条分页查询（`db/queries` 38 条 + 内联 7 条），约占查询总数的 19%。

### 4.5 方案取舍与已选定方案

曾评估过三条路线（全量双份 / 全量单份可移植 / 单份+分叉收敛）。**已选定：每方言各自维护一套，全量双份**——并把 PostgreSQL 一套**由 MySQL 一套机械派生**，而不是并行手写。理由：1. **分页是模式而非边缘**：仓库现有 55 条分页查询、38 个分页接口，占列表型查询的 45%。`LIMIT`/`OFFSET` 位置在语法上不可能两方言共用（§4.4），所以每新增一个列表接口就多一处方言差异——"单份共用"的收益会随功能增长持续被侵蚀。
2. **共用方案的签名一致性无法保证**：`sqlc.narg` 的去重与编号由 sqlc 内部决定（§4.3），OR-IS-NULL 可选过滤在两侧生成不同签名，"共用一份"实际仍要一层适配层。
3. **实测：机械派生能让两侧的 SQL 语义与字段名对齐，但 Go 类型不是逐字相同**（§4.6.1：263 个共有结构体中 205 个一致、58 个类型不同、10 个只在 MySQL 侧存在）。即便如此"双份"的维护成本仍显著低：写 MySQL 一份 + 跑一次派生，漂移在构造上不可能发生；需要适配的 68 个查询是 sqlc 两个引擎对命名参数的处理差异，与选哪条路线无关。

### 4.6 目录结构与派生规则

```
autoadmin/
├── sqlc.yaml                       两个 block：mysql + postgresql（一次生成两侧产物）
├── db/
│   ├── schema/                     建表 DDL：sqlc 的输入，不是可执行脚本（见 §4.7）
│   │   ├── mysql/                  001_identity_rbac_config … 006_baseline（6 个域文件，人工维护）
│   │   └── postgres/               同上 6 个；由 mysql 翻译而成，之后各自维护
│   ├── queries/                    查询定义
│   │   ├── mysql/                  assets/audit/audit_history/automation/baseline/inspection/menu/
│   │   │                           monitor/role/scheduler/sys_config/user
│   │   │                           （12 文件 559 条查询；228 → 559 是 P2-1/P2-2/P2-3 新增的 331 条，
│   │                            其中 P2-3 新增 234 条、monitor.sql 一个文件占 175 条）
│   │   │                           ← 唯一人工维护来源
│   │   └── postgres/               同名文件 + README；make derive 的产物，禁止手改
│   └── migrations/                 基线之后的结构变更
│       ├── mysql/                  000001…000024，每个都有 .up.sql/.down.sql（48 文件）
│       │                           MIGRATION_SOURCE_URL 指向此处
│       └── postgres/               同上 48 文件（000001…000024，版本号与文件名逐字对齐 mysql 侧）
└── internal/platform/database/
    ├── configuration.go            连接参数（MySQLDSN 与 PostgresDSN 都在这里，由 tag 选用）
    ├── mysql.go                    MySQL 连接池实现
    ├── open_mysql.go               //go:build !postgres → Open 连 MySQL
    ├── open_postgres.go            //go:build postgres  → Open 连 PostgreSQL（pgx）
    ├── param_guard_test.go         守卫：生成物出现 Xxx_2 类重复参数字段而未被确认即失败（§2.5）
    ├── derive/                     派生与门面的实现
    │   ├── derive.go               ?→$n、反引号→双引号、按查询切分与编号
    │   ├── overrides.go            override 表（GROUP_CONCAT→string_agg 等方言构造）
    │   ├── derive_test.go          断言 db/queries/postgres == 现场派生结果
    │   ├── facade.go               生成方言门面的别名文件
    │   └── facade_test.go          门面漂移 + 适配清单一致性
    └── generated/                  应用代码 import 的就是这个包（方言门面）
        ├── dialect_mysql.go                ← make facade 生成（!postgres：别名 + New 转发）
        ├── dialect_postgres.go             ← make facade 生成（postgres：别名 + Queries 包装）
        ├── dialect_postgres_adapters.go    ← 手写：两侧签名分歧的少量查询适配（§4.8）
        ├── mysql/                          sqlc 产物 15 个文件（默认构建）
        └── postgres/                       sqlc 产物 15 个文件（-tags postgres）
```

**谁生成谁**（只有 `db/*/mysql` 是手写的，其余都是产物）：

| 位置 | 性质 | 生成方式 |
|---|---|---|
| `db/schema/mysql`、`db/queries/mysql` | 人工维护源 | — |
| `db/schema/postgres` | 翻译后各自维护 | 人工（改 mysql 侧时同步翻译） |
| `db/queries/postgres` | 派生产物 | `make derive` |
| `generated/mysql`、`generated/postgres` | sqlc 产物 | `make generate` |
| `generated/dialect_*.go` | 门面产物 | `make facade`（必须在 `make generate` 之后） |
| `generated/dialect_postgres_adapters.go` | 人工维护 | — |

**改 SQL 的固定顺序**（顺序不能颠倒，漏一步会被守卫测试挡住）：

```bash
# 改 db/schema/mysql 或 db/queries/mysql（+ 需要时加 db/migrations/mysql 的 up/down）
make derive     # 重新派生 db/queries/postgres
make generate SQLC=<v1.30.0>   # 两侧 sqlc 产物
make facade     # 重建方言门面
make test       # 三道守卫：查询派生一致性、门面漂移、重复参数字段
```

**派生规则（把 MySQL 一份机械变成 PG 一份）**——已实现于 `internal/platform/database/derive`：

| 项 | 处理 |
|---|---|
| `?` → `$n` | 机械，**按出现顺序编号，每个 `?` 一个独立参数，不做去重**（去重会让两侧签名差一个字段） |
| 反引号标识符 | 机械转**双引号**（不是「去掉」：`ORDER BY order` 在 MySQL 里同样是语法错误，反引号是必需的） |
| `TRUE`/`FALSE`、`sqlc.arg`/`sqlc.narg` | 两侧通用，原样保留（PG 侧靠 `sqlc.narg` 保住参数名） |
| **`:execlastid` → `:one` + 语句末尾 `RETURNING id`** | 机械改写（`appendReturningID`）：PG 侧取不回 `LastInsertId`，两侧签名仍是 `(int64, error)`。**只能是 `id` 列**，非 `id` 主键的表会在生成期报错（不会静默）；多行 INSERT 同样在派生期报错 |
| **`IN (sqlc.slice(x))` → `x = ANY(sqlc.arg(x)::bigint[])`** | 机械改写（`rewriteSlices`）：PG 侧的可变长 IN 只能用数组参数。两侧签名都是 `[]int64`，**残留的 `sqlc.slice` 会让派生直接失败**（防 P4-7 那种静默坏产物） |
| **冲突子句 / `RETURNING` 的插入点是「第一条语句的结束分号处」** | `splitAtStatementEnd`：查询体包含"本条语句 + 其后、下一条 `-- name:` 之前的注释"（那些注释是**下一条**语句的文档）。按查询体末尾插入会让子句落进注释里（产出 `-- 说明… ON CONFLICT DO NOTHING`，sqlc 照样解析通过，只有真跑才炸）。见 P5 陷阱 29 |
| **可空 json 列 → `database/sql.NullString`（列级 override）** | 生成物默认是 `json.RawMessage`，而它**扫不了 NULL**（Go 1.25 的 `jsontext.Value` 只接受 `[]byte`/`string`）。用 NULL 表达"未设置/继承"的可空 json 列（`monitor_notification_policy.media_ids` / `user_group_ids`）必须在 `sqlc.yaml` 里 override；PG block 的列级 override 要排在 `db_type: jsonb nullable` 那条**之后**（后匹配者生效）。见 P5 陷阱 26 |
| **列级 override 的列名不会随表改名自动更新（静默失效）** | `overrides[].column` 是 `表名.列名`；表改名后旧条目匹配不到任何列，该列在 PG 侧悄悄退回退化类型（`int unsigned` → `int32`，MySQL 侧仍是 `uint32`）。**只有 `-tags postgres` 构建才报错**（`cannot use uint32(...) as int32`），只看 MySQL 构建发现不了，"两侧类型完全一致"的结论也会失真。改表名时必须同步核对 `sqlc.yaml` 的 override 列表并跑 `go build -tags postgres ./...`（2026-09 修 `monitor_elasticsearch_cluster.request_timeout`：表已从 `monitor_opensearch_cluster` 改名，override 仍写旧名） |
| **`INSERT IGNORE INTO t(…) VALUES(…)` → `INSERT INTO t(…) VALUES(…) ON CONFLICT DO NOTHING`** | 机械改写（`rewriteInsertIgnore`）：两侧"影响行数"的表达不同（MySQL 用 `:execresult` 拿 id + RowsAffected；PG 派生后是 `:one`+RETURNING，被跳过时返回 `ErrNoRows`），所以调用点要按方言分叉成两个 build-tag 文件（示例见 `internal/monitor/target_dialect_*.go` 与 `internal/logcollect/collect_target_dialect_*.go`） |
| **`ON DUPLICATE KEY UPDATE a=VALUES(a)` → `ON CONFLICT (<冲突目标>) DO UPDATE SET a=EXCLUDED.a`** | 机械改写（`rewriteUpserts`）：冲突目标取自查询上的 `-- conflict: <列名>` 注释（MySQL 侧看不出打在哪个唯一键上）。缺注释、或改写后仍残留 MySQL 子句都直接报错 |
| **INSERT 的 `:execresult` → `:one` + `RETURNING id`** | 同上（`:execresult` 的调用点也靠 `LastInsertId()`）；UPDATE/DELETE 的 `:execresult` 保持原样。PG 侧返回的是 id，由门面的 `insertResult` 包回 `sql.Result` 以对齐 MySQL 签名 |
| `GROUP_CONCAT`→`string_agg`、`JSON_ARRAYAGG`→`json_agg`、`JSON_UNQUOTE(JSON_EXTRACT(x,'$.k'))`→`x->>'k'`、`CAST(… AS CHAR/SIGNED)`→`AS text/bigint` | 显式 override 表（`derive/overrides.go`）；**未命中即报错**，静默跳过等于产出语义错的 SQL |
| 时间函数 `NOW(6)`/`UTC_TIMESTAMP` | **在 MySQL 源里改掉**，由应用层传时间参数（baseline 的 16 处已改） |
| `JSON_OBJECT()` 空对象字面量 | **在 MySQL 源里改掉**为 `'{}'`（两方言都接受字符串字面量） |
| `JSON_OBJECT(...)` 拼响应体、`JSON_MERGE_PATCH(...)` 合并 JSON | **在 MySQL 源里改掉**：前者换成取列 + 应用层拼装，后者换成同一事务内 `SELECT … FOR UPDATE` 读回 + 应用层合并（baseline 试点已改，见 §4.2） |

**关于「分页查询只用位置参数」的更正（2026-09-16，实测）**：原文写「`sqlc.arg`/`sqlc.narg` 与位置参数混用时编号由 sqlc 内部决定、不可预测」。实测不成立：**sqlc 会把命名参数的编号排在显式 `$n` 之后**——`... (sqlc.narg(k) IS NULL OR s LIKE sqlc.narg(k)) AND n = $1 LIMIT $2 OFFSET $3` 里 narg 拿到 `$4`，生成正常。

因此 35 条分页查询**不需要**改写成纯位置参数，而且改了更糟：`(? IS NULL OR col = ?)` 里的第一个 `?` 只出现在 `IS NULL` 位置，PG 无法从上下文推断它的类型，运行时会报 `could not determine data type of parameter $1`（MySQL 无所谓，因为它不做参数类型推断）。保留 `sqlc.narg` 则同一个参数同时出现在比较位置、类型可推断，且 PG 侧参数名与 MySQL 一致。

**校验机制（比漂移守卫更强）**：因为 PG 是产物而非并行副本，不需要"两份相同"的守卫，而是
① `derive.TestDerivedQueriesMatchRepository` 断言 `db/queries/postgres/` 内容 == 现场派生结果（改一个字符、多一个少一个文件都会失败，报首个差异行）；
② `make derive` 是唯一的重新生成入口，`WriteAll` 会删除源里已删掉的孤儿产物；
③ `make generate` 一次产出两侧且实测幂等（连续两次生成零差异）。


### 4.6.1 两侧产物差异的实测结果（2026-09-16）

早先此处是按文本触发条件做的**静态预估**（77.7% 可完全机械派生、8.2% 需人工判断），
现已被实测取代。`db/queries/mysql` 的 228 条查询全部派生成功、两侧都生成并编译通过，
差异按「生成结构体」统计（共 263 个共有 `Params`/`Row`/model 结构体）：

| 结果 | 数量 | 说明 |
|---|---|---|
| 字段名与类型逐字段一致 | 205 | 可直接共用调用点 |
| `sqlc.narg` 可选过滤的类型差异 | 58 | MySQL `sql.NullX` vs PG `interface{}`，字段名相同 |
| Params 结构体只在 MySQL 侧存在 | 10 | 同一查询里重复的 `sqlc.arg(x)`：MySQL 拆成 `Pattern`/`Pattern_2`/…，PG 合并为一个 `$n` |
| 只在 PG 侧存在 | 0 | — |

**结论**：SQL 语义与字段名可以完全机械对齐；**Go 类型有约 26% 的查询不一致**，根因是 sqlc
两个引擎对「命名参数」的处理不同（可空性推导、重复参数的拆分/合并），不是派生脚本能改写的。
接入调用方时应按 §4.5 的结论保留一层适配或按方言分叉这 68 个查询，而不是为对齐类型去改 SQL。

另：`internal/assets/service.go` 的 `translate` 是唯一依赖方言错误的代码，已补 PG SQLSTATE 分支（§2.4）。

### 4.6.2 两条真库操作经验（2026-09-19 迁移 000035 / 2026-09-20 迁移 000045 现场）

**① 删列前必须先摘掉"快照里没有、真库上有"的 Django 时代外键。**
`db/schema` 的折叠快照缺真库外键是老问题（`inspection_target_execution.host_id` 就是这么发现的，
完整清单见计划文档 `docs/plans/SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md` 的陷阱 24）。真库上
`assets_application_service_log_setting.processing_rule_id` 挂着
`assets_application_s_processing_rule_id_b56ded30_fk_monitor_l`（Django 生成的约束名，被 MySQL 按
`max_name_length` 从中间截断），直接 `DROP COLUMN` 会报 `Error 1828: Cannot drop column ... needed in
a foreign key constraint`。**两类库都要能跑**，所以迁移里按列现查现删，不写死约束名：

- MySQL（没有 `DROP FOREIGN KEY IF EXISTS`，只能动态 SQL）：从 `information_schema.KEY_COLUMN_USAGE`
  查该列上的外键 → `IF(@fk IS NULL, 'SELECT 1', CONCAT('ALTER TABLE … DROP FOREIGN KEY `', @fk, '`'))`
  → `PREPARE` / `EXECUTE` / `DEALLOCATE`。多语句文件能这么用，前提是 DSN 带 `multiStatements=true`
  （两侧既有迁移早就是多语句文件，说明运行时已满足）。
- PG：`DO $$ … FOR fk IN SELECT conname FROM pg_constraint … contype='f' … LOOP EXECUTE format('… DROP
  CONSTRAINT %I', fk.conname) END LOOP … $$;` 同样按列删，不猜约束名。

down 迁移**不重建该外键**（与 `000005.down` 同一处理：快照里本就没有它，重建会与折叠态不一致；
up 的"现查现删"保证幂等）。

**同一个坑的 CHECK 版本（2026-09-20，迁移 000045 现场）：改列名之前要看这一列上有没有 CHECK 约束。**
真库上 `monitor_log_retention_tier` 挂着 Django 4.1 给 `PositiveIntegerField` 自动生成的
`CONSTRAINT monitor_log_retention_tier_chk_1 CHECK ((retention_days >= 0))`，
而 `000045` 要把这一列改名成 `retention_value` —— MySQL 直接以
`Error 3959: Check constraint '...' uses column 'retention_days', hence column cannot be dropped
or renamed` 拒绝，`CHANGE COLUMN` 和 `DROP COLUMN` 都会被它挡住。处理手法与外键一模一样
（MySQL 无 `DROP CHECK IF EXISTS`，只能动态 SQL；约束名 `<表>_chk_<n>` 是 Django 生成的，**不写死**）：

- MySQL：从 `information_schema.CHECK_CONSTRAINTS`（按 `CHECK_CLAUSE LIKE '%<列>%'` 筛，**必须** join
  `information_schema.TABLE_CONSTRAINTS` 才能拿到表名）+ `IF(@c IS NULL, 'SELECT 1', CONCAT('… DROP CHECK …'))`
  → `PREPARE`/`EXECUTE`/`DEALLOCATE`，然后才改名。
- PG **不需要**这一步：PG 的约束按列序号（attnum）绑定，`RENAME COLUMN` 之后自动跟着新列名，
  不报错；而且 PG 侧那一列是有符号 `integer`（§4.7 无 unsigned），`>= 0` 是真校验，留着更对。
  按 README 约定，这个差异写在 `postgres/000045_….up.sql` 顶部的 `-- PG 侧差异：` 注释里。
- 摘掉不损失语义：MySQL 侧那一列是 `int unsigned`，`>= 0` 恒真；折叠快照（`db/schema`）里
  本就没有这条约束，摘掉正是让真库与快照收敛（守卫：`internal/platform/migration/retention_unit_guard_test.go`）。

**改任何列（改名、删列、改类型）之前，先在真库上 `SHOW CREATE TABLE` 看一眼**——外键与 CHECK
都是"快照里没有、真库上有"的对象，而这类清单目前只有人读文档才知道。

**② 迁移失败后版本表会变脏，用 `migrate force` 恢复。**
golang-migrate 在跑某个版本前就写 `(version=N, dirty=1)`，失败后 `migrate` 会直接拒绝执行。
DDL 失败那一步通常没有落库（报错即回滚该语句），所以顺序是：
**改好迁移文件 → `./bin/autoadmin migrate force <失败前的版本号>` → 再 `./bin/autoadmin migrate`**。
`force` 只改版本号、不动 schema，置错会让迁移链与库内结构错位（实现见
`internal/platform/migration/migration.go` 的 `Force`）。

### 4.7 DDL 映射与两侧模型一致性

`db/schema/postgres/` 已由 `db/schema/mysql/` 翻译完成，映射规则：

| MySQL | PostgreSQL |
|---|---|
| `bigint AUTO_INCREMENT` | `bigint GENERATED BY DEFAULT AS IDENTITY` |
| `datetime(6)` | `timestamp(6)` |
| `longtext` / `mediumtext` | `text` |
| `tinyint(1)` / `BOOLEAN` | `boolean` |
| `json` | `jsonb` |
| `double` | `double precision` |
| `decimal(p,s)` | `numeric(p,s)` |
| `char(n)` | `varchar(n)`（PG 的 `char` 是定长 bpchar，补空格、比较忽略尾空格，语义不同） |
| 反引号 | 去掉；**PG 保留字列名必须加双引号**（本仓库有 `"order"`、`"limit"`） |
| 反引号 `KEY` | 独立的 `CREATE INDEX` 语句 |
| `int unsigned` / `bigint unsigned` | PG 无 unsigned → `integer`/`bigint`（**类型会退化**，见下） |

**已验证的一致性结果**：两侧各生成 78 个 model 结构体，**字段与类型完全一致**（含可空性）。达成一致需要在 `sqlc.yaml` 的 postgresql block 里加两类 `overrides`：

- **可空 jsonb**：MySQL 侧生成 `json.RawMessage`，PG 侧默认生成 `pqtype.NullRawMessage` → 强制 `encoding/json.RawMessage`。
- **24 个 unsigned 列**：PG 无 unsigned，`int unsigned` 会退化成 `int32`（MySQL 侧是 `uint32`）→ 逐列 override 回 `uint32`/`uint64`。这些列语义非负（端口 / 计数 / 超时秒数），其中可空的那一个（`assets_hostruntime.metrics_sample_window_ms`）须 override 成 `database/sql.NullInt32` 以免丢掉 NULL 语义。

> 更彻底的做法是从 MySQL schema 里去掉 `unsigned`（它是 MySQL 专属写法，且 MySQL 的 unsigned 算术有溢出陷阱）。但那需要迁移 + 约 140 处 Go 引用从 `uint32` 调整，收益不及 override 表，故暂用 override 对齐。

**已执行的缺陷修复（2026-09-16，用真 PostgreSQL 14 装载后才暴露）**：`db/schema/postgres` 此前**建不出库**——sqlc 只解析、不执行，所以这类错误一直没有告警。用真库装载发现并修掉三类问题：

| 问题 | 现象 | 修法 |
|---|---|---|
| **约束名全库重名** | MySQL 的索引名是**每表独立**的，直译成 PG 约束名后全库冲突：`name` 用了 14 次、`code` 6 次、`host_id` 4 次等，装载报 `relation "name" already exists` | 重名者改成表名前缀（`sys_menu_name`、`assets_project_code`…）；另修 3 个超过 PG 63 字节标识符上限的名字。共 34 处 |
| **列默认值类型不匹配** | `` `position` int NOT NULL DEFAULT '0' `` 被误翻成 `DEFAULT false`（把 tinyint(1) 的 0/1→false/true 规则套到了 int 列上），PG 报 `column "position" is of type integer but default expression is of type boolean` | 改回 `DEFAULT 0` |
| **跨文件外键导致装载顺序敏感** | `assets_webssh_session_log`（001 域文件）外键指向 `assets_host`（002 域文件），按文件名顺序执行时被引用表还不存在（MySQL 同理，是这份 schema 基线一贯的性质，不是翻译引入的） | 装载时按外键依赖拓扑排序；**表内自引用外键**（`assets_hostgroup.parent_id`、`monitor_notification_policy.parent_id`）不算依赖 |

修复后：78 张表、71 个外键，零错误装载成功。**注意 PG 侧新增/改列时也要守这两条**：约束与索引名必须全库唯一（用表名前缀），整数列的默认值不要写成布尔。

**装载顺序**：`db/schema/postgres/*.sql` 是"折叠后的当前状态"，按 Django 域切分，**不能按文件名顺序直接执行**。需要按 `REFERENCES` 依赖排序（跳过自引用）后再建索引；`assets_host` 一类被大量引用的表会被排到靠后。

**迁移文件的 PG 平行版本（`db/migrations/postgres/`）**

`db/migrations/postgres/` 与 `db/migrations/mysql/` **同版本号、同文件名**（000001…000024，各含 `.up.sql`/`.down.sql`，共 48 文件），由 mysql 侧逐条翻译而来；down 是对应 up 的严格逆操作。DDL 的类型与命名一律照 §4.7 与 `db/schema/postgres` 的既有写法，语句级另有下列非机械改写（每个受影响文件顶部都有 `-- PG 侧差异：` 注释说明）：

| MySQL 写法 | PG 写法 | 理由 |
|---|---|---|
| 会话变量 `SET @x := (SELECT ...)` + 后续引用 | 子查询 / 自连接 `sys_menu p` 按 `name`、`path` 取父 id | PG 无会话变量（菜单种子 000008/000016/000018/000019） |
| 多表 `UPDATE ... JOIN ... SET` | `UPDATE ... FROM`（自连接） | 多表更新在 PG 就是 `UPDATE ... FROM`；且 PG 的 `SET` 目标列不能带别名 |
| 多表 `DELETE t FROM t JOIN u ...` | `DELETE FROM t WHERE NOT EXISTS (...)` / `WHERE menu_id IN (子查询)` | 同语义，PG 无多表 DELETE |
| `INSERT IGNORE` | `INSERT ... ON CONFLICT DO NOTHING` | `sys_role_menu` 的唯一键是 `(menu_id, role_id)` |
| `CHANGE COLUMN old new <类型>` | `RENAME COLUMN`（仅类型真变时才追加 `ALTER COLUMN ... TYPE`） | PG 改名与改类型是两条语句（000020） |
| `MODIFY COLUMN c <类型> NOT NULL` | `ALTER COLUMN c SET NOT NULL` | 同上（000011） |
| `DROP INDEX` / `DROP KEY`（唯一键）、`DROP FOREIGN KEY` | `DROP CONSTRAINT` | MySQL 的 UNIQUE KEY / FK 在 PG 都是约束 |
| `KEY x (col)` 内联声明 | 独立 `CREATE INDEX x ON t (col)` | PG 无内联非唯一索引声明 |
| 布尔列写 `0` / `1` | `FALSE` / `TRUE` | PG 不接受整数隐式转 boolean（000018/000019 的 `is_expanded`） |
| `NOW(6)` / `CURDATE()` | `now()` / `CURRENT_DATE` | `sys_menu.create_time` 是 `date`，由 PG 隐式赋值截断为当天 |
| `ADD COLUMN c text NULL AFTER x` | 去掉 `AFTER`（新列落在表末尾） | PG 不支持列位置 |
| 字面量里的 `\\.` | 只写一个 `\` | MySQL 用反斜杠转义，PG 的 `standard_conforming_strings=on` 下反斜杠是普通字符。判据是**落库字符串两侧必须逐字节一致**（000001 的模板正文已实测相同） |

不可逆的部分沿用 mysql 侧语义，不额外补默认值：`ADD COLUMN ... jsonb NOT NULL`（000005/000006 的 down）在 PG 同样要求表内无数据；`000005.down` 同样只恢复列结构、不恢复 Django 时代的外键；`000010.down` / `000020.down` 也只恢复列与索引。**约束名全库唯一**这条在迁移里同样要守：mysql 侧名为 `name` / `agent_id` 的唯一键，在 PG 侧按 `<表名>_<键名>` 约定命名（`inspection_task_name`、`monitor_alert_route_name`；`agent_id` 全库无冲突故保持原名）。

**验收方式**（没有"迁移前的 PG 基线"，所以不能用空库灌迁移）：① 用 `db/schema/postgres` 在真 PG 14 建库（当时 79 表 0 错误；未使用的 `assets_agent_job_event` 已于 2026-09-17 移除，现为 78 表）；② 倒序执行全部 down 迁移（该轮为 22→1，每个文件按 golang-migrate 的方式作为**一个多语句批次**执行），回到折叠前状态；③ 再正序执行全部 up 迁移（该轮 1→22；此后又新增 000023 索引迁移与 000024 `sys_agent_token.api_id` 改名）。②③ 均 0 失败，最终 `information_schema` 的列（914）、约束（216）、索引（171）与折叠态逐条一致（数字为含 `assets_agent_job_event` 的 22 版快照）。唯一需要人工补的是 `000005.up` 要删的那个 Django 时代外键——`000005.down` 本身就不恢复它（mysql 侧同样如此），重放前按该名字补回即可。

**已知限制**：`autoadmin migrate` 角色仍只注册了 MySQL 驱动（见 §5.1 末尾），所以这 44 个文件目前只能用 `MIGRATION_SOURCE_URL=file://db/migrations/postgres` 配合 PG 驱动使用，角色侧接线仍属计划 P1-7。

### 4.8 方言切换：固定路径门面 + 构建标签（已落地 2026-09-16）

**机制**：应用代码只 import 一个包 `internal/platform/database/generated`，它是**方言门面**；背后是哪个方言的 sqlc 产物由**构建标签**决定：

| 构建 | 查询产物 | 连接 | 说明 |
|---|---|---|---|
| `make build`（默认，不带 tag） | `generated/mysql` | MySQL | 与改造前行为一致，应用代码零改动 |
| `make build-postgres`（`-tags postgres`） | `generated/postgres` | PostgreSQL（pgx） | 需要 `POSTGRES_DSN` |

门面由 `make facade` 生成（实现在 `internal/platform/database/derive/facade.go`），内容只有两类：
MySQL 侧是全部导出类型的别名 + `New` 转发；PG 侧是「嵌入 `*postgres.Queries` 的包装结构体」+ 非分歧类型的别名。**应用代码不受影响**：50 个 import 该包的 Go 文件一行没改。

**为什么 PG 侧要包装结构体而不是纯别名**：少数查询两侧签名不一致（§4.6.1 实测：205/263 逐字段一致，58 个字段类型不同、10 个 Params 只在 MySQL 侧存在），直接别名会让调用点编译不过。包装结构体让未分歧的方法由嵌入类型自动提升，分歧的少量查询在**手写**的 `dialect_postgres_adapters.go` 里提供与 MySQL 侧一致的签名：

| 分歧形态 | 适配做法 |
|---|---|
| 只传一个 pattern 的 count 查询（PG 把结构体展开成单个入参） | 补回 `XxxParams` 结构体 + 薄转发 |
| INSERT 的 `:execresult`（PG 侧派生为 `:one` + RETURNING，返回 `int32`/`int64`） | `insertResult{id}`：实现 `sql.Result`（`LastInsertId` 返回真实主键、`RowsAffected` 为 1），让调用点继续用 `result.LastInsertId()` |
| 告警历史的 `LabelKey`/`LabelValue`（MySQL `interface{}` vs PG `string`/`sql.NullString`） | 结构体按 MySQL 侧定义，转发时用 `labelKeyArg`/`nullStringArg` 显式换算 |
| 部署模板的 `Column3` ↔ PG 的 `Column1` | 字段改名映射 |
| `ListProjectsRow` 的聚合列（`string_agg` 在 PG 侧是 `interface{}`） | 定义行结构体 + 逐行转换 |
| `sql.NullInt64` → PG `interface{}` 这类**可赋值**方向 | 不需要适配（`database/sql` 走 `driver.Valuer`，NULL 语义一致） |

**守卫**（三条，都在 `go test` 里跑）：
1. `derive.TestFacadeMatchesRepository`：门面文件 == 现场生成结果（改 schema/查询后忘了 `make facade` 会失败）；
2. `derive.TestFacadeAdapterTypesDeclared`：排除出别名的类型必须在适配文件里真的声明（防止排除集与手写文件脱节）；
3. `database_test.TestNoUnreviewedDuplicateParams`：生成物里出现 `Xxx_2` 后缀字段而未被人工确认即失败（§2.5 那个静默 bug 的守卫）。

**已验证**：`go build` / `go vet` / `go test ./...` 在两个 tag 下都是 20 个包全绿；PG 变体实际连本机 PostgreSQL 跑通（按 code 搜索、按 name 搜索、不过滤、主机列表都返回正确结果）。

**残留的适配债（要正视）**：`dialect_postgres_adapters.go` 是手写的，两侧产物再出现分歧时它不会自动跟上——只有 `-tags postgres` 的构建/测试失败才会暴露。所以 **CI 必须两个 tag 都跑**（`make test && make test TESTFLAGS=-tags=postgres`，或至少 `make vet-postgres` + `make build-postgres`）。此外内联 SQL（P2 尚未迁移的其余包）在 PG 变体下仍是 MySQL 写法，**PG 变体目前只有 sqlc 查询部分真正可用**：`baseline`、`identity`、`inspection`、`automation` 四个域已于 2026-09-16 全部迁完并在真 PG 上跑通（§6.3），**所有 sqlc 的 INSERT 取主键已在 PG 侧修好（P1-10）**；`assets`/`monitor` 的内联 SQL 仍是 `ExecContext` + `LastInsertId()`，在 PG 下本来就因为 `?` 占位符不可用，随 P2-3 消除。

**P2-3 新增的适配**：`FailStaleAgentInstallJobsParams` / `CountActiveAgentInstallJobs`（可变长 IN 的元素类型分歧，见 §4.2）、`CountAutomationHostOptionsParams`（P2-2 的单 pattern 计数）。**新增适配的判定方式**：分歧不是靠事先盘点的——写查询时不用管，**`-tags postgres` 的编译会指名道姓地报出缺哪个类型/方法**，再去 `facadeAdapterTypes`（`derive/facade.go`）登记并把结构体与薄转发写进 `dialect_postgres_adapters.go`（`TestFacadeAdapterTypesDeclared` 会盯着两边一致）。P2-2 只新增了 1 条（`CountAutomationHostOptionsParams`：PG 侧单 pattern 入参被展开成裸参数）；`:execlastid` 与 INSERT 的 `:execresult` **不需要**适配——派生后两侧签名都是 `(int64, error)`；不需要 id 的 INSERT 一律写 `:execrows`/`:exec`，直接绕开 `LastInsertId` 这条方言分叉。

## 5. 落地机制

1. **sqlc 版本固定**：`go.mod` 的 tool 指令已 pin 到 v1.30.0（2026-09-17，之前是 v1.31.1），`go tool sqlc` 与仓库产物同版本；v1.31.1 会把重复的 `sqlc.arg` 去重（`Pattern_2/3/4` 合并），直接重新生成会打断一批调用方。**改动 schema/查询后请用 v1.30.0 生成**（`make generate`，Makefile 里仍有版本守卫兜底），并确认 `git diff` 只有预期变化。
2. **一个生成配置**：`sqlc.yaml` 里有两个 block（mysql → `internal/platform/database/generated/mysql/`、postgresql → `.../generated/postgres/`），`make generate` 一次产出两侧。
3. **生成必须幂等**：同一个配置连续两次 `sqlc generate` 应零差异（2026-09-16 实测两侧同时生成仍为零差异）；`make derive` 与 `make facade` 同样实测幂等。
4. **守卫测试**（把编译期抓不到的错误挡在 CI）：
   - `TestInlineSQLColumnArityMatchesScan`：内联 SQL 的 SELECT 列数 == `Scan` 目标数；
   - `TestNoInlineSQLReferencesDroppedHostAgentIDColumn`：源码中不得再引用已删列；
   - `TestLoadAgentTargetHostsColumnArity`：具体查询的列数/Scan 数锁定；
   - `derive.TestDerivedQueriesMatchRepository`：`db/queries/postgres/` 内容 == 现场派生结果（已实现，见 §4.6）；
   - `database_test.TestGeneratedCodeMatchesQueries`：**生成物漂移检查（P3-2）**。把 `db/schema` + `db/queries` 复制到临时目录、用 `go tool sqlc`（版本由 go.mod 的 tool 指令锁到 v1.30.0）重新生成，再与 `internal/platform/database/generated/{mysql,postgres}/` 已提交的产物逐字节比对，**改了查询/schema 没跑 `make generate` 即失败**，孤儿/新增产物也会报；不改工作区、不依赖数据库（`go test -short` 跳过）。
   - `database_test.TestSchemaMatchesRealDatabase`：**schema 与真库一致性（P3-4）**。解析 `db/schema/<dialect>` 的表与列，逐项断言真实库 `information_schema` 里存在（只查 schema→库方向，真库多出的 Django 框架表按设计忽略）；需 `SCHEMA_GUARD_DSN`，未设即跳过，两个 tag 各连本方言。
   - `database_test.TestInsertStatementsCoverRequiredColumns`：**INSERT 列集完整性（P4-10）**。纯文本解析：从 schema 取出"NOT NULL 且无默认值且非自增"的必填列，从 `db/queries/mysql` 取出每条显式列名的 INSERT，缺任一必填列即失败（P5 陷阱 27 的 `monitor_opensearch_cluster` 就是这么漏的）；不依赖数据库。
   - `derive.TestDeriveRewritesSliceToArrayParameter` / `TestDeriveRejectsUnrewrittenSlice`：可变长 IN 必须改写成 `= ANY(sqlc.arg(x)::bigint[])`，**残留 `sqlc.slice` 即失败**（P4-7 的静默陷阱）；
   - `derive.TestDeriveRewritesUpsertToOnConflict` / `TestDeriveRejectsUpsertWithoutConflictTarget`：UPSERT 必须改写成 `ON CONFLICT (<目标>) DO UPDATE`，**缺 `-- conflict:` 声明即失败**，且 INSERT 自己的值列表不能被误改；
   - `derive.TestDeriveIsDeterministic`：同一输入两次派生结果一致；
   - `derive.TestDeriveRewritesLastInsertIDToReturning`：`:execlastid` 与 INSERT 的 `:execresult` 必须被改写成 `:one` + `RETURNING id`，UPDATE 的 `:execresult` 必须保持原样（§4.3；漏了它 PG 变体的新建接口全部运行时失败）；
   - `derive.TestDeriveRejectsMultiRowInsertWithReturning`：多行 INSERT 不能用 RETURNING 取主键，必须在派生期报错；
   - `database_test.TestInsertQueriesReturnLastInsertIDAgainstRealDatabase`：**真库**插入冒烟（`DB_SMOKE_DSN` 未设即跳过），覆盖 14 条 INSERT 的 `LastInsertId`/`RowsAffected` 契约，两个方言各跑一次；
   - `inspection.TestSmokeInspectionQueriesAgainstRealDatabase`（`INSPECTION_SMOKE_DSN`）/ `automation.TestSmokeAutomationQueriesAgainstRealDatabase`（`AUTOMATION_SMOKE_DSN`）/ `assets.TestSmokeHostDomainQueriesAgainstRealDatabase`（`ASSETS_SMOKE_DSN`）：**真库**业务流程冒烟，两个方言各跑一次；含可变长 IN 的数组参数、UPSERT 的冲突分支、零行聚合、可空 json/数值列、`double` 列等只有真库能验的东西（见 §6.3）；
   - `derive.TestFacadeMatchesRepository` / `TestFacadeAdapterTypesDeclared`：门面（§4.8）漂移与适配清单一致性；
   - `database_test.TestNoUnreviewedDuplicateParams`：重复参数字段（§2.5）。
5. **改 schema 的完整动作**：改 `db/schema/mysql` → 加迁移（`db/migrations/mysql`，up/down）→ `make generate` → **同步翻译 `db/schema/postgres` 并重生成 PG 产物** → 跑守卫与全量测试。只改一侧即为半成品。
6. **改查询的完整动作**：只改 `db/queries/mysql` → `make derive` 重新派生 `db/queries/postgres` → `make generate` 两侧各自生成 → `make facade` 重建门面 → `make test`。**不要手改 PG 侧**（派生一致性测试会失败）。
7. **分页选型**：需要页码直达的列表走 OFFSET（本就双份）；只需"下一页/加载更多"的列表优先用 keyset 游标 + 字面量 LIMIT——它在两个方言里语法一致（§4.4），且深翻页更快。

### 5.1 构建与验证命令（两个方言变体）

```bash
cd autoadmin

# —— 默认构建：MySQL（与改造前行为一致）——
make test                     # go test ./...
make vet                      # go vet ./...
make build                    # 产物 bin/autoadmin（CGO_ENABLED=0，版本号由 git describe 注入）

# —— PostgreSQL 变体：查询走 PG 产物、连接走 pgx ——
make build-postgres           # 等价于 go build -tags postgres，产物 bin/autoadmin-postgres
make vet-postgres             # go vet -tags postgres ./...
go test -tags postgres ./...  # 全量测试也要在 PG 变体下跑

# —— 运行（两种变体同一个二进制入口，四个角色）——
./bin/autoadmin api           # HTTP :9000 + agent gRPC :9001
./bin/autoadmin scheduler     # 定时任务
./bin/autoadmin worker        # 消息消费
./bin/autoadmin migrate       # 执行 db/migrations/<dialect> 的 up 迁移
```

**环境变量**：默认构建读 `MYSQL_DSN`；`-tags postgres` 的构建读 `POSTGRES_DSN`（pgx DSN，须带 `TimeZone=UTC`，否则 timestamp 的时区语义与 MySQL 侧不一致）。`MIGRATION_DATABASE_URL`/`MIGRATION_SOURCE_URL` 供 `migrate` 角色使用。配置不自动读 dotenv，需自行 `set -a; . ./config.env; set +a` 或由部署环境注入。

**已知限制（P1-7 之前）**：`autoadmin migrate` 目前只注册了 MySQL 驱动（`internal/platform/migration/migration.go` 只 import `migrate/v4/database/mysql`），所以 `-tags postgres` 的构建虽然查询与连接都走 PG，**migrate 角色仍连不上 PostgreSQL**——需要时为它补一个构建标签分支并注册 `migrate/v4/database/postgres`（这一项与 `db/migrations/postgres/` 一起属于计划 P1-7）。

**CI 必须两个 tag 都跑**：`dialect_postgres_adapters.go` 是手写的，两侧产物再出现分歧时只有 PG 变体的构建/测试会暴露（§4.8）。最小集是 `make vet && make vet-postgres && make test` + `go test -tags postgres ./...`。


---

## 6. 已发现的欠账

### 6.1 幽灵表（已修）

`monitor_alert_route` 由迁移 `000015_notification_policy` 删除（被 `monitor_notification_policy` 策略树取代），但 `db/schema` 与 `db/queries` 未同步，导致 sqlc 生成了 3 条打不存在表的查询（无调用方）。已删除表定义与查询定义并重新生成。

**教训**：迁移与 schema 必须同时改；否则 sqlc 会静默生成错误代码，且只有真被调用时才炸。

### 6.2 未建模的表（已补，仅剩框架表）

真实库 91 张表，`db/schema` 已建模 78 张。此前缺失的 14 张业务表已补入（`agent_package`、`assets_agent_job`、`assets_cloudaccount`、`assets_hostdisk`、`assets_hostruntime`、`assets_webssh_temp_credential`、`automation_controller_ssh_key`、`automation_execution_host_log`、`monitor_log_collection_target`、`monitor_notification_policy`、`monitor_user_alert_media_binding`、`sys_user_group`、`sys_user_group_member`、以及后经确认无 Go 调用方而于 2026-09-17 移除的 `assets_agent_job_event`），解开了约 80 条内联 SQL 的迁移阻塞。2026-09-18 又补入两张日志采集批量作业表（`monitor_log_batch_job` / `monitor_log_batch_job_item`，见 migration 000033 与[LOG_COLLECTION_ARCHITECTURE](LOG_COLLECTION_ARCHITECTURE.md) §8.8）。

剩余 12 张未建模的是 Django 框架记账表（`auth_*`、`django_*`、`schema_migrations`），**不应建模**。

补充新表时的注意点：
- 列类型必须与真实库一致，否则 sqlc 会生成错误的 Go 类型。实测需注意 `bigint unsigned` → `uint64`、`tinyint(1)` → `BOOLEAN` → `bool`、`json` → `json.RawMessage`。
- `db/schema` 的既有风格是不带 `ENGINE=` / `CHARSET` / `COLLATE`；新增表应保持一致（`SHOW CREATE TABLE` 输出需手工归一化）。
- 新增表只应产生**新增** model；如果生成差异里出现既有类型的字段变动，说明类型映射写错了，必须回查。

### 6.3 内联 SQL 现状（迁移中，试点已完成）

原 359 条内联语句，按「能否 sqlc」分类：

| 形态 | 条数 | 说明 |
|---|---|---|
| 静态语句 | 323（89%） | 可直接迁 |
| 运行时拼 WHERE | 21（5%） | 用 `sqlc.narg` 表达 |
| 可变长 `IN (?,?,?)` | 12（3%） | sqlc 表达不了；**首选改成"取回集合在应用层比对"**（baseline 的类目归属校验就是这么做的），确实不适合的再保留内联 |
| 动态行数的多行 INSERT | 1 | 同上（占位符个数随入参变化） |
| UPSERT | 3（1%） | 按方言分叉 |

**P2 已完成（2026-09-16）**：七个包内联 SQL 全部清零，全仓库 `grep` 无 SELECT/INSERT/UPDATE/DELETE 字面量
（只剩派生脚本自身的常量与注释），`db/queries/mysql` 的查询定义从 228 条增到 559 条。

按包分布与进度：`baseline` 47 → **0**、`identity` 14 → **0**、`inspection` 33 → **0**、`automation` 36 → **0**、
`assets` 91 → **0**（主机域、agent 作业/安装包/应用控制、Agent 安装与更新流程、部署模板含 5 类嵌套子表 +
docker/compose 配置、逻辑服务与部署实例）、**`monitor` 138 → 0**（2026-09-16：监控软件包管理 15、监控目标域 36、
告警域 47、日志采集域 44、配置资源与 OpenSearch 11 —— `monitor.sql` 从 30 条定义增到 175 条）。

`monitor` 这一轮用到的构造（对上游规律有增补，已并入 §4.6 的规则表与 P5 陷阱 26–30）：
- **两类"插入即跳过"**（通知事件按 `deduplication_key`、日志目标按 `host_id`）→ `INSERT IGNORE` + 派生改写成
  `ON CONFLICT DO NOTHING`，两侧判定收敛到按方言分文件的 `insertIgnoreOutcome`；
- **单地址投递的 get-or-create**（唯一键含可空列）→ MySQL 的 `id=LAST_INSERT_ID(id)`，PG 侧由 perQueryOverride
  换成 `DO UPDATE SET id=<表>.id RETURNING id`（**不能写 `EXCLUDED.id`**）；
- **通用配置资源的"运行时拼表名 + 列名"** → 表名按资源分派到显式语句，"只写提交了的列"改由
  "读回整行 + 应用层合并 + 整行写"承担（`COALESCE(narg,col)` 表达不了"显式写入空值"）；
- 其余方言构造就地改掉：`TIMESTAMPDIFF`/`UTC_TIMESTAMP` → 应用层算、`IF(...)`+`JSON_LENGTH(...)` →
  加锁读回后应用层合并、`JSON_UNQUOTE(JSON_EXTRACT(...))` → 取列应用层解析、`SUM(布尔)` →
  `COUNT(CASE WHEN …)`、动态 `SET` 列名 → 整行写。

**三条与"生成物 vs 真库"有关的教训**（都是真库冒烟逮到的，见 P5 陷阱 26–28）：
可空 json 列扫不了 NULL（要 override 成 `sql.NullString`）、INSERT 必须列全"NOT NULL 且无默认值"的列
（否则严格模式 1364；`monitor_opensearch_cluster` 的建表语句一直缺三列）、含可空列的唯一键上做 get-or-create
必须给非 NULL 值（UNIQUE 把 NULL 视为互不相同）。

已迁移的七个包各自留下一个**真库冒烟**（`*_SMOKE_DSN` 触发、默认跳过）：`internal/baseline/smoke_test.go`
（该域全流程 44 步，事务内回滚）、`internal/identity/smoke_test.go`、`internal/inspection/smoke_test.go`、
`internal/automation/smoke_test.go`（后四个是 handler/queries 级写路径 + 按 id 清理自己造的行）、
`internal/monitor/smoke_test.go`（监控目标域）、`internal/monitor/smoke_alert_test.go`（告警/通知/监控总览）
与 `internal/logcollect/smoke_log_test.go`（日志采集与 ES 存储，含通用配置资源写路径），三者共用
`MONITOR_SMOKE_DSN`；另有跨域的
`internal/platform/database/insert_smoke_test.go` 覆盖"插入取主键"契约。

两条**真库冒烟的纪律**（2026-09-16，P2-2 踩出来的）：
1. **会改动全局数据的语句只在回滚事务里验语义**。保留期清理的 cutoff 是全局的、控制器 SSH 密钥一换
   就打断线上自动化链路——这类语句在共享库上直接跑会删掉/改掉别人的数据。做法是：开一个事务跑语句、
   在事务内断言效果、然后 `Rollback`，自己造的行再用**按 id 的精确删除**收尾。
2. **`db/schema` 的快照可能没有真库上的外键**。`inspection_target_execution.host_id → assets_host(id)`
   只存在于真库（`db/schema/mysql/003_monitor_inspection.sql` 是"够 JOIN 用"的部分声明），
   sqlc 不校验外键所以生成期无感，写数据时才被拒——造数据要么取真实主机 id，要么写 NULL。
   这类漂移归 P3-4 的守卫管。

路径类差异也在这一步暴露：inspection/automation 的迁移里，`NOW()`/`UTC_TIMESTAMP` 全部改成应用层传时间，
`JSON_SET`/`JSON_MERGE_PATCH` 改成"`FOR UPDATE` 读回 + 应用层合并"，`TIMESTAMPDIFF` 改成应用层算时长
（取消路径先读回 `start_time`），`SUM(布尔)` 改成 `COUNT(CASE WHEN … THEN 1 END)`（零行从 NULL 变 0），
MySQL 多表 `DELETE … JOIN …` 改成 `WHERE … IN (子查询)`，运行时拼 `WHERE`/`ORDER BY` 改成
`sqlc.narg` 可选过滤 / 按 `sort_key` 选择的 CASE 表达式（标识符不能被参数化，代价是排序不再走索引）。

`baseline` 试点（2026-09-16）验证了迁移的固定动作与代价，后续包按同一套做（`identity` 已照此完成）：

1. 先接上已有定义（该域原有 30 条定义闲置、22 个生成方法无调用方），再补缺口（扫描列表/详情/取消、级联删除）；
2. 方言构造就地在源里改掉（JSON 拼装、`FIELD`、`SUM(布尔)`），能靠应用层消化的不做 override；
3. 测试从"断言 MySQL 文本"改成"断言方言无关片段 + 片段内稳定的参数顺序"（含一个按 tag 编译的
   `expectCreateReturnsID` 辅助，屏蔽 `:execlastid`(Exec) 与 `:one`+RETURNING(Query) 的差异）；
4. 补上原来没有覆盖的响应组装用例（列表/详情的字段映射、NULL 时间、decimal 换算）；
5. 留一个 opt-in 的真库冒烟测试（`internal/baseline/smoke_test.go`，`BASELINE_SMOKE_DSN` 未设即跳过、
   事务内跑完整业务流程后回滚），两个方言各跑一次——mock 全绿挡不住驱动层差异。

