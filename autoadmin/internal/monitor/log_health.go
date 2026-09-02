package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// 日志采集链路对账：逐层比对「期望状态」与「集群/主机实际状态」，对应 Django monitor/log_health.py。
// 存在意义是消除静默失败——模板没下发、policy 没发布、配置改了没同步，这些问题
// 原先只能等聚合报错或登上主机看 journalctl 才发现。全程只读，不做任何写入或自动修复。

const (
	logHealthOK    = "ok"
	logHealthWarn  = "warn"
	logHealthDrift = "drift"
	logHealthError = "error"

	logHealthDataFlowWindowMinutes = 15
)

var logHealthStatusRank = map[string]int{logHealthOK: 0, logHealthWarn: 1, logHealthDrift: 2, logHealthError: 3}

func worstLogHealthStatus(statuses []string) string {
	worst := logHealthOK
	for _, status := range statuses {
		if logHealthStatusRank[status] > logHealthStatusRank[worst] {
			worst = status
		}
	}
	return worst
}

func logHealthItem(name, status, detail string) gin.H {
	return gin.H{"name": name, "status": status, "detail": detail}
}

func logHealthLayer(key, name, status, summary string, items []gin.H) gin.H {
	if items == nil {
		items = []gin.H{}
	}
	return gin.H{"key": key, "name": name, "status": status, "summary": summary, "items": items}
}

func logHealthLayerFromItems(key, name string, items []gin.H, emptySummary string) gin.H {
	if len(items) == 0 {
		return logHealthLayer(key, name, logHealthWarn, emptySummary, items)
	}
	abnormal := 0
	statuses := make([]string, 0, len(items))
	for _, item := range items {
		statuses = append(statuses, item["status"].(string))
		if item["status"] != logHealthOK {
			abnormal++
		}
	}
	summary := fmt.Sprintf("%d 项全部一致", len(items))
	if abnormal > 0 {
		summary = fmt.Sprintf("%d/%d 项需要处理", abnormal, len(items))
	}
	return logHealthLayer(key, name, worstLogHealthStatus(statuses), summary, items)
}

// isOpenSearchNotFound：openSearchRequest 的错误形如 "opensearch 404 Not Found: <body>"。
func isOpenSearchNotFound(err error) bool {
	return strings.HasPrefix(err.Error(), "opensearch 404")
}

func (handler *Handler) OpenSearchLogHealth(context *gin.Context) {
	cluster, err := handler.loadOpenSearchCluster(context)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "OpenSearch cluster not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	layers := []gin.H{
		handler.checkLogIndexTemplate(context, cluster),
		handler.checkLogISMPolicies(context, cluster),
		handler.checkLogPipelines(context, cluster),
		handler.checkLogHostConfigs(context),
		handler.checkLogRuntime(context),
		handler.checkLogDataFlow(context, cluster),
	}
	response.Success(context, gin.H{
		"status":     worstLogHealthStatus(layerStatuses(layers)),
		"checked_at": time.Now().UTC().Format(time.RFC3339),
		"layers":     layers,
	})
}

func layerStatuses(layers []gin.H) []string {
	statuses := make([]string, 0, len(layers))
	for _, layer := range layers {
		statuses = append(statuses, layer["status"].(string))
	}
	return statuses
}

func logHealthPrefix(cluster openSearchCluster) string {
	if strings.TrimSpace(cluster.IndexPrefix) == "" {
		return "logs"
	}
	return cluster.IndexPrefix
}

