package logcollect

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// Elasticsearch 日志存储管理面：索引命名、index template、ILM policy、ingest pipeline
// 的期望态构建与下发编排，对应 Django 时代的同名模块（源码已移出版本库）。
// 构建器全部为纯函数便于单测；bootstrapElasticsearchStorage 是唯一执行写入的编排入口。

var indexSegmentPattern = regexp.MustCompile(`[^a-z0-9_-]+`)

// safeIndexSegment 索引名段只允许小写字母/数字/下划线/连字符，避免非法索引名。
func safeIndexSegment(value string) string {
	segment := strings.Trim(indexSegmentPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-"), "-")
	if segment == "" {
		return "unknown"
	}
	return segment
}

func buildIndexTemplateName(indexPrefix string) string {
	return safeIndexSegment(indexPrefix) + "-template"
}

// safeOptionalSegment 与 safeIndexSegment 类似，但空值返回空串（不兜底 unknown），
// 便于调用方自行决定默认段。
func safeOptionalSegment(value string) string {
	return strings.Trim(indexSegmentPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-"), "-")
}

// processingPipelineName ES ingest pipeline id：<索引前缀(默认 autoadmin)>-<应用 code 或 general>-<规则名>。
// 采集端 Filebeat 的 pipeline 字段、后端发布/删除、链路体检都必须用同一个名字，禁止各自拼接。
func processingPipelineName(prefix, application, rule string) string {
	prefixSegment := safeOptionalSegment(prefix)
	if prefixSegment == "" {
		prefixSegment = "autoadmin"
	}
	applicationSegment := safeOptionalSegment(application)
	if applicationSegment == "" {
		applicationSegment = "general"
	}
	ruleSegment := safeOptionalSegment(rule)
	if ruleSegment == "" {
		ruleSegment = "rule"
	}
	return strings.Join([]string{prefixSegment, applicationSegment, ruleSegment}, "-")
}

// buildIndexTemplateBody data stream 模板：单分片、限制字段总数、标准字段之外不再自动建 mapping。
// 字段契约与 Django log_schema.STANDARD_LOG_FIELDS 一致（2026-09-19 起删掉没人读的 log_time，
// 并加一个平台标记字段 mapping_violation，见 buildMappingGuardPipelineBody）。
func buildIndexTemplateBody(indexPrefix string) gin.H {
	prefix := safeIndexSegment(indexPrefix)
	return gin.H{
		"index_patterns": []string{prefix + "-*"},
		"data_stream":    gin.H{},
		"template": gin.H{
			"settings": gin.H{
				"number_of_shards":                 1,
				"number_of_replicas":               0,
				"index.refresh_interval":           "10s",
				"index.mapping.total_fields.limit": 2000,
				// 平台级"必备字段"闸门：所有写入这些流的文档最后都过它。
				// 挂在 final_pipeline 而不是 default_pipeline，是因为规则自己的 pipeline 是
				// Filebeat 片段里显式指定的（`pipeline:`），final_pipeline 在它之后执行，
				// 因此能判定规则产出的最终结果；规则作者改不到这里。
				"index.final_pipeline": buildMappingGuardPipelineName(prefix),
			},
			"mappings": gin.H{
				"dynamic":    false,
				"properties": standardLogFields,
			},
		},
	}
}

// mappingGuardMode 的取值（配置项 LOG_MAPPING_GUARD_MODE）：
//   - tag（默认）：不丢数据，给不齐必备字段的文档打 mapping_violation 标记 + 记录缺哪些字段，
//     由检索侧默认排除、并由巡检/告警看见；
//   - drop：直接丢弃不齐的文档（严格"必须满足"，代价是不可逆地丢日志，且丢弃后 ES 里不留痕迹，
//     所以切换前必须先看「处理规则巡检」里哪些规则不满足必备字段）。
const (
	mappingGuardModeTag  = "tag"
	mappingGuardModeDrop = "drop"
)

func buildMappingGuardPipelineName(indexPrefix string) string {
	return safeIndexSegment(indexPrefix) + "-mapping-guard"
}

// buildMappingGuardPipelineBody 生成平台级必备字段校验 pipeline（幂等 PUT，由 bootstrap 写入）。
// 判定条件直接由 requiredProcessingRuleOutputs 拼出来，与规则保存校验、调试页 missing_fields
// 共用同一份定义，不会出现"校验说齐了、guard 说没齐"。
func buildMappingGuardPipelineBody(mode string) gin.H {
	conditions := make([]string, 0, len(requiredProcessingRuleOutputs))
	for _, field := range requiredProcessingRuleOutputs {
		conditions = append(conditions, fmt.Sprintf("ctx.%s == null", field))
	}
	missing := "[" + strings.Join(quoteAll(requiredProcessingRuleOutputs), ",") + "]"
	processors := []any{
		// app_fields 是索引模板里的字段，规则没往它写时兜一个空对象，让"字段齐"的口径干净。
		gin.H{"set": gin.H{"field": "app_fields", "value": gin.H{}, "override": false, "if": "ctx.app_fields == null"}},
	}
	if strings.EqualFold(strings.TrimSpace(mode), mappingGuardModeDrop) {
		processors = append(processors, gin.H{"drop": gin.H{
			"if":             strings.Join(conditions, " || "),
			"description":    "必备字段不齐，按 LOG_MAPPING_GUARD_MODE=drop 丢弃",
			"tag":            "mapping_guard",
		}})
	} else {
		processors = append(processors, gin.H{"set": gin.H{
			"field":    "mapping_violation",
			"value":    true,
			"override": false,
			"if":       "(" + strings.Join(conditions, " || ") + ") && ctx.mapping_violation == null",
			"description": "必备字段不齐，打标但不丢弃（LOG_MAPPING_GUARD_MODE=tag）",
		}})
	}
	return gin.H{"processors": processors, "description": "平台级必备字段校验（" + missing + " 少一个都不行）"}
}

// quoteAll 给字段名加引号，只用于 pipeline 的描述文案。
func quoteAll(values []string) []string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, `"`+value+`"`)
	}
	return quoted
}

