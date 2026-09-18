package logcollect

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

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

// isElasticsearchNotFound：elasticsearchRequest 的错误形如 "elasticsearch 404 Not Found: <body>"。
func isElasticsearchNotFound(err error) bool {
	return strings.HasPrefix(err.Error(), "elasticsearch 404")
}

func (handler *Handler) ElasticsearchLogHealth(context *gin.Context) {
	cluster, err := handler.loadElasticsearchCluster(context)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "Elasticsearch cluster not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	layers := []gin.H{
		handler.checkLogIndexTemplate(context, cluster),
		handler.checkLogRetentionPolicies(context, cluster),
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

func logHealthPrefix(cluster elasticsearchCluster) string {
	if strings.TrimSpace(cluster.IndexPrefix) == "" {
		return "autoadmin"
	}
	return cluster.IndexPrefix
}

// checkLogIndexTemplate 模板决定字段类型：dynamic 漏成 true 会把 keyword 建成 text，聚合功能全部失效。
func (handler *Handler) checkLogIndexTemplate(context *gin.Context, cluster elasticsearchCluster) gin.H {
	prefix := logHealthPrefix(cluster)
	name := buildIndexTemplateName(prefix)
	responseBody, err := handler.elasticsearchRequest(context, cluster, "GET", "/_index_template/"+name, nil)
	if err != nil {
		if isElasticsearchNotFound(err) {
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

// ilmPolicySignature 只提取影响保留行为的字段：ES 会补 hot.min_age=0ms、delete_searchable_snapshot
// 等默认值，全量比对必然误报。
func ilmPolicySignature(policy map[string]any) gin.H {
	signature := gin.H{"rollover_size": nil, "rollover_age": nil, "delete_after": nil}
	phases := asStringMap(policy["phases"])
	if phases == nil {
		return signature
	}
	if hot := asStringMap(phases["hot"]); hot != nil {
		if rollover := asStringMap(asStringMap(hot["actions"])["rollover"]); rollover != nil {
			signature["rollover_size"] = rollover["max_primary_shard_size"]
			signature["rollover_age"] = rollover["max_age"]
		}
	}
	if del := asStringMap(phases["delete"]); del != nil {
		signature["delete_after"] = del["min_age"]
	}
	return signature
}

// checkLogRetentionPolicies ILM 策略缺失时索引既不滚动也不清理，磁盘会被慢慢撑满，属于静默故障。
// GET `_ilm/policy/<name>` 的响应形如 {<name>:{version,modified_date,policy:{phases:...},in_use_by:{}}}。
func (handler *Handler) checkLogRetentionPolicies(context *gin.Context, cluster elasticsearchCluster) gin.H {
	prefix := logHealthPrefix(cluster)
	tiers, err := handler.loadEnabledRetentionTiers(context)
	if err != nil {
		return logHealthLayer("retention_policies", "保留策略", logHealthError, "读取保留档位失败: "+err.Error(), nil)
	}
	items := []gin.H{}
	for _, tier := range tiers {
		name := buildILMPolicyName(prefix, tier.Code)
		desired := ilmPolicySignature(buildILMPolicyBody(prefix, tier)["policy"].(gin.H))
		remote, err := handler.elasticsearchRequest(context, cluster, "GET", "/_ilm/policy/"+name, nil)
		if err != nil {
			if isElasticsearchNotFound(err) {
				items = append(items, logHealthItem(name, logHealthDrift, "策略不存在，索引不会自动滚动与清理"))
			} else {
				items = append(items, logHealthItem(name, logHealthError, truncateElasticsearchError(err)))
			}
			continue
		}
		policy, _ := asStringMap(remote[name])["policy"].(map[string]any)
		if policy == nil {
			policy = map[string]any{}
		}
		actual := ilmPolicySignature(policy)
		differing := differingSignatureKeys(actual, desired)
		if len(differing) == 0 {
			items = append(items, logHealthItem(name, logHealthOK, fmt.Sprintf("保留 %d 天", tier.RetentionDays)))
			continue
		}
		items = append(items, logHealthItem(name, logHealthDrift, "与档位配置不一致: "+strings.Join(differing, ", ")))
	}
	return logHealthLayerFromItems("retention_policies", "保留策略", items, "没有启用中的保留档位")
}

func differingSignatureKeys(actual, desired gin.H) []string {
	differing := make([]string, 0, 4)
	for _, key := range []string{"rollover_size", "rollover_age", "delete_after"} {
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
func (handler *Handler) checkLogPipelines(context *gin.Context, cluster elasticsearchCluster) gin.H {
	rules, err := db.New(handler.db).ListProcessingRulesByCluster(context, cluster.ID)
	if err != nil {
		return logHealthLayer("pipelines", "解析规则", logHealthError, "读取解析规则失败: "+err.Error(), nil)
	}

	items := []gin.H{}
	for _, rule := range rules {
		// pipeline id 与发布/删除/渲染统一：<前缀>-<应用 code|general>-<规则名>。
		pipelineName := processingPipelineName(logHealthPrefix(cluster), handler.applicationPipelineSegment(context, rule.ApplicationID), rule.Name)
		remote, err := handler.elasticsearchRequest(context, cluster, "GET", "/_ingest/pipeline/"+pipelineName, nil)
		if err != nil {
			if isElasticsearchNotFound(err) {
				items = append(items, logHealthItem(pipelineName, logHealthDrift, "集群上不存在该 pipeline，日志不会被解析"))
			} else {
				items = append(items, logHealthItem(pipelineName, logHealthError, truncateElasticsearchError(err)))
			}
			continue
		}
		body, _ := remote[pipelineName].(map[string]any)
		if body == nil {
			items = append(items, logHealthItem(pipelineName, logHealthDrift, "集群上不存在该 pipeline，日志不会被解析"))
		} else if pipelineSignature(body) != pipelineSignature(json.RawMessage(rule.PipelineBody)) {
			items = append(items, logHealthItem(pipelineName, logHealthDrift, "集群上的 pipeline 与页面配置不一致，需重新发布"))
		} else {
			var decoded map[string]any
			_ = json.Unmarshal(rule.PipelineBody, &decoded)
			processors, _ := decoded["processors"].([]any)
			items = append(items, logHealthItem(pipelineName, logHealthOK, fmt.Sprintf("%d 个处理器", len(processors))))
		}
	}
	return logHealthLayerFromItems("pipelines", "解析规则", items, "尚未配置解析规则")
}

// checkLogHostConfigs 逐主机比对**期望配置与已下发配置**：后端实时渲染采集配置算出期望指纹，
// 与 monitor_log_collection_target.config_fingerprint 比对，暴露"从未下发"与"配置已变更未下发"。
//
// 此前这一层只看指纹是否为空、不比对内容，所以主机上的配置过期（改了路径/档位/日志开关，
// 或换了默认集群）在体检里完全看不出来。渲染是纯函数，偏差由数据流层与主机侧共同兜底。
func (handler *Handler) checkLogHostConfigs(context *gin.Context) gin.H {
	targets, err := db.New(handler.db).ListManagedLogTargetConfigs(context)
	if err != nil {
		return logHealthLayer("host_configs", "主机配置", logHealthError, "读取采集目标失败: "+err.Error(), nil)
	}

	// 一次批量评估覆盖全部纳管目标（不逐台查库/渲染）。期望配置算不出来时（典型是
	// 没有启用的默认集群）退回"是否下发过"的判断，并把原因写进明细，不谎报"一致"。
	refs := make([]LogConfigTargetRef, 0, len(targets))
	for _, target := range targets {
		refs = append(refs, LogConfigTargetRef{HostID: target.HostID, AppliedFingerprint: target.ConfigFingerprint})
	}
	states, stateErr := handler.EvaluateLogConfigStates(context, refs)

	items := []gin.H{}
	for _, target := range targets {
		label := target.Ip
		if label == "" {
			label = fmt.Sprintf("host-%d", target.HostID)
		}
		if !target.AgentInstalled {
			items = append(items, logHealthItem(label, logHealthWarn, "Filebeat 未安装"))
			continue
		}
		if stateErr != nil {
			reason := "无法比对采集配置内容：" + stateErr.Error()
			if strings.TrimSpace(target.ConfigFingerprint) == "" {
				items = append(items, logHealthItem(label, logHealthDrift, "从未下发过采集配置（"+reason+"）"))
			} else {
				items = append(items, logHealthItem(label, logHealthWarn, reason))
			}
			continue
		}
		switch states[target.HostID].Status {
		case LogConfigSynced:
			items = append(items, logHealthItem(label, logHealthOK, hostConfigSyncedDetail(states[target.HostID])))
		case LogConfigNever:
			items = append(items, logHealthItem(label, logHealthDrift, "从未下发过采集配置"))
		default:
			items = append(items, logHealthItem(label, logHealthDrift, "配置已变更，主机上的采集配置已过期，需重新下发"))
		}
	}
	return logHealthLayerFromItems("host_configs", "主机配置", items, "没有纳管中的采集目标")
}

// hostConfigSyncedDetail 生成"配置一致"的明细：覆盖的服务数 + 渲染告警
// （未关联处理规则、路径宏展不开等会让片段被跳过，与主机是否上报配置无关，必须显式提示）。
func hostConfigSyncedDetail(state LogConfigState) string {
	detail := fmt.Sprintf("已下发采集配置（%d 个服务）", state.ServiceNum)
	if len(state.Warnings) > 0 {
		detail += "；" + strings.Join(state.Warnings, "；")
	}
	return detail
}

// checkLogRuntime 运行状态取自数据库缓存，反映最近一次探测结果，不是实时探活。
func (handler *Handler) checkLogRuntime(context *gin.Context) gin.H {
	targets, err := db.New(handler.db).ListInstalledLogTargetRuntime(context)
	if err != nil {
		return logHealthLayer("runtime", "采集进程", logHealthError, "读取采集目标失败: "+err.Error(), nil)
	}

	items := []gin.H{}
	for _, target := range targets {
		label := target.Ip
		if label == "" {
			label = fmt.Sprintf("host-%d", target.ID)
		}
		switch target.RuntimeStatus {
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
	return logHealthLayerFromItems("runtime", "采集进程", items, "没有已安装 Filebeat 的主机")
}

// checkLogDataFlow 前面几层全绿也可能没数据，这一层是唯一能证明链路真正通了的证据。
func (handler *Handler) checkLogDataFlow(context *gin.Context, cluster elasticsearchCluster) gin.H {
	prefix := logHealthPrefix(cluster)
	body := gin.H{
		"size":  0,
		"query": gin.H{"range": gin.H{"@timestamp": gin.H{"gte": fmt.Sprintf("now-%dm", logHealthDataFlowWindowMinutes)}}},
		"aggs":  gin.H{"by_service": gin.H{"terms": gin.H{"field": "service", "size": 50}}},
	}
	result, err := handler.elasticsearchRequest(context, cluster, "POST", "/"+prefix+"-*/_search", body)
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
