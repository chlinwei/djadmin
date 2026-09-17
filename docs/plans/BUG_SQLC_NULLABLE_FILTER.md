# 问题记录：sqlc 列表查询的可选 application_id 过滤失配（0 vs NULL）

- **日期**：2026-09-15
- **状态**：已修复；后续机制建设并入双库改造计划（见文末）
- **发现路径**：编辑逻辑服务弹窗"应用版本"下拉为空 → 抓包 `GET /assets/application-versions/?enabled=true` 返回 `count:0`

## 现象

前端编辑逻辑服务/新建服务等弹窗里，**应用版本下拉永远没有选项**；集群模型下拉同类风险。
数据库里 `assets_application_version` 明明有 9 行。

## 根因

`autoadmin/db/queries/assets.sql` 中 4 个列表/计数查询用「可选过滤」写法：

```sql
WHERE (sqlc.arg(application_id) IS NULL OR v.application_id=sqlc.arg(application_id))
```

但 sqlc 生成物把参数落成了**非空 `int64`**（`ListApplicationVersionsParams.ApplicationID int64`）。
handler 在前端不传 `application` 参数时经 `queryID` 返回 **`0`**（handler.go:77，`""`/`"0"` → 0），
SQL 里 `0 IS NULL` 恒假 → 走 `v.application_id=0` → **恒 0 行**。
即：**全量拉取路径全坏，只有显式带 `application` 参数的入口碰巧能用**。

受影响查询（4 个，同模式）：

- `CountApplicationVersions` / `ListApplicationVersions`
- `CountClusterProfiles` / `ListClusterProfiles`

## 修复

语义改为 **`0 = 不过滤`**（与 `queryID` 的 0 值约定对齐，避免引入可空参数类型）：

```sql
WHERE (sqlc.arg(application_id) = 0 OR v.application_id=sqlc.arg(application_id))
```

- `.sql` 与 `internal/platform/database/generated/assets.sql.go` 常量**手工同步**（本仓库
  生成代码为手同步约定，sqlc 工具链未接入 CI）。
- Go 侧无需改动：`queryID` 缺省 0 即"不过滤"。
- 验证：修复后同接口返回 `count:9`（9 个版本全部可见）。

## 教训与待办

1. **sqlc 可选过滤禁止用 `IS NULL` + 非空参数**：要么参数类型用 `sql.NullInt64`，要么约定
   `0 = 不过滤`。新增列表查询时检查 handler 缺省值与 SQL 语义是否一致。
   **精确化（2026-09-16 补充）**：用 `sqlc.narg(x)` 代替 `sqlc.arg(x)` 即可 —— `narg` 生成可空
   参数（`sql.NullInt64` / `sql.NullString`），`IS NULL` 语义成立；`sqlc.arg` 生成非空参数，
   `IS NULL` 恒假。本仓库 200+ 处可选过滤已经是 `sqlc.narg` 写法（正确），出问题的 4 处是
   `sqlc.arg`。**新增可选过滤一律用 `sqlc.narg`。**
2. **生成代码与 `.sql` 的手同步缺校验**：两者已漂移过一次（`IS NULL` 版本在生成物里查不到
   行却无人发现）。
   **2026-09-16 进展**：`make generate`（sqlc）已接入并加了 **版本守卫**（`go.mod` 的 tool 指令是
   v1.31.1，会去重参数名、静默产出不一致产物，因此版本不符时直接失败，用
   `make generate SQLC=<v1.30.0 路径>`）。**仍待办**：生成物漂移即失败的检查、以及列表接口
   "全量拉取应有数据"的集成测试 —— 见 [SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md](SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md) 的 P3-2。
3. **回归范围提醒**：同模式还可能存在于后续新增的 `sqlc.arg(x) IS NULL` 查询，review 时按
   教训 1 检查。

---

## 家族里的第二个 bug：同名参数被拆成多个字段，调用点漏设即静默失效（2026-09-16）

同一次 SQL 治理里发现的另一处「sqlc 参数生成语义」故障，与本文的 `0 vs NULL` 是同一家族
（都是"参数类型/个数与调用点之间没有编译器保护"）：

- **现象**：`assets` 域列表接口（项目、业务系统、环境、凭据、主机分组、主机、应用、应用版本、
  集群模型、部署模板）按编码 / 负责人 / IP / 备注**搜索恒返回空**，按名称搜索正常。无任何报错。
