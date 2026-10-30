# 应用目录与集群模型（autoadmin）

描述应用/版本/集群模型目录的查询逻辑与"按应用筛选"语义。后端唯一实现为 Go 版 autoadmin（Django 已废弃）。

## 数据流

### 主机分组模型

`assets_hostgroup` 是单层字典：保留 ID、全局唯一名称、备注和创建/更新时间，不保存父级。
`assets_host.group_id` 继续引用同一个分组 ID，主机可不分组。
分组列表、详情和不分页选项均返回直接主机数 `host_count`，不累计其他组；
请求指定非空 `parent_id` 会明确返回参数错误，不会静默创建层级。
已有 `/assets/host-groups/tree/` 选项入口返回平铺数组，没有父级或嵌套子组字段；
自动化及监控的现有分组选项入口也只返回独立分组。
Inventory 可在组下展示主机叶节点，监控按精确组 ID 过滤。

删除只提供 `POST /assets/host-groups/batch-delete/`，服务端逐组检查主机引用。
有主机时返回该项失败，空组才允许删除；数据库主机外键继续防止并发新增引用时误删。
前端确认前读取最新主机数量，确认展示名称和 ID，并检查批删每项结果，不能仅按 HTTP 200 判成功。

**数据库升级**使用 MySQL/PostgreSQL 的 `000060_flat_host_groups` 迁移：
移除 `parent_id`、所属自关联外键和索引，以及废弃的 `sys.assets.hostgroup.max_tree_depth` 配置。
MySQL 从实际数据库元数据查找父级外键名称，覆盖既有安装中名称不同的约束；
PostgreSQL 只删除该列及其自身约束/索引，不使用 `CASCADE`。
所有原分组及其 ID、名称、主机归属保持不变，原子分组直接成为独立分组。
升级需协调数据库迁移与 API 版本切换，不能继续用旧版查询访问已移除的列。
Down 迁移只恢复可空父级列、外键与索引，不恢复已经移除的父子关系和层级配置；
需要保留这些旧信息时，应在升级前备份。

### 应用类别

应用定义的新增、编辑与列表使用以下类别：

| 编码 | 名称 | 用途示例 |
|---|---|---|
| `web_container` | Web 容器 | Tomcat |
| `database` | 数据库 | MySQL、PostgreSQL |
| `cache` | 缓存 | Redis、Memcached |
| `message_queue` | 消息中间件 | IBM MQ、RabbitMQ、Kafka |
| `middleware` | 中间件 | 不属于缓存或消息中间件的其他中间件 |
| `business` | 业务应用 | 业务 API、业务后台 |
| `other` | 其他 | 尚未归入以上类别的应用 |

类别保存在应用的 `category` 字符串字段，创建默认选择“其他”。
既有应用不按名称自动重分类，需要在应用定义中手动调整；不涉及数据库结构变更。
类别仅用于应用分类与展示，依赖关系图不据此切换节点形状。

### 服务树面板布局

服务树页左侧面板默认宽 260px，可通过顶部「放大面板 / 还原宽度」切换宽度，
通过「收起树面板 / 展开树面板」隐藏或恢复导航。拖动两栏间的分隔条可调整宽度；
分隔条支持键盘左右方向键（每次 20px）及 Home / End（最小 / 最大宽度）。
宽度范围为 260px 至 640px，同时为右侧详情预留至少 360px（容器空间充足时）。
放大后的还原操作恢复此前手动宽度。收起仅隐藏树，不销毁组件，不清空选择、搜索、
项目 / 环境筛选或节点展开状态；状态在当前页面实例内保留，不写入数据库或浏览器存储。
服务树选中节点与「服务信息 / 依赖关系」页签同步到当前 URL 查询参数，
与日志中心复用 `useServiceTreeLocation` 的定位规则，
不需要为每个服务创建独立页面路由。节点通过 `node_type` 及对应
`project_id` / `business_system_id` / `environment_id` / `application_service_id` /
`deployment_id` 定位；`panel=dependencies` 打开依赖关系，省略时显示服务信息。
名称与标题参数只用于展示，不代替 ID 鉴权或数据读取。既有仅带 `application_service_id`
的服务链接继续直接定位；带服务与部署 ID 时定位部署实例。
选择节点、详情内导航和页签切换使用路由 replace 更新地址，保留无关参数与 hash，
不为每次点击新增浏览器历史。回到全部业务或删除当前节点时清除旧节点参数。
顶部服务树标签按页面路径复用，更新为最新定位链接，不因节点或页签参数改变新增标签；
其他页面原有的按完整地址区分标签行为不变。
刷新、重新打开链接及已缓存页面收到新的定位参数时恢复节点与页签，
树数据加载完成后展开目标祖先；不响应其他页面的路由变化。
树的临时筛选、搜索与手动折叠不写入 URL，依赖图布局仍按查看范围保存在当前浏览器。
窄屏（不超过 900px）改为上下布局，树占满行宽，不显示拖动条。
服务树页与日志中心通过公共 `ServiceTreeLayout` 复用同一布局与交互，每页独立维护面板状态；
服务树资源图表随详情区域尺寸更新。日志中心在宽屏保留左侧面板随页面滚动保持可见的行为。

删除当前选中的逻辑服务成功后，页面先切换到全部业务并等待子组件范围同步，再刷新树与内容，
避免继续使用已删除服务的 ID 请求详情而出现“删除成功”后又提示“资产不存在”。

服务树详情工具栏的运行状态提示使用固定子项承载，刷新时只切换提示内容与可见性，
不改变操作按钮在 Space 中的子项位置，避免状态检查前后按钮实例错位或残留。

项目、业务系统、环境、集群模型和逻辑服务的编码支持大小写英文字母、数字、下划线与连字符，
首字符必须为字母或数字。创建与编辑保留用户输入的字母大小写，不自动转为小写；
既有唯一性约束保持不变。日志索引名称仍由日志模块独立归一化为小写安全段，不限制业务编码大小写。

### 主机详情的服务树归属

服务树的逻辑服务列表和服务详情均提供「复制」，权限与服务管理一致。复制打开预填新增表单，
带入业务系统、环境、应用、版本、模板、拓扑、宏值、访问地址、保留档位和备注等配置，
清空 ID 与成员选择，名称和编码分别使用源值追加 `-copy` 后缀，并关闭日志采集；
不复制运行状态、认证结果及逐条日志覆盖。用户可在保存前修改预填名称和编码，重复值仍按现有唯一性规则报错。
用户填写新身份、确认宏值并选择成员后，保存走普通创建接口，源服务不变。
打开、取消复制均不创建数据，不下发应用启停命令；新服务唯一性仍由现有服务端校验。

