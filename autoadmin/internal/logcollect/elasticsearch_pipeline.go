package logcollect

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// requiredProcessingRuleOutputs 处理规则产物必须具备的标准字段（"少一个都不行"的那一组）。
//
// 为什么是这三个：索引模板里其余字段全部由平台保证（Filebeat 带 @timestamp/message，
// 下发片段注入 project/business_system/environment/service/application/instance/host_ip/
// log_name/log_path，guard 兜 app_fields），只有这三个必须由 pipeline 产出：
//   - log_level：级别筛选、错误清单（terms 聚合）与查询页的级别列；
//   - log_message：**日志检索的 default_field**，也是查询页的「消息」列与详情（不产出它 =
//     日志能查到但消息为空、直接搜关键词搜不到内容）；
//   - error_fingerprint：错误清单/聚类按它聚合。
//
// 三处共用这一份定义，改这里等于同时收紧规则保存校验、调试页的 missing_fields、以及
// 索引模板上挂的 `<prefix>-mapping-guard`（写入时强制判定）。
var requiredProcessingRuleOutputs = []string{"log_level", "log_message", "error_fingerprint"}

func (handler *Handler) SimulateElasticsearchPipeline(context *gin.Context) {
	var input struct {
		Name     string         `json:"name"`
		Pipeline map[string]any `json:"pipeline"`
		Docs     []any          `json:"docs"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	if len(input.Docs) == 0 {
		response.BusinessError(context, 400, "docs must be a non-empty array", nil)
		return
	}
	cluster, err := handler.loadElasticsearchCluster(context)
	if err != nil {
		response.BusinessError(context, 404, "Elasticsearch cluster not found", nil)
		return
	}
	name := strings.TrimSpace(input.Name)
	if input.Pipeline == nil && name == "" {
		response.BusinessError(context, 400, "pipeline or name is required", nil)
		return
	}
	data, err := handler.simulatePipeline(context, cluster, name, input.Pipeline, input.Docs)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, data)
}

// simulatePipeline 跑一次 _ingest/pipeline/_simulate，并在 ES 原始响应上附加两项判定：
// schema_violations（输出字段不在索引 mapping 里的）与 missing_fields（缺必备字段的）。
//
// 抽成非 gin 的函数是为了让**后台任务（日志格式认证）与调试页走同一条实现**：认证不带请求
// 上下文，不能复用原先整段写在 handler 里的逻辑，而两处判定口径必须一致，否则"调试页看起来
// 通过、认证却不通过"。
//
// pipelineName 与 pipelineBody 二选一：传 body 走 inline pipeline（认证与调试页用），
// 传 name 让 ES 用集群里已发布的同名 pipeline。
func (handler *Handler) simulatePipeline(ctx context.Context, cluster elasticsearchCluster, pipelineName string, pipelineBody map[string]any, docs []any) (map[string]any, error) {
	if len(docs) == 0 {
		return nil, fmt.Errorf("docs must be a non-empty array")
	}
	// Elasticsearch _simulate 要求每个 doc 是 {"_source": {...}} 形态，与 Django
	// ElasticsearchClient.simulate_pipeline(_body) 保持一致，否则报 "[_source] required property is missing"。
	wrappedDocs := make([]any, 0, len(docs))
	for _, doc := range docs {
		wrappedDocs = append(wrappedDocs, gin.H{"_source": doc})
	}
	path := "/_ingest/pipeline/_simulate"
	body := gin.H{"docs": wrappedDocs}
	if pipelineBody != nil {
		body["pipeline"] = pipelineBody
	} else {
		path = "/_ingest/pipeline/" + url.PathEscape(pipelineName) + "/_simulate"
	}
	data, err := handler.elasticsearchRequest(ctx, cluster, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	// 以集群实际索引模板 mapping 的字段为准校验，而不是硬编码列表；取不到时回退内置标准字段。
	allowed := handler.processingRuleAllowedFields(ctx, cluster)
	data["schema_violations"] = nonStandardDocumentFields(data, allowed)
	data["missing_fields"] = missingRequiredDocumentFields(data)
	return data, nil
}

// processingRuleAllowedFields 读取 `<prefix>-template` 索引模板 mapping 的顶层字段集合；
// 模板不存在或读取失败时回退到内置 `standardLogFields`。
func (handler *Handler) processingRuleAllowedFields(context context.Context, cluster elasticsearchCluster) map[string]bool {
	prefix := strings.TrimSpace(cluster.IndexPrefix)
	if prefix == "" {
		prefix = "autoadmin"
	}
	if payload, err := handler.elasticsearchRequest(context, cluster, http.MethodGet, "/_index_template/"+buildIndexTemplateName(prefix), nil); err == nil {
		if fields := indexTemplateMappingFields(payload); len(fields) > 0 {
			return fields
		}
	}
	fallback := map[string]bool{}
	for name := range standardLogFields {
		fallback[name] = true
	}
	return fallback
}

// indexTemplateMappingFields 从 GET /_index_template/<name> 的响应里取 mapping.properties 的顶层字段。
func indexTemplateMappingFields(payload map[string]any) map[string]bool {
	templates, _ := payload["index_templates"].([]any)
	if len(templates) == 0 {
		return nil
	}
	first, _ := templates[0].(map[string]any)
	definition, _ := first["index_template"].(map[string]any)
	template, _ := definition["template"].(map[string]any)
	mappings, _ := template["mappings"].(map[string]any)
	properties, _ := mappings["properties"].(map[string]any)
	fields := map[string]bool{}
	for name := range properties {
		fields[name] = true
	}
	return fields
}

// nonStandardDocumentFields 输出文档里不在索引 mapping 顶层字段内的字段：dynamic=false 下会被静默丢弃。
// app_fields 等 flattened/object 容器本身在 mapping 里，其子字段不逐个校验。
func nonStandardDocumentFields(result map[string]any, allowed map[string]bool) []string {
	violations := map[string]bool{}
	docs, _ := result["docs"].([]any)
	for _, rawDoc := range docs {
		doc, _ := rawDoc.(map[string]any)
		detail, _ := doc["doc"].(map[string]any)
		source, _ := detail["_source"].(map[string]any)
		for field := range source {
			if !allowed[field] {
				violations[field] = true
			}
		}
	}
	fields := make([]string, 0, len(violations))
	for field := range violations {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// missingRequiredDocumentFields 输出文档里缺少的必备字段（当前是 error_fingerprint）。
func missingRequiredDocumentFields(result map[string]any) []string {
	missing := map[string]bool{}
	docs, _ := result["docs"].([]any)
	for _, rawDoc := range docs {
		doc, _ := rawDoc.(map[string]any)
		detail, _ := doc["doc"].(map[string]any)
		source, _ := detail["_source"].(map[string]any)
		for _, field := range requiredProcessingRuleOutputs {
			if _, ok := source[field]; !ok {
				missing[field] = true
			}
		}
	}
	fields := make([]string, 0, len(missing))
	for field := range missing {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}
