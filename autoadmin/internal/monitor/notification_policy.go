package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// 通知策略树（对齐 Grafana notification policy）：
// - 树根为「默认策略」（parent_id IS NULL），matchers=[] 恒命中，不可删除、不可挂子父变更；
// - 节点条件：label matcher（=/!=/=~/!~，缺 label 视为空串，Prometheus 语义）与
//   tree matcher（服务树节点，命中告警主机的归属节点集合即命中），节点内 AND；
// - 出口：media_ids NULL=继承父节点，[]=显式静音；
// - 路由：从根向下，每层按 position,id 取第一条命中的子策略，最深命中节点决定出口与事件开关。

const policyMaxMatchers = 20

// policyMatcher 策略匹配条件：type=label（label+operator+value）或 type=tree（node_type+id）。
type policyMatcher struct {
	Type     string `json:"type"`
	Label    string `json:"label,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
	NodeType string `json:"node_type,omitempty"`
	ID       int64  `json:"id,omitempty"`
}

var (
	policyMatcherOperators = map[string]bool{"=": true, "!=": true, "=~": true, "!~": true}
	policyTreeTypes        = map[string]bool{"service": true, "environment": true, "business": true, "project": true}
)

// normalizePolicyMatchers 归一化 matchers：nil → []（恒命中）；校验条目合法性。
// 返回值第二个为用户可读错误文案（空串=通过）。
func normalizePolicyMatchers(raw json.RawMessage) ([]policyMatcher, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return []policyMatcher{}, ""
	}
	var matchers []policyMatcher
	if err := json.Unmarshal(raw, &matchers); err != nil {
		return nil, "matchers 必须是数组"
	}
	if len(matchers) > policyMaxMatchers {
		return nil, fmt.Sprintf("matchers 条目不能超过 %d 个", policyMaxMatchers)
	}
	normalized := make([]policyMatcher, 0, len(matchers))
	for _, matcher := range matchers {
		switch matcher.Type {
		case "label":
			matcher.Label = strings.TrimSpace(matcher.Label)
			matcher.Value = strings.TrimSpace(matcher.Value)
			if matcher.Label == "" {
				return nil, "label matcher 的 label 不能为空"
			}
			if !policyMatcherOperators[matcher.Operator] {
				return nil, "label matcher 的 operator 仅支持 = / != / =~ / !~"
			}
			if (matcher.Operator == "=" || matcher.Operator == "!=") && matcher.Value == "" {
				return nil, "label matcher 的 value 不能为空"
			}
			if matcher.Operator == "=~" || matcher.Operator == "!~" {
				if _, err := regexp.Compile(matcher.Value); err != nil {
					return nil, fmt.Sprintf("正则 %q 无效：%v", matcher.Value, err)
				}
			}
		case "tree":
			if !policyTreeTypes[matcher.NodeType] {
				return nil, "tree matcher 的 node_type 仅支持 service/environment/business/project"
			}
			if matcher.ID <= 0 {
				return nil, "tree matcher 的 id 必须是正整数"
			}
		default:
			return nil, "matchers.type 仅支持 label / tree"
		}
		normalized = append(normalized, matcher)
	}
	return normalized, ""
}

// policyNode 策略树节点（含已挂接的子节点，Children 按 position,id 有序）。
type policyNode struct {
	ID                 int64
	ParentID           int64 // 0 = 根
	Name               string
	Position           int
	Remark             string
	Matchers           []policyMatcher
	MediaIDs           []int64
	MediaInherited     bool // media_ids 列为 NULL
	UserGroupIDs       []int64
	UserGroupInherited bool // user_group_ids 列为 NULL
	NotifyOnFiring     bool
	NotifyOnResolved   bool
	Children           []*policyNode
}

// loadNotificationPolicyTree 读取全部策略并建树。坏 matchers JSON 的节点按「无条件命中」处理
// （不因配置数据问题静默吞掉通知），与管理端写入前强校验配合。
func (handler *Handler) loadNotificationPolicyTree(ctx context.Context) (*policyNode, error) {
	rows, err := handler.db.QueryContext(ctx, `SELECT id,COALESCE(parent_id,0),name,position,COALESCE(remark,''),matchers,media_ids,user_group_ids,notify_on_firing,notify_on_resolved
FROM monitor_notification_policy ORDER BY position,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := map[int64]*policyNode{}
	var root *policyNode
	for rows.Next() {
		node := &policyNode{}
		var matchersRaw, mediaIDsRaw, userGroupIDsRaw []byte
		var remark string
		if err = rows.Scan(&node.ID, &node.ParentID, &node.Name, &node.Position, &remark, &matchersRaw, &mediaIDsRaw, &userGroupIDsRaw, &node.NotifyOnFiring, &node.NotifyOnResolved); err != nil {
			return nil, err
		}
		node.Remark = remark
		if err := json.Unmarshal(matchersRaw, &node.Matchers); err != nil {
			node.Matchers = nil
		}
		if mediaIDsRaw == nil || string(mediaIDsRaw) == "null" {
			node.MediaInherited = true
		} else {
			_ = json.Unmarshal(mediaIDsRaw, &node.MediaIDs)
		}
		if userGroupIDsRaw == nil || string(userGroupIDsRaw) == "null" {
			node.UserGroupInherited = true
		} else {
			_ = json.Unmarshal(userGroupIDsRaw, &node.UserGroupIDs)
		}
		nodes[node.ID] = node
		if node.ParentID == 0 {
			root = node
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("通知策略树缺少根节点（parent_id IS NULL）")
	}
	for _, node := range nodes {
		if node.ParentID == 0 {
			continue
		}
		parent, ok := nodes[node.ParentID]
		if !ok {
			continue // 孤儿节点（理论上有 FK 不会出现）：路由时忽略
		}
		parent.Children = append(parent.Children, node)
	}
	for _, node := range nodes {
		children := node.Children
		sort.Slice(children, func(i, j int) bool {
			if children[i].Position != children[j].Position {
				return children[i].Position < children[j].Position
			}
			return children[i].ID < children[j].ID
		})
	}
	return root, nil
}

// policyMatcherMatch 单条 matcher 判定；返回命中与否与未命中说明（空串=命中）。
// label 不存在按空串参与匹配（Prometheus 语义：!= / !~ 可命中无该 label 的告警）。
func policyMatcherMatch(matcher policyMatcher, labels map[string]string, scopeNodes map[string]bool) (bool, string) {
	switch matcher.Type {
	case "label":
		actual := labels[matcher.Label]
		switch matcher.Operator {
		case "=":
			if actual == matcher.Value {
				return true, ""
			}
			return false, fmt.Sprintf("labels.%s=%q 不等于 %q", matcher.Label, actual, matcher.Value)
		case "!=":
			if actual != matcher.Value {
				return true, ""
			}
			return false, fmt.Sprintf("labels.%s 等于排除值 %q", matcher.Label, matcher.Value)
		case "=~":
			re, err := regexp.Compile(matcher.Value)
			if err != nil {
				return false, fmt.Sprintf("正则 %q 无效", matcher.Value)
			}
			if re.MatchString(actual) {
				return true, ""
			}
			return false, fmt.Sprintf("labels.%s=%q 不匹配正则 %q", matcher.Label, actual, matcher.Value)
		case "!~":
			re, err := regexp.Compile(matcher.Value)
			if err != nil {
				return false, fmt.Sprintf("正则 %q 无效", matcher.Value)
			}
			if !re.MatchString(actual) {
				return true, ""
			}
			return false, fmt.Sprintf("labels.%s=%q 命中排除正则 %q", matcher.Label, actual, matcher.Value)
		}
		return false, "未知 operator"
	case "tree":
		if scopeNodes[fmt.Sprintf("%s:%d", matcher.NodeType, matcher.ID)] {
			return true, ""
		}
		return false, fmt.Sprintf("告警归属服务树不含 %s:%d", matcher.NodeType, matcher.ID)
	}
	return false, "未知 matcher 类型"
}

// policyNodeMatches 节点内全部 matcher AND。
func policyNodeMatches(node *policyNode, labels map[string]string, scopeNodes map[string]bool) (bool, string) {
	for _, matcher := range node.Matchers {
		if matched, missReason := policyMatcherMatch(matcher, labels, scopeNodes); !matched {
			return false, missReason
		}
	}
	return true, ""
}

// resolvePolicyRoute 从根向下逐层取第一条命中的子策略，返回根到最深命中节点的路径（至少含根）。
func resolvePolicyRoute(root *policyNode, labels map[string]string, scopeNodes map[string]bool) []*policyNode {
	path := []*policyNode{root}
	current := root
	for {
		next := (*policyNode)(nil)
		for _, child := range current.Children {
			if matched, _ := policyNodeMatches(child, labels, scopeNodes); matched {
				next = child
				break
			}
		}
		if next == nil {
			return path
		}
		path = append(path, next)
		current = next
	}
}

// effectivePolicyMedia 沿路径向下取最后一个显式 media_ids 的节点出口（默认空 = 不投递）。
// MediaIDs 为 nil 视为继承（防手工构造的节点误判为显式静音）。
func effectivePolicyMedia(path []*policyNode) []int64 {
	mediaIDs := []int64{}
	for _, node := range path {
		if !node.MediaInherited && node.MediaIDs != nil {
			mediaIDs = append(mediaIDs[:0], node.MediaIDs...)
		}
	}
	return mediaIDs
}

// effectivePolicyUserGroups 沿路径取最后一个显式 user_group_ids 的节点；
// 全路径都未显式设置（含根）返回 nil = 不限组（该媒介上的全部绑定都可收）。
// 显式 [] = 收敛为空（无人可收），可用来做按组隔离。
func effectivePolicyUserGroups(path []*policyNode) []int64 {
	var groupIDs []int64
	for _, node := range path {
		if !node.UserGroupInherited && node.UserGroupIDs != nil {
			groupIDs = append(groupIDs[:0], node.UserGroupIDs...)
		}
	}
	return groupIDs
}

// policyAllowsEvent 最深命中节点的事件开关决定该事件类型是否投递。
func policyAllowsEvent(node *policyNode, eventType string) bool {
	if eventType == "resolved" {
		return node.NotifyOnResolved
	}
	return node.NotifyOnFiring
}

// matchedPolicyMedias 策略树版的媒介解析：路由 → 事件开关 → 出口媒介（仅 enabled）。
// 第三个返回值为生效的用户组限制：nil=不限组（该媒介全部绑定），[]/非空=仅这些组的成员绑定。
func (handler *Handler) matchedPolicyMedias(ctx context.Context, target alertNotificationTarget, eventType string) ([]alertNotificationMedia, []int64, error) {
	root, err := handler.loadNotificationPolicyTree(ctx)
	if err != nil {
		return nil, nil, err
	}
	merged := mergeAlertLabels(target.labels, target.alertname, target.severity, target.instance)
	path := resolvePolicyRoute(root, merged, handler.alertScopeNodes(target))
	final := path[len(path)-1]
	mediaIDs := effectivePolicyMedia(path)
	userGroupIDs := effectivePolicyUserGroups(path)
	if len(mediaIDs) == 0 || !policyAllowsEvent(final, eventType) {
		return nil, userGroupIDs, nil
	}
	placeholders := ""
	args := make([]any, 0, len(mediaIDs))
	for index, mediaID := range mediaIDs {
		if index > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, mediaID)
	}
	rows, err := handler.db.QueryContext(ctx, `SELECT id,name,media_type,config FROM monitor_alert_media WHERE enabled=TRUE AND id IN (`+placeholders+`) ORDER BY id`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	medias := make([]alertNotificationMedia, 0, len(mediaIDs))
	for rows.Next() {
		media := alertNotificationMedia{}
		if err = rows.Scan(&media.id, &media.name, &media.mediaType, &media.config); err != nil {
			return nil, nil, err
		}
		medias = append(medias, media)
	}
	return medias, userGroupIDs, rows.Err()
}

// ---- 管理 API ----

type notificationPolicyItem struct {
	ID               int64           `json:"id"`
	ParentID         *int64          `json:"parent_id"`
	Name             string          `json:"name"`
	Position         int             `json:"position"`
	Remark           string          `json:"remark"`
	Matchers         json.RawMessage `json:"matchers"`
	MediaIDs         *[]int64        `json:"media_ids"`
	MediaNames       []string        `json:"media_names"`
	UserGroupIDs     *[]int64        `json:"user_group_ids"`
	UserGroupNames   []string        `json:"user_group_names"`
	NotifyOnFiring   bool            `json:"notify_on_firing"`
	NotifyOnResolved bool            `json:"notify_on_resolved"`
	IsRoot           bool            `json:"is_root"`
	CreateTime       time.Time       `json:"create_time"`
	UpdateTime       time.Time       `json:"update_time"`
}

func (handler *Handler) ListNotificationPolicies(context *gin.Context) {
	items, err := handler.listNotificationPolicies(context.Request.Context())
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"items": items})
}

