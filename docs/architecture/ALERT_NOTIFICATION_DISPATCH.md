# 告警通知分发链路（Go 版 autoadmin）

描述告警通知"建事件 → 异步投递 → 失联对账"的最终逻辑（Go 实现）。Django 参考实现：`enqueue_notification`（alert_history.py）、`resolve_alert_media` / `send_alert_notification`（tasks.py）。链路诊断接口（user-chain / chain）见 `ALERT_NOTIFICATION_CHAIN.md`。

## 入口与文件

- 匹配 / 入队 / 发送 / 对账：`autoadmin/internal/monitor/alert_notification.go`
- webhook 摄取接线：`autoadmin/internal/monitor/alert_webhook.go`（`AlertWebhook`）
- 对账 ticker 启动：`autoadmin/internal/monitor/handler.go`（`NewHandler`）
- SMTP 发信能力：`autoadmin/internal/monitor/media_send.go` 的 `sendSMTPMedia`（经 `Handler.smtpSend` 注入，单测可替换）

## 分发时序（最终逻辑）

1. **摄取**：`AlertWebhook` 解析 Prometheus webhook 载荷（含规则快照解析，不变），事务内写入 `monitor_alert_history`。对新 created 的 firing 告警与 resolved 更新的告警收集"通知目标"（alert_id、alertname、severity、instance、labels、event_type）。
2. **入队（事务提交后，响应前同步执行）**：对每个目标先做"有无可投递媒介"预判（见下）；无命中不建事件（`notifications` 计数不含），有命中则 `INSERT IGNORE INTO monitor_alert_notification_event`（`deduplication_key={alert_id}:{event_type}` 唯一键幂等去重），`notifications` 只统计实际新建的事件数，webhook 响应不等待后续发送。
3. **匹配（对齐 resolve_alert_media）**：SQL 取 enabled 路由 × `monitor_alert_route_media` × enabled 媒介，并按事件类型过滤 `notify_on_firing` / `notify_on_resolved`；Go 侧对 matchers 做全量等值匹配，参与匹配的 labels 先合并 alertname/severity/instance 三个便捷键（覆盖同名 label）。命中路由的媒介去重后作为可投递集合。
4. **发送（提交事务后的 goroutine，异步）**：`dispatchAlertNotificationEvent` 循环调用 `sendAlertNotificationEvent`：
   - 事件置 `sending`、`attempt_count+1`；媒介在入队后被停用/路由变更（预判为空）→ 事件直接 `failed`（"媒介在入队后被停用或路由已变更"，不重试）。
   - 非 email 媒介跳过；查 `monitor_user_alert_media_binding`（media 匹配且 enabled=TRUE）联 `sys_user`；无绑定 → 错误"媒介 … 没有任何用户绑定"；一个地址都没投出去且无其他错误 → "匹配的告警媒介没有可投递的用户地址"。
   - 每绑定 × 每 recipient（去重保序）一条 delivery，get-or-create（唯一键 event,media,user,address），已 success 的跳过；否则置 `sending`、attempt+1，逐地址经 `sendSMTPMedia` 发信；成功 `success+sent_at`，失败 `failed+error_message`。
   - 存在可重试错误且未到次数上限（首次尝试+最多 5 次重试）时事件回 `pending`，按 10s 起步、翻倍、上限 300s 退避后重试；全部处理完事件置 `success` 或最终 `failed`（error_message 汇总各地址错误）。
5. **失联对账**：`NewHandler` 启动进程内 goroutine（首次 tick 延迟 1 分钟，间隔 5 分钟；单实例部署，进程内唯一，随进程退出终止，不做优雅停止、无分布式锁）。把 `state='firing' AND source='prometheus' AND last_seen_at < UTC_TIMESTAMP - 10 分钟` 的告警置 `resolved`（`resolved_by_reconciliation=TRUE`），对命中且开启 `notify_on_resolved` 的路由走同一 enqueue→发送链路（event_type=`resolved`）。

## 失败语义

- 入队预判查询失败：该目标不建事件（webhook 响应不受影响）。
- 发送过程中 SQL 错误：本次尝试中止（事件停留在 sending，等下次触发或对账；当前无看门狗重扫 pending/sending，见"保守处理点"）。
- SMTP 失败计入 delivery `failed` 并触发事件级退避重试；重试只重投非 success 的 delivery。

## 与 Django 的语义差异点

- **重试载体**：Django 用 Celery `self.retry`（跨进程队列）；Go 用事件 goroutine 内 sleep 退避循环。次数语义一致（首次+5 次重试），退避公式对齐 `min(300, 10*2^retries)`。
- **发送时机**：Django `transaction.on_commit` + Celery worker；Go 为提交事务后直接 `go dispatch...`，webhook 响应仍不等待。
- **labels 便捷键**：Django `labels.update(...)` 覆盖同名 label；Go 等价覆盖（注意 `alert_chain.go` 的 `chainMergeLabels` 是"缺省才补"，两处语义不同，属各自对齐目标不同）。
- **对账范围**：Django `reconcile_alert_history` 每日跑、还负责删除已下线规则的记录；Go 对账为 5 分钟 ticker，只做失联置 resolved + 通知，不删记录。
- **delivery error_message**：Django 事件级错误前缀含 `用户名/媒介/地址`；Go 保持相同格式。

## 保守处理点（有意为之）

- `monitor_alert_route_media` 的列名按 Django m2m 默认约定取 `alertroute_id` / `alertmedia_id`（该表未纳入 `db/schema` 快照）。
- 失联阈值简化为固定 10 分钟（未解析 Prometheus 规则的 for/duration）。
- 无 pending/sending 事件的进程重启后重扫（Django 由 Celery 队列兜底）；重启丢 sending 事件属可接受降级。
- 非 email 媒介在发送阶段跳过（与 Django 一致，webhook 等类型待后续实现）。

## 绑定订阅范围（路线三，迁移 000014）

- `monitor_user_alert_media_binding.scope`：JSON，`NULL` = 全局订阅（收到其媒介命中的所有告警）；`[{"type":"service|environment|business|project","id":N}]` = 仅订阅归属命中任一节点的告警（数组内 OR，≤50 条）。
- 投递筛选：告警 `labels.host_id` → `assets_application_deployment → assets_application_service_deployment → assets_application_service(业务/环境) → assets_business_system(项目)` 解析归属节点集合；scope 为空恒命中，否则取交集。无 `host_id` 或归属解析为空的告警只有全局订阅者能收到。
- 绑定读写：`updateAlertMediaBindings` 请求项带 `scope`（可空）；校验 type 枚举、id 正整数、≤50 条；列表返回原样 JSON。
