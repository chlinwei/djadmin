# 告警通知链路视图（前端）

描述告警通知链路诊断的前端最终逻辑：用户视角（P1）与单告警链路（P2）。后端契约：`GET monitor/alert-notification/user-chain/` 与 `GET monitor/alert-notification/chain/:historyId/`（Go 版 autoadmin 实现中，字段以本契约为准）。

## 数据流与入口

- API 封装：`fronted/src/api/monitor.js` 的 `getUserNotificationChain(userId?)`（userId 为空时不传 `user_id`，后端按当前登录用户处理）和 `getAlertNotificationChain(historyId)`。
- 公共展示组件：`fronted/src/views/monitor/alerts/UserNotificationChain.vue`，props `userId` 可空。

### P1 用户视角（个人中心 + 用户管理）

- 个人中心 `fronted/src/views/userCenter/index.vue` 的"告警媒介"tab 内、绑定表格下方渲染"通知链路诊断"卡片，不传 `userId`（查自己）。
- 用户管理 `fronted/src/views/sys/user/index.vue` 操作列新增"通知链路"按钮（沿用 `v-permission.remove="'system:users:update'"`，操作列本身仅对具备 update/delete 权限的用户渲染），点击弹窗内以 `record.id` 作为 `userId` 渲染同一组件。
- 组件最终逻辑：顶部 `a-alert` 横幅——`can_receive=true` 绿色"你当前会收到告警通知"；`false` 红色"你当前不会收到告警通知"并逐条列出 `summary_issues`。主体按 `bindings` 用 `a-timeline` 逐绑定分组：绑定（启用/禁用 tag + issues tag）→ 媒介（名称/类型/启用 tag + recipients tag 列表）→ 关联路由列表（名称、启用、`notify_on_firing`/`notify_on_resolved` 开关 tag、matchers、issues tag）。issues 颜色：命中"警告/未/建议/禁用"字样橙色，其余红色。

### P2 单告警链路（告警详情）

- `fronted/src/views/monitor/alerts/index.vue` 的历史/当前告警"查看日志"入口改为打开"通知链路"弹窗，调用 `getAlertNotificationChain(record.record.id / history_id)`（即告警历史 id）。
- 弹窗最终逻辑：顶部三列 descriptions 告警摘要（alertname / labels.instance / state tag）；`summary_issues` 非空时置顶黄色 `a-alert`；主体按 `routes` 每路由一张小卡片：标题行含命中结果（✅命中绿色 / ❌未命中红色 + tooltip 与正文展示 `miss_reason`）、路由启用 tag、firing/resolved 开关 tag → 媒介区块（启用 tag + 绑定用户 tag：用户名+收件人）→ 事件行（firing/resolved tag、状态 tag、尝试次数、error）→ 投递明细表（size=small、`pagination=false`、列为用户/地址/状态/错误，状态 tag：success=green、failed=red、sending=processing、pending=orange）。路由无媒介或无投递时展示空态。

## 失败语义

- 组件加载失败仅结束 loading，保留空态；不弹全局错误（个人中心诊断属辅助信息）。P2 弹窗加载失败 `message.error` 提示并保持空态。
- `can_receive` 为 `null`（接口异常/字段缺失）时不渲染成功/失败横幅，只渲染绑定列表。

## 与 Django 版对齐

前端仅对接上述 Go 契约；Django 版如实现同接口需保持字段名与嵌套结构一致（`bindings[].media/routes`、`routes[].media[].event/deliveries`），否则前端需按差异适配。

## Go 版 autoadmin 后端最终逻辑（已实现）

入口：`autoadmin/internal/monitor/alert_chain.go`，路由注册在 `autoadmin/internal/api/router/router.go` 的 `monitorRoutes` 组（`/monitor` 前缀，`Authenticate(tokens)` + `monitor:view` 权限，与相邻 alert-histories/alert-routes 路由口径一致）：

- `GET /monitor/alert-notification/user-chain/` → `Handler.UserAlertChain`
- `GET /monitor/alert-notification/chain/:historyId/` → `Handler.AlertChainEvaluation`

