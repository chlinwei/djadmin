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
- 响应采集与落库文本使用独立上限：最多缓存 1 MiB 的完整响应 JSON，先解析并递归脱敏，再将请求与响应各自的落库文本限制在 16 KiB（16384 字节）内；超长审计文本以 `[truncated]` 明确标记，标记计入上限，并保持 UTF-8 字符完整。浏览器收到的业务响应始终原样透传，不受审计截断影响。
- 审计 JSON 文本编码不对 `<`、`>`、`&` 做 HTML 转义，因此包含 XML 的请求与响应即使截断也保留可读标签，而不是显示大量 `\u003c`、`\u003e`、`\u0026`。前端仍以纯文本插值展示，不把日志内容作为 HTML 执行；标准 JSON 引号、换行和控制字符转义保持不变。
- 响应超过 1 MiB 时，不解析或保存不完整的原文前缀，以免遗漏敏感字段脱敏；返回内容改记明确摘要 `{"capture_status":"response_too_large","limit_bytes":1048576}`，而不是空值。审计缓存保持有界，完整响应中的 `msg` 在可解析时独立保存，不受落库文本长度限制。
- 历史记录中已丢失的响应内容不能从审计记录恢复；采集规则仅作用于使用该实现处理的新请求。
- 操作日志列表通过响应模型输出，`duration_ms` 为毫秒整数或 `null`，`user_id` 为整数或 `null`，不暴露数据库可空类型的内部结构；`created_at` 统一返回 UTC 时间。前端列表与详情直接显示耗时，缺失值显示 `-`，零毫秒保留为 `0`。
- 详情弹窗的请求参数与返回结果分别提供复制按钮，复制文本与页面展示保持一致：完整 JSON 使用格式化文本，截断记录保留原文与截断标记，空内容不复制并给出提示。复用公共剪贴板工具支持 HTTP 页面，复制成功与失败均明确通知；复制不会恢复已截断或脱敏的内容。
- 终端历史日志清洗 ANSI 转义、可读命令解析（`history_test.go`）。

### WebSSH 会话审计

- 连接入口为 `/ws/assets/hosts/:id/webssh/`，主机下的 HTTP WebSocket 入口复用同一 handler。
  通过既有登录与主机查看授权、确认主机及 Agent 身份后，先向 `assets_webssh_session_log`
  写入主机 ID、当前登录用户、客户端 IP/User-Agent、请求的目标系统用户与 UTC 开始时间，
  再请求 Agent 打开终端；审计创建失败明确报错，不开启未记录的终端。
- 会话开始及等待 Agent 应答期间使用 `connected`，目标用户切换状态为 `pending`；
  Agent 确认打开后保存真实系统用户和切换成功状态。列表按数据库状态原样返回：
  `connected` 为连接中、`closed` 为正常关闭、`failed` 为失败，状态筛选精确匹配，
  不把仍连接的记录转换为已关闭。
- 终端桥接在一个事件循环内顺序处理 Agent 输出、浏览器输入与审计快照，避免并发修改记录。
  成功发送到 Agent 的输入计入输入字节数；CR/LF 提交计数合并跨帧 CRLF，计入 `command_count`。
  该数值代表输入行提交次数，不代表经 shell 确认执行的命令数；可读输入解析也不是 shell 执行凭证。
  Agent 输出按原有协议实时转发给浏览器，审计内容截断不截断终端显示。
- 输入和输出共用 **1 MiB** 原始内容预算，超限停止追加内容，保留输入计数并设置截断标记。
  SQL 快照重新拼合跨帧 UTF-8；非法字节及 NUL 使用替代字符，兼容两侧数据库文本字段，
  存储的 UTF-8 输入/输出合计同样不超过 1 MiB，字节数以实际存储文本为准。
  Agent 确认连接时立即保存；有变化的会话每 5 秒保存一次，空闲或截断后未变化的输出不反复写库。
- 关闭窗口、主动关闭或 Agent 正常退出时保存 `closed`、结束时间、时长与关闭码；
  Agent 离线、打开失败、远端异常退出、异常断网或审计更新失败保存 `failed` 和原因，
  并结束终端连接。所有退出路径尝试关闭远端终端；结束写入使用独立的 5 秒上下文，
  不受浏览器请求取消影响。数据库写入失败写结构化错误日志并尽可能提示浏览器，不输出终端内容到服务日志。