// checkLogIndexTemplate 模板决定字段类型：dynamic 漏成 true 会把 keyword 建成 text，聚合功能全部失效。
func (handler *Handler) checkLogIndexTemplate(context *gin.Context, cluster openSearchCluster) gin.H {
	prefix := logHealthPrefix(cluster)
	name := buildIndexTemplateName(prefix)
	responseBody, err := handler.openSearchRequest(context, cluster, "GET", "/_index_template/"+safeIndexSegment(prefix), nil)
	if err != nil {
		if isOpenSearchNotFound(err) {
			return logHealthLayer("index_template", "索引模板", logHealthDrift, fmt.Sprintf("模板 %s 不存在，新建索引会走动态映射", name), nil)
		}
		return logHealthLayer("index_template", "索引模板", logHealthError, fmt.Sprintf("读取模板失败: %v", err), nil)
	}
	desiredBody := buildIndexTemplateBody(prefix)
	desiredTemplate := desiredBody["template"].(gin.H)["mappings"].(gin.H)
	actualMappings := map[string]any{}
	if templates, ok := responseBody["index_templates"].([]any); ok && len(templates) > 0 {
		if first, ok := templates[0].(map[string]any); ok {
			if indexTemplate, ok := first["index_template"].(map[string]any); ok {
				if template, ok := indexTemplate["template"].(map[string]any); ok {
					mappings, _ := template["mappings"].(map[string]any)
					actualMappings = mappings
				}
			}
		}
	}
	items := []gin.H{}
	// JSON 反序列化后 dynamic 缺省即 true，与 Django get('dynamic', True) 语义一致。
	desiredDynamic, _ := desiredTemplate["dynamic"].(bool)
	actualDynamic := true
	if value, ok := actualMappings["dynamic"].(bool); ok {
		actualDynamic = value
	}
	if actualDynamic == desiredDynamic {
		items = append(items, logHealthItem("dynamic", logHealthOK, fmt.Sprintf("期望 %v，实际 %v", desiredDynamic, actualDynamic)))
	} else {
		items = append(items, logHealthItem("dynamic", logHealthDrift, fmt.Sprintf("期望 %v，实际 %v", desiredDynamic, actualDynamic)))
	}
	actualProperties, _ := actualMappings["properties"].(map[string]any)
	for field, specRaw := range desiredTemplate["properties"].(gin.H) {
		spec := specRaw.(gin.H)
		expectedType := fmt.Sprint(spec["type"])
		current, _ := actualProperties[field].(map[string]any)
		if current == nil {
			items = append(items, logHealthItem(field, logHealthDrift, fmt.Sprintf("缺失，期望 %s", expectedType)))
			continue
		}
		currentType, _ := current["type"].(string)
		if currentType != expectedType {
			items = append(items, logHealthItem(field, logHealthDrift, fmt.Sprintf("类型不符：期望 %s，实际 %s", expectedType, currentType)))
			continue
		}
		items = append(items, logHealthItem(field, logHealthOK, expectedType))
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return logHealthLayerFromItems("index_template", "索引模板", items, "模板无字段定义")
}

// asStringMap / asAnySlice：gin.H 与 json 反序列化的 map[string]any 是不同动态类型，
// 直接断言会失败，这里统一归一（gin.H 的底层类型就是 map[string]any）。
func asStringMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case gin.H:
		return typed
	default:
		return nil
	}
}

func asAnySlice(value any) []any {
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}

// ismPolicySignature 只提取影响保留行为的字段：集群会补 policy_id/last_updated_time 等元数据，全量比对必然误报。
func ismPolicySignature(policy map[string]any) gin.H {
	signature := gin.H{"rollover_size": nil, "rollover_age": nil, "delete_after": nil, "index_patterns": []string{}}
	hot := map[string]any{}
	for _, stateRaw := range asAnySlice(policy["states"]) {
		state := asStringMap(stateRaw)
		if name, _ := state["name"].(string); name == "hot" {
			hot = state
		}
	}
	for _, actionRaw := range asAnySlice(hot["actions"]) {
		action := asStringMap(actionRaw)
		if rollover := asStringMap(action["rollover"]); rollover != nil {
			signature["rollover_size"] = rollover["min_primary_shard_size"]
			signature["rollover_age"] = rollover["min_index_age"]
			break
		}
	}
	for _, transitionRaw := range asAnySlice(hot["transitions"]) {
		transition := asStringMap(transitionRaw)
		if stateName, _ := transition["state_name"].(string); stateName == "delete" {
			conditions := asStringMap(transition["conditions"])
			signature["delete_after"] = conditions["min_index_age"]
			break
		}
	}
	patterns := []string{}
	templates := asAnySlice(policy["ism_template"])
	if templateDict := asStringMap(policy["ism_template"]); templateDict != nil {
		templates = []any{templateDict}
	}
	for _, templateRaw := range templates {
		template := asStringMap(templateRaw)
		for _, patternRaw := range asAnySlice(template["index_patterns"]) {
			patterns = append(patterns, fmt.Sprint(patternRaw))
		}
	}
	sort.Strings(patterns)
	signature["index_patterns"] = patterns
	return signature
}

