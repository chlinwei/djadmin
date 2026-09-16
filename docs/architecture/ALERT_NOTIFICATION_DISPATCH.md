# 告警通知分发链路（Go 版 autoadmin）

描述告警通知"策略路由 → 建事件 → 异步投递 → 失联对账"的最终逻辑（Go 实现）。范围路由由管理员维护的**通知策略树**（对齐 Grafana notification policy）决定，用户绑定只是收件配置。链路诊断接口（user-chain / chain）见 `ALERT_NOTIFICATION_CHAIN.md`。

## 入口与文件

- 匹配 / 入队 / 发送 / 对账：`autoadmin/internal/monitor/alert_notification.go`
- 策略树模型 / 管理 API / 匹配路由：`autoadmin/internal/monitor/notification_policy.go`
- webhook 摄取接线：`autoadmin/internal/monitor/alert_webhook.go`（`AlertWebhook`）
- 对账 ticker 启动：`autoadmin/internal/monitor/handler.go`（`NewHandler`）
- SMTP 发信能力：`autoadmin/internal/monitor/media_send.go` 的 `sendSMTPMedia`（经 `Handler.smtpSend` 注入，单测可替换）

## 通知策略树（最终语义）

表 `monitor_notification_policy`（迁移 000015，取代旧 `monitor_alert_route` / `monitor_alert_route_media`）：

- 根节点 `parent_id IS NULL`，名称「默认策略」，`matchers=[]` 恒命中；系统内置，不可删除、不可变更父节点。
- **匹配条件（matchers JSON，节点内 AND）**：
  - `{"type":"label","label":..,"operator":..,"value":..}`：operator 为 `=` / `!=` / `=~` / `!~`；缺失标签按空串参与匹配（Prometheus 语义，`!=` / `!~` 可命中无该标签的告警）；正则写入前校验。
  - `{"type":"tree","node_type":..,"id":N}`：`node_type ∈ service|environment|business|project`；命中告警主机的服务树归属节点集合即命中（选业务节点等效覆盖其子树）。
    **归属查询（`ListHostAlertScopeNodes`）注意**：原实现的 SQL 里写的是 `bs.project`，而 `assets_business_system` 只有 `project_id` —— 语句在真库上恒报 1054（Unknown column），错误被 `alertScopeNodes` 的 `return nodes` 吞掉，于是**归属集合恒为空、tree matcher 永远匹配不上**（只有不带 tree matcher 的策略能接住所有告警）。2026-09-16 随 SQL 迁移修掉（改取 `bs.project_id`），真库冒烟里有断言盯着这条查询能跑通。
- **出口（media_ids JSON）**：`NULL` = 继承父节点出口；`[]` = 显式静音（命中也不投递）。
- **接收组（user_group_ids JSON，迁移 000017）**：`NULL` = 继承父节点的组限制；`[]` = 显式不限组（出口媒介上的全部绑定可收）；`[id...]` = 仅这些 `sys_user_group` 组成员的绑定可收。沿路径取最后一个显式设置；全路径未显式设置（含根）= 不限组。成员在「系统管理 > 用户组」维护，与角色（权限）无关。
- **事件开关**：`notify_on_firing` / `notify_on_resolved`，由最深命中节点决定。
- **路由规则**：从根向下，每层按 `position,id` 顺序取第一条命中的子策略继续下钻；最深命中节点的（继承后的）出口为可投递媒介。无 `host_id` 或归属解析为空的告警：归属节点集合为空，只有不带 tree matcher 的策略能接住。
- 管理 API（`/monitor` 分组，`monitor:view` 权限）：`GET notification-policies/`、`POST notification-policies/create/`、`POST notification-policies/update/`（改父节点做防环校验）、`POST notification-policies/batch-delete/`（根不可删，子树随 FK CASCADE 删除）。

## 分发时序（最终逻辑）