- 进程被强杀或机器掉电时无法执行结束写入：只能保留最后一次成功快照，最多可能丢失一个刷新周期内
  尚未落库的内容，`connected` 记录也可能残留。会话历史不是实时在线人数的权威来源，
  不在查询时把残留记录伪装成正常关闭；未写入的历史连接无法补录。
- 审计列表、内容查看及下载仍由 `audit:webssh_sessions:view` 控制。终端输入/输出可能包含口令、
  业务数据等敏感内容，不承诺自动识别或脱敏，必须严格限制该权限；保存期继续由 scheduler 的
  WebSSH 会话日志清理任务控制。沿用现有表结构，不需要数据库迁移。

## 测试覆盖

- `internal/rbac/menu_test.go`：层级/排序、孤儿丢弃、环保护（双向/自引用）、NULL 根、可空字段映射、validateParent。
- `internal/sysconfig/service_test.go`：normalize 全类型（含 secret 哈希+掩码透传）、typed 类型还原与掩码、mapConfig 输出契约（掩码+UTC 时间格式）。
- `internal/audit/history_test.go` + `middleware_test.go`：见上。
- `internal/audit/repository_test.go`：操作日志响应字段、可空整数及 UTC 时间序列化。
- `internal/assets/webssh_audit_test.go`：终端双向转发、运行中快照、正常/异常结束、数据库失败、
  内容上限、UTF-8 跨帧和输入提交计数；审计仓储测试覆盖连接状态与筛选不被改写。


<a id="user-avatar"></a>

## 用户头像与静态文件

> 适用范围：`autoadmin/internal/identity/avatar.go`、`internal/api/router/router.go` 的 `/media` 路由、
> `fronted/src/views/userCenter/components/Avatar.vue`、`fronted/src/layout/header/components/avatar.vue`。

### 数据与接口

- `sys_user.avatar` 保存 `userAvatar/<随机文件名>.png`，用户详情、登录和列表响应仅包含此地址，不携带图片数据。
- `sys_user_avatar` 每用户一行，保存随机文件名、PNG 二进制和 UTC 更新时间；`user_id` 外键随用户删除级联清理。MySQL 使用 `MEDIUMBLOB`，PostgreSQL 使用 `bytea`，两侧数据库均用 CHECK 保证内容为 1～1048576 字节，不存 Base64。
- 上传：`POST /user/changeAvatar`（登录即可，`multipart` 字段名 `avatar`）。
- 展示：`GET /media/userAvatar/<file>`（支持 HEAD）从数据库读取图片二进制，匿名访问语义不变；图片响应不使用 JSON 包装。其他 `/media/*` 仍读取进程工作目录下的 `media/`，不影响软件包等下载。

### 上传链路（`ChangeAvatar`）

1. 从 token 取当前用户，只能修改自己的头像，无需用户管理权限。上传图片原文件最多 **5 MiB**；multipart 请求体额外允许 64 KiB 封装开销，在解析前限制请求体，解析临时文件结束后清理。
2. 后缀白名单 `png/jpg/jpeg/gif/webp`，同时实际解码图片，拒绝伪造扩展名和损坏文件。解码前检查每边最多 8192 像素、总像素最多 32×1024×1024，限制图片解压内存。
3. 保持比例、不放大，最长边缩至最多 1024 像素。重新编码 PNG，保留透明度、去掉元数据；GIF 取首帧，不支持的 WebP 动画明确拒绝。若编码结果仍超过 **1 MiB（1048576 字节）**，继续按比例缩小并编码，直到满足限制。该上限针对**处理后的图片二进制**，不是原文件、请求体或 JSON 大小。
4. 生成随机文件名；事务中锁当前用户行，替换该用户的数据库图片并更新 `sys_user.avatar`。任一步失败全部回滚，旧头像保持可用；并发上传不会使地址和图片来自不同请求。管理员编辑资料或切换用户状态不回写头像字段，避免覆盖并发上传的新地址。新上传不写磁盘。
5. 返回 `{new_file_name, avatar, avatar_url}`，其中 `avatar` 为 `userAvatar/<file>`，`avatar_url` 为 `/media/userAvatar/<file>`。