// checkLogISMPolicies ISM 缺失时索引既不滚动也不清理，磁盘会被慢慢撑满，属于静默故障。
func (handler *Handler) checkLogISMPolicies(context *gin.Context, cluster openSearchCluster) gin.H {
	prefix := logHealthPrefix(cluster)
	tiers, err := handler.loadEnabledRetentionTiers(context)
	if err != nil {
		return logHealthLayer("ism_policies", "保留策略", logHealthError, "读取保留档位失败: "+err.Error(), nil)
	}
	items := []gin.H{}
	for _, tier := range tiers {
		name := buildISMPolicyName(prefix, tier.Code)
		desired := ismPolicySignature(buildISMPolicyBody(prefix, tier)["policy"].(gin.H))
		remote, err := handler.openSearchRequest(context, cluster, "GET", "/_plugins/_ism/policies/"+name, nil)
		if err != nil {
			if isOpenSearchNotFound(err) {
				items = append(items, logHealthItem(name, logHealthDrift, "策略不存在，索引不会自动滚动与清理"))
			} else {
				items = append(items, logHealthItem(name, logHealthError, truncateOpenSearchError(err)))
			}
			continue
		}
		policy, _ := remote["policy"].(map[string]any)
		if policy == nil {
			policy = map[string]any{}
		}
		actual := ismPolicySignature(policy)
		differing := differingSignatureKeys(actual, desired)
		if len(differing) == 0 {
			items = append(items, logHealthItem(name, logHealthOK, fmt.Sprintf("保留 %d 天", tier.RetentionDays)))
			continue
		}
		items = append(items, logHealthItem(name, logHealthDrift, "与档位配置不一致: "+strings.Join(differing, ", ")))
	}
	return logHealthLayerFromItems("ism_policies", "保留策略", items, "没有启用中的保留档位")
}

func differingSignatureKeys(actual, desired gin.H) []string {
	differing := make([]string, 0, 4)
	for _, key := range []string{"rollover_size", "rollover_age", "delete_after", "index_patterns"} {
		if !sameSignatureValue(actual[key], desired[key]) {
			differing = append(differing, key)
		}
	}
	return differing
}

func sameSignatureValue(actual, desired any) bool {
	switch left := actual.(type) {
	case string:
		right, ok := desired.(string)
		return ok && left == right
	case float64:
		right, ok := desired.(float64)
		return ok && left == right
	case bool:
		right, ok := desired.(bool)
		return ok && left == right
	case nil:
		return desired == nil
	default:
		leftJSON, _ := json.Marshal(actual)
		rightJSON, _ := json.Marshal(desired)
		return string(leftJSON) == string(rightJSON)
	}
}

