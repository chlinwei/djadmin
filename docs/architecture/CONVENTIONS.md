# 开发与设计约定（强制）

> 本文是项目级约定的**唯一汇总入口**，避免规范碎片化。新增约定先查本文，属同类主题的并入对应章节，不要另起新文件。违反约定的代码在 review 时直接打回。
> API 响应体格式另见 [.github/API_RULES.md](../../.github/API_RULES.md)。

<a id="browser-tool-approval"></a>

## AI 浏览器工具审批

当前仓库默认使用源码、单测和构建验证；浏览器操作必须逐次经用户确认，不能把共享页面、
历史批准或前端工作本身当作授权。没有经授权的测试登录条件时，停止浏览器验收，
不重复访问登录页，不宣称功能已验证。不得通过终端、子代理或批量包装绕过此限制。

Copilot 运行时通过仓库级 [browser-approval.json](../../.github/hooks/browser-approval.json)
注册 `preToolUse` 命令钩子，执行 [browser-approval.cjs](../../.github/hooks/browser-approval.cjs)：

- VS Code 浏览器工具及带浏览器/Playwright 命名空间的工具返回 `permissionDecision: "ask"`，
  每次调用都重新请求确认，不记录或复用批准；不支持交互确认的模式应拒绝调用。
- 混入浏览器调用的批量包装直接拒绝，必须拆成逐次审批。
- 非浏览器工具返回空决定，保留宿主原有权限检查，不自动放行。
- 输入不合法时返回明确拒绝；脚本不访问网络，也不输出工具参数、URL、登录口令或令牌。

本地逻辑测试：`node --test .github/hooks/browser-approval.test.cjs`。
这些测试只给脚本传入模拟 JSON，不调用浏览器或访问网站。
宿主加载测试使用无害 Bash 命令 `printf DJADMIN_BROWSER_APPROVAL_PROBE`：
正确加载时必须被钩子拒绝并返回 `probe intercepted successfully`；
如果命令实际输出标记，则当前宿主尚未加载钩子，不能宣称审批闸已生效。