### 静态文件服务

- `/media/userAvatar/` 仅查询数据库，找不到返回 404，不回退读取磁盘。路径规范化后再分流，不能通过 `..` 绕回旧头像目录；数据库故障明确返回错误。
- 图片固定输出 `image/png` 和 `X-Content-Type-Options: nosniff`，提供 ETag、Last-Modified 和一小时缓存。更新时随机地址改变，避免旧缓存遮蔽新头像。
- 头像上传显式将 Ant Design 的 `X-Requested-With` 配置为 `null`，使用组件支持的方式关闭这个不参与业务的默认请求头，实际请求仍携带 `Authorization`，由浏览器生成 multipart Content-Type；跨域预检不再额外依赖该标记。API 公共 CORS 白名单同时允许其他上传入口使用的 `X-Requested-With`，不改变配置的来源限制或登录要求。
- 其他 media 路径继续使用 `http.Dir`；`audit` 中间件已跳过 `/media` 与 `/static` 前缀，图片请求不落操作审计。
- 前端 `getMediaUrl(file)` 统一把文件名/相对路径拼成 `getServerUrl() + /media/...`（开发环境后端在 9000 端口，
  生产可同源或由 nginx 转发 `/media`）。

### 前端

- `userCenter/components/Avatar.vue`：进入时用本地缓存的 `currentUser.avatar` 回显；上传成功后把响应里的
  `avatar` 写回 localStorage 的 `currentUser`，并即时刷新预览。
- `layout/header/components/avatar.vue`：顶栏头像用 `currentUser.avatar` 显示，无头像时回退 `SvgIcon user`；
  用户信息响应式读取共享 store，刷新后的初始回显使用本地缓存。上传成功通过已有 `saveCurrentUser` 同时更新 store 和缓存，
  顶栏与个人中心预览立即切换新图片地址，无需刷新或重新登录；上传失败不改变共享状态或顶栏头像。

### 部署与已有头像导入

- 先使用 `./bin/autoadmin migrate` 应用两侧对应的 `000062_user_avatar` 表结构迁移，再在保存旧头像文件的 `autoadmin/` 工作目录执行 `./bin/autoadmin migrate avatars`。导入命令使用业务数据库连接 `MYSQL_DSN`／`POSTGRES_DSN`，不需要 API、RabbitMQ 或 schema 迁移专用 URL。
- 显式导入读取 `media/userAvatar/`，支持旧字段中的文件名、`userAvatar/...`、`media/userAvatar/...`、`/media/userAvatar/...`，使用与新上传相同的处理和大小规则。限制读取根目录，拒绝路径越界与非普通文件；不存在、损坏、超限文件明确报告对应用户并以失败退出，不伪装成导入成功。
- 每用户独立事务，已成功导入的用户再次运行时跳过；并发修改头像时不覆盖新值。失败修复后可重新执行。源文件不自动删除，作为回滚备份；运行时不使用旧磁盘兜底。建议维护窗口停止旧版头像写入，完成导入并切换新 API 后重新登录，刷新浏览器缓存中的头像地址。
- 回滚表结构会删除数据库图片，且不会自动恢复 `sys_user.avatar` 的旧地址；回滚前应备份数据库及旧字段映射。迁移不删除或改动原用户字段、旧外键。

### 失败语义

- 上传未登录：沿用 token 校验失败响应。
- 无文件、后缀不在白名单、原文件超 5 MiB、图片无效或尺寸超限：业务码 400，页面显示明确原因。
- 落库失败：HTTP 500，事务回滚，不产生孤立图片或改变旧地址。
- 前端对 HTTP 非 2xx 上传失败也读取服务端 JSON 的 `msg`，显示 HTTP 状态码；代理返回 HTML 时不展示原文，413 明确提示请求体过大，其他状态提示检查后端／代理日志。无 HTTP 响应则提示检查网络、跨域或服务可用性。错误同时用提示消息及头像下方常驻错误区域展示，直到重新上传或成功，避免提示消失后无法定位；失败不更新旧头像和缓存。


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