// checkLogPipelines pipeline 没发布时日志照样写入，只是不被解析，界面上看不出任何异常。
func (handler *Handler) checkLogPipelines(context *gin.Context, cluster openSearchCluster) gin.H {
	type processingRuleRow struct {
		Name         string
		PipelineBody json.RawMessage
	}
	rows, err := handler.db.QueryContext(context, `SELECT name,pipeline_body FROM monitor_log_processing_rule WHERE cluster_id=? ORDER BY name`, cluster.ID)
	if err != nil {
		return logHealthLayer("pipelines", "解析规则", logHealthError, "读取解析规则失败: "+err.Error(), nil)
	}
	rules := make([]processingRuleRow, 0, 8)
	for rows.Next() {
		var rule processingRuleRow
		if err := rows.Scan(&rule.Name, &rule.PipelineBody); err != nil {
			rows.Close()
			return logHealthLayer("pipelines", "解析规则", logHealthError, "读取解析规则失败: "+err.Error(), nil)
		}
		rules = append(rules, rule)
	}
	rows.Close()

	items := []gin.H{}
	for _, rule := range rules {
		remote, err := handler.openSearchRequest(context, cluster, "GET", "/_ingest/pipeline/"+rule.Name, nil)
		if err != nil {
			if isOpenSearchNotFound(err) {
				items = append(items, logHealthItem(rule.Name, logHealthDrift, "集群上不存在该 pipeline，日志不会被解析"))
			} else {
				items = append(items, logHealthItem(rule.Name, logHealthError, truncateOpenSearchError(err)))
			}
			continue
		}
		body, _ := remote[rule.Name].(map[string]any)
		if body == nil {
			items = append(items, logHealthItem(rule.Name, logHealthDrift, "集群上不存在该 pipeline，日志不会被解析"))
		} else if pipelineSignature(body) != pipelineSignature(json.RawMessage(rule.PipelineBody)) {
			items = append(items, logHealthItem(rule.Name, logHealthDrift, "集群上的 pipeline 与页面配置不一致，需重新发布"))
		} else {
			var decoded map[string]any
			_ = json.Unmarshal(rule.PipelineBody, &decoded)
			processors, _ := decoded["processors"].([]any)
			items = append(items, logHealthItem(rule.Name, logHealthOK, fmt.Sprintf("%d 个处理器", len(processors))))
		}
	}
	return logHealthLayerFromItems("pipelines", "解析规则", items, "尚未配置解析规则")
}

// checkLogHostConfigs 比对数据库记录，暴露未安装/未下发配置的主机。
// 注意：Go 侧 Fluent Bit 配置由 agent 渲染（configure_fluent_bit_opensearch），
// 后端没有期望指纹生成器，因此这里只判断「已下发与否」，不比对内容一致性；
// 主机被人手工改过的情况由 agent 侧上报与数据流层兜底。
func (handler *Handler) checkLogHostConfigs(context *gin.Context) gin.H {
	type targetRow struct {
		ID               int64
		HostIP           string
		AgentInstalled   bool
		ConfigFingerprint string
	}
	rows, err := handler.db.QueryContext(context, `SELECT l.id,COALESCE(h.ip,''),l.agent_installed,COALESCE(l.config_fingerprint,'') FROM monitor_log_collection_target l JOIN assets_host h ON h.id=l.host_id WHERE l.managed_enabled=TRUE ORDER BY l.id`)
	if err != nil {
		return logHealthLayer("host_configs", "主机配置", logHealthError, "读取采集目标失败: "+err.Error(), nil)
	}
	targets := make([]targetRow, 0, 16)
	for rows.Next() {
		var target targetRow
		if err := rows.Scan(&target.ID, &target.HostIP, &target.AgentInstalled, &target.ConfigFingerprint); err != nil {
			rows.Close()
			return logHealthLayer("host_configs", "主机配置", logHealthError, "读取采集目标失败: "+err.Error(), nil)
		}
		targets = append(targets, target)
	}
	rows.Close()

	items := []gin.H{}
	for _, target := range targets {
		label := target.HostIP
		if label == "" {
			label = fmt.Sprintf("host-%d", target.ID)
		}
		if !target.AgentInstalled {
			items = append(items, logHealthItem(label, logHealthWarn, "Fluent Bit 未安装"))
			continue
		}
		if strings.TrimSpace(target.ConfigFingerprint) == "" {
			items = append(items, logHealthItem(label, logHealthDrift, "从未下发过采集配置"))
			continue
		}
		items = append(items, logHealthItem(label, logHealthOK, "已下发采集配置"))
	}
	return logHealthLayerFromItems("host_configs", "主机配置", items, "没有纳管中的采集目标")
}

