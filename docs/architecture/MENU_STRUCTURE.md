# 菜单结构与路由约定

菜单数据在 `sys_menu`（通过菜单管理 UI 维护），前端路由分两层：

- **静态路由**（`fronted/src/router/index.js` `staticRouterMap`）：声明真实路由与组件映射，**path 与菜单表保持一致**；旧地址用 `redirect` 兜底，保证收藏/书签不断链。
- **动态菜单路由**（`getDynamicalRoutes`）：登录后按菜单表展开叶子注册；与静态路由同名时以静态为准（`router.hasRoute(name)` 去重）。

## 监控中心分组（迁移 000018_monitor_menu_regroup）

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

### 「存储水位」并入「日志中心」（迁移 000038）

原「存储水位」（`/monitor/logging/overview`，迁移 000019）的页面能力已逐条并入「日志中心」，
所以整条入口下线：**菜单行删除**（先清 `sys_role_menu` 再删 `sys_menu`，该菜单 perms 为空、
不涉及权限点）、**页面组件删除**（`views/monitor/log-storage-overview/`）、
**旧地址保留 redirect** `/monitor/logging/overview → /monitor/logging/center`（收藏/书签不断链）。
后端接口 `GET /monitor/elasticsearch-clusters/:id/log-storage-overview/` **不删**——日志中心的水位 tab 在用。
迁移的 `down` 只还原菜单行，回滚它必须同时回滚那次前端改动。

### 日志采集从「智能监控 → 纳管目标」拆出（迁移 000031）

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

## 约定

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
