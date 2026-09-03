# 应用目录与集群模型（autoadmin）

描述应用/版本/集群模型目录的查询逻辑与"按应用筛选"语义。后端唯一实现为 Go 版 autoadmin（Django 已废弃）。

## 数据流

- 入口（`internal/api/router/router.go`）：
  - `GET /assets/applications/` → 应用列表
  - `GET /assets/application-versions/` → 版本列表（可选 `application` 筛选）
  - `GET /assets/cluster-profiles/` → 集群模型列表（可选 `application` 筛选）
- 链路：Handler（`catalog_handler.go`，`queryID("application")` 解析筛选参数）→ Service → Repository（`catalog.go`）→ sqlc 查询（`db/queries/assets.sql`，生成于 `internal/platform/database/generated/`）。

## 按应用筛选语义（关键决策）

- 参数缺省/为 0 → 不筛选，返回全部。
- Repository 统一传 `sql.NullInt64`（不筛选时为 SQL `NULL`），因此所有可选筛选查询必须写成 **`WHERE (sqlc.arg(x) IS NULL OR col=sqlc.arg(x))`**。
- ⚠️ 禁止写 `sqlc.arg(x)=0 OR ...`：NULL 参数与 0 比较恒为假，会导致"不筛选时列表为空"。此类写法曾在 ListApplications/ListApplicationVersions/CountClusterProfiles/ListClusterProfiles 四个查询中出现并已修复；新增带可选筛选的 sqlc 查询时必须用 `IS NULL` 形式。
- 集群模型列表 LEFT JOIN `assets_application` 带出所属应用名，`service_count` 当前恒为 0（占位列）。

## 项目列表的关联业务系统

- 项目与业务系统的关系：`assets_business_system.project_id → assets_project.id`（业务系统挂在项目下）。
- `ListProjects` / `GetProject` 通过 `GROUP_CONCAT(bs.name SEPARATOR '||')` 聚合关联业务系统名，Go 侧拆分为数组以 `business_system_names` 字段返回（前端"关联业务系统"列渲染 tag）。
- 同时聚合 `GROUP_CONCAT(bs.id SEPARATOR '||')` 以 `business_systems`（ID 数组）返回——服务树"资源占比"的"按项目"维度依赖它做部署→服务→项目归集。
- ⚠️ 聚合分隔符必须用 `'||'` 而不是逗号：名称本身可能含逗号；拆分逻辑在 `splitBusinessSystemNames`/`splitBusinessSystemIDs`（`internal/assets/service.go`）。
- 注意 `sqlc` 对 COALESCE 子查询形式的 GROUP_CONCAT 推导为 `interface{}`（GetProject），需在 Go 侧做 string/[]byte 断言（`businessSystemIDsFromAny`/`nameRaw`）。

## 服务树"资源占比"数据链（部署→服务→业务系统/项目）

前端 `ServiceTree.vue` 用四份数据聚合 CPU/内存饼图，Go API 必须提供：

- `GET /assets/application-deployments/`：每条部署带 **`application_service_ids`**（`assets_application_service_deployment` M2M 关联，`attachApplicationServiceIDs` 批量补齐，未关联输出 `[]` 非 null）+ `host`（关联主机 ID）。
- `GET /assets/application-services/`：带 `business_system` + `business_system_name`。
- `GET /assets/hosts/`：带 `hardware.cpu_cores` / `hardware.memory_gb`。
- `GET /assets/projects/`：带 `business_systems`（见上）。

缺失任一关联字段，饼图在该维度恒为空（前端 `linkedServices` 过滤后无归集桶）。

## 失败语义

- 分页/参数非法 → 400；数据库错误 → 500（`response.Error`）。
- 空列表返回 `{results: [], count: 0}`，前端已做数组归一化。
