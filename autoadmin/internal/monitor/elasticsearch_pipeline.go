package monitor

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// requiredProcessingRuleOutputs 处理规则产物必须具备的标准字段：错误清单/聚类按 error_fingerprint 聚合，
// pipeline 不产出它时该能力静默失效，所以校验阶段必须显式提示并拦截发布。
var requiredProcessingRuleOutputs = []string{"error_fingerprint"}

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
	path := "/_ingest/pipeline/_simulate"
	// Elasticsearch _simulate 要求每个 doc 是 {"_source": {...}} 形态，与 Django
	// ElasticsearchClient.simulate_pipeline(_body) 保持一致，否则报 "[_source] required property is missing"。
	wrappedDocs := make([]any, 0, len(input.Docs))
	for _, doc := range input.Docs {
		wrappedDocs = append(wrappedDocs, gin.H{"_source": doc})
	}
	body := gin.H{"docs": wrappedDocs}
	if input.Pipeline != nil {
		body["pipeline"] = input.Pipeline
	} else if name := strings.TrimSpace(input.Name); name != "" {
		path = "/_ingest/pipeline/" + url.PathEscape(name) + "/_simulate"
	} else {
		response.BusinessError(context, 400, "pipeline or name is required", nil)
		return
	}
	data, err := handler.elasticsearchRequest(context, cluster, http.MethodPost, path, body)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	// 以集群实际索引模板 mapping 的字段为准校验，而不是硬编码列表；取不到时回退内置标准字段。
	allowed := handler.processingRuleAllowedFields(context, cluster)
	data["schema_violations"] = nonStandardDocumentFields(data, allowed)
	data["missing_fields"] = missingRequiredDocumentFields(data)
	response.Success(context, data)
}

// processingRuleAllowedFields 读取 `<prefix>-template` 索引模板 mapping 的顶层字段集合；
// 模板不存在或读取失败时回退到内置 `standardLogFields`。
func (handler *Handler) processingRuleAllowedFields(context *gin.Context, cluster elasticsearchCluster) map[string]bool {
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
