package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 通知链路诊断视图（策略树版）：
// - P1 user-chain：用户绑定（收件配置）× 命中该媒介出口的策略树路径；
// - P2 chain/:historyId：沿策略树逐层评估 matcher，展示最深命中策略的出口媒介、
//   用户绑定与实际 event/delivery 记录。
// 范围路由语义与分发侧（alert_notification.go + notification_policy.go）共用同一套判定。

// ---- 契约结构 ----

type alertChainMediaBrief struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Enabled   bool   `json:"enabled"`
}

type alertChainPolicyBrief struct {
	ID               int64           `json:"id"`
	Name             string          `json:"name"`
	Path             string          `json:"path"`
	Matchers         json.RawMessage `json:"matchers"`
	UserGroupIDs     *[]int64        `json:"user_group_ids"`
	UserGroupNames   []string        `json:"user_group_names"`
	UserInGroup      bool            `json:"user_in_group"` // 组限制生效时当前用户是否在组内；未限制恒 true
	NotifyOnFiring   bool            `json:"notify_on_firing"`
	NotifyOnResolved bool            `json:"notify_on_resolved"`
}

type alertChainBinding struct {
	BindingID  int64                   `json:"binding_id"`
	Enabled    bool                    `json:"enabled"`
	Recipients []string                `json:"recipients"`
	Media      alertChainMediaBrief    `json:"media"`
	Policies   []alertChainPolicyBrief `json:"policies"`
	Issues     []string                `json:"issues"`
}

type alertChainUserChain struct {
	User          gin.H               `json:"user"`
	Bindings      []alertChainBinding `json:"bindings"`
	CanReceive    bool                `json:"can_receive"`
	SummaryIssues []string            `json:"summary_issues"`
}

type alertChainDelivery struct {
	UserID   *int32 `json:"user_id"`
	Username string `json:"username"`
	Address  string `json:"address"`
	Status   string `json:"status"`
	Error    string `json:"error"`
}

type alertChainEvent struct {
	ID           int64  `json:"id"`
	EventType    string `json:"event_type"`
	Status       string `json:"status"`
	AttemptCount int64  `json:"attempt_count"`
	Error        string `json:"error"`
}

type alertChainMediaDetail struct {
	ID         int64                `json:"id"`
	Name       string               `json:"name"`
	Enabled    bool                 `json:"enabled"`
	Bindings   []gin.H              `json:"bindings"`
	Event      *alertChainEvent     `json:"event"`
	Deliveries []alertChainDelivery `json:"deliveries"`
}

// alertChainPolicyEval 单层兄弟策略的评估结果。
type alertChainPolicyEval struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	Matchers   json.RawMessage `json:"matchers"`
	Matched    bool            `json:"matched"`
	MissReason string          `json:"miss_reason"`
	Selected   bool            `json:"selected"`
}

type alertChainPolicyDetail struct {
	ID                int64           `json:"id"`
	Name              string          `json:"name"`
	Matchers          json.RawMessage `json:"matchers"`
	MediaIDs          *[]int64        `json:"media_ids"`
	MediaInherited    bool            `json:"media_inherited"`
	EffectiveMedia    []int64         `json:"effective_media_ids"`
	UserGroupIDs      *[]int64        `json:"user_group_ids"`
	UserGroupNames    []string        `json:"user_group_names"`
	UserGroupsLimited bool            `json:"user_groups_limited"` // 是否启用接收组限制
	NotifyOnFiring    bool            `json:"notify_on_firing"`
	NotifyOnResolved  bool            `json:"notify_on_resolved"`
	EventAllowed      bool            `json:"event_allowed"`
}

// ---- 纯判定逻辑 ----

// chainMergeLabels 把 alertname/severity/instance 便捷键合并进 labels（显式 labels 值优先）。
func chainMergeLabels(labels map[string]any, alertname, severity, instance string) map[string]any {
	merged := make(map[string]any, len(labels)+3)
	for key, value := range labels {
		merged[key] = value
	}
	setDefault := func(key, value string) {
		if value != "" {
			if _, exists := merged[key]; !exists {
				merged[key] = value
			}
		}
	}
	setDefault("alertname", alertname)
	setDefault("severity", severity)
	setDefault("instance", instance)
	return merged
}