仓库文件存在、配置解析成功和本地测试通过均不能代替宿主加载验证。
新增钩子后按宿主要求重新加载或新建 Copilot 会话，再用无害探针确认。
本配置使用共享 Copilot 运行时的原生格式，不为其他运行时提供兼容分支。
钩子仍受宿主能力与配置控制，且命令钩子超时会回到宿主原权限流程；
因此不能把项目钩子描述为不依赖宿主、不可绕过的安全沙箱。
格式与失败语义以 [Copilot 官方钩子参考](https://docs.github.com/en/copilot/reference/hooks-reference)
为准。

## 查询请求方法

**所有查询 API 只能使用 GET**，包括列表、详情、搜索、筛选、统计、状态查询和事实观测。
参数通过 URL 路径或查询参数传递，禁止使用 GET 请求体；不能因为需要传 ID 列表、JSON 或多个筛选条件而使用 POST，也不保留 POST 查询分支。ID 列表默认使用逗号分隔的参数，并严格校验；缺失或非法参数不得意外扩大查询范围。创建任务、执行动作及写入业务状态的接口不属于纯查询。
新增或修改查询接口必须同步后端路由、前端调用、文档与回归测试：后端验证 GET 存在且 POST 不提供查询能力，前端断言请求方法和参数传递方式；方法调整不放宽既有权限或改写历史审计。

状态和主机事实观测同样只提供 GET；观测快照的更新不等于修改业务配置。
实时观测返回 `Cache-Control: no-store`，避免浏览器或代理复用过期状态。以下入口保留原权限条件：

| 查询入口 | 参数 |
|---|---|
| `/assets/hosts/:id/refresh-info/` | 路径主机 ID |
| `/assets/hosts/refresh-info/` | 必填 `ids=7,8`，只接受一组正整数列表 |
| `/assets/application-services/:id/refresh-runtime-status/` | 路径逻辑服务 ID |
| `/monitor/log-targets/:id/check-status/` | 路径采集目标 ID |
| `/monitor/elasticsearch-clusters/:id/test-connection/` | 路径集群 ID |

前后端须配套部署，不提供旧 POST 查询兼容入口；GET 请求体不能替代 URL 参数。
批量主机参数缺失、空白、非正整数、溢出或重复传入 `ids` 均拒绝，不扩大到全量主机。
后台状态轮询的自定义超时和错误提示策略通过共享 GET 请求工具传递；失败仍返回调用方处理。
提交脚本做内容校验、提交 Pipeline 与日志样例做解析试算属于内容运算，继续用 POST，
不能为了改变审计分类而将完整代码或日志放入 URL。启停、认证、发布和清理等动作也不改为 GET。

<a id="readonly-permissions"></a>

## 查看与动作权限隔离

`readonly` 是普通 RBAC 授权的结果，不是硬编码的角色特判。`:view` / `:list` 只能查看，
不能触发业务配置变更、数据删除、安装卸载、启停、配置下发或主机终端。命令执行型 GET /
WebSocket 必须使用执行权限；POST 校验不因方法是 POST 就被当成远程执行。

- 后端每个写入/执行入口在进入 handler 前校验明确动作权限。路由组的查看权限只能作为
  共同前提，不能充当动作授权。批量与单条、HTTP 与 WebSocket 入口执行同一规则。
- 创建/删除同时会安装/卸载时必须同时满足资源动作与执行权限，不得以“能创建”推导“能执行”。
- 前端按钮、开关、行内编辑、上传、弹窗提交、批量动作和跨页面入口使用相同权限码；
  必需权限是 AND，`checkPermission([a,b])` 的 ANY 语义不适用于此场景。
- 页面加载、轮询和二阶段补数不得调用安装、配置变更、终端或任务执行接口。
  读取当前主机事实并保存观察结果属于查看，使用 `assets:hosts:view`，不能要求修改权限。
- 新权限同步 MySQL/PostgreSQL 菜单迁移与文档；已有查看角色不自动获得任何写/执行权限。
  扩展只读能力可补只读权限，但不得借升级把 `view` 变成 `manage`。
- 业务拒绝返回既有 HTTP 200 / `code:403` 信封。审计按既有策略记录被拒绝的变更请求，
  不把审计写入等同于业务操作成功。
- 自身资料、头像、经旧密码验证的自身密码修改、自身通知绑定只依赖当前身份；
  纯内容校验、无持久化发布的 Pipeline 模拟、采集状态观测是明确例外，不允许扩大为
  任意用户管理、全局配置修改或命令执行。新例外必须逐接口说明副作用和授权理由。

回归入口：`cd autoadmin && make test PKG=./internal/api/router`。
`TestReadonlyCannotMutateAnyBusinessRoute` 使用非 admin、全量 `:view` / `:list` 权限，
自动遍历业务变更路由并覆盖两个终端入口；新增写路由遗漏动作授权会直接失败。
`TestActionPermissionsReachHandlersWithoutAdmin` 验证完整权限通过至参数校验，逐一移除
必需权限均被拒绝；前端另覆盖只读与有动作权限两种展示/交互状态。
最终矩阵、权限快照和部署顺序见 [登录认证与权限](modules/ACCESS.md)。

## 一、删除类 API：只保留批量删除

列表型资源的删除接口**只允许一个批量删除 API**，不提供单条删除接口；删除单条即传 `ids: [id]`。

- 路由：`POST <资源前缀>/batch-delete/`
- 请求体：`{"ids": [<数字>...]}`
- 权限点：沿用删除权限（如 `inspection:tasks:delete`）
- 响应 data：`{"count": <成功条数>, "results": [{"id": 1, "ok": true, "message": ""}, ...]}`，不存在的 id 记 `ok:false`，不整体失败
- handler 范式：逐 id 复用单删的全部前置校验，参考 `autoadmin/internal/logcollect/log_target_actions.go` 的 `BatchDeleteLogTargets`
- 前端：每个资源在 `src/api/**` 只保留一个 `batchDeleteXxx(ids)`；单删按钮传 `[record.id]`
- 带 body 的删除统一用 POST（避免 DELETE+body 的网关兼容性问题）

**例外**（非列表型删除，允许保留原接口）：

- 树/级联删除：菜单 `deleteMenuById`
- 嵌套子资源：基线分类/检查项（`/baselines/:id/categories/:cid/`）
- 从属资源：主机 WebSSH 文件

### 一.1 批量更新：沿用同一响应口径，但**语义按资源定**

"只保留批量删除"不延伸到更新——更新接口的批量与否由**这一批改动是否可拆**决定，不搞"一律批量"：

- 路由：`POST <资源前缀>/batch-update/` 或嵌在已有子路径下（如
  `/assets/application-services/:id/log-config/settings/batch/`，与单条 `…/settings/` 同级）
- 响应 data：**与批量删除同一口径** —— `{"count": <成功条数>, "results": [{"id": …, "ok": true, "message": ""}, ...]}`，
  不适用/不存在的项记 `ok:false` 并写明原因
- **可拆的改动逐条独立、不整体失败**（每个 id 的改法不同，败一条不该拖累其余）
- **不可拆的改动（整批改的是同一列的同一个值）用一次事务写完**：要么都落库、要么都不落，
  失败即返回错误码，不返回"部分成功"——留半套配置比整体失败更难排查。
  范式：`autoadmin/internal/assets/log_setting.go` 的 `BatchSaveServiceLogOverrides`
  （校验读一次 → 过滤不属于本资源的项 → `UpsertServiceLogOverrides` 一个事务 → 逐条结果）
- 参数非法（空集合、超过上限）在服务层直接 `ErrInvalid`，不进事务
- 前端：一次请求 + 成功后**重拉列表与下发态**；把失败的 id 翻成用户认识的名字（日志名/主机名）再报出来，
  只回 id 用户不知道是哪一条

## 二、前端 a-table 统一风格

所有数据列表性质的 `<a-table>` 统一为：

1. **分页**：从 `@/util/tableStyle` 导入 `createPagination()`，以 `const pagination = reactive(createPagination())` 创建；`total/current/pageSize` 请求后由页面赋值。默认口径：
   - `pageSizeOptions: ['10','20','30','50']`、`showSizeChanger`、`showQuickJumper`
   - `showTotal` 统一为 `共 ${total} 条记录`（改文案只改 tableStyle.js 一处）
   - 特殊需求通过 extra 参数覆盖（如日志场景 `['50','100','200']`）
2. **密度**：`size="small"`。
3. **空状态**：默认 `:locale="tableLocale"`（中文「暂无数据」）；有更精确业务提示时保留自定义。
4. **不分页表格**：显式声明 `:pagination="false"`。

禁止在页面里手写 `showTotal`、`pageSizeOptions` 等分页配置；新增页面一律 import `createPagination` / `tableLocale`。

### 前端运行入口与动态引用边界

- `fronted/src/main.js` 注册路由、Vuex、权限指令、Ant Design Vue、FontAwesome 和全局 `SvgIcon`。
- 路由同时包含静态入口和菜单驱动入口；`src/router/index.js` 通过 `import.meta.glob`
  扫描 `../views/**`，排除 `*.spec.js/ts`、`*.test.js/ts`，避免组件测试及测试框架进入生产懒加载包。
  按菜单的 `component` 字段解析页面，不能仅凭缺少显式 import 判定视图未使用。
- 标签页的 `key` 与 `activeKey` 保留完整地址（含 query/hash），侧栏菜单的 `selectedKeys`
  仅使用页面路径；切换节点或筛选条件不能清除所属菜单高亮。服务树、日志中心按页面路径复用标签，
  其他页面仍可按完整地址保留多个标签；切回标签时恢复完整定位地址。
- SVG 图标由 Vite 的 `createSvgIconsPlugin` 扫描 `src/assets/icons/svg`，通过
  `virtual:svg-icons-register` 注册；图标名属于运行时引用，不以单个 SVG 的 import 数量判定存活。
- API 模块由调用页面/控制器显式导入；保留实际调用的请求封装，后端仍提供某个端点不代表前端需要为其保留未使用的封装。

## 三、懒挂载弹窗的数据加载

父组件用 `v-if` + `open` 懒挂载的弹窗（关闭延迟卸载动画结束后 `v-if` 置 false 的模式），
**首次打开时组件挂载瞬间 `open` 已经是 `true`**。组件内 `watch(() => props.open)` 若不带
`{ immediate: true }`，永远观察不到变更，首开不会加载数据（表格空白且不发请求）。
约定：懒挂载弹窗的 open watch 必须写 `watch(() => props.open, (v) => { if (v) load() }, { immediate: true })`，
加载函数内部自行用 `props.xxx?.id` 等守卫兜住"挂载但未就绪"的场景。

## 四、文档组织约定

1. **新增文档前先查 [文档中心](../README.md)**：按系统菜单找到模块手册，主题已存在则并入，不另起重复文件；
   根 README 与模块 README 只保留导航和运行说明，不再复制架构或 API 契约。
2. 文档按用途归位：
   - `docs/overview/`——项目概览与全局上下文
   - `docs/architecture/`——功能**最终逻辑**（数据流、入口、关键决策、失败语义），不写变更流水账
   - `docs/architecture/modules/`——按系统菜单组织的模块手册，统一使用“重点 → 入口/操作 → 流程图 →
     判断与失败 → 技术细节”。每个模块必须提供可渲染的 Mermaid 流程图；只写当前实现，
     不把未上线方案当成功能。底层架构参考按需链接，不把实现文件名当作主阅读目录。
   - `docs/plans/`——未完成的计划/待办清单（如缺失接口补齐、规模化改造），完成一项更新一项状态，整体完成后归档
   - `docs/archive/`——历史方案、旧实现说明（文件头必须标注"历史归档"）
   - `docs/ops/`——面向部署、验证和排障的操作手册，不复制功能内部设计
3. 变更记录/总结类内容（"本次改了什么"）一律不进 architecture；要么并入对应架构文档的"最终逻辑"，要么进 `docs/archive/`。
4. 后端唯一实现为 Go 版 autoadmin；Django 实现已从版本库移除，文档禁止引用其源码，历史实现只允许引用 `docs/archive/`（见 AGENTS.md）。
5. **数据访问层约定（选 sqlc 还是内联 SQL、字段映射、SQL 方言可移植性）统一见 [SQL_DESIGN.md](SQL_DESIGN.md)**，本文不重复；新增或修改任何 SQL 前先读该文档。

## 配置启停状态

配置型启停状态统一复用 [EnabledSwitch](../../fronted/src/components/EnabledSwitch.vue)，
以巡检中心为唯一基准：Ant Design Vue 默认尺寸，开关内显示「启用 / 停用」。
列表、详情和新增/编辑表单使用相同控件，不使用无文字开关、圆点、标签、复选框或单选组表达同一个启停字段。
用户账号仍提交数字 `1 / 0`，仅在界面映射为布尔值，既有 API 契约不变。

### 交互与失败语义

- 表单开关只修改草稿，随表单提交；不因切换而自动保存。
- 普通资源列表的开关按后端动作权限授权，不按角色名称判断；无动作权限时保留状态展示并禁用交互。
  多项必需权限逐项检查，不能使用 ANY 语义代替 AND。
- 行内保存使用受控状态，等待服务端成功后确认，显示 loading 并阻止重复提交。
  失败或返回状态与请求不一致时保留旧状态并提示原因；刷新替换行对象后更新当前行，不写入陈旧对象，也不把旧请求结果应用到另一资源。
  点击开关不触发服务树行导航。
- 内置集群模型不可编辑；API Token 的状态只读，禁用继续使用既有动作与确认流程，不提供反向启用。
  通知链路诊断、引用服务详情和 Inventory 范围详情中的状态只读，不在诊断中改变配置。
- 运行中/成功/失败、在线/离线、告警级别、自动刷新、必填、TLS 校验、默认项、
  安装/卸载、日志采集及多行模式保留各自的业务语义，不能机械改成「启用 / 停用」。
  依赖图的停用提示与通知绑定的组合故障提示仍是诊断标记，不是写入入口。

### 状态写入

普通资源通过现有详情 `PATCH` 仅提交 `{"enabled":true}` 或 `{"enabled":false}`。
只有请求恰含 `enabled` 时进入窄更新；必须为 JSON 布尔值，拒绝 `null`、字符串、数字、数组和对象。
混合字段请求继续完整编辑的校验与保存路径，不能误当状态请求。
状态写入只更新 `enabled` 与 `update_time`，不从列表回写名称、连接凭据、规则、模板嵌套配置或服务成员。
返回原有资源详情响应；新前端以返回的真实 `data.enabled` 确认状态。

| 资源 | PATCH 入口 | 动作权限及约束 |
|---|---|---|
| 项目 | `/assets/projects/:id/` | `assets:applications:update`，沿用当前后端路由权限 |
| 业务系统 | `/assets/business-systems/:id/` | `assets:service-tree:manage` |
| 环境 | `/assets/business-environments/:id/` | `assets:applications:update` |
| 应用 / 版本 / 集群模型 | `/assets/applications/:id/`、`/assets/application-versions/:id/`、`/assets/cluster-profiles/:id/` | `assets:applications:update`；内置模型拒绝修改 |
| 部署模板 / 服务 / 实例 | `/assets/application-deployment-templates/:id/`、`/assets/application-services/:id/`、`/assets/application-deployments/:id/` | `assets:applications:update`；保持可串行化事务与日志配置一致性校验，失败回滚；服务删除墓碑阻止状态修改 |
| 依赖交互类型 | `/assets/service-dependency-types/:id/` | `assets:applications:update`；停用禁止新引用，已有关系保留 |
| 软件包 | `/monitor/packages/:id/` | `monitor:update`；不改包文件、平台或 Playbook，也不触发安装/卸载 |
| ES 集群 | `/monitor/elasticsearch-clusters/:id/` | `monitor:update`；保留原存储初始化/同步动作 |
| 日志保留档位 | `/monitor/log-retention-tiers/:id/` | `monitor:update`；保留原存储策略同步动作 |
| 日志采集过滤规则 | `/monitor/log-collection-filter-rules/:id/` | `monitor:update`；不回写正则或关联配置 |
| 通知媒介 | `/monitor/media/:id/` | `monitor:update`；不回写 SMTP 配置与密码 |
| 自身通知绑定 | `/sys/usercenter/alertMediaBindings/:id/` | 只依赖当前登录身份；锁定和更新均限定 `user_id`，不能修改其他用户；媒介停用时不能启用绑定 |

状态更新不变更表结构，无新增数据库迁移。SQL 使用生成查询；不存在的目标明确返回 404，
存储或事务失败明确报错，不以修改了 0 行冒充成功。既有账号、自动化任务、Inventory、调度和巡检启停动作继续使用原有接口及限制。
前后端须配套部署：列表直接切换依赖上述状态窄更新，不能只部署前端后继续使用旧 API 进程。

### 页面覆盖与回归入口

以下入口使用同一配置启停规范；路径对应页面或表单组件，不包含业务运行状态的例外控件。

| 入口 | 状态与交互 |
|---|---|
| [项目列表](../../fronted/src/views/assets/projects/index.vue) | 行内启停 |
| [环境列表](../../fronted/src/views/assets/environments/index.vue) | 行内启停 |
| [应用工作区](../../fronted/src/views/assets/application/components/ApplicationWorkspace.vue) | 项目、应用及自定义模型行内启停；内置模型只读 |
| [部署模板管理](../../fronted/src/views/assets/application/components/TemplateManagerDialog.vue) | 行内启停，通知上层刷新 |
| [版本管理](../../fronted/src/views/assets/application/components/VersionDialog.vue) | 行内启停，通知上层刷新 |
| [服务树节点](../../fronted/src/views/assets/service-tree/ServiceTreeNodeContent.vue) | 业务系统、逻辑服务启停，保持导航与运行状态分离 |
| [交互类型管理](../../fronted/src/views/assets/service-tree/ServiceDependencyTypes.vue) | 列表行内启停、表单草稿 |
| [项目表单](../../fronted/src/views/assets/application/components/ProjectDialog.vue) | 草稿启停 |
| [业务系统表单](../../fronted/src/views/assets/application/components/BusinessSystemDialog.vue) | 草稿启停 |
| [环境表单](../../fronted/src/views/assets/application/components/BusinessEnvironmentDialog.vue) | 草稿启停 |
| [应用表单](../../fronted/src/views/assets/application/components/Dialog.vue) | 草稿启停，保留「允许使用」说明 |
| [模型表单](../../fronted/src/views/assets/application/components/ClusterProfileDialog.vue) | 草稿启停 |
| [实例表单](../../fronted/src/views/assets/application/components/DeploymentDialog.vue) | 草稿启停 |
| [模板表单](../../fronted/src/views/assets/application/components/TemplateDialog.vue) | 草稿启停，保留「允许新部署」说明；查看模式只读 |
| [服务表单](../../fronted/src/views/assets/application/components/ApplicationServiceDialog.vue) | 服务与成员草稿启停；采集总开关保留业务语义 |
| [Inventory](../../fronted/src/views/automation/inventory/index.vue) | 既有行内启停、表单草稿、范围详情只读 |
| [自动化任务列表](../../fronted/src/views/automation/automationtask/components/TaskListCard/index.vue) | 既有行内启停与权限/loading |
| [自动化任务表单](../../fronted/src/views/automation/automationtask/components/TaskFormModal/index.vue) | 草稿启停 |
| [用户管理](../../fronted/src/views/sys/user/index.vue) | 既有数字状态接口，行内保存期间防重复 |
| [用户表单](../../fronted/src/views/sys/user/components/Dialog.vue) | 草稿启停，保持数字状态 |
| [系统调度](../../fronted/src/views/sys/scheduler/index.vue) | 既有启停动作、表单草稿；运行中不能切换，保存期间防重复 |
| [API Token](../../fronted/src/views/sys/apiToken/index.vue) | 状态只读，既有单向禁用动作保留 |
| [监控软件仓库](../../fronted/src/views/monitor/index.vue) | 软件包行内启停 |
| [通知媒介](../../fronted/src/views/monitor/media/index.vue) | 既有行内启停、表单草稿 |
| [日志存储](../../fronted/src/views/monitor/log-storage/index.vue) | 集群行内启停、表单草稿 |
| [日志保留档位](../../fronted/src/views/monitor/log-retention/index.vue) | 行内启停、表单草稿、详情只读 |
| [日志解析与过滤](../../fronted/src/views/monitor/log-parsers/index.vue) | 过滤规则行内启停与草稿；关联服务详情只读 |
| [告警历史链路](../../fronted/src/views/monitor/alerts/index.vue) | 媒介只读状态，绑定组合诊断保留 |
| [用户通知链路](../../fronted/src/views/monitor/alerts/UserNotificationChain.vue) | 绑定与媒介状态只读 |
| [个人中心](../../fronted/src/views/userCenter/index.vue) | 自身绑定行内启停、表单草稿 |
| [巡检中心](../../fronted/src/views/inspection/index.vue) | 统一基准，保留任务/定时任务启停与表单规则 |
| [安全基线](../../fronted/src/views/security/baseline/index.vue) | 草稿启停，扫描结果状态不变 |

回归入口：公共控件测试覆盖文字、默认尺寸、表单禁用继承、权限、loading、防重复、成功/失败及刷新竞争；
全站模板守卫覆盖上述入口的控件复用、动作权限、自助范围和只读限制。
各页既有组件测试继续验证原业务交互；后端测试覆盖布尔校验、窄 SQL、资源保护、事务回滚、日志校验与自身绑定所有权，
路由全量只读回归验证查看权限不能触发业务变更。

## 前端查看详情入口

列表操作列的只读详情入口以部署模板列表为统一基准：Ant Design Vue 小号 `type="link"` 按钮，
复用 `detail-action` 样式、Font Awesome `fas/eye` 图标和「详情」文字，不使用带边框的 ⓘ 图标按钮。
默认蓝色无边框，悬停或键盘聚焦显示浅色背景；按钮最小宽度 72px，图文间距 6px，禁用状态保留组件原有样式。
公共样式仅作用于显式标记的详情按钮，不改变新增、编辑、删除、运行、下载及通知状态按钮。

此入口用于部署模板、Inventory、日志保留档位、基线策略及扫描记录、主机列表、
操作审计、WebSSH 审计、调度执行记录、日志查询的记录与聚合样例。
操作列需为图文按钮预留宽度，固定在右侧并配置表格横向滚动；
主机只读用户与管理用户使用相同详情样式，保留原 Tooltip 锚点与权限判断。
每个按钮提供顶部 Tooltip：通常为「查看详情」，日志查询保留「详细日志」语义。
按钮只改变展示，不改变查看内容、详情弹窗、权限、禁用条件或后端 API。

## 五、复制到剪贴板：统一走 `@/util/clipboard`

内网部署多为 **HTTP（非安全上下文）**，此时 `navigator.clipboard` 不存在；直接调
`navigator.clipboard.writeText(...)` 必然抛错，表现为"复制失败，请手动选中复制"
（2026-09-21 日志规则页运行结果复制即此）。约定：所有复制按钮统一用
`import { copyTextWithFallback } from '@/util/clipboard'`——它优先用 Clipboard API，
不可用时回退 `document.execCommand('copy')`；返回布尔值，调用方据此提示成功/失败。


<a id="api-contract"></a>

## 六、API 契约

响应格式以 [API 规则](../../.github/API_RULES.md) 为准；以下补充现有契约及后端实现边界。

### 监控与日志采集的可选列表筛选

两域的可选布尔、整数、字符串及 `search` 条件统一由
`autoadmin/internal/shared/binding/query.go` 解析，返回对应的 `sql.Null*`，
供列表及计数查询复用。参数去除首尾空白；缺失、空白以及无法解析的布尔/整数不启用该筛选。
整数按有符号 64 位十进制解析，零和负数不额外拦截；重复参数取首值。
非空 `search` 两侧加 `%`，不转义输入中的 SQL LIKE 通配符。
这组函数仅用于上述可选筛选，不替代资源 ID、权限条件及其他必须拒绝非法值的参数校验。

### Success

Successful business operations use HTTP 200:

```json
{"code": 200, "msg": "success", "data": {}}
```

### Business errors

Expected business errors retain HTTP 200 and use application codes:

- `300`: account or password error
- `301`: invalid or expired token
- `400`: invalid request
- `403`: permission denied
- `404`: resource not found

Transport, malformed HTTP and unavailable dependency errors may use an appropriate HTTP status, but still use the same envelope.

### Pagination

List endpoints accept `page` and `page_size`. The compatibility default is 10 and the maximum is 30.

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [],
    "count": 0,
    "pageNumber": 1,
    "pageSize": 10,
    "totalPages": 0,
    "next": null,
    "previous": null
  }
}
```

### Compatibility rules

- Preserve existing route prefixes and JSON field names until a coordinated frontend change is approved.
- Preserve JWT permission strings in `module:resource:action` form.
- Keep `/monitor/prometheus/http-sd/` and `/monitor/alert-webhook/api/v2/alerts` as explicitly authenticated machine endpoints according to their current behavior.
- Contract tests pin the frontend-visible response envelope; the authoritative response rules are [API_RULES.md](../../.github/API_RULES.md).


<a id="error-conventions"></a>

## 七、后端错误分层

autoadmin uses three error layers.

### 1. Error mechanism

`internal/shared/apperror/error.go` defines the immutable `Error` type, accessors, `errors.As` support and `WithCause`. It must not contain domain-specific errors.

Repository and infrastructure packages should preserve technical context with `%w`:

```go
return fmt.Errorf("query users: %w", err)
```

These errors are for logs and tracing. They must not be returned directly to clients.

### 2. Common catalog

`internal/shared/apperror/common.go` contains errors whose meaning is identical in every module:

- invalid request, ID and pagination
- invalid or expired token
- permission denied
- generic resource not found
- generic internal failure

Handlers reference these values directly through `response.Error`.

### 3. Module catalogs

Every business module owns an `errors.go`. Examples:

- `identity/errors.go`: invalid credentials, disabled user, user not found
- `rbac/errors.go`: role/menu validation and lookup errors
- `sysconfig/errors.go`: readonly config, missing default, typed-value errors

An error belongs to a module when its wording or code expresses that module's business meaning. Do not duplicate common errors in each module.

### Internal causes

Module catalogs contain stable public errors. Attach an operation failure without mutating the catalog value:

```go
response.Error(ctx, apperror.WithCause(identity.ErrUserQueryInternal, err))
```

The client receives only the stable code/message/status. Logging middleware may inspect `errors.Unwrap` to record the cause after redaction.

### Rules

1. Do not use `errors.New` for API or domain errors.
2. Do not construct code/message pairs inside handlers.
3. Do not return `err.Error()` to clients.
4. Use existing common or module errors before adding a new one.
5. Give new domain modules an `errors.go` as part of their first API slice.
6. Preserve existing frontend application codes during migration.
7. Unknown errors are mapped to common code `600` without exposing the cause.