var standardLogFields = gin.H{
	"@timestamp":        gin.H{"type": "date"},
	"message":           gin.H{"type": "text"},
	"project":           gin.H{"type": "keyword"},
	"business_system":   gin.H{"type": "keyword"},
	"environment":       gin.H{"type": "keyword"},
	"service":           gin.H{"type": "keyword"},
	"application":       gin.H{"type": "keyword"},
	"instance":          gin.H{"type": "keyword"},
	"host_ip":           gin.H{"type": "keyword"},
	"log_name":          gin.H{"type": "keyword"},
	"log_path":          gin.H{"type": "keyword"},
	"log_level":         gin.H{"type": "keyword"},
	"log_message":       gin.H{"type": "text"},
	"error_fingerprint": gin.H{"type": "keyword"},
	// 平台标记字段（不是规则产出要求）：LOG_MAPPING_GUARD_MODE=tag 时，必备字段不齐的文档
	// 会被打上它，检索侧据此默认排除。放进 mapping 是因为 dynamic:false 下未声明的字段会被丢弃。
	"mapping_violation": gin.H{"type": "boolean"},
	// flat_object 是 Elasticsearch 专有类型；Elasticsearch 用 flattened（8.x 自带，无需额外插件）。
	"app_fields": gin.H{"type": "flattened"},
}

