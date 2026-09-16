# 问题记录：sqlc 列表查询的可选 application_id 过滤失配（0 vs NULL）

- **日期**：2026-09-15
- **状态**：已修复（未提交；`schema_migrations` 与生成代码已同步）
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
2. **生成代码与 `.sql` 的手同步缺校验**：两者已漂移过一次（`IS NULL` 版本在生成物里查不到
   行却无人发现）。待办：接通 `make generate`（sqlc）并加"生成物漂移即 CI 失败"的检查；
   或为列表接口补"全量拉取应有数据"的集成测试。
3. **回归范围提醒**：同模式还可能存在于后续新增的 `sqlc.arg(x) IS NULL` 查询，review 时按
   教训 1 检查。