1. **摄取**：`AlertWebhook` 解析 Prometheus webhook 载荷（含规则快照解析），事务内写入 `monitor_alert_history`。对新 created 的 firing 告警与 resolved 更新的告警收集"通知目标"（alert_id、alertname、severity、instance、labels、event_type）。
2. **入队（事务提交后，响应前同步执行）**：对每个目标先做"有无可投递媒介"预判（策略树路由 + 事件开关 + 出口为空则不建事件）；有命中则建 `monitor_alert_notification_event`（`deduplication_key={alert_id}:{event_type}` 唯一键幂等去重，"已入队"即影响行数 0 / 不返回行），**事件 id 由插入语句直接取回**（MySQL 的 `LastInsertId` / PG 的 `RETURNING`，不再"插完再按去重键查一次"），webhook 响应不等待后续发送。
3. **匹配（`matchedPolicyMedias`）**：加载策略树 → 合并 alertname/severity/instance 便捷键进 labels → `alertScopeNodes` 解析告警主机的服务树归属节点集合 → 策略树路由 → 事件开关判断 → 出口媒介按 `enabled=TRUE AND id IN (...)` 过滤，同时返回生效的接收组限制（nil=不限组）。
4. **发送（提交事务后的 goroutine，异步）**：`dispatchAlertNotificationEvent` 循环调用 `sendAlertNotificationEvent`：
   - 事件置 `sending`、`attempt_count+1`；媒介在入队后被停用/策略树变更（预判为空）→ 事件直接 `failed`（"媒介在入队后被停用或策略树已变更"，不重试）。
   - 非 email 媒介跳过；查 `monitor_user_alert_media_binding`（media 匹配且 enabled=TRUE）联 `sys_user`，接收组限制生效时追加 `user_id IN (SELECT user_id FROM sys_user_group_member WHERE group_id IN (...))` 过滤；无绑定 → 错误"媒介 … 没有任何用户绑定"；一个地址都没投出去且无其他错误 → "匹配的告警媒介没有可投递的用户地址"。
   - 每绑定 × 每 recipient（去重保序）一条 delivery，get-or-create（唯一键 event,media,user,address），已 success 的跳过（"同键第二次调用返回同一 id"由真库冒烟守着；MySQL 靠 `id=LAST_INSERT_ID(id)`、PG 靠 `DO UPDATE SET id=<表>.id RETURNING id`）；否则置 `sending`、attempt+1，逐地址经 `sendSMTPMedia` 发信；成功 `success+sent_at`，失败 `failed+error_message`。
   - 存在可重试错误且未到次数上限（首次尝试+最多 5 次重试）时事件回 `pending`，按 10s 起步、翻倍、上限 300s 退避后重试；全部处理完事件置 `success` 或最终 `failed`（error_message 汇总各地址错误）。
5. **失联对账**：`NewHandler` 启动进程内 goroutine（首次 tick 延迟 1 分钟，间隔 5 分钟；单实例部署，进程内唯一，随进程退出终止）。取 `state='firing' AND source='prometheus' AND last_seen_at < 现在-10 分钟` 的告警（**阈值由应用层算**，原实现是 SQL 里的 `UTC_TIMESTAMP(6) - INTERVAL ? MINUTE`，判定跟着库时钟走），置 `resolved`（`resolved_by_reconciliation=TRUE`），走同一 enqueue→发送链路（event_type=`resolved`）。**只有真的把 state 从 firing 翻成 resolved（影响行数 1）才入队通知**：别的路径（webhook / 人工）已恢复过的行跳过 —— 原实现无论如何都入队，靠 `deduplication_key` 兜住重复。

## 用户绑定（最终语义）

`monitor_user_alert_media_binding`：`(user, media)` 唯一，字段 `recipients JSON`（收件邮箱）、`enabled`。读写走 `/sys/usercenter/alertMediaBindings/` 与 `updateAlertMediaBindings/`（整表替换语义）。范围路由完全由策略树决定，绑定不再有 `scope` 列（迁移 000015 已删除）。

## 失败语义

- 入队预判（策略树加载或媒介查询）失败：该目标不建事件（webhook 响应不受影响）。
- 发送过程中 SQL 错误：本次尝试中止（事件停留在 sending，等下次触发或对账；当前无看门狗重扫 pending/sending，见"保守处理点"）。
- SMTP 失败计入 delivery `failed` 并触发事件级退避重试；重试只重投非 success 的 delivery。
- 策略节点 matchers JSON 损坏（理论不会出现，写入前强校验）：按无条件命中处理，不因数据问题静默吞掉通知。

## 保守处理点（有意为之）

- 失联阈值简化为固定 10 分钟（未解析 Prometheus 规则的 for/duration）。
- 无 pending/sending 事件的进程重启后重扫；重启丢 sending 事件属可接受降级。
- 非 email 媒介在发送阶段跳过（webhook 等类型待后续实现）。
- 路由每层只取第一条命中的子策略（Grafana 默认行为），未实现 `continue matching siblings`。
- 策略树每次派发全量加载（表很小、告警频率低，不做缓存）。
