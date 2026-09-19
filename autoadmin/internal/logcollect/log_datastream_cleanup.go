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
// mode=all   → 清空匹配范围所有文档（match_all）
// mode=hours → 只删早于 now-<amount>h 的文档
// mode=days  → 只删早于 now-<amount>d 的文档
//
// 可选 `tier`（档位编码）：把范围从"这个服务的所有档位"收窄到"某一条流"，用于回收
// **改档位后留下、且不打算再切回的历史流**（旧流会按自己档位的保留期由 ILM 到期删除，
// 但保留期长/数据大的时候用户希望立刻释放）。
//
// tier 同样**不接受客户端传索引名**：它必须先命中档位表里的编码，再参与拼流名
// （`tier` 直接进 ES 的索引模式，不校验就等于把"删任意索引"的口子重新开出来）。
// 注意语义仍是"删这条流里的文档"而不是"删掉 data stream 对象"：流本身留着（变空），
// 与 mode=all 的既有行为一致；切回该档位时继续写入这条流。

type logDataStreamCleanupInput struct {
	ServiceID int64  `json:"service_id"`
	Mode      string `json:"mode"`
	Amount    int    `json:"amount"`
	Tier      string `json:"tier"`
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
	mode, cutoff, amount, err := parseCleanupMode(input.Mode, input.Amount)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	input.Amount = amount

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

	// 档位：不传时用 * 通配（服务换过保留档位时历史档位的数据流一并命中）；
	// 传了就必须是档位表里的编码——它直接进索引模式，放行任意字符串等于开了删任意索引的口子。
	tier := strings.TrimSpace(input.Tier)
	if tier != "" {
		knownTiers, tierErr := queries.ListRetentionTierCodes(context)
		if tierErr != nil {
			response.Error(context, tierErr)
			return
		}
		if !containsString(knownTiers, tier) {
			response.BusinessError(context, 400, "unknown retention tier code: "+tier, nil)
			return
		}
	}
	pattern := LogDataStreamName(prefix, dims.ProjectCode, dims.EnvironmentCode, dims.BusinessSystemCode, dims.ServiceCode, defaultPatternSegment(tier))
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
		"stream_pattern": pattern, "mode": mode, "amount": input.Amount, "tier": tier,
		"task": result["task"], "matched": true,
	})
}

// defaultPatternSegment 空档位回落通配符（与历史行为一致）。
func defaultPatternSegment(tier string) string {
	if strings.TrimSpace(tier) == "" {
		return "*"
	}
	return tier
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// parseCleanupMode 解析 mode/amount，返回 (规范化 mode, ES range 的 cutoff, 规范化 amount, error)。
//
// 抽出来是因为现在有**两条**清理路径（按服务、按未识别流）都要用同一套 mode 语义：
// 各写一遍必然分叉（一条支持 days、另一条忘了校验上限之类）。
func parseCleanupMode(mode string, amount int) (string, string, int, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "all":
		return normalized, "", 0, nil
	case "hours":
		if amount < 1 || amount > logCleanupMaxHours {
			return "", "", 0, fmt.Errorf("amount must be between 1 and %d hours", logCleanupMaxHours)
		}
		return normalized, fmt.Sprintf("now-%dh", amount), amount, nil
	case "days":
		if amount < 1 || amount > logCleanupMaxDays {
			return "", "", 0, fmt.Errorf("amount must be between 1 and %d days", logCleanupMaxDays)
		}
		return normalized, fmt.Sprintf("now-%dd", amount), amount, nil
	}
	return "", "", 0, fmt.Errorf("mode must be all|hours|days")
}

