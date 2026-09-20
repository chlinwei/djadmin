# 告警历史（Prometheus webhook 摄取）

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
