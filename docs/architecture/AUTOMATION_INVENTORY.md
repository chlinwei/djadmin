# 自动化 Inventory 管理（autoadmin）

描述 `automation_inventory` 的 CRUD 语义。后端唯一实现为 Go 版 autoadmin（`internal/automation/runtime.go`）。

## API 语义

- `POST /automation/inventories/`：全量创建，`name` 必填；`enabled` 缺省 true，`update_on_launch` 缺省 false，`update_cache_timeout` 缺省 300 且必须非负。
- `PUT/PATCH /automation/inventories/:id/`：**支持部分更新**——未提供的字段保留原值（name/remark/selected_host_ids/enabled/update_on_launch/update_cache_timeout 逐字段合并后整体 UPDATE）。
  - 前端列表"启用状态"开关只发 `{enabled}`，不携带 name，必须走部分更新（此前因全量校验 `name is required` 报错，已修复）。
  - 更新后返回完整资源（`inventoryByID`）。
- `GET`：单条/列表；`DELETE`：物理删除，目标不存在报 404 语义错误。

## 关键决策

- 部分更新在应用层合并（先 SELECT 现值再整体 UPDATE，`resolveInventoryUpdate` 纯函数，见 `runtime_inventory_test.go`），避免动态拼 SET 子句；`selected_host_ids` 以 JSON 数字数组列存储，合并时经 `decodeJSONInt64Array` 解析原值。
- name 语义：显式传空 name 与未传等价，均按"保留原值"处理；仅当现值 name 也为空（脏数据）且未提供 name 时报 400 `name is required`。
- `update_cache_timeout` 在两侧方言里都是 NOT NULL（已对真库核实），所以"存量 NULL 回退 300"这条分支在实现上不可达：默认 300 只在新建或客户端省略该字段时生效，`0` 是合法值且必须原样保留（不能被当成"未设置"覆盖成 300）。

## 数据访问层（sqlc，2026-09-16）

本域（Playbook 模板 / Inventory / 任务 / 作业与主机日志 / 目标主机解析）的**内联 SQL 已清零**：
全部语句定义在 `autoadmin/db/queries/mysql/automation.sql`，Go 侧只调
`internal/platform/database/generated`（方言门面，默认 MySQL、`-tags postgres` 走 PostgreSQL）。
选 sqlc 还是内联、方言可移植规则见 [SQL_DESIGN.md](SQL_DESIGN.md)，这里只记本域的最终逻辑。

- **SQL 归属**：模板 CRUD/列表（含计数与可选过滤）、Inventory CRUD、任务 CRUD 与开关、
  主机选项与主机组树、作业派发/认领/收尾/取消、主机日志写入与读取、控制器 SSH 密钥的
  读改、目标主机快照与聚合计数，共 31 条定义（另有 4 条复用已有定义：
  `GetInventoryTyped`、`GetAutomationPlaybook`、`GetConfigByKey` 等）。
- **列表过滤与排序**：模板列表的搜索/分类过滤用 `sqlc.narg`（NULL 表示不过滤），
  排序由 `sort_key` 参数选择 CASE 表达式（`id`/`name`/`create_time`/`update_time`，
  带 `-` 前缀表示倒序；非法值回落到 `id DESC`）。代价是该表排序不再走索引（表很小，可接受），
  收益是标识符不再由 Go 拼进 SQL。
- **时长统计**：作业的 `duration_seconds` 由应用层计算（取消时先读回 `start_time`，
  为 NULL 则按"现在开始"即时长 0），写入的秒数带小数（列类型是 `double`）。
  迁移前用 MySQL 的 `TIMESTAMPDIFF(MICROSECOND,…)/1000000`，是整数截断且 PG 无法执行。
- **主机解析**：Inventory 的主机集合是可变长的，查询写 `IN (sqlc.slice(host_ids))`，
  派生时 PG 侧改写成 `= ANY($1::bigint[])`（见 SQL_DESIGN §4.2 的更正与计划 P4-7）。
  这条路径此前在 PG 下会因为占位符与参数个数不符而报错，调用点没检查错误时表现为"解析到 0 台主机"。
- **存在性校验**：模板/Inventory 的存在性用取行查询的 `sql.ErrNoRows` 判定（不再 `SELECT COUNT(*)`）。
- **作业来源（`automation_execution_job.source`）**：区分作业由谁派发，取值 `manual`（普通任务，
  `CreateAutomationJob` 显式写）、`agent_install`（Agent 安装/更新，`assets.CreateAgentExecutionJob`）、
  `monitor_target`（exporter/Filebeat 安装，`monitor.CreateMonitorTargetJob`）；列默认 `manual`。
  `CountJobs`/`ListJobsTyped` 支持可选 `source` 过滤（`sqlc.narg`）。运行记录中心是单页无 tab，只展示
  自动化任务运行记录，来源列 + 来源过滤用于区分普通任务 / Agent 安装 / 监控安装；监控安装历史不再单独占 tab，
  纳管目标「查看日志」用历史行的 `automation_job_id_snapshot` 跳到对应作业。
- **验证**：`internal/automation/smoke_test.go`（`AUTOMATION_SMOKE_DSN`）在真 MySQL 与真 PG 上
  跑一遍上述写路径（模板/Inventory/任务 CRUD、作业派发-认领-收尾-取消、主机日志、多值 IN 与聚合计数、
  控制器密钥三条语句）；`scripts` 级的用法写在文件的函数注释里。会改动全局数据的语句
  （控制器密钥替换）只在回滚事务里验证。