func (handler *Handler) listNotificationPolicies(ctx context.Context) ([]notificationPolicyItem, error) {
	rows, err := handler.db.QueryContext(ctx, `SELECT id,COALESCE(parent_id,0),name,position,COALESCE(remark,''),matchers,media_ids,user_group_ids,notify_on_firing,notify_on_resolved,create_time,update_time
FROM monitor_notification_policy ORDER BY position,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]notificationPolicyItem, 0)
	for rows.Next() {
		item := notificationPolicyItem{MediaNames: []string{}, UserGroupNames: []string{}}
		var parentID int64
		var matchersRaw, mediaIDsRaw, userGroupIDsRaw []byte
		var remark string
		if err = rows.Scan(&item.ID, &parentID, &item.Name, &item.Position, &remark, &matchersRaw, &mediaIDsRaw, &userGroupIDsRaw, &item.NotifyOnFiring, &item.NotifyOnResolved, &item.CreateTime, &item.UpdateTime); err != nil {
			return nil, err
		}
		item.Remark = remark
		if parentID > 0 {
			item.ParentID = &parentID
		} else {
			item.IsRoot = true
		}
		item.Matchers = json.RawMessage(matchersRaw)
		if mediaIDsRaw != nil && string(mediaIDsRaw) != "null" {
			mediaIDs := []int64{}
			_ = json.Unmarshal(mediaIDsRaw, &mediaIDs)
			item.MediaIDs = &mediaIDs
			names, err := handler.mediaNamesByIDs(ctx, mediaIDs)
			if err != nil {
				return nil, err
			}
			item.MediaNames = names
		}
		if userGroupIDsRaw != nil && string(userGroupIDsRaw) != "null" {
			groupIDs := []int64{}
			_ = json.Unmarshal(userGroupIDsRaw, &groupIDs)
			item.UserGroupIDs = &groupIDs
			groupNames, err := handler.userGroupNamesByIDs(ctx, groupIDs)
			if err != nil {
				return nil, err
			}
			item.UserGroupNames = groupNames
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (handler *Handler) userGroupNamesByIDs(ctx context.Context, groupIDs []int64) ([]string, error) {
	names := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		var name string
		if err := handler.db.QueryRowContext(ctx, `SELECT name FROM sys_user_group WHERE id=?`, groupID).Scan(&name); err == nil {
			names = append(names, name)
		} else if err != sql.ErrNoRows {
			return nil, err
		}
	}
	return names, nil
}

func (handler *Handler) mediaNamesByIDs(ctx context.Context, mediaIDs []int64) ([]string, error) {
	names := make([]string, 0, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		var name string
		if err := handler.db.QueryRowContext(ctx, `SELECT name FROM monitor_alert_media WHERE id=?`, mediaID).Scan(&name); err == nil {
			names = append(names, name)
		} else if err != sql.ErrNoRows {
			return nil, err
		}
	}
	return names, nil
}

type saveNotificationPolicyInput struct {
	ID               int64           `json:"id"`
	ParentID         *int64          `json:"parent_id"`
	Name             string          `json:"name"`
	Position         *int            `json:"position"`
	Remark           string          `json:"remark"`
	Matchers         json.RawMessage `json:"matchers"`
	MediaIDs         *[]int64        `json:"media_ids"`
	UserGroupIDs     *[]int64        `json:"user_group_ids"`
	NotifyOnFiring   *bool           `json:"notify_on_firing"`
	NotifyOnResolved *bool           `json:"notify_on_resolved"`
}

func (handler *Handler) CreateNotificationPolicy(context *gin.Context) {
	var input saveNotificationPolicyInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	if input.ParentID == nil || *input.ParentID <= 0 {
		response.BusinessError(context, 400, "parent_id 必填（根节点由系统内置）", nil)
		return
	}
	id, errMsg, err := handler.saveNotificationPolicy(context.Request.Context(), 0, &input)
	if err != nil {
		response.Error(context, err)
		return
	}
	if errMsg != "" {
		response.BusinessError(context, 400, errMsg, nil)
		return
	}
	handler.respondNotificationPolicy(context, id)
}

func (handler *Handler) UpdateNotificationPolicy(context *gin.Context) {
	var input saveNotificationPolicyInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	if input.ID <= 0 {
		response.BusinessError(context, 400, "id 必填", nil)
		return
	}
	_, errMsg, err := handler.saveNotificationPolicy(context.Request.Context(), input.ID, &input)
	if err != nil {
		response.Error(context, err)
		return
	}
	if errMsg != "" {
		response.BusinessError(context, 400, errMsg, nil)
		return
	}
	handler.respondNotificationPolicy(context, input.ID)
}

// saveNotificationPolicy 创建/更新策略；返回 (新id, 用户可读错误文案, 内部错误)。
func (handler *Handler) saveNotificationPolicy(ctx context.Context, id int64, input *saveNotificationPolicyInput) (int64, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return 0, "name 不能为空", nil
	}
	matchers, matcherErr := normalizePolicyMatchers(input.Matchers)
	if matcherErr != "" {
		return 0, matcherErr, nil
	}
	matchersRaw, err := json.Marshal(matchers)
	if err != nil {
		return 0, "", err
	}
	firing, resolved := true, true
	if input.NotifyOnFiring != nil {
		firing = *input.NotifyOnFiring
	}
	if input.NotifyOnResolved != nil {
		resolved = *input.NotifyOnResolved
	}
	if !firing && !resolved {
		return 0, "firing 与 resolved 通知至少开启一个", nil
	}
	position := 0
	if input.Position != nil {
		position = *input.Position
	}
	mediaColumn := any(nil) // NULL = 继承
	if input.MediaIDs != nil {
		mediaIDs := *input.MediaIDs
		seen := map[int64]bool{}
		unique := make([]int64, 0, len(mediaIDs))
		for _, mediaID := range mediaIDs {
			if mediaID <= 0 || seen[mediaID] {
				continue
			}
			seen[mediaID] = true
			unique = append(unique, mediaID)
		}
		sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })
		for _, mediaID := range unique {
			var count int
			if err = handler.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM monitor_alert_media WHERE id=?`, mediaID).Scan(&count); err != nil {
				return 0, "", err
			}
			if count == 0 {
				return 0, fmt.Sprintf("媒介 %d 不存在", mediaID), nil
			}
		}
		encoded, err := json.Marshal(unique)
		if err != nil {
			return 0, "", err
		}
		mediaColumn = string(encoded)
	}
	// 用户组出口：nil = 继承；[] = 不限组；[id...] = 仅这些组。校验组存在。
	var userGroupColumn any // NULL = 继承
	if input.UserGroupIDs != nil {
		groupIDs := *input.UserGroupIDs
		seen := map[int64]bool{}
		unique := make([]int64, 0, len(groupIDs))
		for _, groupID := range groupIDs {
			if groupID <= 0 || seen[groupID] {
				continue
			}
			seen[groupID] = true
			unique = append(unique, groupID)
		}
		sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })
		for _, groupID := range unique {
			var count int
			if err = handler.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_user_group WHERE id=?`, groupID).Scan(&count); err != nil {
				return 0, "", err
			}
			if count == 0 {
				return 0, fmt.Sprintf("用户组 %d 不存在", groupID), nil
			}
		}
		encoded, err := json.Marshal(unique)
		if err != nil {
			return 0, "", err
		}
		userGroupColumn = string(encoded)
	}

	// 更新根节点：父节点/位置不可变，仅允许改名称、备注、出口与事件开关。
	var currentParent int64
	if err = handler.db.QueryRowContext(ctx, `SELECT COALESCE(parent_id,0) FROM monitor_notification_policy WHERE id=?`, id).Scan(&currentParent); err != nil && err != sql.ErrNoRows {
		return 0, "", err
	}
	isRoot := id > 0 && currentParent == 0
	if id > 0 && err == sql.ErrNoRows {
		return 0, "策略不存在", nil
	}
	if isRoot && input.ParentID != nil && *input.ParentID != 0 {
		return 0, "根节点不可变更父节点", nil
	}
	parentID := int64(0)
	var parentColumn any
	if !isRoot {
		if input.ParentID == nil || *input.ParentID <= 0 {
			return 0, "parent_id 必填", nil
		}
		parentID = *input.ParentID
		if parentID == id {
			return 0, "父节点不能是自己", nil
		}
		if err = handler.isPolicyDescendant(ctx, parentID, id); err != nil {
			return 0, "", err
		}
	}
	now := time.Now().UTC()
	if id == 0 {
		var parentExists int
		if err = handler.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM monitor_notification_policy WHERE id=?`, parentID).Scan(&parentExists); err != nil {
			return 0, "", err
		}
		if parentExists == 0 {
			return 0, "父策略不存在", nil
		}
		result, execErr := handler.db.ExecContext(ctx, `INSERT INTO monitor_notification_policy(create_time,update_time,remark,parent_id,name,position,matchers,media_ids,user_group_ids,notify_on_firing,notify_on_resolved) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			now, now, input.Remark, parentID, name, position, string(matchersRaw), mediaColumn, userGroupColumn, firing, resolved)
		if execErr != nil {
			return 0, "", execErr
		}
		id, _ = result.LastInsertId()
		return id, "", nil
	}
	if isRoot {
		// FK 约束：根节点父列必须写 NULL 而不是 0。
		parentColumn = nil
	} else {
		parentColumn = parentID
	}
	_, execErr := handler.db.ExecContext(ctx, `UPDATE monitor_notification_policy SET update_time=?,remark=?,parent_id=?,name=?,position=?,matchers=?,media_ids=?,user_group_ids=?,notify_on_firing=?,notify_on_resolved=? WHERE id=?`,
		now, input.Remark, parentColumn, name, position, string(matchersRaw), mediaColumn, userGroupColumn, firing, resolved, id)
	if execErr != nil {
		return 0, "", execErr
	}
	return id, "", nil
}

// isPolicyDescendant 校验 candidateID 不是 excludeID 的后代（防环）。
func (handler *Handler) isPolicyDescendant(ctx context.Context, candidateID, excludeID int64) error {
	current := candidateID
	for hop := 0; hop < 100 && current > 0; hop++ {
		if current == excludeID {
			return fmt.Errorf("父节点不能是自己的后代")
		}
		var parent int64
		if err := handler.db.QueryRowContext(ctx, `SELECT COALESCE(parent_id,0) FROM monitor_notification_policy WHERE id=?`, current).Scan(&parent); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("父策略不存在")
			}
			return err
		}
		current = parent
	}
	// 走到根仍未命中 excludeID：合法。
	return nil
}

func (handler *Handler) respondNotificationPolicy(context *gin.Context, id int64) {
	items, err := handler.listNotificationPolicies(context.Request.Context())
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		if item.ID == id {
			response.Success(context, item)
			return
		}
	}
	response.BusinessError(context, 404, "策略不存在", nil)
}

func (handler *Handler) BatchDeleteNotificationPolicies(context *gin.Context) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.BusinessError(context, 400, "ids 必填", nil)
		return
	}
	deleted := 0
	for _, id := range input.IDs {
		if id <= 0 {
			continue
		}
		var parentID int64
		if err := handler.db.QueryRowContext(context.Request.Context(), `SELECT COALESCE(parent_id,0) FROM monitor_notification_policy WHERE id=?`, id).Scan(&parentID); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			response.Error(context, err)
			return
		}
		if parentID == 0 {
			response.BusinessError(context, 400, "根节点（默认策略）不可删除", nil)
			return
		}
		// FK ON DELETE CASCADE：子树随之删除。
		result, err := handler.db.ExecContext(context.Request.Context(), `DELETE FROM monitor_notification_policy WHERE id=?`, id)
		if err != nil {
			response.Error(context, err)
			return
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			deleted++
		}
	}
	response.Success(context, gin.H{"deleted": deleted})
}
