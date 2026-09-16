# PostgreSQL 查询集（由 MySQL 派生，禁止手改）

本目录与 `../mysql/` 对称，但**方向是单向的**：`db/queries/mysql/` 是唯一人工维护来源，
本目录的 `.sql` 由 `make derive` 机械派生。手改这里的文件不会生效——`go test` 会直接失败。

## 怎么改

```bash
# 改查询：只改 db/queries/mysql/*.sql，然后
make derive                            # 重新派生到本目录
make generate SQLC=<v1.30.0 的 sqlc>   # 两侧产物一起重生成
make test                              # 派生一致性测试会拦住忘了派生的情况
```

派生规则见 [SQL_DESIGN.md §4.6](../../../../docs/architecture/SQL_DESIGN.md)，
实现在 `internal/platform/database/derive`：

1. **override 表**（`derive/overrides.go`）：`GROUP_CONCAT`→`string_agg`、
   `JSON_ARRAYAGG`/`JSON_OBJECT`→`json_agg`/`json_build_object`、
   `JSON_UNQUOTE(JSON_EXTRACT(x,'$.k'))`→`x->>'k'`、`CAST(... AS CHAR/SIGNED)`→`AS text/bigint`。
   override 未命中会直接报错——静默跳过等于产出一份语法通过但语义错的 SQL。
2. **反引号标识符 → 双引号**：MySQL 源里必须用反引号。双引号在 sqlc 的 mysql 引擎里是
   **字符串字面量**而不是标识符（实测 `SELECT id, "order" FROM t` 会生成一个 string 类型的
   `Column3`），所以不存在「一种引号两边通用」的写法。
3. **`?` → `$n`**：按出现顺序逐个编号，每个 `?` 一个独立参数，不去重。
   `sqlc.arg`/`sqlc.narg` 原样保留：两侧引擎都支持，PG 侧靠它保住参数名
   （改写成 `$n` 会让 sqlc 退回 `Column1`/`Column2` 兜底命名）。

## 现状

- ✅ **schema**：`db/schema/postgres/` 由 `db/schema/mysql/` 按 §4.7 翻译而来，
  两侧生成的 Go model 完全一致（79 张表，含可空性），由 `sqlc.yaml` 里 postgresql block 的
  `overrides` 保障（可空 jsonb、24 个 unsigned 列）。**已用真 PostgreSQL 14 装载验证**：
  79 张表、71 个外键零错误建库（装载前需按外键依赖排序，见 SQL_DESIGN §4.7）。
- ✅ **queries**：12 个文件、228 条查询全部派生并生成通过，产物在
  `internal/platform/database/generated/postgres/`。**已用真 PG 逐条 `PREPARE` 验证：228/228 通过**
  （含参数类型推断；此前 46 条因 `sqlc.narg(x) IS NULL` 写在 OR 链首位而解析失败，见 SQL_DESIGN §2.2）。
- ✅ **配置**：postgresql block 已合并进 `sqlc.yaml`，`make generate` 一次产出两侧且幂等。
- ✅ **守卫**：`derive` 包的 `TestDerivedQueriesMatchRepository` 断言本目录 == 现场派生结果
  （内容被改、文件变多或变少都会失败）。
- ✅ **应用侧接线**：`internal/platform/database/generated` 是方言门面，默认构建走 MySQL 产物、
  `-tags postgres` 走本目录产物并连 pgx（`make build-postgres`，需 `POSTGRES_DSN`）。
  门面由 `make facade` 生成，分歧查询的适配见 SQL_DESIGN §4.8。
- ⏳ **未做**：灌入数据后的逐条 `EXPLAIN`（P1-9 的后半）、PG 平行迁移（P1-7）。
  另外内联 SQL（P2 未迁的约 397 条）在 PG 变体下仍是 MySQL 写法，所以 PG 变体目前只有 sqlc 查询部分可用。

## 已知的两侧签名差异（接入调用方时需要适配）

派生保证 SQL 语义等价与**字段名**一致；Go 类型在两类情形下不同，这是 sqlc 两个引擎的行为
差异，不是派生能消除的：

| 情形 | MySQL 产物 | PG 产物 | 规模 |
|---|---|---|---|
| `sqlc.narg(x)` 用作 `IS NULL OR col = ...` 可选过滤 | `sql.NullString`/`sql.NullInt64`/`sql.NullBool`/`sql.NullTime` | `interface{}` | 58 个 Params 结构体 |
| 同一条查询里重复出现同一个 `sqlc.arg(x)` | 拆成多个参数（`Pattern`、`Pattern_2`、`Pattern_3`…） | 合并为一个 `$n` | 10 条查询的 Params 只在 MySQL 侧存在 |

现场统计：263 个共有结构体中 205 个逐字段一致，58 个有上表第二列的类型差异。
接入调用方（方言切换）时，这批查询需要一层适配或按方言分叉。
