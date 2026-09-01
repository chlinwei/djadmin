package monitor

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) SimulateOpenSearchPipeline(context *gin.Context) {
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
	cluster, err := handler.loadOpenSearchCluster(context)
	if err != nil {
		response.BusinessError(context, 404, "OpenSearch cluster not found", nil)
		return
	}
	path := "/_ingest/pipeline/_simulate"
	body := gin.H{"docs": input.Docs}
	if input.Pipeline != nil {
		body["pipeline"] = input.Pipeline
	} else if name := strings.TrimSpace(input.Name); name != "" {
		path = "/_ingest/pipeline/" + url.PathEscape(name) + "/_simulate"
	} else {
		response.BusinessError(context, 400, "pipeline or name is required", nil)
		return
	}
	data, err := handler.openSearchRequest(context, cluster, http.MethodPost, path, body)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	data["schema_violations"] = nonStandardDocumentFields(data)
	response.Success(context, data)
}

func nonStandardDocumentFields(result map[string]any) []string {
	allowed := map[string]bool{
		"@timestamp": true, "message": true, "project": true, "business_system": true,
		"environment": true, "service": true, "application": true, "instance": true,
		"host_ip": true, "log_name": true, "log_path": true, "log_level": true,
		"log_time": true, "log_message": true, "error_fingerprint": true, "app_fields": true,
	}
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