// checkLogRuntime 运行状态取自数据库缓存，反映最近一次探测结果，不是实时探活。
func (handler *Handler) checkLogRuntime(context *gin.Context) gin.H {
	type targetRow struct {
		ID         int64
		HostIP     string
		Runtime    string
		LastError  string
	}
	rows, err := handler.db.QueryContext(context, `SELECT l.id,COALESCE(h.ip,''),COALESCE(l.runtime_status,''),COALESCE(l.last_error,'') FROM monitor_log_collection_target l JOIN assets_host h ON h.id=l.host_id WHERE l.managed_enabled=TRUE AND l.agent_installed=TRUE ORDER BY l.id`)
	if err != nil {
		return logHealthLayer("runtime", "采集进程", logHealthError, "读取采集目标失败: "+err.Error(), nil)
	}
	targets := make([]targetRow, 0, 16)
	for rows.Next() {
		var target targetRow
		if err := rows.Scan(&target.ID, &target.HostIP, &target.Runtime, &target.LastError); err != nil {
			rows.Close()
			return logHealthLayer("runtime", "采集进程", logHealthError, "读取采集目标失败: "+err.Error(), nil)
		}
		targets = append(targets, target)
	}
	rows.Close()

	items := []gin.H{}
	for _, target := range targets {
		label := target.HostIP
		if label == "" {
			label = fmt.Sprintf("host-%d", target.ID)
		}
		switch target.Runtime {
		case "running":
			items = append(items, logHealthItem(label, logHealthOK, "运行中"))
		case "error":
			message := target.LastError
			if strings.TrimSpace(message) == "" {
				message = "异常"
			}
			if len(message) > 200 {
				message = message[:200]
			}
			items = append(items, logHealthItem(label, logHealthError, message))
		case "stopped":
			items = append(items, logHealthItem(label, logHealthDrift, "已停止"))
		default:
			items = append(items, logHealthItem(label, logHealthWarn, "状态未知，需刷新"))
		}
	}
	return logHealthLayerFromItems("runtime", "采集进程", items, "没有已安装 Fluent Bit 的主机")
}

// checkLogDataFlow 前面几层全绿也可能没数据，这一层是唯一能证明链路真正通了的证据。
func (handler *Handler) checkLogDataFlow(context *gin.Context, cluster openSearchCluster) gin.H {
	prefix := logHealthPrefix(cluster)
	body := gin.H{
		"size":  0,
		"query": gin.H{"range": gin.H{"@timestamp": gin.H{"gte": fmt.Sprintf("now-%dm", logHealthDataFlowWindowMinutes)}}},
		"aggs":  gin.H{"by_service": gin.H{"terms": gin.H{"field": "service", "size": 50}}},
	}
	result, err := handler.openSearchRequest(context, cluster, "POST", "/"+prefix+"-*/_search", body)
	if err != nil {
		return logHealthLayer("data_flow", "数据写入", logHealthError, fmt.Sprintf("查询失败: %v", err), nil)
	}
	total := 0.0
	if hits, ok := result["hits"].(map[string]any); ok {
		if totalRaw, ok := hits["total"].(map[string]any); ok {
			total, _ = totalRaw["value"].(float64)
		}
	}
	buckets := []any{}
	if aggregations, ok := result["aggregations"].(map[string]any); ok {
		if byService, ok := aggregations["by_service"].(map[string]any); ok {
			buckets, _ = byService["buckets"].([]any)
		}
	}
	items := []gin.H{}
	for _, bucketRaw := range buckets {
		bucket, _ := bucketRaw.(map[string]any)
		name, _ := bucket["key"].(string)
		count, _ := bucket["doc_count"].(float64)
		items = append(items, logHealthItem(name, logHealthOK, fmt.Sprintf("%.0f 条", count)))
	}
	if total == 0 {
		return logHealthLayer("data_flow", "数据写入", logHealthWarn, fmt.Sprintf("最近 %d 分钟没有新日志写入", logHealthDataFlowWindowMinutes), items)
	}
	return logHealthLayer("data_flow", "数据写入", logHealthOK, fmt.Sprintf("最近 %d 分钟写入 %.0f 条，覆盖 %d 个服务", logHealthDataFlowWindowMinutes, total, len(items)), items)
}