// GetElasticsearchIndexTemplate 展示集群实际的索引模板 mapping（日志存储页「查看 Mapping」）。
// 读不到模板时回退内置标准字段并标记 exists=false，便于对比"期望 vs 实际"。
func (handler *Handler) GetElasticsearchIndexTemplate(context *gin.Context) {
	cluster, err := handler.loadElasticsearchCluster(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	prefix := strings.TrimSpace(cluster.IndexPrefix)
	if prefix == "" {
		prefix = "autoadmin"
	}
	name := buildIndexTemplateName(prefix)
	result := gin.H{
		"template_name":  name,
		"index_prefix":   prefix,
		"exists":         false,
		"index_patterns": []string{},
		"dynamic":        nil,
		"fields":         fieldRowsFromProperties(standardLogFields),
	}
	if payload, err := handler.elasticsearchRequest(context, cluster, "GET", "/_index_template/"+name, nil); err == nil {
		if definition, ok := firstIndexTemplateDefinition(payload); ok {
			result["exists"] = true
			if patterns, ok := definition["index_patterns"].([]any); ok {
				result["index_patterns"] = patterns
			}
			if properties, ok := indexTemplateProperties(definition); ok {
				result["fields"] = fieldRowsFromProperties(properties)
			}
			if template, ok := definition["template"].(map[string]any); ok {
				if mappings, ok := template["mappings"].(map[string]any); ok {
					result["dynamic"] = mappings["dynamic"]
				}
			}
		}
	}
	response.Success(context, result)
}

// firstIndexTemplateDefinition 从 GET /_index_template/<name> 响应取第一个 index_template 定义。
func firstIndexTemplateDefinition(payload map[string]any) (map[string]any, bool) {
	templates, _ := payload["index_templates"].([]any)
	if len(templates) == 0 {
		return nil, false
	}
	first, _ := templates[0].(map[string]any)
	definition, _ := first["index_template"].(map[string]any)
	if definition == nil {
		return nil, false
	}
	return definition, true
}

// indexTemplateProperties 取 index_template.template.mappings.properties。
func indexTemplateProperties(definition map[string]any) (map[string]any, bool) {
	template, _ := definition["template"].(map[string]any)
	mappings, _ := template["mappings"].(map[string]any)
	properties, _ := mappings["properties"].(map[string]any)
	if properties == nil {
		return nil, false
	}
	return properties, true
}

// fieldRowsFromProperties 把 mapping.properties 拍平成按字段名排序的 [{name,type}]，供前端表格展示。
func fieldRowsFromProperties(properties map[string]any) []gin.H {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]gin.H, 0, len(names))
	for _, name := range names {
		fieldType := ""
		if definition, ok := properties[name].(map[string]any); ok {
			if value, ok := definition["type"].(string); ok {
				fieldType = value
			}
		}
		rows = append(rows, gin.H{"name": name, "type": fieldType})
	}
	return rows
}

func buildILMPolicyName(indexPrefix, tierCode string) string {
	return safeIndexSegment(indexPrefix) + "-" + safeIndexSegment(tierCode) + "-retention"
}

// buildTierIndexTemplateName 每档位一个索引模板，负责把该档位的 ILM 策略绑到匹配的 data stream。
// ELM/ILM 没有 ILM 的 ism_template 自动挂载，只能在索引模板里写 index.lifecycle.name。
func buildTierIndexTemplateName(indexPrefix, tierCode string) string {
	return safeIndexSegment(indexPrefix) + "-" + safeIndexSegment(tierCode) + "-template"
}

type retentionTierRow struct {
	Code                string
	RetentionDays       int64
	DailySizeGB         float64
	RolloverMinIndexAge string
}

// buildILMPolicyBody 按档位生成 Elasticsearch ILM policy。
// 滚动阈值不小于 1gb（避免小档位产生大量碎索引）；删除阶段 min_age = 保留天数。
func buildILMPolicyBody(indexPrefix string, tier retentionTierRow) gin.H {
	rolloverSize := fmt.Sprintf("%dgb", int(math.Max(1, math.Round(tier.DailySizeGB))))
	rollover := gin.H{"max_primary_shard_size": rolloverSize}
	if age := strings.TrimSpace(tier.RolloverMinIndexAge); age != "" {
		rollover["max_age"] = age
	}
	return gin.H{"policy": gin.H{
		"phases": gin.H{
			"hot": gin.H{
				"actions": gin.H{"rollover": rollover},
			},
			"delete": gin.H{
				"min_age": fmt.Sprintf("%dd", tier.RetentionDays),
				"actions": gin.H{"delete": gin.H{}},
			},
		},
	}}
}

