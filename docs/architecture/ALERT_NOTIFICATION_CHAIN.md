# 告警通知链路视图（前端）

描述告警通知链路诊断的前端最终逻辑：用户视角（P1）与单告警链路（P2）。范围路由统一由**通知策略树**（`ALERT_NOTIFICATION_DISPATCH.md`）决定，链路视图展示"策略树评估 → 出口媒介 → 用户绑定 → 事件/投递"。后端契约：`GET monitor/alert-notification/user-chain/` 与 `GET monitor/alert-notification/chain/:historyId/`。

## 数据流与入口

- API 封装：`fronted/src/api/monitor.js` 的 `getUserNotificationChain(userId?)`（userId 为空时不传 `user_id`，后端按当前登录用户处理）和 `getAlertNotificationChain(historyId)`。
- 公共展示组件：`fronted/src/views/monitor/alerts/UserNotificationChain.vue`，props `userId` 可空。

### P1 用户视角（个人中心 + 用户管理）

- 个人中心 `fronted/src/views/userCenter/index.vue` 的"告警媒介"tab 内、绑定表格下方渲染"通知链路诊断"卡片，不传 `userId`（查自己）。
- 用户管理 `fronted/src/views/sys/user/index.vue` 操作列的"通知链路"按钮，点击弹窗内以 `record.id` 作为 `userId` 渲染同一组件。
- 组件最终逻辑：顶部 `a-alert` 横幅——`can_receive=true` 绿色"你当前会收到告警通知"；`false` 红色"你当前不会收到告警通知"并逐条列出 `summary_issues`。主体按 `bindings` 用 `a-timeline` 逐绑定分组：绑定（启用/禁用 tag + issues tag）→ 媒介（名称/类型/启用 tag + recipients tag 列表）→ 命中该媒介出口的策略列表（名称、`path`（`根 / 子 / 孙`）、firing/resolved 开关 tag、matchers）。issues 颜色：命中"警告/未/建议/禁用"字样橙色，其余红色。

### P2 单告警链路（告警详情）

- `fronted/src/views/monitor/alerts/index.vue` 的历史/当前告警"查看日志"入口打开"通知链路"弹窗，调用 `getAlertNotificationChain(record.record.id / history_id)`（即告警历史 id）。
- 弹窗最终逻辑：
  - 顶部三列 descriptions 告警摘要（alertname / labels.instance / state tag）；`summary_issues` 非空时置顶黄色 `a-alert`。
  - 策略树卡片：标题行为命中路径 `matched_path`（如 `默认策略 / 生产`）+ `final_policy.event_allowed` tag + firing/resolved 开关 tag；正文 `levels` 逐层展示同层兄弟策略评估（`selected` 绿色 ✅、`matched` 未选中蓝色、未命中灰色 + tooltip `miss_reason`）。
  - 出口媒介（`medias`）：每媒介一张卡片（启用 tag + 绑定用户 tag：用户名+收件人）→ 事件行（firing/resolved tag、状态 tag、尝试次数、error）→ 投递明细表（列为用户/地址/状态/错误，状态 tag：success=green、failed=red、sending=processing、pending=orange）。
  - 出口为空（静音）时展示空态。

## 失败语义

- 组件加载失败仅结束 loading，保留空态；不弹全局错误（个人中心诊断属辅助信息）。P2 弹窗加载失败 `message.error` 提示并保持空态。
- `can_receive` 为 `null`（接口异常/字段缺失）时不渲染成功/失败横幅，只渲染绑定列表。

## Go 版 autoadmin 后端最终逻辑（已实现）

入口：`autoadmin/internal/monitor/alert_chain.go`，路由注册在 `autoadmin/internal/api/router/router.go` 的 `monitorRoutes` 组（`/monitor` 前缀，`Authenticate(tokens)` + `monitor:view` 权限）：

- `GET /monitor/alert-notification/user-chain/` → `Handler.UserAlertChain`
- `GET /monitor/alert-notification/chain/:historyId/` → `Handler.AlertChainEvaluation`

### P1 user-chain 数据流

