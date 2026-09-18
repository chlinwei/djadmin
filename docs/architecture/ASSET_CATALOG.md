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

## 服务树"资源占比"数据链（部署→服务→项目）

服务树"资源占比"只按**项目**聚合（"按业务"维度已移除）。前端 `ServiceTree.vue` 用四份数据聚合 CPU/内存饼图，
Go API 必须提供：

- `GET /assets/application-deployments/`：每条部署带 **`application_service_ids`**（`assets_application_service_deployment` M2M 关联，`attachApplicationServiceIDs` 批量补齐，未关联输出 `[]` 非 null）+ `host`（关联主机 ID）。
- `GET /assets/application-services/`：带 `business_system`（用于经项目的 `business_systems` 归集到项目）。
- `GET /assets/hosts/`：带 `hardware.cpu_cores` / `hardware.memory_gb`。
- `GET /assets/projects/`：带 `business_systems`（见上）。

缺失任一关联字段，饼图恒为空（前端 `linkedServices` 过滤后无归集桶）。

## 逻辑服务 / 部署实例列表的 scope 过滤（服务树右侧面板）

服务树选中节点后，右侧 `ServiceTreeNodeContent.vue` 按节点层级向后端传过滤参数；Go 版
与 Django 版 DRF filter 字段名保持一致，语义为"缺省 = 不筛选"：

- `GET /assets/application-services/`：
  - `search`（名称/编码模糊）；
  - `business_system` → 仅返回该业务系统下的逻辑服务。服务树的"业务系统"节点（展示其下逻辑服务）与"环境"节点（展示当前业务×该环境的逻辑服务）都依赖此参数，缺失会退化为返回全部逻辑服务。
- `GET /assets/application-deployments/`：
  - `application_service` → 仅返回与该逻辑服务存在 M2M 关联（`assets_application_service_deployment`）的部署实例；"逻辑服务"节点右侧的部署实例列表依赖此参数。
  - `application_service__business_system` → 经 M2M 关联到逻辑服务、再按业务系统过滤；"业务系统/环境"节点的部署实例列表依赖。
  - `application_service__environment` → 经 M2M 关联到逻辑服务、再按环境过滤。
- 非法整数参数 → 400（`applicationDeploymentFilterFromQuery` / `optionalIDQuery`）；
- 过滤在 SQL WHERE 层完成（EXISTS 子查询），COUNT 与列表共用同一条件，分页计数正确。

## 逻辑服务 ↔ 部署实例关联：归属与按 id 读取（2026-09-18 修复）

关联表 `assets_application_service_deployment`（实例与逻辑服务的 M2M）有两条必须守住的规则：

- **唯一写入口在服务侧，且按设计不提供"实例侧反向绑定"（2026-09-18 定案）**：
  `POST/PATCH /assets/application-services/` 的 `member_configs`（整组替换：`DeleteServiceDeployments`
  后按键重建），成员级的 `enabled` 也在这里。**实例侧不接收服务关联字段**——
  `ApplicationDeploymentInput`（`internal/assets/service_runtime_write.go`）没有该字段，这是刻意的：
  一个实例可同时属于多个应用下的服务，让实例侧保存去改关联，与"服务侧整组重建"的语义会互相踩
  （实例侧保存若不替换就会失效，要替换就会删掉别的服务的关联）。
  历史缺陷：前端 `DeploymentDialog` 曾把 `application_service` 放进提交体，后端结构体并不接收、
  静默忽略，表现为"在实例弹窗里绑定了服务，其实库里没写"；该字段已从前端 payload 移除。
  绑定入口只有两个：服务编辑弹窗成员区的「新增部署实例」与「从已有实例中选择」，
  两者都只是把实例加进成员列表，真正的关联写入发生在保存服务时（服务侧）。
- **部署实例的读取一律按 id**：`GET /assets/application-deployments/:id/`、`POST .../{id}/control/`
  与"保存后回读"都走 `GetApplicationDeploymentDetail`（`WHERE d.id = ?`）。
  **禁止**改回"用列表查询在内存里找目标行"：列表是 `ORDER BY d.id DESC LIMIT ?`，
  "第 1 页、每页 1 条"恒为全库 id 最大的一台，保存后回读会把刚保存的实例判成不存在
  （2026-09-18「编辑实例报资产不存在」，而写入其实已生效）；`GET` 与 `control` 原先用
  `Size: 100000` 把全部实例拉回来找一行，实例数一多就是每请求一次全表。

