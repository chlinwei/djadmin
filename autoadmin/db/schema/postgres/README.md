# PostgreSQL schema（折叠态）

本目录是 PostgreSQL 的**当前结构快照**，供 sqlc 解析与"从零建一个 PG 库"使用。分工与约定见
[docs/architecture/SQL_DESIGN.md](../../../docs/architecture/SQL_DESIGN.md) §4.7。

- 按 Django 域切分 6 个文件（`001_identity_rbac_config` … `006_baseline`），与 `db/schema/mysql/` 同名对应；
  改 mysql 侧的结构时要同步翻译这里。
- 它是**折叠态**（当前状态的并集），不是可执行迁移的替代：增量变更是 `db/migrations/{mysql,postgres}/`
  的职责，两侧版本号一一对应。

## 不能按文件名顺序灌库

PostgreSQL 建表时要求被引用的表已存在，而本目录按域切分 —— 跨文件外键（例如 `assets_host` 引用
`assets_hostgroup`）与自引用外键让"`for f in *.sql; do psql -f $f; done`"必然失败。sqlc 只解析不执行，
所以这个问题在生成期看不出来。

用仓库里的脚本生成一个可直接灌库的装载脚本：

```bash
python3 db/schema/generate_load_order.py > /tmp/pgschema_ordered.sql
psql "postgres://user@host:5432/scratch?sslmode=disable" -v ON_ERROR_STOP=1 -f /tmp/pgschema_ordered.sql
# 期望：78 张表、0 报错
```

脚本做三件事：`CREATE TABLE` 按外键依赖**拓扑排序** → 其余语句（`ALTER`/`COMMENT`）→ `CREATE INDEX`
放最后（否则索引会先于它依赖的列不存在）。

## 这个装载脚本用来做什么

- **P1-7 的迁移复核**：从折叠 schema 建库 → 倒序回放全部 `down` → 正序回放全部 `up` →
  累积结果必须与折叠态**逐条一致**（列/约束/索引）。实测记录见计划 P1-7。
- **P1-9 的 EXPLAIN 试验**：灌入放大到真实规模的数据后逐条看计划，结论见 SQL_DESIGN §2.6。

索引里有两类值得注意：

1. **外键列索引**：MySQL 建外键时若没有可用索引会**自动创建**一个，PG **不会** —— 两侧因此会有不同的
   执行计划。本目录已按 MySQL 的既有索引逐列补齐（P1-9 实测）。
2. **`monitor_alert_history_firing_recent_idx`**：PG 专属的 partial index（`WHERE state='firing' AND
   source='prometheus'`），供失联对账的每 5 分钟一次的扫描使用。