主机详情通过「服务树归属」页签按需加载部署、逻辑服务、业务系统的完整分页数据，
复用现有接口鉴权；加载失败显示错误和重试入口，不作为空结果处理。
以部署的主机 ID 反查 `application_service_ids`，合并为项目 → 业务系统 → 主机所属环境
→ 逻辑服务 → 部署实例，只保留当前主机分支，默认展开。
主机环境唯一，不从服务反推环境；主机未设环境时提示，环境不一致或关联对象不可读取时单列异常。
未关联服务的部署单独列出，无部署时显示空状态。汇总服务数按正常关联服务去重，
部署数按当前主机部署去重；服务/部署节点可跳转完整服务树，携带项目、业务系统、环境和服务上下文。
该入口仅查看与定位，不提供绑定、解绑或环境编辑。
公共服务树构建时直接挂载所有关联部署实例，默认展开到实例层级，无部署的逻辑服务为叶节点；
仅按项目筛选时，根据业务系统的项目归属保留节点，包括尚未创建逻辑服务的空业务系统；
选择具体环境时才按环境下的服务过滤空分支，不通过服务数量推断业务系统所属项目。
实例可见性不依赖是否访问过详情，搜索与筛选基于同一完整树。用户仍可手动折叠分支。
完整服务树接收外部定位时，选中目标并展开其实际祖先路径，
同时覆盖首次异步加载完成及已打开服务树时的目标切换。

主机列表不展示 ID 列，首个数据列为实例名；主机 ID 仍用于行标识、勾选、操作和按 ID 查询。
最后一个数据列展示接口 `create_time`（资产记录创建时间），操作列仍固定在右侧。
创建时间复用页面时间格式化工具，按当前用户时区展示，缺失值显示 `-`；表格支持横向滚动。
创建时间支持服务端全量升降序排序：`ordering=create_time` / `ordering=-create_time`；
数据库先排序再分页，同时间按 ID 降序稳定排序，在线/离线筛选链路沿用相同顺序。
切换排序回到第一页，后续翻页及刷新保留排序；取消排序恢复默认 ID 降序。非法排序参数返回参数错误。

- 入口（`internal/api/router/router.go`）：
  - `GET /assets/applications/` → 应用列表
  - `GET /assets/application-versions/` → 版本列表（可选 `application` 筛选）
  - `GET /assets/cluster-profiles/` → 集群模型列表（可选 `application` 筛选）
- 链路：Handler（`catalog_handler.go`，`queryID("application")` 解析筛选参数）→ Service → Repository（`catalog.go`）→ sqlc 查询（`db/queries/assets.sql`，生成于 `internal/platform/database/generated/`）。
- **应用行的模板数 / 部署数**（应用定义列表的「模板数」「部署数」两列，2026-09-21 修）：`ListApplications`
  与 `GetApplication` 内联两个子查询实时统计——模板数数 `assets_application_deployment_template`
  （模板表直接带 `application_id`）；部署数经 **服务成员关系** 反查（`service → service_deployment →
  deployment`，`COUNT(DISTINCT d.id)`：一个部署实例可同时承载同一应用的多个服务，去重才不虚高）。
  此前这两列是写死的 `0 AS ...`（迁移时占位没补），页面永远显示 0。

## 按应用筛选语义（关键决策）

### 集群模型类型

唯一内置模型为 HA；用户接口仅创建和编辑 `profile_type=custom`、`cluster_type=custom`
的自定义模型，内置 HA 保持不可编辑和不可删除。MySQL、Redis、Elasticsearch、Nacos 不再作为内置类型。
数据迁移 000051 将被逻辑服务引用的这四类内置模型转为自定义，保留 ID、名称、应用及服务关联；
无服务引用的记录删除。既有自定义模型与业务服务、部署均不删除。
该迁移仅修改数据，无表结构变更；回退不重建已删除模型，恢复原始数据需使用迁移前备份。

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

## 服务树“资源占比”数据链（部署→服务→项目）

服务树右侧的“资源占比”在全部、项目、业务系统、环境节点显示，分别按下一层
项目、业务系统、环境、逻辑服务聚合；逻辑服务和部署实例节点不显示该图。
页面通过 `statsByScope` 启用范围统计，公共树的其他调用方保持原有统计行为。
分组与范围复用 `logLevelOverview` 的层级逻辑，`serviceTreeResources` 负责容量归集。
Go API 必须提供：

- `GET /assets/application-deployments/`：每条部署带 **`application_service_ids`**（`assets_application_service_deployment` M2M 关联，`attachApplicationServiceIDs` 批量补齐，未关联输出 `[]` 非 null）+ `host`（关联主机 ID）。
- `GET /assets/application-services/`：带 `business_system`（用于经项目的 `business_systems` 归集到项目）。
- `GET /assets/hosts/`：带 `hardware.cpu_cores` / `hardware.memory_gb`。
- `GET /assets/projects/`：带 `business_systems`（见上）。
- `GET /assets/business-systems/`：带项目归属，用于范围过滤和业务分组。

缺失任一关联字段，饼图恒为空（前端 `linkedServices` 过滤后无归集桶）。

资源含义是承载主机的 CPU / 内存配置容量，不是实时使用率、服务配额或真实资源分摊。
每个分组内主机 ID 去重，同主机多个部署不重复累加；跨组允许同一主机重复归集。
顶部总容量按当前范围所有分组关联主机的并集计算，不累加分组容量；共享主机只贡献一次。
CPU、内存保留环形饼图，百分比以各分组容量之和为分母，表达容量归集占比，
不以顶部去重总量为分母，不代表实际资源分配、服务配额或使用率。
最多展示前八个分组，其余归集容量合并为“其他”，该合并不影响顶部主机去重总量。
界面明确提示分组容量可能重叠及百分比口径；无关联主机的服务不贡献容量，空范围清空图表。
顶部项目／环境过滤与当前节点范围共同生效，切换节点刷新图表，隐藏后恢复时重新计算尺寸。

**项目与环境筛选联动**：公共服务树用于服务树页和日志中心。未选择项目时，环境下拉保留全局启用环境；
选择项目后，仅列出这些项目的逻辑服务实际使用的启用环境，多项目取并集。项目归属由项目响应的
`business_systems` 与服务的 `business_system` 连接。环境候选从未经过环境筛选的项目服务集计算，
不因已选择某环境而丢失其他可选环境。切换项目或刷新资产后，移除不再有效的已选环境，再向父页面发送范围；
项目没有服务时环境候选为空，不额外自动选择环境。树和资源统计使用相同项目服务集再应用环境筛选。

**树的视觉约定**：节点颜色只区分层级（项目 / 业务系统 / 环境 / 逻辑服务 / 部署实例），
**不编码业务状态**。2026-09-19 移除了"未开启日志采集就把服务节点变灰 + 加禁用标记"的做法：
树是所有资产页共用的，而"采不采日志"只在日志域有意义，灰节点在资产/告警页只是噪声；
何况"有没有日志"是多因结论（服务停用、配置没下发、解析规则没发布都会没有日志），
一个节点的颜色承担不了。要看"为什么没日志"，去日志中心的「采集链路」
（见 [LOG_COLLECTION_ARCHITECTURE](LOG_COLLECTION_ARCHITECTURE.md) §9.5）。

## 逻辑服务 name / code 的唯一域（2026-09-20）