- **根因**：这些查询把同一参数写了多次（`name LIKE ... OR code LIKE ... OR owner LIKE ...`）
  且用 `CAST(sqlc.arg(pattern) AS CHAR)` 包参数。sqlc 的 mysql 引擎**按出现次数**生成
  `Pattern`、`Pattern_2`、`Pattern_3`、`Pattern_4` 四个字段，调用点只设了 `Pattern`，
  其余为 nil → `LIKE NULL` → NULL。
- **与本文 bug 的关系**：两者都源于"sqlc 生成的参数形态 ≠ 调用点的假设"，且都**静默**。
  区别是本文的 bug 让过滤恒不生效（恒 0 行），这个 bug 让过滤只在一个字段上生效。
- **修法**：列统一 `COALESCE(col,'')` + `sqlc.narg` → 合并成一个 `Pattern sql.NullString`
  字段，生成的 Go 把它传给每个占位符。完整的写法矩阵与规则见
  [SQL_DESIGN.md §2.5](../architecture/SQL_DESIGN.md)。
- **验证**：真库 SQL 层面对比（按 IP 搜索：旧传参 0 行 / 全部传值 1 行）；修后经 API 端到端
  验证（`GET /assets/hosts/?search=10.25.66.201` 返回 1 条，按 instance_name 仍 1 条，
  不存在的 IP 为 0 条）。

**新增教训**：`Params` 里出现 `Xxx_2` / `Xxx_3` 这类后缀字段时，**必须逐个确认调用点是否传值**——
sqlc 不会为"漏设字段"报错，编译器也不会（字段存在且可零值）。这是 review 时的固定检查项。

---

## 家族里的第三个 bug：`= 0` 约定下传了 NULL，列表恒空（2026-09-17）

- **现象**：`GET /assets/cluster-profiles/`（不选应用、`search=` 空）返回 `count:0`，a-table 无数据。
- **根因**：查询用 `sqlc.arg(application_id) = 0` 表示"不过滤"（本文档第一处修复的约定）。但
  `assets_cluster_profile.application_id` 可空，sqlc 把这个 `sqlc.arg` 生成为 **`sql.NullInt64`**；
  仓储层又写成 `sql.NullInt64{Int64: applicationID, Valid: applicationID > 0}` —— app=0 时是
  `{0, false}` 即 **NULL**，`NULL = 0` 求值为 NULL，整条 WHERE 变 NULL → 恒 0 行。
  这是 [P5 陷阱 22](SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md)（"空值表示不过滤"必须传**有效的**
  空串/0，不能传 NULL）的一个实例，也是本文档第一处 bug 的复发。
- **修法**：`Valid: true`（app=0 时传有效的 0）。修在 `internal/assets/catalog.go` 的 `ListProfiles`。
- **守卫**：新增 `TestListProfilesWithoutApplicationFilterReturnsRows`（sqlmock，`notNilArg`
  匹配器断言 application_id 位置参数**非 nil**）；参数退回 NULL 时 mock 不匹配、用例失败。
- **同类排查**：全仓库只有这一处 `Valid: .. > 0` 喂给 `= 0` 查询；另 4 处同模式查询
  （versions / hosts 的 group、environment）调用点传的是有效值，未中招。

## 附：可空标量子查询不能直接 Scan 进非空 int64（同一轮发现的 500）

`GET /assets/application-deployments/` 返回 500：`sql: Scan error on column index 13, name
"application_id": converting NULL to int64 is unsupported`。部署实例可以没有任何关联逻辑服务
（`assets_application_service_deployment` 无行），此时查询里的标量子查询
`(SELECT s.application_id ... LIMIT 1) AS application_id` 返回 NULL，而 sqlc 按列的非空性生成了
`int64`。

修法：在 SQL 里用 `CAST(COALESCE((...), 0) AS SIGNED) AS application_id` 消掉 NULL（Go 侧本就
把 0 当"无关联"）。**注意不能只写 `COALESCE(subquery, 0)`**：sqlc 的 MySQL 引擎对没 CAST 的
`COALESCE(标量子查询, 0)` 推断成 `interface{}`（PG 侧同样），调用点用不了；`CAST(... AS SIGNED)`
才能钉成 int64，并由 `derive` 的 global override 把 `AS SIGNED` 翻成 PG 的 `AS bigint`。