`application_id`（"部署关联的首个服务所属应用"）是**派生展示字段**，不等于"实例自己的应用"：
一个实例可以同时属于多个应用下的服务（如 105 同时在 redis 与 nginx 下），取的是它**最早**
那条关联（`ORDER BY l.id LIMIT 1`）。因此：

- 前端**不得**用它过滤成员列表或候选实例。此前 `ApplicationServiceDialog.vue` 用
  `application_id === form.application` 过滤已选成员，两个 watcher 又拿这份列表去删
  `selectedDeploymentIds`，于是已绑定的实例一进编辑弹窗就被静默剔除（库里关联还在），
  保存时还会把空成员写回去。现在该过滤已整体移除，成员列表只以服务端返回的
  `member_instances` 为准，移除成员只走成员行的删除按钮。
- 绑定入口有两处：成员区的「新增部署实例」（走模板/版本校验的新建流程）与
  **「从已有实例中选择」**（候选 = 全部实例刨掉已绑定的，同应用仅用于排序）；
  两者都只是把实例加进成员列表，真正的关联写入仍在保存服务时（`member_configs`）。

失败语义：目标实例不存在时，读取与保存后回读都返回 404「资产不存在」（`translate(sql.ErrNoRows)`）。

## 失败语义

- 分页/参数非法 → 400；数据库错误 → 500（`response.Error`）。
- 空列表返回 `{results: [], count: 0}`，前端已做数组归一化。

## 数据访问层（sqlc，迁移中 2026-09-16）

assets 域的内联 SQL 正在按 [SQL_DESIGN.md](SQL_DESIGN.md) 的约定收敛到 sqlc：语句定义在
`autoadmin/db/queries/mysql/assets.sql`，Go 侧只调 `internal/platform/database/generated`
（方言门面，默认 MySQL、`-tags postgres` 走 PostgreSQL）。**已完成主机域与 agent 作业/安装包部分**，
模板与逻辑服务域待迁（进度与剩余清单见 `docs/plans/SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md` 的 P2-3）。

已迁部分的最终逻辑与取舍：

- **采集结果落库（`persistHostInfo`）**：`assets_hostruntime`/`assets_hostsystem`/`assets_hosthardware`
  三张表按 `host_id` 唯一键 UPSERT。MySQL 源写 `ON DUPLICATE KEY UPDATE a=VALUES(a)`，查询头声明
  `-- conflict: host_id`，派生脚本改写成 PG 的 `ON CONFLICT (host_id) DO UPDATE SET a=EXCLUDED.a`
  （两侧生成的参数结构体一致，调用点不分叉；缺声明或残留 MySQL 子句会让派生失败）。
  「静态指纹未变则跳过 system/hardware 落库」的判断保持在应用层（读 `GetHostRuntimeFingerprint`）。
- **采集状态两态**：成功写 `collect_time`，失败传 NULL 由 `collect_time = COALESCE(?, collect_time)`
  保留上次采集时间（迁移前是两条不同的 UPDATE）。
- **磁盘整表重建**：`assets_hostdisk` 没有按 device 的唯一键，采集时先 `DeleteHostDisks` 再逐条
  `CreateHostDisk`（顺序与事务范围不变）。
- **主机详情**：`GetHostSystem`/`GetHostHardware`/`GetHostRuntime`/`ListHostDisks`/`ListHostMonitors`
  五个只读查询分别对应详情页的 system/hardware/runtime/磁盘/监控目标区块；缺行时是 `sql.ErrNoRows`
  而不是驱动错误（迁移前依赖手写 Scan 的零值语义）。
- **身份唯一性校验**：`HostIPExists`/`InstanceNameExists` 由 `CountOtherHostsByIP` /
  `CountOtherHostsByInstanceName` 承担（`WHERE col = ? AND id <> ?`），仍是服务层校验而非 DB 唯一约束
  （IP 侧为存量重复数据留收敛余地；`instance_name` 侧另有迁移 000022 的 UNIQUE KEY 兜底）。