`assets_application_service` 的 `name` 与 `code` 都在 **`(业务系统, 环境)`** 内唯一
（`unique_business_environment_service` / `unique_business_environment_service_code`，见逻辑服务
migration 000044）。项目由业务系统隐含（`assets_business_system.project_id`），环境是全局表，所以
"项目 → 业务系统 → 环境"这一层级实际就落成这两个外键。

- **编码不再是全局唯一**：同一逻辑服务在不同业务/环境下可以复用同一 code（例如 `artemis`）。
  旧行为（全局唯一）会迫使人为加后缀（`artemis-aos-poc`），反而丢掉跨环境按同一 code 关联的能力。
- **删除完成后可重建**：同一业务/环境下允许重新使用已删除服务的名称与编码，新服务使用新的 ID。
  删除尚未完成时返回明确业务错误，必须先完成删除。永久墓碑仅撤销旧服务 ID，不永久占用编码；
  新日志携带 `service_id`，ES 与 Agent 按 ID 隔离新旧服务，旧缓冲事件继续拦截。
  部署与清理边界见 [日志撤销](LOG_COLLECTION_ARCHITECTURE.md#逻辑服务删除与日志撤销)。
- 因此 **code 不再是可靠的全局连接键**：日志检索/水位等按服务维度走 **服务 id**
  （`application_service_id`），ES 文档另带 `project/business_system/environment` 字段做维度收窄；
  运行时的服务级身份（`service_fingerprints` key、Filebeat 片段文件名/input id）也改用 id。
  详见 [LOG_COLLECTION_ARCHITECTURE](LOG_COLLECTION_ARCHITECTURE.md) §9.7 与 §8.3。
- 服务层强制 `environment` 必填；唯一约束对 NULL 不去重，留空会让约束失效。
- 唯一冲突的错误文案按约束名区分为"名称已存在"/"编码已存在"（`internal/assets/service.go` 的
  `duplicateError`），不再统一成"名称或编码已存在"。

## 逻辑服务 / 部署实例列表的 scope 过滤（服务树右侧面板）

右侧详情顶部的面包屑支持点击祖先层级返回，当前节点仅显示文本。
从部署实例点击逻辑服务名称，按当前路径保留服务 ID、业务系统和环境返回服务详情，
不从共享实例的成员关系中猜测父服务，也不依赖浏览器历史。
上钻时只携带目标层级及祖先字段，清除更深层实例／服务身份；
下钻保留已知项目上下文。名称相同的不同层级仍分别展示，没有足够身份的祖先只显示文本。

服务树选中节点后，右侧 `ServiceTreeNodeContent.vue` 按节点层级向后端传过滤参数；Go 版
与 Django 版 DRF filter 字段名保持一致，语义为"缺省 = 不筛选"：

顶层「全部业务」的业务系统表格首列为「项目」，直接显示业务系统响应的 `project_name`，
无名称时显示 `-`，其后为业务系统名称、编码、负责人、状态和操作，不展示部署实例数量列。
项目节点下不重复展示项目列，同样不展示部署实例数量列。
顶层表格横向滚动宽度为 990，项目节点表格为 810，操作列固定在右侧并保留明确宽度；
顶部部署实例等汇总指标保持不变，环境及逻辑服务节点的列表列配置不受影响。
环境节点的逻辑服务列表在「应用」右侧展示「版本」，取逻辑服务的
`application_version_name`，为空显示 `-`；横向滚动宽度为 1350，操作列保持固定在右侧。
逻辑服务节点的部署实例列表在地址后展示「主机 CPU」「主机内存」，按实例关联的主机 ID
读取主机快照的 `hardware.cpu_cores`、`hardware.memory_gb`，单位为核、GiB，缺失显示 `-`。
主机快照复用公共树加载时已有的主机列表请求，经 `hosts-loaded` 事件传给页面及详情组件；
不为每个实例追加主机请求，不触发 `get_host_info`，保持现有资产采集时机。
部署列表不展示重复的主机实例名，单独展示「主机名」，按关联主机 ID
读取快照的 `system.hostname`，即 dj-agent 采集的操作系统 hostname；
为空显示 `-`，不使用主机或部署记录的 `instance_name` 替代。
主机名与 CPU／内存复用异步加载的资产快照，浏览列表不额外触发 Agent 采集。
实例详情中的主机名称按主机 ID 从快照读取，
优先展示主机 `instance_name`，其次为已采集的 `system.hostname`，均缺失显示 `-`；
不依赖部署接口未提供的 `host_name`，也不使用应用部署实例名冒充主机名。
主机列表完成资产采集后，刷新服务树可读取最新快照。
部署实例列表以「操作系统」替代「版本」列，按关联主机 ID 读取主机列表快照顶层的 `os_type` 和 `os_version`
（列表的 `system` 仅含 hostname、Agent 版本等字段，不含 OS 版本；主机详情接口的结构不同），
Agent 分别从 `/etc/os-release` 的 `NAME` 和 `VERSION` 采集这两个字段，
页面以空格连接系统名称和版本（如 `Red Hat Enterprise Linux 8.10 (Ootpa)`），与实例详情共用格式化；
缺一项时仅显示已采集项，均缺失时显示 `-`。不写死发行版名称，不额外触发 Agent 采集。
服务详情和环境下服务列表的应用版本不受影响。
表格横向滚动宽度为 1460，操作系统列宽为 300，页面提示这些是整机配置，不是实例配额或实时占用；
同主机的多个实例显示相同的配置容量。
部署实例列表的操作列固定在右侧、宽度为 90，提供与实例详情一致的小号 terminal 图标按钮，
在按钮上方显示 WebSSH 提示；点击不触发行下钻，新窗口打开实例所在主机的 WebSSH，
不是直接进入数据库或容器。按钮沿用 `assets:hosts:view` 权限，未关联主机时禁用。
列表和实例详情共用主机终端导航，使用关联主机 ID 及已加载的主机实例名；
不把应用部署实例名作为 Agent 主机身份。未带登录用户时由终端页面按既有规则处理。
逻辑服务下的部署实例列表仅通过首列实例名称链接进入详情，普通单元格和行空白处不触发导航，
也不为整行注册键盘跳转；SSH 操作保持独立。其他层级的行导航行为保持不变。
主备状态仅在 `cluster_type=ha` 时展示，包括服务下实例列表的列、实例详情顶部统计卡片和详情字段；
非 HA 或未配置集群类型时均不显示主备状态。HA 角色未确定时仍显示「未知」，不等同于运行状态异常。
部署实例详情不展示「监听端口」数量；配置端口来自逻辑服务的部署模板，只在逻辑服务页面展示，
不把实例接口缺少端口字段解释为零个监听端口，也不将模板端口定义视为主机实际监听的探测结果。
实例详情顶部状态卡片按内容紧凑排列，单项最小宽度 140px，非 HA 一项、HA 两项，
不沿用其他层级的四列满宽统计栏；窄屏允许换行，不保留空统计列。

- `GET /assets/application-services/`：
  - `search`（名称/编码模糊）；
  - `business_system` → 仅返回该业务系统下的逻辑服务。服务树的"业务系统"节点（展示其下逻辑服务）与"环境"节点（展示当前业务×该环境的逻辑服务）都依赖此参数，缺失会退化为返回全部逻辑服务。
- `GET /assets/business-systems/`：
  - `search`（名称/编码/负责人/备注/所属项目名模糊）；
  - `project` → 仅返回该项目下的业务系统（`assets_business_system.project_id`）。服务树的「**项目**」节点依赖此参数。
    **2026-09-20 修复**：前端一直在传 `project`，而这条查询里原先没有这个条件——静默地返回**全部**
    业务系统（项目节点下列出所有业务系统，连它下面聚合的服务数/实例数一起错）。这类"参数传了没人理"
    的 bug 不报错、不告警，所以守卫测试断言的是"参数进了 SQL"（`business_system_filter_test.go`）。
    注意搜索词在 SQL 里出现在 5 个占位符上、项目过滤 2 个（`= sqlc.narg(x) OR sqlc.narg(x) IS NULL`），
    生成的 Params 会各合成一个字段、由生成代码重复传同一个值——漏传是静默失效（见 SQL_DESIGN §2.5）。
- `GET /assets/application-deployments/`：
  - `application_service` → 仅返回与该逻辑服务存在 M2M 关联（`assets_application_service_deployment`）的部署实例；"逻辑服务"节点右侧的部署实例列表依赖此参数。
  - `application_service__business_system` → 经 M2M 关联到逻辑服务、再按业务系统过滤；"业务系统/环境"节点的部署实例列表依赖。
  - `application_service__environment` → 经 M2M 关联到逻辑服务、再按环境过滤。
- 非法整数参数 → 400（`applicationDeploymentFilterFromQuery` / `optionalIDQuery`）；
- 过滤在 SQL WHERE 层完成（EXISTS 子查询），COUNT 与列表共用同一条件，分页计数正确。

### 服务树的选中态：scope ↔ 节点 key 必须逐层对齐（2026-09-20 修复）

`ServiceTree.vue` 的选中态是**受控 + 反推**的两段式：

1. 点节点 → `handleSelect` 记下节点 key 并 `emit('select', scopeByKey.get(key))`；
2. 父组件（服务树页与日志中心页都是 `@select="scope = $event"`）把 scope 存起来再传回
   `:selected-scope`，组件用 `scopeKey(scope)` 把它反推回节点 key（外部改 scope 时也要能选中对应节点）。

所以 **`scopeKey` 必须覆盖 `buildTree` 写进 `scopeByKey` 的每一种键**：`all` / `project:<项目 id>` /
`system:<业务系统 id>` / `environment:<业务系统 id>:<环境 id|unassigned>` / `service:<服务 id>` /
`deployment:<实例 id>`。少一种的后果不是"数据不对"，而是**这个层级的节点第一次点不上、光标弹回顶层**：
反推失败退回 `all`，刚点上的高亮立刻被抹掉；第二次点因为父组件的 scope 引用没变、watch 不再触发，
才"显得"生效（现场反馈"项目/环境节点要点两次"就是这个）。
守卫：`ServiceTree.spec.js` 的 "keeps the selection on a project or environment node after the parent
echoes the scope back"（回传 scope 后选中态必须仍在该节点上）。
watch 里还有一条"键相同就不改"的短路：父组件的原样回传不该产生无意义的重渲染。

### 服务树禁用虚拟滚动（2026-09-21 修复）

`ServiceTree.vue` 的 `a-tree` 使用 `:virtual="false"`（保留 `:height` 做固定高度滚动），**不得改回 `virtual`**。

- **现象**：页面被 keep-alive 缓存（`layout/index.vue`），切到别的 tab 再切回服务树后，树顶部出现整块
  空白、节点被推到下方（现场截图 `/tmp/abc`）。
- **机理**：a-tree 的虚拟滚动由 `rc-virtual-list` 实现。页面失活时容器高度变 0，重新激活后其内部
  `scrollTop`/偏移未复位，虚拟列表从中间下标开始渲染，前面留出与"滚动掉的行"等高的空白。
  这是 ant-design-vue 4.2.6 的上游竞态 bug，与 `userCenter` 时区下拉（`virtual=false` 注释）同根因。
  通用约定见 [FRONTEND_TABS_CACHE](FRONTEND_TABS_CACHE.md) §与虚拟滚动（rc-virtual-list）的冲突。
- **代价**：树的部署实例本就设计为展开服务节点时才挂载的懒加载子节点（`hasLazyChildren`），
  非虚拟全量渲染的节点量可控；用可接受的渲染开销换正确性。
- 守卫：`ServiceTree.spec.js` 断言 `ATree` 的 `virtual` prop 为 `false`。

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
  读取直接使用 `GetActiveAgentPackage` 的生成行类型，并在 HTTP 层映射响应字段与创建时间。
- **Agent 安装/更新流程**：`agent_install.go` 与 `agent_update.go` 原本各写一份重复的裸 SQL，现在收敛到
  `agent_job_queries.go` 的一组共用封装（作业与主机日志的 running→failed/timeout/finished 状态流转、
  stdout 追加、执行作业收尾）；作业行的 `duration_seconds` 由应用层算（读回 `start_time`，复用 automation 域的
  `GetAutomationJobStartTime`，替换 MySQL 的 `TIMESTAMPDIFF`）。列表/拦截用的是可变长 `IN (sqlc.slice(...))`。
- **服务的「监听端口」= 它所属模板的端口**（2026-09-20 修）：端口只在 `assets_application_port`
  上定义（`name`/`protocol`/`bind_address`/`port`，模板的唯一键是 `(模板, protocol, port)`），
  **服务侧不单独维护端口**，所以服务详情 `GET /assets/application-services/:id/` 按服务的
  `deployment_template` 反查一次模板端口带出来（`ports`，排序与模板一致：`protocol, port`）。
  服务树右侧「监听端口」一节渲染的就是它（`名称 · 协议 端口`）。此前这个字段从没被带出过，
  页面永远显示"未配置端口"——前端单测里 mock 了 `ports` 所以一直没暴露；
  现在读端口失败会整体报错，不再静默返回一份"没有端口"的详情。
- **服务树的「日志文件」一节**：逻辑服务节点在「监听端口」下方展示该服务所属模板的日志定义
  （`日志名 + 路径`，如 `message /var/log/messages`）。数据来自既有的日志配置接口
  `GET /assets/application-services/:id/log-config/`（`ServiceLogConfig.logs`，即
  `ListServiceTemplateLogs`）：前端 `ServiceTreeNodeContent.vue` 在 service 节点加载时并行请求它，
  每条渲染 `name` + `resolved_path`（无则回退 `path_pattern`）。
  **路径的宏解析口径 = "服务层尽力展开"**：后端 `resolved_path` 已按 `shared/logmacro` 的同一合并顺序
  （模板默认值（含 `app_home` 作 `APP_HOME`）→ 服务 `macro_values`）替换 `${VAR}`；
  实例级宏（`runtime_variables`，只在主机上才知道）不猜值，仍在路径里的宏由 `pending_macros`
  列出，界面用橙色标签 + tooltip 标注"待展开"。log-config 接口失败时该节显示"模板未配置日志"
  空态，不阻塞节点其他信息。**「监听端口」与「日志文件」的空态用一行灰色小字**（`.section-empty`）
  而不是 `a-empty`——默认 empty 会撑出约 80px 高度，两项都空时把版面拉得很开（2026-09-21 现场）。
- **部署模板**：模板主体 + 5 类嵌套子表（端口/路径/配置文件/日志定义/控制动作）+ docker 与 compose 配置。
  原实现删子表时运行时拼表名（`DELETE FROM `+table+` WHERE …`），现在按表名分派到 7 条显式语句；
  子表读取保持各自的排序（端口按 `protocol,port`、路径按 `path_type,id`、其余按 `id`）。
- **模板端口值**：创建、编辑、复制均接受 `1–65535` 的整数或完整 `${PORT}` 宏，
  宏名为字母/下划线开头，后续可用字母、数字、下划线；表达式最长 64 字符。
  前后端拒绝空值、0、越界、非整数及不完整宏，不将非法输入转换为 0。
  数据库 `assets_application_port.port` 使用 `varchar(64)` 原样存储；API 普通端口仍返回数字，
  宏端口返回字符串。模板详情保留宏表达式；服务详情 `ports.port` 同样保留定义，
  另返回 `resolved_port` 与 `resolution_error`，按模板宏默认值 → 服务覆盖值解析。
  成员实例按同样顺序追加实例 `runtime_variables` 覆盖；任一实例结果与服务级结果不同时，
  `instance_ports` 列出全部实例的名称、部署 ID、解析值及错误，避免省略继承实例造成歧义。
  服务树显示服务级解析值，并在存在差异时逐实例展示；Tooltip 保留模板原文。
  缺失宏、循环引用、空值、非整数或越界结果明确标注，不转换为 0，不伪装成实际探测。
  排序沿用 `protocol,port`，端口改为表达式后同协议内按文本排序。
  发布前执行两侧迁移至 `000057_template_port_legacy_check`：56 将现有数字端口转换为等值文本，
  57 按表与完整检查表达式查找并移除历史 `port >= 0` 检查（无约束的库直接跳过），
  不依赖自动生成的约束名，也不删除其他检查。仅执行 56 会遗留拒绝宏的旧整数约束。
  不删除字段或外键，唯一域仍为 `(模板, protocol, port)`。已被旧保存流程写成 0 的值无法
  反推出原宏，需编辑模板重新填写。回滚前须将所有宏和非法端口改成有效数字，否则回滚明确失败，
  禁止把宏强转成 0。
  57 的回滚不恢复已过时的整数检查；真正回退到整数列仍由 56 的回滚校验负责。
- **部署模板删除**：外键**无级联**，删除在**一个事务里自底向上**执行——先按日志定义 id 清服务级覆盖行
  （`assets_application_service_log_setting`），再删日志定义/端口/路径/配置文件/控制动作/Docker/Compose 子表，
  最后删模板行，任一步失败整段回滚。模板仍被逻辑服务引用（`assets_application_service.deployment_template_id`）时，
  删父行命中外键 1451 → `translate` 转 `ErrDeleteProtected`，不会留下"子表删了、模板还在"的半成品。
  前端 `TemplateManagerDialog.vue` 必须读取批删结果的每条 `ok`：批删接口 HTTP 恒 200，只看 HTTP 会把
  `ok:false`（被引用）当成功——这正是现场"提示删除成功、刷新后记录还在"的成因。
- **逻辑服务与部署实例**：服务 CRUD 含成员（`assets_application_service_deployment`）与日志设置
  （`assets_application_service_log_setting`）的**整表替换**；列表的搜索/业务系统/环境过滤从运行时拼 `WHERE`
  （含 `EXISTS` 子查询）改成 `sqlc.narg`（NULL 表示不过滤，`IS NULL` 写在 OR 链末尾）。
- **验证**：`internal/assets/smoke_test.go` 与 `smoke_template_test.go`（`ASSETS_SMOKE_DSN`）在真 MySQL 与真 PG 上
  跑一遍上述写路径（主机域：三类快照各写两次验 UPSERT 真走 UPDATE 分支、磁盘重建、采集两态、唯一性计数；
  模板与服务域：模板 + 全部嵌套子表建成后读回、服务 + 成员 + 日志设置、部署实例 CRUD、列表的三种过滤组合），
  全程一个事务并回滚，不往库里留数据。

## 部署实例身份与共享约束

- 部署实例以 `id` 标识，`(host_id, instance_name)` 唯一。同一主机上的 nginx、redis、
  Artemis 等不同程序必须登记为不同部署实例；主机的 `instance_name` 是 Agent 身份，
  不等同于应用部署实例名。
- 新增实例始终调用创建接口，不按主机和名称查找后改成更新。同主机同名时明确拒绝并提示修改实例名；
  确需复用同一个程序时，通过逻辑服务成员中的「从已有实例中选择」建立关联。
- 逻辑服务编辑时固定已保存的环境和业务系统，不在普通编辑表单中迁移归属；
  从指定环境新建服务时同样固定当前环境及其业务系统。无指定环境的通用新增入口仍可选择归属。
  从逻辑服务表单新增或编辑实例时，主机候选按父表单当前环境过滤，包括尚未创建的服务；
  实例弹窗使用父表单显式传入的环境，不查询已保存服务的旧环境覆盖它。
  独立打开实例编辑时仍按已保存实例关联的环境加载候选，不自动移除已有成员。
- 模板宏未设置服务覆盖时展示模板默认值；用户清空输入框时保留显式空覆盖，允许继续输入，
  不自动恢复继承。仅点击「重置为默认」才删除覆盖值。最终有效值为空时显示校验提示并阻止保存，
  即使模板本来有默认值也不把空覆盖当成继承；重新填写或显式重置后按最终值重新校验。
- 一个部署实例只有一份运行状态，因此多个逻辑服务共享它时必须使用**同一部署模板 ID**，
  不能仅凭控制类型或模板名称相同判断。服务保存会在同一事务内检查保存后的全部成员，
  包括只修改服务模板、未提交成员列表的情况；冲突时整个保存回滚。
- 校验和控制参数读取均先锁部署实例，再以 `FOR UPDATE` 读取全部服务/模板关联；
  多实例按 ID 排序校验，避免把任意一条关联作为控制依据。并发事务发生锁冲突或死锁时，
  由数据库中止并回滚相应事务，错误向调用方返回，不忽略冲突继续提交。
- 既有多模板共享记录不会自动拆分、删除或启动/停止程序。它们的控制请求明确报错；
  状态检查写入 `error` 与冲突原因，不再随机选一个服务执行。
- 修复既有数据时，先核对全部服务成员、监控目标及日志抽样认证等实例 ID 引用和运行变量，
  确认哪一个程序保留原 ID，再为其他程序创建不同名称的实例，逐个替换相应服务成员。
  保留其他服务成员、成员日志变量和既有配置，完成后刷新各服务运行状态；
  不以删除原实例作为拆分手段。本约束不改变表结构，无 DDL 迁移，也不自动改写业务数据。

## 应用控制参数的宏解析

- API 在调用 Agent 前读取模板 `macro_definitions`、所属服务 `macro_values`、
  实例 `runtime_variables`，按此顺序合并，后者覆盖前者；实例未显式定义 `APP_HOME`
  且服务/模板宏也未提供该值时，以模板 `app_home` 作为默认值，与日志宏共用解析基础工具。
- `run_user`、`work_directory`、`app_home`、`service_name` 和全部控制动作命令中的
  `${NAME}` 使用合并后的值展开，支持宏之间的引用。用户名、目录、systemd 服务名等
  结构字段有未解析宏时，或运行用户宏解析为空时，明确报错，不原样下发或改用 root。
  命令中未声明的 shell 变量保留，交给目标用户的 shell/profile 处理；循环引用明确拒绝。
- 多个服务共享同一模板和实例时，API 还会比较各服务解析后的最终控制参数与动作；
  若宏覆盖导致控制目标不同，则拒绝控制，不任取一个服务的配置。无关的日志宏差异
  不影响控制参数一致性判断。
- 此解析在 API 侧完成，Agent 仍只执行收到的具体控制参数，无须因宏解析更新 Agent。
  该逻辑不改变表结构和既有宏数据，无 DDL 或业务数据迁移。

## 逻辑服务依赖与拓扑 API

服务树继续描述项目 → 业务系统 → 环境 → 逻辑服务 → 部署实例的归属。
依赖独立保存为有向关系，`source_service_id → target_service_id` 表示前者依赖后者，
不表示网络响应或消息流向。双向依赖是两条独立记录，分别维护类型、强弱和功能范围。
允许成环，不用于自动执行启停顺序。正式入口为服务树右侧的「依赖关系」页签，
与原「服务信息」详情共用左侧选中范围。

### 前端依赖工作台

- 先明确项目、环境，再读取该项目环境的完整拓扑快照。树节点包含业务而缺少项目时，
  根据业务元数据解析项目；缺失环境时，只有一个可用环境便自动选中并加载快照，
  多个候选则等待用户选择，不默认生产环境。
  项目或环境由树节点确定时，相应选择器锁定。未关联项目的业务需先完善归属。
  环境候选按当前项目下逻辑服务的实际环境归属与启用环境字典取交集，
  业务/服务节点进一步按该业务/服务收窄，不显示其他项目或尚无服务归属的环境。
  服务目录完整分页加载；未选项目时环境选择器禁用，范围内无环境时显式提示。
  切换项目、业务后立即清除已不在候选内的旧环境，并在唯一候选时自动选中，
  不发送新项目配旧环境的快照请求；树节点显式指定的环境无效时提示刷新，
  不擅自替换为其他环境。服务目录失败明确报错，不回退全局环境列表。
- 同一份完整快照用于新增/编辑的服务候选，以及当前业务/服务的图和关系列表。
  业务视图保留本业务孤立服务和直接跨业务邻居；服务视图只含选中服务及直接关系，
  不递归展开邻居，范围外节点使用虚线边框与标记。搜索、类型和强弱过滤同步作用于图、
  列表，列表在完整已返回关系上本地分页，不静默截断；整个环境超出快照预算时明确报错。
- 「新增依赖」可创建单向或双向关系，两方向分别填写类型、强度、必填功能范围。
  协议/通道、故障影响与处理说明放入选填区。新增仅显示启用类型；
  编辑允许保留原停用类型，单次只编辑一个方向。交互类型管理支持新增、编辑、停用与删除，
  类型编码创建后锁定。
- 关系列表提供单条与批量删除，统一居中确认并列出方向、功能与 ID。
  每个 `results[].ok` 必须成功才提示删除成功；部分失败仍刷新列表并明确保留失败信息。
  反向关系不会因为正向被编辑或删除而联动改变。
- 架构图使用 Vue Flow 交互和 ELK 分层自动布局，含孤立节点、环与平行边；
  箭头永远从依赖方指向被依赖方。强/弱/待确认分别为实线/虚线/点线，平行边错开，
  反向边绕行节点外侧。支持拖动、缩放、适应画布、重新布局、SVG/PNG 导出和复制图片。
  连线标签采用固定尺寸，按关系 ID 确定顺序统一避让其他标签与服务卡片，
  不仅处理相同端点的平行边，也处理不同服务间交叉连线的标签重叠。
  标签从连线中点移开时用浅色虚线引导回原位置；拖动节点后重新计算，
  完整文字可悬停查看，点击仍打开对应关系详情。适应画布与 SVG 导出均包含避让后的标签边界。
  标签可直接拖动，移动达到 4 个屏幕像素后进入拖动状态，并按画布缩放换算坐标；
  连线改为经过标签中心的曲线，两端仍固定连接原依赖方与被依赖方，不修改业务关系。
  手动标签优先保留用户指定位置，不再被自动避让挪走；其他自动标签避让这些手动标签。
  手动位置按关系 ID 保存为相对自动连线中点的偏移，因此移动服务节点时标签与连线仍跟随。
  手动曲线不保证自动绕开所有卡片，用户可继续拖动调整；未手动调整的连线保留自动路由。
  拖动结束不触发关系详情，普通点击与键盘激活仍查看详情；取消指针或窗口失焦回滚本次拖动，
  切换范围、切走页面和卸载时清理拖动监听。
  工具栏提供「全屏显示 / 退出全屏」，支持浏览器 Esc 退出；全屏仅放大图与工具栏，
  节点、关系详情及编辑弹窗仍可正常叠加显示。切换模式不重建图实例、不重新布局，
  尺寸更新后自动适应画布；切走页面或卸载图时退出本组件开启的全屏。
  全屏外层有明确视口宽高，加载层通过 `wrapperClassName` 将伸缩高度施加到实际外层，
  内容容器与画布继承该高度，不将布局样式误施加到加载指示器而导致画布尺寸为零。
  浏览器不支持或拒绝全屏时明确提示，不显示虚假的全屏状态。
  相同节点集合刷新快照时保留拖动位置。工具栏「保存布局」将服务 ID 与 x/y 坐标写入
  当前站点浏览器的 localStorage，按项目、环境、业务和服务查看范围分别保存；不写入后端，
  不跨浏览器或电脑共享，清理网站数据后不再保留。刷新、重新进入或从关系列表切回架构图时，
  恢复已保存坐标；新增服务自动排列在已保存节点右侧，已删除服务的坐标不参与绘图。
  筛选后的保存合并当前可见服务坐标，不清除被筛掉服务的布局。重新布局只调整当前图，
  再次点击保存才覆盖已保存位置。读取或写入存储失败显式提示，不显示虚假的保存成功。
  「还原布局」丢弃当前未保存的节点与连线标签调整，恢复上次成功保存的位置；
  从未保存或新增节点没有保存坐标时，恢复当前节点集合初次展示的位置。
  还原不刷新页面、不切换服务树范围、不修改依赖关系或覆盖浏览器存储，完成后适应画布。
  没有位置修改、无节点或正在布局时禁用还原；已保存的布局不提供历史版本撤销。
  同一布局记录的 `labelOffsets` 保存关系标签的 x/y 偏移；保存筛选视图时合并当前可见关系，
  不清除隐藏关系的位置，已删除关系的记录不参与绘图。标签位置与节点位置一起恢复，
  并共同参与未保存状态判断；拖回原位置清除该标签的修改。重新布局清除当前可见关系的
  手动偏移、恢复自动标签与走线，保存后才覆盖存储。保存失败保留标签调整和未保存标记。
  工具栏持续显示布局保存状态：初始未存储时为「尚未保存」；节点坐标偏离初始或已保存位置时，
  显示橙色「有未保存的修改」；保存成功或恢复已保存坐标后显示绿色「已保存」并禁用重复保存。
  将节点拖回基准位置可清除修改标记，缩放、适应画布及只更新状态数据不算布局修改；
  重新布局改变坐标时标记未保存，写入失败不清除修改标记。
  布局完成后以统一元素模型提交节点与连线，画布先注册端点再注册关系，
  不在异步布局期间单独推送新连线。初始化与手动适应画布使用无动画视口定位，
  避免动画被重复初始化打断后阻塞布局加载状态。
  SVG 使用当前坐标、真实关系和状态计数，保留顶部范围标题，不嵌入底部图例或状态说明；
  页面仅保留工具栏的线条图例，不显示画布底部长段帮助说明；HTTP 图片复制弹窗仍保留操作指引。
  导出文本转义，不依赖外部图片或 AI 服务。
  PNG 与复制图片复用同一份完整 SVG，不截取屏幕可见区域；保留当前拖动坐标、标签避让、
  手动标签及对应曲线路径、背景和顶部标题，不包含底部说明。
  适应画布与导出边界均包含手动曲线控制点与标签。
  以两倍像素尺寸在本地画布生成 `image/png`，不上传图像。
  单边超过 8192 像素或总像素超过 16777216 时明确报错，提示缩小范围或使用 SVG，
  不静默缩放或截断。转换与下载结束释放临时对象 URL 和画布。
  「复制图片」按浏览器安全上下文选择交互：HTTPS（证书受信任）/localhost 下将 PNG
  写入系统剪贴板，可粘贴到支持图片的微信聊天框或 Word 等应用。
  内网普通 HTTP 下生成同一份 PNG 并在居中弹窗展示，用户在图片上右键选择「复制图片」，
  不选择「复制图片地址」；不要求下载，也不宣称网页已写入剪贴板。
  浏览器未提供原生图片复制菜单时仍可使用「导出 PNG」。关闭、替换预览、切走页面或卸载时
  释放预览对象 URL，异步转换在页面离开后完成不打开预览；预览加载失败明确提示。
  剪贴板调用保持在点击的用户手势中，以 Promise 提供 PNG 数据；权限拒绝、浏览器不支持、
  图像加载或编码失败均明确提示，不用文本、SVG 或下载冒充复制成功。
- 点击服务查看启用实例的主机、HA 角色、运行状态、检查输出与各自检查时间；
  点击关系或列表功能范围查看完整依赖说明。所有时间由用户时区工具格式化。
  「刷新快照」仅 GET 已存状态，不调用检查脚本、不自动轮询、不传播故障。
  切入依赖页会卸载原服务详情以停止其状态检查轮询，返回「服务信息」恢复原详情能力。
- 范围切换通过请求代次隔离旧响应，旧范围数据不能覆盖新环境。
  读取失败显示错误与重试，不显示空图冒充成功；保存失败保留输入。
  写入成功但随后刷新失败时明确提示“已保存，但刷新失败”，不把写入当作失败重提。
- UI 读写权限与下文 API 权限一致，不沿用服务树旧的 manage 权限替代应用权限。
  新增按钮采用凭据页的大号默认按钮、plus-circle 和文字；
  编辑/删除采用统一小号图标、顶部 Tooltip，删除复用公共确认及 `delBtn`。
  操作列固定右侧、宽度明确，表格同时开启横向滚动，所有下拉显式复用上下文容器。

### 范围与写入

- 两端必须有非空项目与环境，且项目、环境分别相同；允许同项目同环境跨业务依赖，
  不允许自依赖。未关联项目或环境的服务不能建立依赖。
- `strength` 为 `strong` / `weak` / `unknown`，缺省 `unknown`，交互类型不决定强弱。
  `function_scope` 必填（最多 120 字符），`protocol` 最多 128 字符；
  `failure_impact`、`fallback_plan` 各最多 500 字符，均选填。
  处理说明描述已有行为，不会自动执行降级动作。
- `(source_service_id,target_service_id,interaction_type_id,function_scope)` 唯一。
  同一服务对可按不同类型或功能保存多条关系。
- 新增可传 `reverse` 对象，具有独立的 `interaction_type_id/strength/protocol/function_scope/
  failure_impact/fallback_plan`，端点由正向交换而来。两个方向同事务写入，
  任一失败全部回滚。修改不接受 `reverse`，只编辑指定一条关系。
- 写入按类型和服务 ID 排序锁定，与类型停用/删除及服务删除串行化。
  服务删除保护使用锁定读校验，避免等待另一端服务锁期间读到旧快照；
  已进入删除流程的服务不能再新增或修改依赖。

### 交互类型字典

字典保存编码、名称、启用状态及说明。迁移预置 `http`（服务调用）、`database`、
`cache`、`publish`、`consume`，可使用现有应用管理写权限扩展。
编码以小写字母开头，仅允许小写字母、数字、下划线、连字符，最长 64 字符，
创建后不可修改；名称必填、最多 128 字符，说明最多 500 字符，名称和编码分别唯一。
新建默认启用，编辑省略 `enabled` 时保留原状态。

停用后已有关系继续展示，原关系不切换类型时仍可编辑；
不能用于新增或从其他类型切换。被引用类型禁止删除，可以停用，不级联删除关系。

### API 与权限

| 路径 | 方法 | 行为 |
|---|---|---|
| `/assets/service-dependency-types/` | GET / POST | 类型分页列表 / 新增 |
| `/assets/service-dependency-types/{id}/` | GET / PUT | 类型详情 / 完整编辑 |
| `/assets/service-dependency-types/batch-delete/` | POST | `{"ids":[...]}` 批删类型 |
| `/assets/service-dependencies/` | GET / POST | 关系分页列表 / 新增，可含反向 |
| `/assets/service-dependencies/{id}/` | GET / PUT | 关系详情 / 完整编辑单条 |
| `/assets/service-dependencies/batch-delete/` | POST | `{"ids":[...]}` 批删关系 |
| `/assets/service-dependencies/topology/` | GET | 拓扑与运行状态快照 |

全部经过现有认证与审计。读权限为 `assets:applications:view`；
新增、编辑、删除分别复用 `assets:applications:create/update/delete`，
不新增单删接口，不改变现有服务树权限范围。
成功 HTTP 200、`code=200`；分页沿用统一契约。
批删最多 500 个不同的正整数 ID，返回 `{count,results:[{id,ok,message}]}`；
不存在或受引用保护的项明确失败，其他项仍可成功。

类型列表支持 `search`、`enabled=true|false`。关系列表支持 `project`、`environment`、
`business_system`、`service`（任一端匹配）、`source_service`、`target_service`、
`interaction_type`、`strength`、`search`（服务名称、功能范围、协议）和统一分页，
计数与列表使用相同过滤条件。非法 ID、强度、布尔值返回参数错误。
创建/编辑关系返回 `data.results`，含本事务写入的 1 或 2 条关系。

新增示例，ID 必须取自实际服务和启用字典：

```json
{
  "source_service_id": 101,
  "target_service_id": 102,
  "interaction_type_id": 1,
  "strength": "strong",
  "protocol": "HTTPS",
  "function_scope": "订单创建",
  "reverse": {
    "interaction_type_id": 1,
    "strength": "weak",
    "function_scope": "补充信息展示",
    "fallback_plan": "隐藏补充信息"
  }
}
```

### 拓扑与状态快照

拓扑要求 `project`、`environment`，可选 `business_system`、`service` 聚焦；
同时指定时，服务必须属于所选业务。全环境含无关系的孤立服务；
业务视图保留直接跨业务关系的另一端，服务视图只含该服务及其直接关系。
范围外相邻节点标记 `external_to_selection=true`，不静默丢失边界关系。

返回 `scope`、`snapshot_time`、`state_source=stored_runtime_snapshot`、`nodes`、`edges`。
双向仍为两条边，前端自行布局，后端不保存画布坐标或生成图片。
节点含身份、业务归属、启用标记、`instances`、
`runtime_summary={total,running,stopped,error,unknown}`。
只计入成员与部署实例均启用的关联，共享实例在各所属服务下分别展示。
实例含主机、运行状态、检查输出、`last_status_check_time`、HA 角色；
没有检查时间或状态值不在已知集合内时归为 `unknown`，无实例返回空数组与零计数。
检查失败 `error` 不视为停止，不自动生成服务健康结论。

节点、关系、实例来自同一个只读可重复读事务；GET 不调用 Agent、不执行脚本、
不启动全图轮询。时间按现有 UTC 规则返回；结果可能陈旧，调用方必须显示检查时间，
不得把 `snapshot_time` 当成每个实例的检查时间或把缓存状态当作实时业务健康。
强弱不等于连通性，不根据节点运行状态标记连线“正常”。

项目环境最多读取 1000 服务、5000 条返回关系、10000 条启用实例关联，
超限明确报错，不静默截断。聚焦仍受项目环境节点和实例预算约束；
查询失败明确返回错误，不返回空图冒充成功。

### 删除保护与迁移

删除服务持有服务锁后，先检查任一方向依赖；存在关系则拒绝，
不创建删除保护、不关闭采集、不调用 ES 或 Agent，须先解除关系。
新建关系与服务删除共用端点锁，已进入删除流程的服务不能被新引用。

迁移 `000059_service_dependencies` 在两种方言新增字典表与依赖表，
并为业务 `(id,project_id)`、服务 `(id,business_system_id,environment_id)` 增加唯一约束。
复合外键在数据库层保证两端实际业务、项目、环境一致；
修改服务归属或业务所属项目而破坏关系也会被拒绝。
原服务、部署、日志和状态列不变，无历史关系推断或回填，不删除原有列或外键。
部署需先备份并执行迁移再启用接口；创建唯一索引可能持锁，按维护窗口执行。
回滚删除关系表、字典和新增唯一约束，**丢弃全部依赖配置和自定义类型**，
但不删除原服务、部署与日志数据。

## 部署实例运行状态检查

- **单实例**：`POST /assets/application-deployments/{id}/control/`，`action=start|stop|status`。
  经 Agent 数据面（`agent.Gateway`）下发 `control_application`；`ListDeploymentControlContexts`
  确认关联模板唯一后，按该模板 ID 读取控制动作，不使用 `LIMIT 1` 任取模板。
  `status` 退出码为 0 记为 `running`；systemd 仅在退出码 3 且标准输出为 `inactive` 或
  `failed` 时记为 `stopped`，未知单元、权限、总线错误等记为 `error`；
  其他控制类型保留非零退出码记为 `stopped` 的约定。`response.Status != success`
  或 Agent 调用失败同样记为 `error`，结果写回
  `runtime_status` / `runtime_status_output` / `last_status_check_time`。
- **逻辑服务批量**：`POST /assets/application-services/{id}/refresh-runtime-status/`，对服务名下**所有启用**部署实例并发（最多 8）执行一次 `status` 检查，逐实例写回状态，返回 `{summary:{running,stopped,error,unknown}, total}`。服务树选中逻辑服务时的手动刷新与 15s 轮询都走这里。
- **诊断与失败语义**：状态输出保留动作、控制类型、systemd 服务名与 scope、退出码、
  stdout/stderr/Agent 错误。Agent 未连接或模板冲突等提前失败时，
  `executeDeploymentControl` 写入 `error` 与原始原因；正常结果写库失败直接返回错误，
  失败状态也无法落库时记录带部署 ID 的服务端错误日志。服务树在所有状态下均可通过
  tooltip 查看保存的诊断，`ApplicationWorkspace` 保留错误状态诊断提示。
- **服务树页面加载**：切换节点或手动刷新时，先读取数据库中的服务详情、日志配置与部署实例快照；
  页面加载结束后独立发起运行状态检查，不等待 Agent 才显示页面。顶部刷新按钮只等待页面数据，
  慢状态检查以「状态检查中」标签提示，不遮罩页面，也不阻止切换节点。
- **状态轮询与局部更新**：当前服务每 15 秒尝试一次检查，同一服务仍有在途请求时复用，
  不重复向 Agent 发起检查。检查完成后补读该服务实例列表，仅合并
  `runtime_status`、`runtime_status_output`、`last_status_check_time` 并更新派生统计；
  不清空表格、不重新请求详情或日志配置、不覆盖实例其他字段。手动刷新完成检查后按
  `summary` 提示结果，自动检查失败则保留已有状态并显示「状态刷新失败」及原因。
- **切换与缓存生命周期**：状态请求按服务隔离，使用页面加载代次与当前服务 ID 校验结果；
  旧节点响应不能覆盖新节点。离开服务节点后不再补拉其状态列表。keep-alive 页面失活时停止轮询、
  失效未完成的页面更新，恢复时重新加载快照并恢复检查。已经下发的检查可继续落库，但不更新隐藏页面。
  状态接口自身仍在 HTTP 请求内等待 Agent，并非新增后台任务；等待已从页面主加载链路移除。

> 迁移进度与剩余清单见 `docs/plans/SQL_DUAL_DIALECT_AND_SQLC_MIGRATION.md` 的 P2-3（`assets` 已清零，`monitor` 待迁）。