// buildTierIndexTemplateBody 在基础模板之上叠加「按档位后缀匹配 + 绑定 ILM 策略」。
// 必须是自包含模板（同时带 data_stream/settings/mappings）：实测 ES 8.13 下高优先级模板
// 与基础模板重叠时不会把基础模板的 mappings/settings 合并进 data stream，缺了会丢字段映射。
func buildTierIndexTemplateBody(indexPrefix string, tier retentionTierRow) gin.H {
	prefix := safeIndexSegment(indexPrefix)
	code := safeIndexSegment(tier.Code)
	body := buildIndexTemplateBody(indexPrefix)
	body["index_patterns"] = []string{fmt.Sprintf("%s-*-%s", prefix, code)}
	body["priority"] = 200
	settings := body["template"].(gin.H)["settings"].(gin.H)
	settings["index.lifecycle.name"] = buildILMPolicyName(indexPrefix, tier.Code)
	return body
}

// pipelineSignature 只取行为字段：processors 有序直接序列化，描述等非行为字段不参与比对。
// encoding/json 对 map 按 key 排序输出，与 Django json.dumps(sort_keys=True) 一致。
func pipelineSignature(body any) string {
	var decoded map[string]any
	switch typed := body.(type) {
	case map[string]any:
		decoded = typed
	case gin.H:
		decoded = typed
	case json.RawMessage:
		_ = json.Unmarshal(typed, &decoded)
	case []byte:
		_ = json.Unmarshal(typed, &decoded)
	}
	processors, onFailure := []any{}, []any{}
	if decoded != nil {
		if items, ok := decoded["processors"].([]any); ok {
			processors = items
		}
		if items, ok := decoded["on_failure"].([]any); ok {
			onFailure = items
		}
	}
	encoded, _ := json.Marshal(gin.H{"processors": processors, "on_failure": onFailure})
	return string(encoded)
}

// indexPatternsOverlap 判断两个索引 pattern 是否可能匹配同一索引：去掉通配符后互为前缀即视为重叠。
func indexPatternsOverlap(left, right string) bool {
	a := strings.ReplaceAll(strings.TrimSpace(left), "*", "")
	b := strings.ReplaceAll(strings.TrimSpace(right), "*", "")
	if a == "" || b == "" {
		return true
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// splitCatPatterns 解析 _cat/templates 的 index_patterns（形如 "[logs*, logs-*]"）为列表。
func splitCatPatterns(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	result := []string{}
	for _, pattern := range strings.Split(raw, ",") {
		if pattern = strings.TrimSpace(pattern); pattern != "" {
			result = append(result, pattern)
		}
	}
	return result
}

// conflictingIndexTemplates 查同 priority 且 index_patterns 与本模板重叠的已有模板（排除自身）。
// ES 本身也会 400，但错误体很长；提前给出简明报错，让用户直接删掉冲突模板或调整其 priority。
func (handler *Handler) conflictingIndexTemplates(context *gin.Context, cluster elasticsearchCluster, selfName string, patterns []string, priority int) []string {
	rows, err := handler.elasticsearchRequestArray(context, cluster, "GET", "/_cat/templates?format=json&h=name,index_patterns,priority")
	if err != nil {
		return nil
	}
	conflicts := []string{}
	for _, row := range rows {
		name := catString(row, "name")
		if name == "" || name == selfName {
			continue
		}
		candidatePriority := 0
		if raw, ok := row["priority"]; ok {
			candidatePriority, _ = strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw)))
		}
		if candidatePriority != priority {
			continue
		}
		for _, candidate := range splitCatPatterns(catString(row, "index_patterns")) {
			for _, want := range patterns {
				if indexPatternsOverlap(candidate, want) {
					conflicts = append(conflicts, fmt.Sprintf("%s (index_patterns=%s, priority=%d)", name, candidate, priority))
				}
			}
		}
	}
	return conflicts
}