// userBindingIssues 计算单条绑定的媒介级 issues。
func userBindingIssues(bindingEnabled bool, mediaEnabled bool, mediaType string, recipients []string) []string {
	issues := make([]string, 0, 4)
	if !bindingEnabled {
		issues = append(issues, "该绑定已禁用")
	}
	if !mediaEnabled {
		issues = append(issues, "媒介已停用")
	}
	if mediaType != "email" {
		issues = append(issues, "非邮件媒介，暂不支持自动发送")
	}
	if len(recipients) == 0 {
		issues = append(issues, "绑定未配置收件地址")
	}
	return issues
}

func truncateError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 120 {
		return message[:120] + "..."
	}
	return message
}

func derefInt32(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

// ---- API 1: GET /monitor/alert-notification/user-chain/ ----

func (handler *Handler) UserAlertChain(context *gin.Context) {
	userID, ok := resolveTargetUser(context)
	if !ok {
		return
	}
	queries := db.New(handler.db)
	username, err := queries.GetUsernameByID(context, int32(userID))
	if err != nil {
		response.Error(context, err)
		return
	}

	bindingRows, err := queries.ListUserAlertMediaBindings(context, int32(userID))
	if err != nil {
		response.Error(context, err)
		return
	}

	root, err := handler.loadNotificationPolicyTree(context.Request.Context())
	if err != nil {
		response.Error(context, err)
		return
	}

	bindings := make([]alertChainBinding, 0)
	summaryIssues := make([]string, 0)
	seenSummary := map[string]bool{}
	addSummary := func(item string) {
		if !seenSummary[item] {
			seenSummary[item] = true
			summaryIssues = append(summaryIssues, item)
		}
	}
	for _, row := range bindingRows {
		bindingID, mediaID := row.ID, row.MediaID
		enabled, mediaEnabled := row.Enabled, row.MediaEnabled
		mediaName, mediaType := row.MediaName, row.MediaType
		recipients := make([]string, 0)
		_ = json.Unmarshal(row.Recipients, &recipients)

		issues := userBindingIssues(enabled, mediaEnabled, mediaType, recipients)
		for _, issue := range issues {
			addSummary(fmt.Sprintf("绑定 %d：%s", bindingID, issue))
		}

		// 生效出口含该媒介的策略：沿父链继承判定（与分发侧 effectivePolicyMedia 同语义）。
		policies, policyErr := handler.policiesEffectiveForMedia(context.Request.Context(), root, mediaID, int32(userID))
		if policyErr != nil {
			response.Error(context, policyErr)
			return
		}
		for _, policy := range policies {
			if !policy.NotifyOnFiring && policy.NotifyOnResolved {
				addSummary(fmt.Sprintf("策略 %s 仅通知 resolved，firing 通知未开启", policy.Name))
			}
		}
		if len(policies) == 0 {
			addSummary(fmt.Sprintf("媒介 %s 未被任何通知策略出口命中", mediaName))
		}
		// 用户组限制：仅当所有覆盖该媒介的策略都把当前用户排除在组外时才算断点
		// （分发侧只投递"组成员的绑定"，用户不在组内即收不到）。
		restrictedOut := len(policies) > 0
		for _, policy := range policies {
			if policy.UserGroupIDs == nil || policy.UserInGroup {
				restrictedOut = false
				break
			}
		}
		if restrictedOut {
			issue := "当前用户不在该策略的接收组内，收不到对应告警"
			issues = append(issues, issue)
			addSummary(fmt.Sprintf("绑定 %d：%s", bindingID, issue))
		}
		bindings = append(bindings, alertChainBinding{
			BindingID: bindingID, Enabled: enabled, Recipients: recipients,
			Media:    alertChainMediaBrief{ID: mediaID, Name: mediaName, MediaType: mediaType, Enabled: mediaEnabled},
			Policies: policies, Issues: issues,
		})
	}
	if len(bindings) == 0 {
		addSummary(fmt.Sprintf("用户 %s 未配置任何告警媒介绑定", username))
	}

	canReceive := userCanReceive(bindings)
	if !canReceive && len(bindings) > 0 {
		addSummary("不存在「绑定启用 + 媒介启用 + 邮件媒介 + 收件地址 + 策略出口命中 + firing 通知开启」的完整通路")
	}

	response.Success(context, alertChainUserChain{
		User:     gin.H{"id": userID, "username": username},
		Bindings: bindings, CanReceive: canReceive, SummaryIssues: summaryIssues,
	})
}

// policiesEffectiveForMedia 返回生效出口包含 mediaID 的策略（带 "根 / 子 / 孙" 路径名、
// 生效用户组限制及当前用户是否受限）。userID<1 时（如管理员视角外部邮箱）不判定 user_in_group。
func (handler *Handler) policiesEffectiveForMedia(ctx context.Context, root *policyNode, mediaID int64, userID int32) ([]alertChainPolicyBrief, error) {
	policies := make([]alertChainPolicyBrief, 0)
	var walk func(node *policyNode, path []*policyNode, media []int64, groups []int64) error
	walk = func(node *policyNode, path []*policyNode, media []int64, groups []int64) error {
		currentPath := append(append([]*policyNode{}, path...), node)
		if !node.MediaInherited && node.MediaIDs != nil {
			media = append(media[:0:0], node.MediaIDs...)
		}
		if !node.UserGroupInherited && node.UserGroupIDs != nil {
			groups = append(groups[:0:0], node.UserGroupIDs...)
		}
		for _, id := range media {
			if id != mediaID {
				continue
			}
			names := make([]string, 0, len(currentPath))
			for _, item := range currentPath {
				names = append(names, item.Name)
			}
			matchersRaw, err := json.Marshal(node.Matchers)
			if err != nil {
				return err
			}
			brief := alertChainPolicyBrief{
				ID: node.ID, Name: node.Name, Path: strings.Join(names, " / "), Matchers: matchersRaw,
				UserGroupNames: []string{}, UserInGroup: true,
				NotifyOnFiring: node.NotifyOnFiring, NotifyOnResolved: node.NotifyOnResolved,
			}
			if groups != nil {
				groupCopy := append([]int64{}, groups...)
				brief.UserGroupIDs = &groupCopy
				groupNames, err := handler.userGroupNamesByIDs(ctx, groupCopy)
				if err != nil {
					return err
				}
				brief.UserGroupNames = groupNames
				if userID > 0 {
					brief.UserInGroup = handler.userInGroups(ctx, userID, groupCopy)
				}
			}
			policies = append(policies, brief)
			break
		}
		for _, child := range node.Children {
			if err := walk(child, currentPath, media, groups); err != nil {
				return err
			}
		}
		return nil
	}
	media := []int64{}
	if !root.MediaInherited && root.MediaIDs != nil {
		media = append(media, root.MediaIDs...)
	}
	if err := walk(root, []*policyNode{}, media, nil); err != nil {
		return nil, err
	}
	return policies, nil
}

// userInGroups 判断用户是否属于任一组；组列表为空（显式不限/静音语义由调用方区分）返回 false。
func (handler *Handler) userInGroups(ctx context.Context, userID int32, groupIDs []int64) bool {
	if len(groupIDs) == 0 {
		return false
	}
	count, err := db.New(handler.db).CountUserGroupMemberships(ctx, db.CountUserGroupMembershipsParams{
		UserID: userID, GroupIds: groupIDs,
	})
	if err != nil {
		return false
	}
	return count > 0
}

func userCanReceive(bindings []alertChainBinding) bool {
	for _, binding := range bindings {
		if !binding.Enabled || !binding.Media.Enabled || binding.Media.MediaType != "email" || len(binding.Recipients) == 0 {
			continue
		}
		for _, policy := range binding.Policies {
			if policy.NotifyOnFiring && (policy.UserGroupIDs == nil || policy.UserInGroup) {
				return true
			}
		}
	}
	return false
}

func resolveTargetUser(context *gin.Context) (int64, bool) {
	if raw := strings.TrimSpace(context.Query("user_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			response.BusinessError(context, 400, "user_id 无效", nil)
			return 0, false
		}
		return parsed, true
	}
	claims, ok := identity.ClaimsFromContext(context)
	if !ok {
		response.Error(context, fmt.Errorf("未获取到登录用户"))
		return 0, false
	}
	return int64(claims.UserID), true
}

// ---- API 2: GET /monitor/alert-notification/chain/:historyId/ ----

func (handler *Handler) AlertChainEvaluation(context *gin.Context) {
	historyID := parseID(context.Param("historyId"))
	queries := db.New(handler.db)
	history, err := queries.GetAlertHistoryForChain(context, historyID)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "alert history not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	alertname, severity, instance, state, startedAt := history.Alertname, history.Severity, history.Instance, history.State, history.StartedAt
	labels := map[string]any{}
	_ = json.Unmarshal(history.Labels, &labels)
	mergedLabels := chainMergeLabels(labels, alertname, severity, instance)
	mergedString := map[string]string{}
	for key, value := range mergedLabels {
		mergedString[key] = fmt.Sprintf("%v", value)
	}

	root, err := handler.loadNotificationPolicyTree(context.Request.Context())
	if err != nil {
		response.Error(context, err)
		return
	}
	scopeNodes := handler.alertScopeNodes(alertNotificationTarget{id: historyID, labels: labels})

	// 沿命中路径逐层评估兄弟策略（与分发侧 resolvePolicyRoute 同序：每层取第一条命中）。
	levels := make([][]alertChainPolicyEval, 0)
	path := []*policyNode{root}
	summaryIssues := make([]string, 0)
	current := root
	for {
		level := make([]alertChainPolicyEval, 0, len(current.Children))
		next := (*policyNode)(nil)
		for _, child := range current.Children {
			matched, missReason := policyNodeMatches(child, mergedString, scopeNodes)
			eval := alertChainPolicyEval{
				ID: child.ID, Name: child.Name,
				Matchers: mustMarshalMatchers(child.Matchers),
				Matched:  matched, MissReason: missReason,
			}
			if matched && next == nil {
				next = child
				eval.Selected = true
			}
			if !matched {
				summaryIssues = append(summaryIssues, fmt.Sprintf("策略 %s 未命中：%s", child.Name, missReason))
			}
			level = append(level, eval)
		}
		if len(level) > 0 {
			levels = append(levels, level)
		}
		if next == nil {
			break
		}
		path = append(path, next)
		current = next
	}

	final := path[len(path)-1]
	eventType := strings.ToLower(state)
	eventAllowed := policyAllowsEvent(final, eventType)
	mediaIDs := effectivePolicyMedia(path)
	userGroupIDs := effectivePolicyUserGroups(path)

	policyDetail := alertChainPolicyDetail{
		ID: final.ID, Name: final.Name, Matchers: mustMarshalMatchers(final.Matchers),
		MediaInherited: final.MediaInherited, EffectiveMedia: mediaIDs,
		UserGroupNames: []string{}, UserGroupsLimited: userGroupIDs != nil,
		NotifyOnFiring: final.NotifyOnFiring, NotifyOnResolved: final.NotifyOnResolved,
		EventAllowed: eventAllowed,
	}
	if userGroupIDs != nil {
		groupCopy := append([]int64{}, userGroupIDs...)
		policyDetail.UserGroupIDs = &groupCopy
		groupNames, err := handler.userGroupNamesByIDs(context.Request.Context(), groupCopy)
		if err != nil {
			response.Error(context, err)
			return
		}
		policyDetail.UserGroupNames = groupNames
	}
	if !final.MediaInherited {
		mediaCopy := append([]int64{}, final.MediaIDs...)
		policyDetail.MediaIDs = &mediaCopy
	}

	medias := make([]alertChainMediaDetail, 0)
	if len(mediaIDs) > 0 {
		medias, err = handler.policyMediasDetail(context, mediaIDs, historyID, eventType, userGroupIDs)
		if err != nil {
			response.Error(context, err)
			return
		}
	} else {
		summaryIssues = append(summaryIssues, fmt.Sprintf("最深命中策略 %s 的出口为空（静音），不会投递", final.Name))
	}
	if !eventAllowed {
		summaryIssues = append(summaryIssues, fmt.Sprintf("策略 %s 未开启 %s 通知", final.Name, eventType))
	}
	for _, media := range medias {
		if media.Event == nil {
			summaryIssues = append(summaryIssues, fmt.Sprintf("媒介 %s：无 %s 事件记录，通知未触发", media.Name, eventType))
		}
		for _, delivery := range media.Deliveries {
			if delivery.Status != "success" {
				name := delivery.Username
				if name == "" {
					name = fmt.Sprintf("%d", derefInt32(delivery.UserID))
				}
				summaryIssues = append(summaryIssues, fmt.Sprintf("用户 %s 的投递失败：%s", name, truncateError(delivery.Error)))
			}
		}
	}

	pathNames := make([]string, 0, len(path))
	pathIDs := make([]int64, 0, len(path))
	for _, node := range path {
		pathNames = append(pathNames, node.Name)
		pathIDs = append(pathIDs, node.ID)
	}

	response.Success(context, gin.H{
		"alert": gin.H{
			"id": historyID, "alertname": alertname, "state": state,
			"labels": mergedLabels, "started_at": startedAt,
		},
		"policy_tree": gin.H{
			"matched_path_ids": pathIDs,
			"matched_path":     strings.Join(pathNames, " / "),
			"levels":           levels,
			"final_policy":     policyDetail,
		},
		"medias":         medias,
		"summary_issues": summaryIssues,
	})
}

func mustMarshalMatchers(matchers []policyMatcher) json.RawMessage {
	raw, err := json.Marshal(matchers)
	if err != nil {
		return json.RawMessage("[]")
	}
	return raw
}

// policyMediasDetail 填充出口媒介的绑定与实际 event/delivery 记录（含停用媒介，便于展示断点）。
func (handler *Handler) policyMediasDetail(context *gin.Context, mediaIDs []int64, historyID int64, eventType string, userGroupIDs []int64) ([]alertChainMediaDetail, error) {
	queries := db.New(handler.db)
	mediaRows, err := queries.ListAlertMediaBriefByIDs(context, mediaIDs)
	if err != nil {
		return nil, err
	}

	details := make([]alertChainMediaDetail, 0, len(mediaRows))
	for _, media := range mediaRows {
		mediaDetail := alertChainMediaDetail{
			ID: media.ID, Name: media.Name, Enabled: media.Enabled,
			Bindings: make([]gin.H, 0), Deliveries: make([]alertChainDelivery, 0),
		}
		bindingRows, err := queries.ListAlertMediaBindingsWithUser(context, media.ID)
		if err != nil {
			return nil, err
		}
		for _, row := range bindingRows {
			recipients := make([]string, 0)
			_ = json.Unmarshal(row.Recipients, &recipients)
			// 用户组限制生效时标注成员归属（未标注 = 不限组，全部绑定都可收）。
			bindingView := gin.H{
				"user_id": row.UserID, "username": row.Username, "recipients": recipients, "enabled": row.Enabled,
			}
			if userGroupIDs != nil {
				inGroup := handler.userInGroups(context.Request.Context(), row.UserID, userGroupIDs)
				bindingView["in_group"] = inGroup
				if !inGroup {
					bindingView["issue"] = "不在命中策略的接收组内，不会收到该告警"
				}
			}
			mediaDetail.Bindings = append(mediaDetail.Bindings, bindingView)
		}

		event, eventErr := queries.GetLatestAlertNotificationEventForAlert(context, db.GetLatestAlertNotificationEventForAlertParams{
			AlertID: historyID, EventType: eventType,
		})
		switch {
		case eventErr == sql.ErrNoRows:
			mediaDetail.Event = nil
		case eventErr != nil:
			return nil, eventErr
		default:
			mediaDetail.Event = &alertChainEvent{
				ID: event.ID, EventType: event.EventType, Status: event.Status,
				AttemptCount: int64(event.AttemptCount), Error: event.ErrorMessage,
			}
			deliveryRows, err := queries.ListAlertNotificationDeliveriesForChain(context, event.ID)
			if err != nil {
				return nil, err
			}
			for _, row := range deliveryRows {
				delivery := alertChainDelivery{
					Address: row.Address, Status: row.Status, Error: row.ErrorMessage,
					Username: row.Username.String,
				}
				if row.UserID.Valid {
					value := row.UserID.Int32
					delivery.UserID = &value
				}
				mediaDetail.Deliveries = append(mediaDetail.Deliveries, delivery)
			}
		}
		details = append(details, mediaDetail)
	}
	return details, nil
}
