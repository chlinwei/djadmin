package logcollect

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 逻辑服务维度的数据流清理：按 service_id 自行解析 <prefix>-<项目>-<业务>-<环境>-<服务>-*
// 的流名模式（覆盖换过档位的历史流），只接受 mode/amount，不接受客户端传索引名，
// 避免变成任意删索引的口子。删除走 _delete_by_query（异步），保留 data stream 本身。
//
// mode=all   → 清空该服务所有文档（match_all）
// mode=hours → 只删早于 now-<amount>h 的文档
// mode=days  → 只删早于 now-<amount>d 的文档

type logDataStreamCleanupInput struct {
	ServiceID int64  `json:"service_id"`
	Mode      string `json:"mode"`
	Amount    int    `json:"amount"`
}

const (
	logCleanupMaxHours = 24 * 3650
	logCleanupMaxDays  = 3650
)

func (handler *Handler) CleanupLogDataStream(context *gin.Context) {
	var input logDataStreamCleanupInput
	if err := context.ShouldBindJSON(&input); err != nil || input.ServiceID <= 0 {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	var cutoff string
	switch mode {
	case "all":
		input.Amount = 0
	case "hours":
		if input.Amount < 1 || input.Amount > logCleanupMaxHours {
			response.BusinessError(context, 400, fmt.Sprintf("amount must be between 1 and %d hours", logCleanupMaxHours), nil)
			return
		}
		cutoff = fmt.Sprintf("now-%dh", input.Amount)
	case "days":
		if input.Amount < 1 || input.Amount > logCleanupMaxDays {
			response.BusinessError(context, 400, fmt.Sprintf("amount must be between 1 and %d days", logCleanupMaxDays), nil)
			return
		}
		cutoff = fmt.Sprintf("now-%dd", input.Amount)
	default:
		response.BusinessError(context, 400, "mode must be all|hours|days", nil)
		return
	}

	queries := db.New(handler.db)
	dims, err := queries.GetApplicationServiceStreamDims(context, input.ServiceID)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "application service not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}

	clusterRow, err := queries.GetDefaultEnabledElasticsearchClusterConnection(context)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 400, "no enabled default Elasticsearch cluster", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	prefix := strings.TrimSpace(clusterRow.IndexPrefix)
	if prefix == "" {
		prefix = "autoadmin"
	}
	cluster := elasticsearchCluster{
		ID: clusterRow.ID, Hosts: clusterRow.Hosts, Username: clusterRow.Username,
		Password: clusterRow.Password, VerifyTLS: clusterRow.VerifyTls, CACert: clusterRow.CaCert,
		IndexPrefix: prefix, Timeout: int(clusterRow.RequestTimeout), Enabled: clusterRow.Enabled,
	}
	if cluster.Password, err = handler.secrets.Decrypt(cluster.Password); err != nil {
		response.Error(context, err)
		return
	}

	// 档位用 * 通配：服务换过保留档位时，历史档位的数据流也一并命中。
	pattern := LogDataStreamName(prefix, dims.ProjectCode, dims.EnvironmentCode, dims.BusinessSystemCode, dims.ServiceCode, "*")
	body := gin.H{"query": gin.H{"match_all": gin.H{}}}
	if cutoff != "" {
		body = gin.H{"query": gin.H{"range": gin.H{"@timestamp": gin.H{"lt": cutoff}}}}
	}
	path := "/" + pattern + "/_delete_by_query?wait_for_completion=false&conflicts=proceed&refresh=false"
	result, err := handler.elasticsearchRequest(context, cluster, http.MethodPost, path, body)
	if err != nil {
		if isElasticsearchNotFound(err) {
			// 没有任何匹配的数据流：视为已清理，不报错。
			response.Success(context, gin.H{"stream_pattern": pattern, "mode": mode, "amount": input.Amount, "task": nil, "matched": false})
			return
		}
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, gin.H{
		"stream_pattern": pattern, "mode": mode, "amount": input.Amount,
		"task": result["task"], "matched": true,
	})
}