// bootstrapElasticsearchStorage 确保基础索引模板、各档位的 ILM policy 与档位索引模板存在（幂等）。
func (handler *Handler) bootstrapElasticsearchStorage(context *gin.Context, cluster elasticsearchCluster) error {
	prefix := cluster.IndexPrefix
	if strings.TrimSpace(prefix) == "" {
		prefix = "autoadmin"
	}
	// 模板名必须是 `<prefix>-template`（与 Django / LOG_COLLECTION_ARCHITECTURE §4.4 一致）。
	// 早期版本误用了不带后缀的 `<prefix>`，导致健康检查查 `logs-template` 却报了另一个名字；
	// 这里同时清理那个错名模板，避免同一 index_patterns 挂着两份模板。
	baseName, basePatterns := buildIndexTemplateName(prefix), []string{prefix + "-*"}
	// 必备字段 guard 必须先于引用它的索引模板写入（模板 settings 里挂了 final_pipeline）。
	if _, err := handler.elasticsearchRequest(context, cluster, "PUT",
		"/_ingest/pipeline/"+buildMappingGuardPipelineName(prefix), buildMappingGuardPipelineBody(handler.mappingGuardMode())); err != nil {
		return err
	}
	if conflicts := handler.conflictingIndexTemplates(context, cluster, baseName, basePatterns, 0); len(conflicts) > 0 {
		return fmt.Errorf("存在与索引模板 %s（priority=0）冲突的模板：%s；请删除该模板或调整其 priority 后重试",
			baseName, strings.Join(conflicts, "; "))
	}
	if _, err := handler.elasticsearchRequest(context, cluster, "PUT", "/_index_template/"+baseName, buildIndexTemplateBody(prefix)); err != nil {
		return err
	}
	handler.cleanupLegacyIndexTemplate(context, cluster, prefix)
	tiers, err := handler.loadEnabledRetentionTiers(context)
	if err != nil {
		return err
	}
	for _, tier := range tiers {
		// PUT _ilm/policy 是幂等覆盖（不像 ISM 需要 if_seq_no 乐观锁）。
		if _, err := handler.elasticsearchRequest(context, cluster, "PUT", "/_ilm/policy/"+buildILMPolicyName(prefix, tier.Code), buildILMPolicyBody(prefix, tier)); err != nil {
			return err
		}
		// 档位模板负责把 ILM 策略绑到 `logs-*-<tier>` 的 data stream。
		tierName, tierPatterns := buildTierIndexTemplateName(prefix, tier.Code), []string{fmt.Sprintf("%s-*-%s", safeIndexSegment(prefix), safeIndexSegment(tier.Code))}
		if conflicts := handler.conflictingIndexTemplates(context, cluster, tierName, tierPatterns, 200); len(conflicts) > 0 {
			return fmt.Errorf("存在与索引模板 %s（priority=200）冲突的模板：%s；请删除该模板或调整其 priority 后重试",
				tierName, strings.Join(conflicts, "; "))
		}
		if _, err := handler.elasticsearchRequest(context, cluster, "PUT", "/_index_template/"+tierName, buildTierIndexTemplateBody(prefix, tier)); err != nil {
			return err
		}
	}
	return nil
}

// cleanupLegacyIndexTemplate 删除历史版本用错名字（`<prefix>` 而非 `<prefix>-template`）建的模板。
// 只在该名字确实存在、且 index_patterns 命中 `<prefix>-*` 时才删，避免误删恰好同名的其它模板。
// 失败不影响 bootstrap 结果（`logs-template` 已写入，且同优先级下后建者生效）。
func (handler *Handler) cleanupLegacyIndexTemplate(context *gin.Context, cluster elasticsearchCluster, prefix string) {
	legacy := safeIndexSegment(prefix)
	if legacy == buildIndexTemplateName(prefix) {
		return
	}
	existing, err := handler.elasticsearchRequest(context, cluster, "GET", "/_index_template/"+legacy, nil)
	if err != nil || !indexTemplateMatchesPrefix(existing, legacy) {
		return
	}
	_, _ = handler.elasticsearchRequest(context, cluster, "DELETE", "/_index_template/"+legacy, nil)
}

// indexTemplateMatchesPrefix 判断 GET /_index_template/<name> 的响应里 pattern 是否正好是 `<prefix>-*`。
func indexTemplateMatchesPrefix(payload map[string]any, prefix string) bool {
	templates, _ := payload["index_templates"].([]any)
	if len(templates) == 0 {
		return false
	}
	first, _ := templates[0].(map[string]any)
	definition, _ := first["index_template"].(map[string]any)
	patterns, _ := definition["index_patterns"].([]any)
	want := prefix + "-*"
	for _, pattern := range patterns {
		if fmt.Sprint(pattern) == want {
			return true
		}
	}
	return false
}

