# 系统管理与操作审计（autoadmin）

统一描述系统参数、RBAC、操作审计、[用户头像](#user-avatar) 和 [菜单与路由](#menu-structure)。
纳管目标的在线守卫属于 [监控架构](MONITOR_PROMETHEUS_PROXY.md)。

## RBAC 菜单树（internal/rbac/menu.go）

- 根菜单约定 `parent_id=0`（Django 遗留），但**历史数据存在 NULL**：`menuTree` 两者都按根处理（⚠️ 修复前 NULL 根菜单会被整条丢弃，勿回退）。
- 排序：同级按 `order_num` 升序，缺失/并列时按 `id`。
- 异常拓扑语义：孤儿节点（parent 指向不存在的 ID）、双节点环、自引用环都**不得输出到根**，直接丢弃。
- 叶子节点 `children` 输出空数组而非 null（前端依赖可迭代）。
- `validateParent`：parent 为 nil/0 合法；`parent=自身` 拒绝（ErrMenuSelfParent，纯内存判断不触库）。

## 系统参数（internal/sysconfig/service.go）

- `normalize` 按 `value_type` 严格校验：`int`（Atoi，不 trim）；`bool` 接受 true/1/yes、false/0/no（大小写不敏感）；`json` 接受对象/数组/合法 JSON 字符串；`secret` 哈希落库，**掩码 `******` 原样透传**（编辑页未改密时防重复哈希）；未知类型按字符串。
- `typed`：读取侧类型还原；`secret` 值对所有客户端输出一律掩码 `******`（含 default_value）。

## 操作审计（internal/audit）

- 审计中间件记录变更类请求并脱敏，豁免清单请求跳过（`middleware_test.go`）。
- 操作日志列表通过响应模型输出，`duration_ms` 为毫秒整数或 `null`，`user_id` 为整数或 `null`，不暴露数据库可空类型的内部结构；`created_at` 统一返回 UTC 时间。前端列表与详情直接显示耗时，缺失值显示 `-`，零毫秒保留为 `0`。
- 终端历史日志清洗 ANSI 转义、可读命令解析（`history_test.go`）。

## 测试覆盖

- `internal/rbac/menu_test.go`：层级/排序、孤儿丢弃、环保护（双向/自引用）、NULL 根、可空字段映射、validateParent。
- `internal/sysconfig/service_test.go`：normalize 全类型（含 secret 哈希+掩码透传）、typed 类型还原与掩码、mapConfig 输出契约（掩码+UTC 时间格式）。
- `internal/audit/history_test.go` + `middleware_test.go`：见上。
- `internal/audit/repository_test.go`：操作日志响应字段、可空整数及 UTC 时间序列化。


<a id="user-avatar"></a>

## 用户头像与静态文件

> 适用范围：`autoadmin/internal/identity/avatar.go`、`internal/api/router/router.go` 的 `/media` 路由、
> `fronted/src/views/userCenter/components/Avatar.vue`、`fronted/src/layout/header/components/avatar.vue`。

### 数据与接口

- 头像文件名存在 `sys_user.avatar`（`varchar(255)`，只存文件名，如 `20250918123000.jpg`）。
- 上传：`POST /user/changeAvatar`（登录即可，`multipart` 字段名 `avatar`）。
- 展示：静态文件服务 `GET /media/*`，根目录为进程工作目录下的 `media/`（即 `autoadmin/media`）。

### 上传链路（`ChangeAvatar`）

1. 从 token 取当前用户；请求体限制 5MB（在解析 multipart 前用 `http.MaxBytesReader`）。
2. 后缀白名单 `png/jpg/jpeg/gif/webp`，其余 400。
3. 文件写 `media/userAvatar/<时间戳><后缀>`；写盘失败清理残留并 400。
4. 成功落库：`UpdateUserAvatar` 写 `sys_user.avatar`；落库失败则删除刚落的新文件并报错。
5. 删除该用户旧头像文件（失败不影响上传结论）。
6. 返回 `{new_file_name, avatar, avatar_url}`，`avatar_url` 形如 `/media/userAvatar/<file>`（相对地址）。

### 静态文件服务

- `router.NewWithGateway` 注册 `engine.Static("/media", <cwd>/media)`，匿名可访问；目录不存在时返回 404。
- 路径穿越由 `http.Dir` 拦截；`audit` 中间件已跳过 `/media` 与 `/static` 前缀，静态请求不落审计。
- 前端 `getMediaUrl(file)` 统一把文件名/相对路径拼成 `getServerUrl() + /media/...`（开发环境后端在 9000 端口，
  生产可同源或由 nginx 转发 `/media`）。

### 前端

- `userCenter/components/Avatar.vue`：进入时用本地缓存的 `currentUser.avatar` 回显；上传成功后把响应里的
  `avatar` 写回 localStorage 的 `currentUser`，并即时刷新预览。
- `layout/header/components/avatar.vue`：顶栏头像用 `currentUser.avatar` 显示，无头像时回退 `SvgIcon user`；
  本地缓存是 setup 时读取，上传后需刷新页面（或重新登录）顶栏才更新。

### 与 Django 的差异

- 旧 Django `changeAvatar` 只存文件、只返回文件名，不更新 `sys_user.avatar`，头像实际不可用。
- Go 版在保存文件后**写入 `sys_user.avatar` 并删除旧文件**，配合 `/media` 静态服务形成完整可显示链路。

### 失败语义

- 未登录：401。
- 无文件 / 后缀不在白名单 / 超 5MB：400，文案见实现。
- 落库失败：删除新文件并 500，不留下孤儿文件。


<a id="menu-structure"></a>

## 菜单结构与路由

菜单数据在 `sys_menu`（通过菜单管理 UI 维护），前端路由分两层：

- **静态路由**（`fronted/src/router/index.js` `staticRouterMap`）：声明真实路由与组件映射，**path 与菜单表保持一致**；旧地址用 `redirect` 兜底，保证收藏/书签不断链。
- **动态菜单路由**（`getDynamicalRoutes`）：登录后按菜单表展开叶子注册；与静态路由同名时以静态为准（`router.hasRoute(name)` 去重）。

### 公共文档中心

- 顶级菜单「文档中心」对应静态路由 `/docs`，与首页一样由前端布局提供，不写入 `sys_menu`，
  不需要迁移或角色授权，所有已登录用户可见。路由进入时检查登录 token；未登录跳转登录页。
- 页面左侧按菜单展示可搜索、可展开的模块目录，右侧显示 Markdown 正文。首次进入展示
  `docs/README.md`；主目录为登录认证与权限、首页、资产管理、自动化运维、监控中心、安全中心、
  系统管理、个人中心及文档中心。技术文件归入默认折叠的开发参考。
  多级菜单可定位同一模块手册的不同章节，展开实际祖先目录；正文提供本篇二级章节导航。
  同一页面内切换文档，不为每篇文档新增系统标签页；刷新页面重新打开文档中心首页。
- 文档通过 Vite 的静态白名单 `import.meta.glob` 按需加载，随前端构建发布：包含模块手册、
  当前架构与操作说明、项目与模块 README、数据库/部署 README 和 Agent 协议。
  计划、历史归档、尚未落地的 Kubernetes 方案及未接通后端的 Workflow 专题保留在仓库，不发布到阅读器。
  不包含协作指令、技能配置、源码、环境文件或任意服务器文件；新增白名单范围外的专题需显式登记。
- 正文相对文档链接在阅读器内切换；有 README 的目录打开 README，无 README 的文档目录展示文件列表。
  支持中文标题、重复标题和合并文档显式锚点的章节跳转。外链新窗口打开并隔离 opener；
  未发布文件链接和缺失章节给出明确提示，加载失败显示错误，快速切换时只接受最新一次加载结果。
- 原始 HTML 不执行，仅识别严格的 `<a id="..."></a>` 显式章节标记；图片只显示替代文本，不发起外部请求。
  表格、代码块、引用等基础 Markdown 正常渲染。Mermaid 按需加载后以 strict 模式渲染 SVG，
  在全局及 flowchart 配置中显式设置 `htmlLabels: false`，使节点和连线标签使用 SVG text，
  避免 HTML 标签被 SVG 白名单清洗后只剩空框；拒绝配置指令、frontmatter 和点击脚本，产物经 DOMPurify SVG 白名单清洗，
  不绑定图表回调。渲染在离屏容器完成且检查最新请求 ID，旧异步结果不能覆盖新文档；
  加载或渲染错误明确展示，不用成功形态静默回退。重点使用醒目引用块，章节分隔清晰。
- 此方案是内置的**非机密项目手册**，不是权限隔离的文件服务：前端登录门禁不等于静态资源保密，
  不应在打包文档中放入密码、私钥或内部敏感信息。修改文档后，开发模式即时更新；生产环境需重新构建并发布前端。
- 回归入口：文档目录/渲染工具、阅读页、公共菜单和路由的 Vitest 用例；覆盖真实文档加载、
  链接/目录/章节导航、并发切换、加载失败、未发布文件提示与登录门禁。

### 监控中心分组（迁移 000018_monitor_menu_regroup）

监控中心（id=104）下 9 个平铺菜单按职能挂入 3 个二级目录，path 同步嵌套：

```
监控中心 /monitor
├── 监控告警 /monitor/alerting
│   ├── 智能监控        /monitor/alerting/dashboard
│   ├── 告警规则        /monitor/alerting/rules
│   ├── 告警            /monitor/alerting/alerts
│   └── Explore         /monitor/alerting/explore
├── 通知管理 /monitor/notification
│   ├── 媒介            /monitor/notification/media
│   └── 通知策略        /monitor/notification/policies
└── 日志管理 /monitor/logging
    ├── 日志采集        /monitor/logging/collectors
    ├── 日志存储        /monitor/logging/storage
    ├── 日志处理规则    /monitor/logging/parsers
    ├── 日志保留档位    /monitor/logging/retention
    └── 日志中心        /monitor/logging/center        （迁移 000037）
```

> 「日志中心」见 [LOG_COLLECTION_ARCHITECTURE.md](LOG_COLLECTION_ARCHITECTURE.md) §9.5。

#### 「存储水位」并入「日志中心」（迁移 000038）

原「存储水位」（`/monitor/logging/overview`，迁移 000019）的页面能力已逐条并入「日志中心」，
所以整条入口下线：**菜单行删除**（先清 `sys_role_menu` 再删 `sys_menu`，该菜单 perms 为空、
不涉及权限点）、**页面组件删除**（`views/monitor/log-storage-overview/`）、
**旧地址保留 redirect** `/monitor/logging/overview → /monitor/logging/center`（收藏/书签不断链）。
后端接口 `GET /monitor/elasticsearch-clusters/:id/log-storage-overview/` **不删**——日志中心的水位 tab 在用。
迁移的 `down` 只还原菜单行，回滚它必须同时回滚那次前端改动。

#### 日志采集从「智能监控 → 纳管目标」拆出（迁移 000031）

原来是 exporter 与 Filebeat 混在同一张主机表里、用 segmented 切换目标类型。2026-09-18 拆开：

- **日志管理 → 日志采集**（`/monitor/logging/collectors`）承载 Filebeat 纳管目标：安装/卸载、启停、
  下发采集配置、配置状态（期望 vs 已下发）。放到这里的理由是采集与日志存储/处理规则/保留档位同属
  一条链路，而监控页只保留 exporter 目标（该 tab 同时更名为「Exporter 目标」）。
- **权限**：新菜单带 `monitor:log_collect:view`，`/monitor/log-targets/*` 按它鉴权；主机列表
  `/monitor/targets/host-overview/` 两个页面共用，仍留在组级的 `monitor:view` 下。
  权限码随 JWT 签发，升级后存量登录用户需要重新登录才能访问采集接口。
- **前端共享**：主机树/搜索/分页/行选择/运行态刷新抽到 `fronted/src/util/hostTargetTable.js`，
  表格外壳抽到 `fronted/src/views/monitor/components/HostTargetPanel.vue`，两个页面共用同一份实现
  （避免"拆成两个页面 = 两套机械"的漂移）。

### 约定

- **菜单管理页保存时的字段类型**（2026-09-19 现场）：`parent_id` / `order_num` / `location` 在接口侧是整数
  （`menuRequest` 的 `*int32` / `int16`），前端提交前必须归一成数字——文本框给出的字符串会让
  `ShouldBindJSON` 整个请求失败，表现是"保存菜单失败"，而服务端日志只留 `<nil>`、弹窗只弹一个 `400`。
  所以：这三个字段用数字型控件（「显示顺序」是 `a-input-number`），`Dialog.vue` 的 `normalizeMenuPayload`
  再做一道兜底；绑定失败时后端把**字段名**带进消息（`menuBindError`，形如
  `json: cannot unmarshal string into Go struct field menuRequest.order_num …`），前端 `handleApiError`
  优先显示信封里的 `msg`（此前取 `Object.keys()[0]` = `code`，把数字 400 当消息弹出来）。
- 新增监控类页面：菜单挂入对应二级目录，path 用嵌套前缀，同时在 `staticRouterMap` 加路由；旧 path 如有历史引用，保留 redirect 行。
- 迁移 000018 的 SQL 幂等（按 path 条件定位），对新环境与已手动整理的环境都安全；down 脚本可完整还原扁平结构。
- 目录节点（menu_type=M）不注册组件路由，仅作侧边栏分组；分组同时是权限边界，可按目录粒度授权（如值班人员只授"监控告警"）。
- **新增目录必须补授权**：`sys_role_menu` 按菜单 id 授权，目录不授给角色时前端菜单树会整枝裁掉（子菜单全部不可见）。新建目录后需为拥有父菜单的角色 `INSERT IGNORE INTO sys_role_menu(role_id, menu_id)` 补授（迁移 000018 已内置该步骤）。
