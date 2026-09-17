package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// Elasticsearch 日志存储管理面：索引命名、index template、ILM policy、ingest pipeline
// 的期望态构建与下发编排，对应 Django monitor/log_management.py。
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
// 字段契约与 Django log_schema.STANDARD_LOG_FIELDS 一致。
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
			},
			"mappings": gin.H{
				"dynamic":    false,
				"properties": standardLogFields,
			},
		},
	}
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
	"log_time":          gin.H{"type": "keyword"},
	"log_message":       gin.H{"type": "text"},
	"error_fingerprint": gin.H{"type": "keyword"},
	// flat_object 是 Elasticsearch 专有类型；Elasticsearch 用 flattened（8.x 自带，无需额外插件）。
	"app_fields": gin.H{"type": "flattened"},
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