### P1 user-chain 数据流

1. 目标用户：query `user_id` 合法时用之（供管理员查他人）；否则取登录态 `identity.ClaimsFromContext` 的 `UserID`。随后 `SELECT username FROM sys_user WHERE id=?` 取用户名。
2. 绑定：`monitor_user_alert_media_binding JOIN monitor_alert_media`（按 user_id，`ORDER BY b.id`），recipients 为 JSON 数组。
3. 每个媒介挂载的路由：`monitor_alert_route JOIN monitor_alert_route_media`（按 alertmedia_id，`ORDER BY r.id`），matchers 原样 JSON 返回（对告警的匹配条件，用户视角不做匹配校验，issues 留空除非路由/开关本身有问题）。
4. media `config` 字段一律不返回（含 SMTP 密码等敏感信息）。

issues 判定（按序并入，可叠加）：

- 绑定禁用 → `该绑定已禁用`（summary：`绑定 {id} 已禁用`）
- 媒介禁用 → `媒介已停用`（summary：`媒介 {id} 已停用`）
- `media_type != "email"` → `非邮件媒介，暂不支持自动发送`（summary：`绑定 {id}：…`）
- 绑定 recipients 为空 → `绑定未配置收件地址`
- 路由禁用 → `路由已禁用`；`notify_on_firing=false` 且 resolved 开启 → `该路由仅通知 resolved，firing 通知未开启`；两个开关都关闭 → `该路由未开启任何事件通知`（summary：`路由 {name}：…`）
- 无任何绑定 → summary 追加 `用户 {username} 未配置任何告警媒介绑定`

`can_receive` = 存在至少一条完整通路：绑定启用 + 媒介启用 + media_type=email + recipients 非空 + 该媒介挂载的某条路由 enabled 且 notify_on_firing。`summary_issues` 汇总所有断点（去重）。

### P2 chain/:historyId 数据流

1. 取 `monitor_alert_history` 行（alertname/severity/instance/labels/state/started_at），不存在返回业务码 404。labels JSON 解析后合并便捷键：显式 labels 值优先，缺失时补 alertname/severity/instance。
2. 遍历**全部** `monitor_alert_route`（含禁用路由，便于前端展示 miss 原因），按序判定 `matched`/`miss_reason`：路由禁用 → `路由已禁用`；matchers 对合并后 labels 全量等值匹配失败 → `labels 不匹配（matchers 需要 {key}={value}）`（第一个不匹配的键）；notify 开关与告警 state 不符（firing 需 notify_on_firing，resolved 需 notify_on_resolved）→ 对应说明。三者全过才 `matched=true`。
3. 每条路由（无论是否命中）列出挂载媒介（enabled+name）、媒介上的用户绑定（JOIN sys_user 取 username、recipients、enabled）。
4. 事件：`monitor_alert_notification_event` 按 `alert_id + event_type=告警 state` 取最新一条；无记录 `event=null`。有事件时再取 `monitor_alert_notification_delivery`（JOIN sys_user）投递明细。

`summary_issues` 汇总：未命中路由（`路由 {name} 未命中：{miss_reason}`）、命中但无事件（`路由 {name}：无 {state} 事件记录，通知未触发`）、失败投递（`用户 {username} 的投递失败：{error 截断 120 字符}`，username 缺失时回退用户 id）。

失败语义：任何 SQL 错误按通用 500 处理；单条告警不存在返回业务码 404；`parseID` 对非法 `historyId` 归零走 404。与 Django 版无同接口实现，字段结构以本文档契约为准。

## 订阅范围（路线三）在链路视图中的体现

- P1 `user-chain`：每个 binding 带 `scope`（解析出节点名，`missing:true` 表示节点已删除）；相关 issues："订阅范围包含已删除的服务树节点"、"订阅范围不含任何存在的服务树节点，等同于收不到告警"。
- P2 `chain/:historyId/`：每个 binding 带 `scope`、`scoped_in`（告警归属节点是否落在订阅范围内；全局绑定恒 true）与 issue"订阅范围不含该告警的归属节点"。
