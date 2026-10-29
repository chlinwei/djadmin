# 告警架构：摄取、通知与链路诊断

本文是告警域的唯一架构主文档。按链路阅读：历史摄取 → [通知策略与分发](#notification-dispatch)
→ [链路诊断](#notification-chain)。部署与邮箱验证见 [告警媒介操作手册](../ops/ALERT_MEDIA_SETUP_GUIDE.md)。

描述 Prometheus 告警经 webhook 落库（`monitor_alert_history`）的最终逻辑。后端唯一实现：`autoadmin/internal/monitor/alert_webhook.go`。Agent 来源的巡检告警不入此链路（其规则快照在巡检侧生成）。

## 摄取流程（`AlertWebhook`）

1. Prometheus notifier 以 Alertmanager v2 数组推送；按 `labels` 的 SHA1 指纹幂等 upsert：
   - `endsAt` 非零且已过 → resolved：更新对应 firing 记录；本地无 firing 记录的孤立 resolved 直接丢弃。
   - 否则 firing：已有未恢复记录只刷新心跳（last_seen/labels/annotations），没有则插入新行。
2. **规则快照补全**（与 Django 版 `ingest_alert_webhook_alerts` 对齐）：webhook 载荷不含规则定义，摄取前查一次 Prometheus `/api/v1/rules` 建双索引：
   - `by_fingerprint`：遍历各规则当前 active alerts 的 labels → 规则组名（精确归属）；
   - `by_alertname`：规则名 → `{group_name, name, query, duration, labels, annotations}`（含 PromQL）。
   - 每条告警的 `rule_group` 优先指纹精确匹配、回退 alertname；`rule_snapshot` 存规则定义 JSON。
   - rules 接口失败/超时返回空索引，**不阻塞告警入库**（此时快照为 `{}`）。
3. 心跳与 resolved 更新时，若存量记录的 `rule_group` 为空或 `rule_snapshot` 为空对象则一并回填——存量告警在下一次心跳时自动补全表达式。

## 展示

前端历史告警列表 `rule_details.query` 即规则快照中的 PromQL；`monitor_alert_history.rule_snapshot` 为 JSON 列。

**「问题」列两处同源**（2026-09-20 修）：列表里的"问题"文案取告警自身的 annotation ——
`summary`，没写就退回 `description`（`alertSummaryText`，`internal/monitor/handler.go`）。
- 当前告警：Prometheus `/api/v1/alerts` 的 `annotations` 是平铺 map，handler 现场算；
- 历史告警：读落库的 `monitor_alert_history.annotations`（同一份东西的 JSON 快照），
  DTO 里由**同一个函数**算出 `summary` 字段（`history_typed.go` 的 `alertHistoryResponseFrom`）。

历史行原先没有 `summary` 字段、列表也就没有「问题」列，现场表现为"当前告警有这一栏、历史告警没有，
同一个告警在两处长得不一样"。旧数据没写注释时该列显示"-"（解析失败同样降级成空，不把坏 JSON 当文案）。

**当前告警**（`GET /monitor/targets/prometheus/alerts/`）同样带 `rule_group` / `rule_details`：`/api/v1/alerts`
本身不含规则表达式（PromQL 只在 `/api/v1/rules`），所以 handler 复用同一套 `prometheusAlertRuleIndexes`
按 alertname 关联补全；否则"当前告警"的规则组列与展开行 PromQL 恒为空（历史告警读落库快照，不受影响）。

## 列表过滤（历史告警）

`GET /monitor/alert-histories/` 支持在既有 `state/severity/keyword/start_time/end_time` 之外按 label 精确过滤：

- `label_key` + `label_value`：`JSON_UNQUOTE(JSON_EXTRACT(labels, '$.{key}')) = {value}` 等值匹配；label 缺失或值不等即排除。`label_value` 为空时不过滤（WHERE 短路，`label_key` 兜底为 `alertname` 安全 path）。
- `label_key` 服务端清洗为 `[A-Za-z0-9_]`，防 JSON path 注入。
- 过滤在 SQL 层执行（Count + List 同条件），分页计数正确；当前告警（Prometheus 实时数据）由前端按 `row.labels` 同语义过滤。

## 双实现对齐

与 Django 版语义一致（索引构建、指纹优先、空快照回填）。差异：Django 为先读后写；Go 版原先用单条
UPDATE + `IF()`（`IF(rule_group='',?,rule_group)`、`IF(IFNULL(JSON_LENGTH(rule_snapshot),0)=0,?,rule_snapshot)`）
在库里原子回填，2026-09-16 随 SQL 迁移改成**同一事务内 `FOR UPDATE` 加锁读回 + 应用层合并**
（`keepExistingRuleSnapshot`，MySQL 的 `IF`/`JSON_LENGTH` 是方言函数；语义等价：NULL/无效 JSON/空对象/空数组
都算"没内容"）。


<a id="notification-dispatch"></a>

## 通知策略与分发

描述告警通知"策略路由 → 建事件 → 异步投递 → 失联对账"的最终逻辑（Go 实现）。范围路由由管理员维护的**通知策略树**（对齐 Grafana notification policy）决定，用户绑定只是收件配置。链路诊断接口见 [通知链路诊断](#notification-chain)。

### 入口与文件

- 匹配 / 入队 / 发送 / 对账：`autoadmin/internal/monitor/alert_notification.go`
- 策略树模型 / 管理 API / 匹配路由：`autoadmin/internal/monitor/notification_policy.go`
- webhook 摄取接线：`autoadmin/internal/monitor/alert_webhook.go`（`AlertWebhook`）
- 对账 ticker 启动：`autoadmin/internal/monitor/handler.go`（`NewHandler`）
- SMTP 发信能力：`autoadmin/internal/monitor/media_send.go` 的 `sendSMTPMedia`（经 `Handler.smtpSend` 注入，单测可替换）

### 通知策略树（最终语义）

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

### 分发时序（最终逻辑）

事件与投递记录的状态写入由 sqlc 的 `MarkAlertNotificationEvent*` /
`MarkAlertNotificationDelivery*` 查询定义，Go 层调用对应状态操作并判断已成功记录以避免重复投递。

1. **摄取**：`AlertWebhook` 解析 Prometheus webhook 载荷（含规则快照解析），事务内写入 `monitor_alert_history`。对新 created 的 firing 告警与 resolved 更新的告警收集"通知目标"（alert_id、alertname、severity、instance、labels、event_type）。
2. **入队（事务提交后，响应前同步执行）**：对每个目标先做"有无可投递媒介"预判（策略树路由 + 事件开关 + 出口为空则不建事件）；有命中则建 `monitor_alert_notification_event`（`deduplication_key={alert_id}:{event_type}` 唯一键幂等去重，"已入队"即影响行数 0 / 不返回行），**事件 id 由插入语句直接取回**（MySQL 的 `LastInsertId` / PG 的 `RETURNING`，不再"插完再按去重键查一次"），webhook 响应不等待后续发送。
3. **匹配（`matchedPolicyMedias`）**：加载策略树 → 合并 alertname/severity/instance 便捷键进 labels → `alertScopeNodes` 解析告警主机的服务树归属节点集合 → 策略树路由 → 事件开关判断 → 出口媒介按 `enabled=TRUE AND id IN (...)` 过滤，同时返回生效的接收组限制（nil=不限组）。
4. **发送（提交事务后的 goroutine，异步）**：`dispatchAlertNotificationEvent` 循环调用 `sendAlertNotificationEvent`：
   - 事件置 `sending`、`attempt_count+1`；媒介在入队后被停用/策略树变更（预判为空）→ 事件直接 `failed`（"媒介在入队后被停用或策略树已变更"，不重试）。
   - 非 email 媒介跳过；查 `monitor_user_alert_media_binding`（media 匹配且 enabled=TRUE）联 `sys_user`，接收组限制生效时追加 `user_id IN (SELECT user_id FROM sys_user_group_member WHERE group_id IN (...))` 过滤；无绑定 → 错误"媒介 … 没有任何用户绑定"；一个地址都没投出去且无其他错误 → "匹配的告警媒介没有可投递的用户地址"。
   - 每绑定 × 每 recipient（去重保序）一条 delivery，get-or-create（唯一键 event,media,user,address），已 success 的跳过（"同键第二次调用返回同一 id"由真库冒烟守着；MySQL 靠 `id=LAST_INSERT_ID(id)`、PG 靠 `DO UPDATE SET id=<表>.id RETURNING id`）；否则置 `sending`、attempt+1，逐地址经 `sendSMTPMedia` 发信；成功 `success+sent_at`，失败 `failed+error_message`。
   - 存在可重试错误且未到次数上限（首次尝试+最多 5 次重试）时事件回 `pending`，按 10s 起步、翻倍、上限 300s 退避后重试；全部处理完事件置 `success` 或最终 `failed`（error_message 汇总各地址错误）。
5. **失联对账**：`NewHandler` 启动进程内 goroutine（首次 tick 延迟 1 分钟，间隔 5 分钟；单实例部署，进程内唯一，随进程退出终止）。取 `state='firing' AND source='prometheus' AND last_seen_at < 现在-10 分钟` 的告警（**阈值由应用层算**，原实现是 SQL 里的 `UTC_TIMESTAMP(6) - INTERVAL ? MINUTE`，判定跟着库时钟走），置 `resolved`（`resolved_by_reconciliation=TRUE`），走同一 enqueue→发送链路（event_type=`resolved`）。**只有真的把 state 从 firing 翻成 resolved（影响行数 1）才入队通知**：别的路径（webhook / 人工）已恢复过的行跳过 —— 原实现无论如何都入队，靠 `deduplication_key` 兜住重复。

### 用户绑定（最终语义）

`monitor_user_alert_media_binding`：`(user, media)` 唯一，字段 `recipients JSON`（收件邮箱）、`enabled`。读写走 `/sys/usercenter/alertMediaBindings/` 与 `updateAlertMediaBindings/`（整表替换语义）。范围路由完全由策略树决定，绑定不再有 `scope` 列（迁移 000015 已删除）。

### 失败语义

- 入队预判（策略树加载或媒介查询）失败：该目标不建事件（webhook 响应不受影响）。
- 发送过程中 SQL 错误：本次尝试中止（事件停留在 sending，等下次触发或对账；当前无看门狗重扫 pending/sending，见"保守处理点"）。
- SMTP 失败计入 delivery `failed` 并触发事件级退避重试；重试只重投非 success 的 delivery。
- 策略节点 matchers JSON 损坏（理论不会出现，写入前强校验）：按无条件命中处理，不因数据问题静默吞掉通知。

### 保守处理点（有意为之）

- 失联阈值简化为固定 10 分钟（未解析 Prometheus 规则的 for/duration）。
- 无 pending/sending 事件的进程重启后重扫；重启丢 sending 事件属可接受降级。
- 非 email 媒介在发送阶段跳过（webhook 等类型待后续实现）。
- 路由每层只取第一条命中的子策略（Grafana 默认行为），未实现 `continue matching siblings`。
- 策略树每次派发全量加载（表很小、告警频率低，不做缓存）。


<a id="notification-chain"></a>

## 通知链路诊断

描述告警通知链路诊断的前端最终逻辑：用户视角（P1）与单告警链路（P2）。范围路由统一由 [通知策略与分发](#notification-dispatch) 决定，链路视图展示"策略树评估 → 出口媒介 → 用户绑定 → 事件/投递"。后端契约：`GET monitor/alert-notification/user-chain/` 与 `GET monitor/alert-notification/chain/:historyId/`。

### 数据流与入口

- API 封装：`fronted/src/api/monitor.js` 的 `getUserNotificationChain(userId?)`（userId 为空时不传 `user_id`，后端按当前登录用户处理）和 `getAlertNotificationChain(historyId)`。
- 公共展示组件：`fronted/src/views/monitor/alerts/UserNotificationChain.vue`，props `userId` 可空。

#### P1 用户视角（个人中心 + 用户管理）

- 个人中心 `fronted/src/views/userCenter/index.vue` 的"告警媒介"tab 内、绑定表格下方渲染"通知链路诊断"卡片，不传 `userId`（查自己）。
- 用户管理 `fronted/src/views/sys/user/index.vue` 操作列的"通知链路"按钮，点击弹窗内以 `record.id` 作为 `userId` 渲染同一组件。
- 组件最终逻辑：顶部 `a-alert` 横幅——`can_receive=true` 绿色"你当前会收到告警通知"；`false` 红色"你当前不会收到告警通知"并逐条列出 `summary_issues`。主体按 `bindings` 用 `a-timeline` 逐绑定分组：绑定（启用/禁用 tag + issues tag）→ 媒介（名称/类型/启用 tag + recipients tag 列表）→ 命中该媒介出口的策略列表（名称、`path`（`根 / 子 / 孙`）、firing/resolved 开关 tag、matchers）。issues 颜色：命中"警告/未/建议/禁用"字样橙色，其余红色。

#### P2 单告警链路（告警详情）

- `fronted/src/views/monitor/alerts/index.vue` 的历史/当前告警"查看日志"入口打开"通知链路"弹窗，调用 `getAlertNotificationChain(record.record.id / history_id)`（即告警历史 id）。
- 弹窗最终逻辑：
  - 顶部三列 descriptions 告警摘要（alertname / labels.instance / state tag）；`summary_issues` 非空时置顶黄色 `a-alert`。
  - 策略树卡片：标题行为命中路径 `matched_path`（如 `默认策略 / 生产`）+ `final_policy.event_allowed` tag + firing/resolved 开关 tag；正文 `levels` 逐层展示同层兄弟策略评估（`selected` 绿色 ✅、`matched` 未选中蓝色、未命中灰色 + tooltip `miss_reason`）。
  - 出口媒介（`medias`）：每媒介一张卡片（启用 tag + 绑定用户 tag：用户名+收件人）→ 事件行（firing/resolved tag、状态 tag、尝试次数、error）→ 投递明细表（列为用户/地址/状态/错误，状态 tag：success=green、failed=red、sending=processing、pending=orange）。
  - 出口为空（静音）时展示空态。

### 失败语义

- 组件加载失败仅结束 loading，保留空态；不弹全局错误（个人中心诊断属辅助信息）。P2 弹窗加载失败 `message.error` 提示并保持空态。
- `can_receive` 为 `null`（接口异常/字段缺失）时不渲染成功/失败横幅，只渲染绑定列表。

### Go 版 autoadmin 后端最终逻辑（已实现）

入口：`autoadmin/internal/monitor/alert_chain.go`，路由注册在 `autoadmin/internal/api/router/router.go` 的 `monitorRoutes` 组（`/monitor` 前缀，`Authenticate(tokens)` + `monitor:view` 权限）：

- `GET /monitor/alert-notification/user-chain/` → `Handler.UserAlertChain`
- `GET /monitor/alert-notification/chain/:historyId/` → `Handler.AlertChainEvaluation`

#### P1 user-chain 数据流

1. 目标用户：query `user_id` 合法时用之（供管理员查他人）；否则取登录态 `identity.ClaimsFromContext` 的 `UserID`。随后取用户名（`GetUsernameByID`）。
2. 绑定：`monitor_user_alert_media_binding JOIN monitor_alert_media`（按 user_id，`ORDER BY b.id`），recipients 为 JSON 数组；**不再返回 scope/routes**。
3. 每绑定的 `policies`：加载策略树后整树遍历，凡（继承后）出口包含该媒介 id 的策略均返回 `{id,name,path,matchers,user_group_ids,user_group_names,user_in_group,notify_on_firing,notify_on_resolved}`（`path` 为 `根 / 子 / 孙` 名称链；`user_group_ids=null` 表示不限组，`user_in_group` 在组限制生效时判定当前用户是否在组内，未限制恒 true）。media `config` 一律不返回（含 SMTP 密码等敏感信息）。

issues 判定（按序并入，可叠加）：

- 绑定禁用 → `该绑定已禁用`；媒介禁用 → `媒介已停用`；`media_type != "email"` → `非邮件媒介，暂不支持自动发送`；recipients 为空 → `绑定未配置收件地址`（均以 `绑定 {id}：…` 进 summary）
- 媒介未被任何策略出口命中 → summary：`媒介 {name} 未被任何通知策略出口命中`
- 命中策略仅通知 resolved → summary：`策略 {name} 仅通知 resolved，firing 通知未开启`
- 无任何绑定 → summary：`用户 {username} 未配置任何告警媒介绑定`；有不完整通路时再追加总断点说明

- 接收组限制：若覆盖该媒介的全部策略都将当前用户排除在组外（`user_group_ids` 非空且 `user_in_group=false`），绑定 issues 追加 `当前用户不在该策略的接收组内，收不到对应告警`。

`can_receive` = 存在至少一条完整通路：绑定启用 + 媒介启用 + media_type=email + recipients 非空 + 某命中策略 `notify_on_firing=true` 且（未限制组或用户在组内）。`summary_issues` 汇总所有断点（去重）。

#### P2 chain/:historyId 数据流

1. 取 `monitor_alert_history` 行（alertname/severity/instance/labels/state/started_at），不存在返回业务码 404。labels JSON 解析后合并便捷键：显式 labels 值优先，缺失时补 alertname/severity/instance；再全值转字符串供 matcher 匹配。
2. 加载策略树 + `alertScopeNodes`（告警 `labels.host_id` → 服务树归属节点集合，语义见 [通知策略与分发](#notification-dispatch)），沿命中路径**逐层评估全部兄弟策略**：`levels[i][j] = {id,name,matchers,matched,miss_reason,selected}`，`selected` 为该层按 `position,id` 序第一条命中（与分发侧 `resolvePolicyRoute` 同序同语义）；未命中兄弟进 summary（`策略 {name} 未命中：{miss_reason}`）。
3. `policy_tree`：`{matched_path_ids, matched_path, levels, final_policy}`。`final_policy` 为最深命中节点：`{matchers, media_ids（null=继承）, media_inherited, effective_media_ids, user_group_ids, user_group_names, user_groups_limited, notify_on_firing, notify_on_resolved, event_allowed}`。
4. `medias`：`final_policy.effective_media_ids` 对应媒介（含停用媒介，便于展示断点；字段 id/name/enabled）——每媒介带用户绑定（username、recipients、enabled，接收组限制生效时附 `in_group` 与 `issue`"不在命中策略的接收组内，不会收到该告警"）、事件（按 `alert_id + event_type=告警 state` 最新一条；无记录 `event=null`）与投递明细（delivery JOIN sys_user）。


`summary_issues` 汇总：未命中兄弟策略、`event_allowed=false`（`策略 {name} 未开启 {state} 通知`）、出口为空（`策略 {name} 的出口为空（静音），不会投递`）、无事件记录（`媒介 {name}：无 {state} 事件记录，通知未触发`）、失败投递（`用户 {username} 的投递失败：{error 截断 120 字符}`）。

失败语义：任何 SQL 错误按通用 500 处理；单条告警不存在返回业务码 404；策略树缺根节点按 500 处理（迁移保证根存在）。
