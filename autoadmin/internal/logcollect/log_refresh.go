package logcollect

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 逻辑服务级「强制刷新索引」。
//
// 背景：data stream 的索引模板把 `index.refresh_interval` 设成 10s（见 log_management.go
// 的 buildIndexTemplateBody），Elasticsearch 默认是 1s。这是用搜索实时性换写入吞吐的取舍，
// 代价是"日志已经进 ES、但前端要等一会儿才查得到"。本接口按需对该服务**采集中的**
// data stream 做一次 `_refresh`，让已写入的文档立即进入可搜索 segment。
//
// 范围严格限定为"该服务采集中的流"：
//   - 维度码来自 GetApplicationServiceStreamDims（与清理路径同源）；
//   - 档位集合来自 ListServiceActiveStreamTiers（服务 × 日志定义去重后的生效档位）——
//     流名 = <prefix>-<项目>-<业务系统>-<环境>-<服务>-<档位>，不含 log_name，
//     所以同一服务多条日志若档位相同则共用同一条流，这里按档位去重即得该服务的全部采集流。
//   - 只刷新采集中的档位，不带历史档位（历史流已停止写入，刷它没有意义）。
//
// 客户端只能传 application_service_id，不能传索引名——流名由后端按维度码拼装，
// 与清理路径同样的防口子考虑（用户传任意串直接进 ES 索引模式）。集群取请求 URL 上的集群，
// 与日志查询面板选中的是同一个，保证"刷的和查的是同一个"。
func (handler *Handler) ElasticsearchLogRefresh(context *gin.Context) {
	var input struct {
		ApplicationServiceID int64 `json:"application_service_id"`
	}
	if err := context.ShouldBindJSON(&input); err != nil || input.ApplicationServiceID < 1 {
		response.BusinessError(context, 400, "application_service_id is required", nil)
		return
	}
	cluster, err := handler.loadElasticsearchCluster(context)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "Elasticsearch cluster not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	streams, err := handler.resolveServiceCollectingStreams(context, cluster, input.ApplicationServiceID)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "application service not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if len(streams) == 0 {
		// 没有采集中的流（未挂日志定义/无生效档位）：没什么可刷的，明确告诉调用方而不是静默成功。
		response.Success(context, gin.H{
			"streams": []string{}, "count": 0,
			"message": "该服务当前没有采集中的 data stream",
		})
		return
	}
	// `_refresh` 支持逗号分隔的索引/流名列表；ignore_unavailable 兜住"还没产生第一条数据的流"。
	if _, err = handler.elasticsearchRequest(context, cluster, http.MethodPost,
		"/"+strings.Join(streams, ",")+"/_refresh?ignore_unavailable=true&expand_wildcards=open", nil); err != nil {
		response.BusinessError(context, 502, "refresh failed: "+err.Error(), nil)
		return
	}
	response.Success(context, gin.H{"streams": streams, "count": len(streams)})
}

// resolveServiceCollectingStreams 解析逻辑服务采集中的 data stream 名列表。
// 服务不存在时返回 sql.ErrNoRows（由调用方转 404）。
func (handler *Handler) resolveServiceCollectingStreams(context context.Context, cluster elasticsearchCluster, serviceID int64) ([]string, error) {
	queries := db.New(handler.db)
	dims, err := queries.GetApplicationServiceStreamDims(context, serviceID)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(cluster.IndexPrefix)
	if prefix == "" {
		prefix = "autoadmin"
	}
	tiers := make([]string, 0)
	for tier := range loadActiveStreamTiers(context, queries)[serviceID] {
		if strings.TrimSpace(tier) != "" {
			tiers = append(tiers, tier)
		}
	}
	// 排序只为返回结果稳定（同一集合每次顺序一致，便于日志/断言）。
	sort.Strings(tiers)
	streams := make([]string, 0, len(tiers))
	seen := map[string]bool{}
	for _, tier := range tiers {
		stream := LogDataStreamName(prefix, dims.ProjectCode, dims.EnvironmentCode, dims.BusinessSystemCode, dims.ServiceCode, tier)
		if !seen[stream] {
			seen[stream] = true
			streams = append(streams, stream)
		}
	}
	return streams, nil
}