- **agent 作业列表**：`GET /agent/jobs` 的 host_id/action 过滤从运行时拼 `WHERE (?=0 OR …)` 改成
  `sqlc.narg`（NULL 表示不过滤），四条查询共用同一组条件；`group_by=action` 与状态汇总分别走
  `ListAgentJobActionCounts` / `ListAgentJobStatusCounts`。
- **agent 安装包**：仍是"单槽位当前包"语义（`version='default'`，`is_active` 唯一激活），
  列表/下载取 `is_active=1` 的最新一行，激活时先把其它行置 0。**注意 `agent_package.id` 是
  `bigint unsigned`，两侧都生成为 `uint64`**，调用点按 `uint64(id)` 传（HTTP 层的 id 仍是 int64）。
- **Agent 安装/更新流程**：`agent_install.go` 与 `agent_update.go` 原本各写一份重复的裸 SQL，现在收敛到
  `agent_job_queries.go` 的一组共用封装（作业与主机日志的 running→failed/timeout/finished 状态流转、
  stdout 追加、执行作业收尾）；作业行的 `duration_seconds` 由应用层算（读回 `start_time`，复用 automation 域的
  `GetAutomationJobStartTime`，替换 MySQL 的 `TIMESTAMPDIFF`）。列表/拦截用的是可变长 `IN (sqlc.slice(...))`。
- **部署模板**：模板主体 + 5 类嵌套子表（端口/路径/配置文件/日志定义/控制动作）+ docker 与 compose 配置。
  原实现删子表时运行时拼表名（`DELETE FROM `+table+` WHERE …`），现在按表名分派到 7 条显式语句；
  子表读取保持各自的排序（端口按 `protocol,port`、路径按 `path_type,id`、其余按 `id`）。
- **逻辑服务与部署实例**：服务 CRUD 含成员（`assets_application_service_deployment`）与日志设置
  （`assets_application_service_log_setting`）的**整表替换**；列表的搜索/业务系统/环境过滤从运行时拼 `WHERE`
  （含 `EXISTS` 子查询）改成 `sqlc.narg`（NULL 表示不过滤，`IS NULL` 写在 OR 链末尾）。
- **验证**：`internal/assets/smoke_test.go` 与 `smoke_template_test.go`（`ASSETS_SMOKE_DSN`）在真 MySQL 与真 PG 上
  跑一遍上述写路径（主机域：三类快照各写两次验 UPSERT 真走 UPDATE 分支、磁盘重建、采集两态、唯一性计数；
  模板与服务域：模板 + 全部嵌套子表建成后读回、服务 + 成员 + 日志设置、部署实例 CRUD、列表的三种过滤组合），
  全程一个事务并回滚，不往库里留数据。

## 部署实例运行状态检查

- **单实例**：`POST /assets/application-deployments/{id}/control/`，`action=start|stop|status`。经 Agent 数据面（`agent.Gateway`）下发 `control_application`，用 `GetDeploymentControlContext` + 模板控制动作（含成功退出码）执行；`status` 按退出码判定 `running(0)/stopped(非0)`，`response.Status != success` 或 Agent 调用失败记为 `error`，结果写回 `runtime_status` / `runtime_status_output` / `last_status_check_time`。
- **逻辑服务批量**：`POST /assets/application-services/{id}/refresh-runtime-status/`，对服务名下**所有启用**部署实例并发（最多 8）执行一次 `status` 检查，逐实例写回状态，返回 `{summary:{running,stopped,error,unknown}, total}`。服务树选中逻辑服务时的手动刷新与 15s 轮询都走这里。
- **失败语义**：Agent 未连接 / 命令执行失败时 `executeDeploymentControl` 会提前返回，`checkDeploymentRuntimeStatus` 必须补写 `error` + 错误原因，否则前端只会显示「未知」且没有任何报错。前端在服务树与 `ApplicationWorkspace` 对 `runtime_status=error` 时用 `runtime_status_output` 展示 tooltip。
- **前端消费**：逻辑服务节点刷新成功后按 `summary` 提示「运行中/已停止/检查失败」，随后重载实例列表读取最新状态。

> 迁移进度与剩余清单见 `docs/plans/SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md` 的 P2-3（`assets` 已清零，`monitor` 待迁）。