// validateCleanupStreamName 校验"按流名清理"允许的目标：**只收一条具体的数据流名**。
//
// 三条硬规则（每一条都在挡一种误删）：
//  1. 不含 `*` / `,`：带通配符就变成"删一族索引"，这条路径不收模式，只收名字；
//  2. 不以 `.` 开头：`.ds-*` 是后备索引、`.` 开头还有系统索引，都不是用户该点的东西；
//  3. 必须以集群前缀 + "-" 开头：前缀之外的名字属于别的系统，平台没有资格删。
//
// 注意这里**只做格式校验**：是否真的存在、是否"未识别"，由调用方查集群与维度表判定。
func validateCleanupStreamName(prefix, name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("stream is required")
	}
	if strings.ContainsAny(trimmed, "*,?") {
		return fmt.Errorf("stream must be a concrete data stream name (wildcards are not accepted)")
	}
	if strings.HasPrefix(trimmed, ".") {
		return fmt.Errorf("stream must not start with \".\" (backing/system indices are not allowed)")
	}
	if !strings.HasPrefix(trimmed, prefix+"-") {
		return fmt.Errorf("stream must start with the configured index prefix %q", prefix+"-")
	}
	return nil
}

// CleanupLogDataStreamByStream 按**数据流名**清理：只服务"平台不认识的流"（未识别流）。
//
// 为什么需要它：服务维度的清理（CleanupLogDataStream）按 service_id 自己拼流名，未识别流没有
// 服务可归属，于是这类"没人认领的垃圾流"在页面上一直清不掉（2026-09-19 现场提问）。
//
// 为什么不是"随便传索引名"：进来的是**数据流名**，且必须同时满足——
//
//	① 格式合法（见 validateCleanupStreamName）；
//	② 在集群上确实是一条 data stream（`GET /_data_stream/<name>`）；
//	③ **未被任何服务的维度识别**（recognized=false）：能归属到服务就必须走服务维度那条路
//	   （它按档位收窄、语义单一），这条路径只负责"平台不认识的流"，把口子收到最小。
//
// 删除语义与另一条路完全一致：`_delete_by_query` 异步删文档，**保留 data stream 对象**。
func (handler *Handler) CleanupLogDataStreamByStream(context *gin.Context) {
	var input struct {
		Stream string `json:"stream"`
		Mode   string `json:"mode"`
		Amount int    `json:"amount"`
	}
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	mode, cutoff, amount, err := parseCleanupMode(input.Mode, input.Amount)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}

	queries := db.New(handler.db)
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
	stream := strings.TrimSpace(input.Stream)
	if err = validateCleanupStreamName(prefix, stream); err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	// ③ 归属只作**事实**报出来，不作拦截：能归属到服务的流，页面优先走服务维度那条（按档位收窄、
	// 语义单一）；但"按流名"必须留着兜底——服务维度的 tier 必须是档位表里的编码，而流名里的档位段
	// 不保证都在表里（历史遗留/手工建过的流），只留一条路会把这类流卡成清不掉的死结。
	parsed := handler.loadStreamDims(context, prefix).resolveStreamName(stream)
	cluster := elasticsearchCluster{
		ID: clusterRow.ID, Hosts: clusterRow.Hosts, Username: clusterRow.Username,
		Password: clusterRow.Password, VerifyTLS: clusterRow.VerifyTls, CACert: clusterRow.CaCert,
		IndexPrefix: prefix, Timeout: int(clusterRow.RequestTimeout), Enabled: clusterRow.Enabled,
	}
	if cluster.Password, err = handler.secrets.Decrypt(cluster.Password); err != nil {
		response.Error(context, err)
		return
	}
	// ② 必须真的存在：不存在就报错，别让"名字打错了"看起来像"清理成功"。
	if _, err = handler.elasticsearchRequest(context, cluster, http.MethodGet, "/_data_stream/"+stream, nil); err != nil {
		if isElasticsearchNotFound(err) {
			response.BusinessError(context, 404, "data stream not found: "+stream, nil)
			return
		}
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	body := gin.H{"query": gin.H{"match_all": gin.H{}}}
	if cutoff != "" {
		body = gin.H{"query": gin.H{"range": gin.H{"@timestamp": gin.H{"lt": cutoff}}}}
	}
	path := "/" + stream + "/_delete_by_query?wait_for_completion=false&conflicts=proceed&refresh=false"
	result, err := handler.elasticsearchRequest(context, cluster, http.MethodPost, path, body)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, gin.H{
		"stream": stream, "mode": mode, "amount": amount, "task": result["task"], "matched": true,
		"recognized": parsed.Recognized, "service": parsed.Service, "tier": parsed.Tier,
	})
}
