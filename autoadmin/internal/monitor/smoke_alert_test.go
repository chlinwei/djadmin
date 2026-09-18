package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// TestSmokeAlertConfigQueriesAgainstRealDatabase 把告警/通知/监控总览三域的 sqlc 语句在**真库**上跑一遍写路径。
// mock 用例全绿不等于方言可用，这里专门验只有真库能暴露的点：
//
//  1. "已入队就跳过"的两种表达：MySQL 的 `INSERT IGNORE`（影响行数 0）vs PG 的
//     `ON CONFLICT DO NOTHING`（被跳过时 RETURNING 不返回行 → sql.ErrNoRows）；
//  2. 单地址投递的 get-or-create：MySQL 的 `id=LAST_INSERT_ID(id)` vs PG 的
//     `DO UPDATE SET id=<表>.id RETURNING id` —— 两次同键调用必须返回同一个 id（写错成 EXCLUDED.id
//     会返回序列的下一个值，等于把主键改掉）；
//  3. 可空 json 列（monitor_notification_policy.media_ids：NULL = 继承，`[]` = 显式静音）；
//  4. 失联对账的阈值由应用层算；已恢复行再恢复必须受 WHERE state='firing' 守卫。
//
// 全程一个事务 + 回滚。
//
// 用法：
//
//	MONITOR_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/monitor/ -run RealDatabase -v
//	MONITOR_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/monitor/ -run RealDatabase -v
func TestSmokeAlertConfigQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("MONITOR_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MONITOR_SMOKE_DSN 未设置：跳过真库冒烟（说明见本函数注释）")
	}
	ctx := context.Background()
	pool, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	queries, now := db.New(tx), time.Now().UTC()
	suffix := now.Format("150405.000000")

	// 被监控的主机（日志采集目标与告警服务树归属都要外键指向真实主机）。
	hostResult, err := queries.CreateHost(ctx, db.CreateHostParams{
		CreateTime: now, UpdateTime: now, Status: "active", IsDeletedInCloud: false,
		InstanceName:  sql.NullString{String: "smoke-alert-log-" + suffix, Valid: true},
		Ip:            sql.NullString{String: "10.255.255.204", Valid: true},
		CollectStatus: "pending", CollectMessage: "", AgentOnline: false,
		WebsshDefaultUsername: "", WebsshLoginUsers: "",
	})
	if err != nil {
		t.Fatalf("建主机：%v", err)
	}
	hostID, err := hostResult.LastInsertId()
	if err != nil {
		t.Fatalf("主机主键：%v", err)
	}

	// ---- 告警媒介 ----
	mediaID, err := queries.CreateAlertMedia(ctx, db.CreateAlertMediaParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{String: "冒烟", Valid: true},
		Name: "smoke-media-" + suffix, MediaType: "email",
		Config: []byte(`{"smtpServer":"smtp.smoke","password":"secret"}`), Enabled: true,
	})
	if err != nil {
		t.Fatalf("建告警媒介：%v", err)
	}
	if name, err := queries.GetAlertMediaName(ctx, mediaID); err != nil || name == "" {
		t.Fatalf("读媒介名：%q err=%v", name, err)
	}
	if config, err := queries.GetAlertMediaConfig(ctx, mediaID); err != nil || len(config) == 0 {
		t.Fatalf("读媒介配置：%s err=%v", config, err)
	}
	if affected, err := queries.UpdateAlertMedia(ctx, db.UpdateAlertMediaParams{
		UpdateTime: time.Now().UTC(), Remark: sql.NullString{String: "改过", Valid: true},
		Name: "smoke-media-" + suffix, MediaType: "email",
		Config: []byte(`{"smtpServer":"smtp.smoke2"}`), Enabled: false, ID: mediaID,
	}); err != nil || affected != 1 {
		t.Fatalf("更新媒介：affected=%d err=%v", affected, err)
	}
	if _, err := queries.ListAlertMediaBriefByIDs(ctx, []int64{mediaID}); err != nil {
		t.Fatalf("按 id 列媒介简报：%v", err)
	}
	if _, err := queries.ListEnabledAlertMediaByIDs(ctx, []int64{mediaID}); err != nil {
		t.Fatalf("按 id 列启用媒介：%v", err)
	}

	// ---- 告警历史：webhook 摄取的三条语句（新告警 / 心跳 / 恢复）----
	fingerprint := "smoke-fp-" + suffix
	alertID, err := queries.CreateAlertHistory(ctx, db.CreateAlertHistoryParams{
		CreateTime: now, UpdateTime: now, Source: "prometheus", Fingerprint: fingerprint,
		Alertname: "SmokeAlert", RuleGroup: "smoke-group", RuleSnapshot: json.RawMessage(`{"name":"SmokeAlert"}`),
		Severity: "warning", Instance: "host-smoke", Labels: json.RawMessage(`{"host_id":"1"}`),
		Annotations: json.RawMessage(`{}`), GeneratorUrl: "", StartedAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("建告警历史：%v", err)
	}
	open, err := queries.GetOpenFiringAlertForUpdate(ctx, fingerprint)
	if err != nil || open.ID != alertID {
		t.Fatalf("加锁读未恢复告警：%+v err=%v", open, err)
	}
	// 已有 rule_snapshot 有内容 → 合并时保留既有值（复刻原 SQL 的 IF(JSON_LENGTH(...)=0, 新值, 旧值)）。
	// 只做语义断言：MySQL 会把 json 列规范化（`{"name":"x"}` → `{"name": "x"}`），比字面量会误报。
	mergedSnapshot := keepExistingRuleSnapshot(open.RuleSnapshot, []byte(`{"name":"newRule"}`))
	if !jsonValueHasContent(mergedSnapshot) || strings.Contains(string(mergedSnapshot), "newRule") {
		t.Fatalf("rule_snapshot 合并语义不符：%s", mergedSnapshot)
	}
	if err := queries.UpdateAlertHistoryHeartbeat(ctx, db.UpdateAlertHistoryHeartbeatParams{
		LastSeenAt: time.Now().UTC(), Labels: json.RawMessage(`{"host_id":"1"}`),
		Annotations: json.RawMessage(`{}`), UpdateTime: time.Now().UTC(),
		RuleGroup: "smoke-group", RuleSnapshot: json.RawMessage(`{"name":"SmokeAlert"}`), ID: alertID,
	}); err != nil {
		t.Fatalf("告警心跳：%v", err)
	}
	if _, err := queries.GetAlertHistoryForChain(ctx, alertID); err != nil {
		t.Fatalf("链诊断读告警：%v", err)
	}
	if _, err := queries.ListHostAlertScopeNodes(ctx, hostID); err != nil {
		// 原实现写的是 bs.project（不存在的列），真库上恒报 1054 —— 这条断言就是那次修复的回归守卫。
		t.Fatalf("告警主机服务树归属：%v", err)
	}

	// ---- 通知策略树：根节点 + 子节点（NULL / [] 两种出口语义）----
	rootID, err := queries.CreateNotificationPolicy(ctx, db.CreateNotificationPolicyParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{String: "默认策略", Valid: true},
		ParentID: sql.NullInt64{}, Name: "默认策略-" + suffix, Position: 0,
		Matchers: json.RawMessage(`[]`),
		MediaIds: sql.NullString{}, UserGroupIds: sql.NullString{}, // NULL = 继承
		NotifyOnFiring: true, NotifyOnResolved: true,
	})
	if err != nil {
		t.Fatalf("建策略根节点：%v", err)
	}
	childID, err := queries.CreateNotificationPolicy(ctx, db.CreateNotificationPolicyParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{String: "静音", Valid: true},
		ParentID: sql.NullInt64{Int64: rootID, Valid: true}, Name: "静音-" + suffix, Position: 1,
		Matchers:       json.RawMessage(`[{"type":"label","label":"severity","operator":"=","value":"critical"}]`),
		MediaIds:       sql.NullString{String: "[]", Valid: true}, // 显式静音
		UserGroupIds:   sql.NullString{String: `[]`, Valid: true},
		NotifyOnFiring: false, NotifyOnResolved: true,
	})
	if err != nil {
		t.Fatalf("建策略子节点：%v", err)
	}
	if parent, err := queries.GetNotificationPolicyParent(ctx, childID); err != nil || parent != rootID {
		t.Fatalf("读策略父节点：%d err=%v", parent, err)
	}
	nodes, err := queries.ListNotificationPolicyNodes(ctx)
	if err != nil {
		t.Fatalf("列策略节点：%v", err)
	}
	seenRoot, seenChild := false, false
	for _, node := range nodes {
		if node.ID == rootID {
			seenRoot = true
			if jsonColumnSet(node.MediaIds) {
				t.Fatalf("根节点 media_ids 应为 NULL（继承），实际 %q", node.MediaIds.String)
			}
		}
		if node.ID == childID {
			seenChild = true
			if !jsonColumnSet(node.MediaIds) {
				t.Fatalf("子节点 media_ids 应为显式 []，实际 %v", node.MediaIds)
			}
		}
	}
	if !seenRoot || !seenChild {
		t.Fatalf("策略树缺节点：root=%v child=%v", seenRoot, seenChild)
	}
	if err := queries.UpdateNotificationPolicy(ctx, db.UpdateNotificationPolicyParams{
		UpdateTime: time.Now().UTC(), Remark: sql.NullString{String: "改名", Valid: true},
		ParentID: sql.NullInt64{Int64: rootID, Valid: true}, Name: "静音2-" + suffix, Position: 2,
		Matchers: json.RawMessage(`[]`), MediaIds: sql.NullString{},
		UserGroupIds: sql.NullString{}, NotifyOnFiring: true, NotifyOnResolved: true, ID: childID,
	}); err != nil {
		t.Fatalf("更新策略：%v", err)
	}

	// ---- 通知事件：入队去重 + 派发上下文 + 状态流转 ----
	eventParams := db.CreateAlertNotificationEventIfAbsentParams{
		CreateTime: now, UpdateTime: now, EventType: "firing",
		DeduplicationKey: dedupeKey(alertID, "firing"), AlertID: alertID,
	}
	eventID, created, err := createAlertNotificationEventIfAbsent(ctx, queries, eventParams)
	if err != nil || !created || eventID <= 0 {
		t.Fatalf("首次入队事件：id=%d created=%v err=%v", eventID, created, err)
	}
	if _, created, err = createAlertNotificationEventIfAbsent(ctx, queries, eventParams); err != nil || created {
		t.Fatalf("重复入队应跳过：created=%v err=%v", created, err)
	}
	dispatch, err := queries.GetAlertNotificationEventDispatch(ctx, eventID)
	if err != nil || dispatch.Alertname != "SmokeAlert" || dispatch.Status != "pending" {
		t.Fatalf("派发上下文：%+v err=%v", dispatch, err)
	}
	if err = queries.MarkAlertNotificationEventSending(ctx, db.MarkAlertNotificationEventSendingParams{
		UpdateTime: time.Now().UTC(), ID: eventID,
	}); err != nil {
		t.Fatalf("事件置 sending：%v", err)
	}
	if err = queries.MarkAlertNotificationEventPending(ctx, db.MarkAlertNotificationEventPendingParams{
		ErrorMessage: "重试", UpdateTime: time.Now().UTC(), ID: eventID,
	}); err != nil {
		t.Fatalf("事件回 pending：%v", err)
	}
	if err = queries.MarkAlertNotificationEventSuccess(ctx, db.MarkAlertNotificationEventSuccessParams{
		SentAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, UpdateTime: time.Now().UTC(), ID: eventID,
	}); err != nil {
		t.Fatalf("事件置 success：%v", err)
	}
	if err = queries.MarkAlertNotificationEventFailed(ctx, db.MarkAlertNotificationEventFailedParams{
		ErrorMessage: "失败", UpdateTime: time.Now().UTC(), ID: eventID,
	}); err != nil {
		t.Fatalf("事件置 failed：%v", err)
	}
	if event, err := queries.GetLatestAlertNotificationEventForAlert(ctx, db.GetLatestAlertNotificationEventForAlertParams{
		AlertID: alertID, EventType: "firing",
	}); err != nil || event.ID != eventID {
		t.Fatalf("按告警取最新事件：%+v err=%v", event, err)
	}

	// ---- 单地址投递的 get-or-create：两次同键必须拿到同一个 id ----
	// 自建一个用户：唯一键 (event_id, media_id, user_id, address) 含可空列，而 SQL 的唯一约束
	// 把 NULL 视为互不相同 —— 这里给 user_id 一个真实值，才能真的走"撞唯一键"这条路径
	//（没有用户时两次调用会各插一行，那是 SQL 语义而不是 bug）。sys_user 还带 FK，所以只能用真行。
	// 走 sqlc 的 CreateUser（两条语句都是 `?`，只有 sqlc 会把 PG 侧渲染成 $n）。
	userResult, err := queries.CreateUser(ctx, db.CreateUserParams{
		Username: "smoke-user-" + suffix, Password: "x", Status: 1, Timezone: "Asia/Shanghai",
		CreateTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("建冒烟用户：%v", err)
	}
	createdUserID, err := userResult.LastInsertId()
	if err != nil {
		t.Fatalf("用户主键：%v", err)
	}
	realUserID := int32(createdUserID)
	if _, err := queries.GetUsernameByID(ctx, realUserID); err != nil {
		t.Fatalf("读用户名：%v", err)
	}
	deliveryParams := db.CreateAlertNotificationDeliveryOrGetIDParams{
		CreateTime: now, UpdateTime: now, Address: "smoke@x.com", EventID: eventID,
		MediaID: sql.NullInt64{Int64: mediaID, Valid: true},
		UserID:  sql.NullInt32{Int32: realUserID, Valid: true},
	}
	firstDeliveryID, err := queries.CreateAlertNotificationDeliveryOrGetID(ctx, deliveryParams)
	if err != nil || firstDeliveryID <= 0 {
		t.Fatalf("首次投递记录：id=%d err=%v", firstDeliveryID, err)
	}
	secondDeliveryID, err := queries.CreateAlertNotificationDeliveryOrGetID(ctx, deliveryParams)
	if err != nil || secondDeliveryID != firstDeliveryID {
		t.Fatalf("同键投递应返回既有 id：first=%d second=%d err=%v", firstDeliveryID, secondDeliveryID, err)
	}
	if status, err := queries.GetAlertNotificationDeliveryStatus(ctx, firstDeliveryID); err != nil || status != "pending" {
		t.Fatalf("投递状态：%q err=%v", status, err)
	}
	if err = queries.MarkAlertNotificationDeliverySending(ctx, db.MarkAlertNotificationDeliverySendingParams{
		UpdateTime: time.Now().UTC(), ID: firstDeliveryID,
	}); err != nil {
		t.Fatalf("投递置 sending：%v", err)
	}
	if err = queries.MarkAlertNotificationDeliveryFailed(ctx, db.MarkAlertNotificationDeliveryFailedParams{
		ErrorMessage: "smtp down", UpdateTime: time.Now().UTC(), ID: firstDeliveryID,
	}); err != nil {
		t.Fatalf("投递置 failed：%v", err)
	}
	if err = queries.MarkAlertNotificationDeliverySuccess(ctx, db.MarkAlertNotificationDeliverySuccessParams{
		SentAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, UpdateTime: time.Now().UTC(), ID: firstDeliveryID,
	}); err != nil {
		t.Fatalf("投递置 success：%v", err)
	}
	if _, err := queries.ListAlertNotificationDeliveriesForChain(ctx, eventID); err != nil {
		t.Fatalf("链诊断投递记录：%v", err)
	}
	if _, err := queries.ListAlertMediaBindingsByMedia(ctx, mediaID); err != nil {
		t.Fatalf("按媒介列绑定：%v", err)
	}
	if _, err := queries.ListAlertMediaBindingsByMediaInUserGroups(ctx, db.ListAlertMediaBindingsByMediaInUserGroupsParams{
		MediaID: mediaID, GroupIds: []int64{1, 2},
	}); err != nil {
		t.Fatalf("按用户组列绑定：%v", err)
	}
	if _, err := queries.CountUserGroupMemberships(ctx, db.CountUserGroupMembershipsParams{
		UserID: realUserID, GroupIds: []int64{1, 2},
	}); err != nil {
		t.Fatalf("组成员计数：%v", err)
	}

	// ---- 失联对账：阈值由应用层算 ----
	stale, err := queries.ListStaleFiringAlerts(ctx, time.Now().UTC().Add(reconcileStaleAfter))
	if err != nil {
		t.Fatalf("列失联告警：%v", err)
	}
	if len(stale) == 0 {
		t.Fatalf("失联判定应命中刚建的 firing 告警")
	}
	if affected, err := queries.ResolveStaleAlert(ctx, db.ResolveStaleAlertParams{
		ResolvedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, UpdateTime: time.Now().UTC(), ID: alertID,
	}); err != nil || affected != 1 {
		t.Fatalf("恢复失联告警：affected=%d err=%v", affected, err)
	}
	// 已恢复的行再恢复应为 0 行（WHERE state='firing' 守卫）。
	if affected, err := queries.ResolveStaleAlert(ctx, db.ResolveStaleAlertParams{
		ResolvedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, UpdateTime: time.Now().UTC(), ID: alertID,
	}); err != nil || affected != 0 {
		t.Fatalf("重复恢复应 0 行：affected=%d err=%v", affected, err)
	}
	if err := queries.ResolveAlertHistoryFromWebhook(ctx, db.ResolveAlertHistoryFromWebhookParams{
		ResolvedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, LastSeenAt: time.Now().UTC(),
		Annotations: json.RawMessage(`{}`), UpdateTime: time.Now().UTC(),
		RuleGroup: "smoke-group", RuleSnapshot: json.RawMessage(`{}`), ID: alertID,
	}); err != nil {
		t.Fatalf("webhook 恢复告警：%v", err)
	}

	// ---- 只读：模块总览 / 服务发现 / 机器令牌 ----
	if _, err := queries.CountMonitorTargetSummary(ctx); err != nil {
		t.Fatalf("监控目标总览：%v", err)
	}
	if _, err := queries.ListPrometheusServiceDiscoveryTargets(ctx); err != nil {
		t.Fatalf("服务发现目标：%v", err)
	}
	if _, err := queries.GetConfigValueByKey(ctx, "monitor.prometheus.base_url"); err != nil && err != sql.ErrNoRows {
		t.Fatalf("读 Prometheus 基地址：%v", err)
	}
	if _, err := queries.ListActiveAgentTokens(ctx, sql.NullTime{Time: time.Now().UTC(), Valid: true}); err != nil {
		t.Fatalf("列有效机器令牌：%v", err)
	}

	// ---- 收尾：删除路径（含外键解绑）----
	// 第一条媒介被投递记录以 FK 引用（不能直接删），另建一条验证删除路径 + "删不到行"语义。
	throwawayMediaID, err := queries.CreateAlertMedia(ctx, db.CreateAlertMediaParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{},
		Name: "smoke-media-throwaway-" + suffix, MediaType: "email",
		Config: []byte(`{}`), Enabled: true,
	})
	if err != nil {
		t.Fatalf("建一次性告警媒介：%v", err)
	}
	if err := deleteRowsAffected(queries.DeleteAlertMedia(ctx, throwawayMediaID)); err != nil {
		t.Fatalf("删告警媒介：%v", err)
	}
	if err := deleteRowsAffected(queries.DeleteAlertMedia(ctx, throwawayMediaID)); err != sql.ErrNoRows {
		t.Fatalf("重复删媒介应 ErrNoRows，实际 %v", err)
	}
	if err := queries.DeleteNotificationPolicy(ctx, childID); err != nil {
		t.Fatalf("删策略子节点：%v", err)
	}
	if err := queries.DeleteNotificationPolicy(ctx, rootID); err != nil {
		t.Fatalf("删策略根节点：%v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("回滚：%v", err)
	}
}