func (handler *Handler) loadEnabledRetentionTiers(context *gin.Context) ([]retentionTierRow, error) {
	rows, err := db.New(handler.db).ListEnabledRetentionTiers(context)
	if err != nil {
		return nil, err
	}
	tiers := make([]retentionTierRow, 0, len(rows))
	for _, row := range rows {
		tiers = append(tiers, retentionTierRow{
			Code: row.Code, RetentionDays: int64(row.RetentionDays), DailySizeGB: row.DailySizeGb,
			RolloverMinIndexAge: row.RolloverMinIndexAge,
		})
	}
	return tiers, nil
}

// syncClusterLogStorage 对单个启用集群下发模板与保留策略，并回写 storage_sync_* 状态，
// 对应 Django 的 celery 任务 sync_log_storage（失败不抛出，只落状态供前端展示）。
func (handler *Handler) syncClusterLogStorage(clusterID int64) {
	ginContext, _ := gin.CreateTestContext(nil)
	queries := db.New(handler.db)
	err := queries.MarkClusterStorageSyncPending(ginContext, db.MarkClusterStorageSyncPendingParams{
		UpdateTime: time.Now().UTC(), ID: clusterID,
	})
	if err != nil {
		return
	}
	cluster, err := handler.loadElasticsearchClusterByID(ginContext, clusterID)
	fail := func(message string) {
		_ = queries.MarkClusterStorageSyncFailed(ginContext, db.MarkClusterStorageSyncFailedParams{
			StorageSyncError: message, StorageSyncTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
			UpdateTime: time.Now().UTC(), ID: clusterID,
		})
	}
	switch {
	case err != nil:
		fail(err.Error())
	case !cluster.Enabled:
		fail("集群未启用，跳过同步")
	default:
		if bootstrapErr := handler.bootstrapElasticsearchStorage(ginContext, cluster); bootstrapErr != nil {
			fail(truncateElasticsearchError(bootstrapErr))
		} else {
			_ = queries.MarkClusterStorageSyncSuccess(ginContext, db.MarkClusterStorageSyncSuccessParams{
				StorageSyncTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
				UpdateTime:      time.Now().UTC(), ID: clusterID,
			})
		}
	}
}

// syncAllClusterLogStorage 对所有启用集群异步下发，对应 Django 档位改动后的 _apply_policies。
func (handler *Handler) syncAllClusterLogStorage() {
	ginContext, _ := gin.CreateTestContext(nil)
	ids, err := db.New(handler.db).ListEnabledElasticsearchClusterIDs(ginContext)
	if err != nil {
		return
	}
	for _, id := range ids {
		handler.syncClusterLogStorage(id)
	}
}

func truncateElasticsearchError(err error) string {
	message := err.Error()
	if len(message) > 500 {
		return message[:500]
	}
	return message
}

// publishProcessingPipeline 把解析规则的 pipeline 发布到集群（Django 规则增改的同步行为）。
func (handler *Handler) publishProcessingPipeline(context *gin.Context, clusterID int64, name string, pipelineBody any) error {
	cluster, err := handler.loadElasticsearchClusterByID(context, clusterID)
	if err != nil {
		return err
	}
	if !cluster.Enabled {
		return fmt.Errorf("Elasticsearch 集群未启用")
	}
	if _, err := handler.elasticsearchRequest(context, cluster, "PUT", "/_ingest/pipeline/"+name, pipelineBody); err != nil {
		return err
	}
	return nil
}

func (handler *Handler) deleteProcessingPipeline(context *gin.Context, clusterID int64, name string) error {
	cluster, err := handler.loadElasticsearchClusterByID(context, clusterID)
	if err != nil {
		return err
	}
	if !cluster.Enabled {
		return nil
	}
	if _, err := handler.elasticsearchRequest(context, cluster, "DELETE", "/_ingest/pipeline/"+name, nil); err != nil {
		return err
	}
	return nil
}
