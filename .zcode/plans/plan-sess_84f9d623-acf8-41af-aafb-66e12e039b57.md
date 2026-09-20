## 目标

日志中心的「日志查询」「日志配置」在**非服务节点**（全部/项目/业务系统/环境）不再只有一句空态，改为**逐层下钻的日志视角**：指标条 + 下一层清单，点行下钻一级，到"环境"层的行就是逻辑服务、点进去进入服务级界面（现有内容完全不动）。

服务层（逻辑服务/部署实例节点）的行为、以及「存储水位」tab，本次不改。

## 界面最终形态

判定：`serviceId` 为空即层级视图。层级映射复用「存储水位」已有的 `groupDimension` 思路（全部→项目、项目→业务系统、业务系统→环境、环境→逻辑服务）。

**指标条**（随层级聚合）：服务数 / 已开采集 / 采集关闭 / 未认证日志条数 / 待下发主机数。

**表格**
- 中间层（项目、业务系统）：`名称 | 下辖服务数 | 采集关闭服务数 | 未认证日志条数 | 待下发主机数 | 操作（下钻）`
- 叶子层（环境）：`逻辑服务 | 采集 | 日志（条数 / 未认证 x / 需复验 y）| 待下发 x/y 台 | 默认档位 | 最近写入* | 操作（进入该服务）`
  - *「最近写入」只在环境层给（有现成接口，见下）；其他层不给，避免为一个装饰性数字引入 N 次 ES 查询。

**交互**：行/操作列点击 = 把页面 `scope` 换成目标节点的 scope（左树高亮会自动跟上——树本来就是按回传的 scope 反推 key）。查询 tab 从叶子层进入服务时保持 `activeTab='query'`。

**保留服务级专属区块**：「采集链路」「本服务下发」「服务级采集总开关」仍只在服务节点出现；空态文案改为"这个范围里还没有逻辑服务"这类具体说明。

## 数据来源

- **层级成员**：`getProjectList` / `getBusinessSystemList` / `getApplicationServiceList`（前端本地按 scope 过滤，与资产服务树页 `ServiceTreeNodeContent` 同一套做法）。服务行的 `log_collection_enabled`、`log_retention_tier_id` 现成可用。
- **服务级日志状态汇总（新接口，一次拿全范围）**：
  - `POST /assets/application-services/log-status-summary/`，body `{service_ids:[...]}`，权限 `assets:applications:view`
  - 返回 `{items:[{service_id, log_count, verified, needs_recheck, unverified, disabled_logs, collection_enabled, retention_tier, hosts:{managed,unmanaged}, pending:{drift,never,unknown}}], summary:{...}}`
  - 日志条数与认证统计在 assets 侧：**复用 `ListServiceTemplateLogs`**（`format_state` 的指纹比对只有这一处实现，不复制）
  - 待下发在 logcollect 侧：assets 定义 `ServiceLogPendingEvaluator` 接口、logcollect 实现、在 `router.NewWithGateway` 注入（**沿用 `LogFormatVerifier` 的既有注入先例**，依赖方向不变）。实现里用 `ListServiceLogApplyTargets` 取已下发子指纹、`loadHostLogRenderInputs`（主机并集，一次 2 条查询）、`renderHostLogConfig`（**每台主机只渲染一次**）、再按服务子指纹比对；语义逐条对齐 `EvaluateServiceLogConfigStates`（未纳管单独计数、expected/applied 都空= synced）
  - 上限：一次最多 200 个服务，超出返回 400 并说明；文档写明扩容路径（把 per-service 的 applied 子指纹查询换成 `IN (sqlc.slice(...))` 批量查询后，整范围只需 4 条查询）
- **最近写入（仅环境层）**：`getLogServiceUsage(clusterId, {business_system, environment, days})` —— 现成接口，前端已有 API 定义但从未被调用，本次激活。

## 前端结构

- 新组件 `fronted/src/views/monitor/log-center/LogLevelOverview.vue`：承接层级视图（指标条 + 表 + 下钻），`variant: 'config' | 'query'` 控制列与文案。
- 数据在**页面级加载一次**（scope 变化时），两个 tab 共用同一份，避免各拉一遍。
- 组装逻辑（scope + 项目/业务系统/服务名单 + 汇总结果 → 行数组 + 指标条数字）抽成纯函数 `fronted/src/util/logLevelOverview.js`，便于单测。
- 两个 tab 的非服务分支从 `<a-empty>` 换成该组件；服务分支不动。

## 明确不做（本次边界）

- 跨服务检索：后端 `/log-search/` 的 `application_service_id` 必填不动，日志检索仍要求选到具体服务。
- 层级上的批量写操作（批量开关采集/批量下发）：层级视图只读 + 导航。

## 执行步骤

1. 后端 assets：汇总聚合（复用 `ListServiceTemplateLogs`）+ handler + 路由 + 权限点。
2. 后端 assets：`ServiceLogPendingEvaluator` 接口定义 + 注入点；logcollect：多服务评估实现（主机只渲染一次）+ 测试。
3. 后端测试：聚合口径（未认证/需复验/关闭条数）、多服务共享主机、未纳管单算、空范围、超上限、handler 契约。
4. 前端：API 函数（`batchLogStatusSummary`）+ `util/logLevelOverview.js` 纯函数 + 单测。
5. 前端：`LogLevelOverview.vue`（指标条/两套列/下钻事件/空态）+ 单测。
6. 前端接线：`log-center/index.vue` 两个 tab 的非服务分支 + scope 下钻 + 环境层激活 `getLogServiceUsage`。
7. 前端页面级测试：非服务节点渲染层级视图（不再是空态）、点行把 scope 换成目标、服务层内容不变。
8. 文档：`LOG_COLLECTION_ARCHITECTURE.md` §9.5 新增「非服务节点 = 层级下钻视图」小节（层级映射、指标条口径、接口与上限、扩容路径），并在该节的 tab 数据来源表里补这一行。
9. 全量回归：`CGO_ENABLED=0 go test ./...`、`npx vitest run`、`npm run check:ui-rules`（已知 `security/baseline/scanDetail.vue`、`baseline/index.vue` 有改动前就存在的违规，不属于本次范围）。

## 取舍说明

- **不给层级做"跨服务检索"**是刻意的：索引/命中上限/聚合语义都会变，且"索引按服务切分"是现有设计前提。
- **新接口用注入式依赖**而不是让 logcollect 复制指纹逻辑：`format_state` 的判定必须只有一处实现，否则界面与日志中心会给出不同的"未认证"数字。
- **v1 不新增 SQL**：认证统计复用 `ListServiceTemplateLogs`、配置态复用现有查询与渲染；代价是查询次数随范围内服务数线性增长（有 200 上限兜住），扩容路径写进文档。若要新增 SQL，按 `SQL_DESIGN.md` 走 `db/queries/mysql` → `make derive` → `make generate`（sqlc v1.30.0）→ `make facade`。
- **层级只读**：写操作仍以逻辑服务为最小单位，避免在层级上出现"部分服务改失败"的中间态。

## 完成标准

- 选中全部/项目/业务系统/环境任一层级，两个 tab 都有内容，且指标条与清单数字与进入服务后看到的一致（同一数据源）。
- 点行能下钻/进入服务，左树高亮同步（依赖前面已修的 `scopeKey` 逐层对齐）。
- 服务节点下页面表现与改动前逐像素一致（现有 40+ 条页面用例保持通过）。
- 新增测试覆盖：聚合口径、超上限、共享主机、空范围、下钻交互。