1. 目标用户：query `user_id` 合法时用之（供管理员查他人）；否则取登录态 `identity.ClaimsFromContext` 的 `UserID`。随后 `SELECT username FROM sys_user WHERE id=?` 取用户名。
2. 绑定：`monitor_user_alert_media_binding JOIN monitor_alert_media`（按 user_id，`ORDER BY b.id`），recipients 为 JSON 数组；**不再返回 scope/routes**。
3. 每绑定的 `policies`：加载策略树后整树遍历，凡（继承后）出口包含该媒介 id 的策略均返回 `{id,name,path,matchers,user_group_ids,user_group_names,user_in_group,notify_on_firing,notify_on_resolved}`（`path` 为 `根 / 子 / 孙` 名称链；`user_group_ids=null` 表示不限组，`user_in_group` 在组限制生效时判定当前用户是否在组内，未限制恒 true）。media `config` 一律不返回（含 SMTP 密码等敏感信息）。

issues 判定（按序并入，可叠加）：

- 绑定禁用 → `该绑定已禁用`；媒介禁用 → `媒介已停用`；`media_type != "email"` → `非邮件媒介，暂不支持自动发送`；recipients 为空 → `绑定未配置收件地址`（均以 `绑定 {id}：…` 进 summary）
- 媒介未被任何策略出口命中 → summary：`媒介 {name} 未被任何通知策略出口命中`
- 命中策略仅通知 resolved → summary：`策略 {name} 仅通知 resolved，firing 通知未开启`
- 无任何绑定 → summary：`用户 {username} 未配置任何告警媒介绑定`；有不完整通路时再追加总断点说明

- 接收组限制：若覆盖该媒介的全部策略都将当前用户排除在组外（`user_group_ids` 非空且 `user_in_group=false`），绑定 issues 追加 `当前用户不在该策略的接收组内，收不到对应告警`。

`can_receive` = 存在至少一条完整通路：绑定启用 + 媒介启用 + media_type=email + recipients 非空 + 某命中策略 `notify_on_firing=true` 且（未限制组或用户在组内）。`summary_issues` 汇总所有断点（去重）。

### P2 chain/:historyId 数据流

1. 取 `monitor_alert_history` 行（alertname/severity/instance/labels/state/started_at），不存在返回业务码 404。labels JSON 解析后合并便捷键：显式 labels 值优先，缺失时补 alertname/severity/instance；再全值转字符串供 matcher 匹配。
2. 加载策略树 + `alertScopeNodes`（告警 `labels.host_id` → 服务树归属节点集合），沿命中路径**逐层评估全部兄弟策略**：`levels[i][j] = {id,name,matchers,matched,miss_reason,selected}`，`selected` 为该层按 `position,id` 序第一条命中（与分发侧 `resolvePolicyRoute` 同序同语义）；未命中兄弟进 summary（`策略 {name} 未命中：{miss_reason}`）。
3. `policy_tree`：`{matched_path_ids, matched_path, levels, final_policy}`。`final_policy` 为最深命中节点：`{matchers, media_ids（null=继承）, media_inherited, effective_media_ids, user_group_ids, user_group_names, user_groups_limited, notify_on_firing, notify_on_resolved, event_allowed}`。
4. `medias`：`final_policy.effective_media_ids` 对应媒介（含停用媒介，便于展示断点；字段 id/name/enabled）——每媒介带用户绑定（username、recipients、enabled，接收组限制生效时附 `in_group` 与 `issue`"不在命中策略的接收组内，不会收到该告警"）、事件（按 `alert_id + event_type=告警 state` 最新一条；无记录 `event=null`）与投递明细（delivery JOIN sys_user）。


`summary_issues` 汇总：未命中兄弟策略、`event_allowed=false`（`策略 {name} 未开启 {state} 通知`）、出口为空（`策略 {name} 的出口为空（静音），不会投递`）、无事件记录（`媒介 {name}：无 {state} 事件记录，通知未触发`）、失败投递（`用户 {username} 的投递失败：{error 截断 120 字符}`）。

失败语义：任何 SQL 错误按通用 500 处理；单条告警不存在返回业务码 404；策略树缺根节点按 500 处理（迁移保证根存在）。
