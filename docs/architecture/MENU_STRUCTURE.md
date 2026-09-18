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
    └── 日志保留档位    /monitor/logging/retention
```

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

- 新增监控类页面：菜单挂入对应二级目录，path 用嵌套前缀，同时在 `staticRouterMap` 加路由；旧 path 如有历史引用，保留 redirect 行。
- 迁移 000018 的 SQL 幂等（按 path 条件定位），对新环境与已手动整理的环境都安全；down 脚本可完整还原扁平结构。
- 目录节点（menu_type=M）不注册组件路由，仅作侧边栏分组；分组同时是权限边界，可按目录粒度授权（如值班人员只授"监控告警"）。
- **新增目录必须补授权**：`sys_role_menu` 按菜单 id 授权，目录不授给角色时前端菜单树会整枝裁掉（子菜单全部不可见）。新建目录后需为拥有父菜单的角色 `INSERT IGNORE INTO sys_role_menu(role_id, menu_id)` 补授（迁移 000018 已内置该步骤）。
