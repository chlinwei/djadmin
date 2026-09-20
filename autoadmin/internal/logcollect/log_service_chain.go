package logcollect

import (
	"database/sql"
	"fmt"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 按**逻辑服务**的采集链路诊断（日志中心 → 日志配置的「采集链路」状态条）。
//
// 解决的问题：日志查不到时，用户不知道断在哪一层——没采集？采集了没解析？写了但时间窗不对？
// 这个接口把链路按层给出判定，异常层一眼可见，并指到该去哪个页面处理。
//
// **判定全部复用既有实现，一层都不另造**（否则会出现"体检说没事、日志中心说没发布"）：
//   - 主机三层（agent 在线 / Filebeat 进程 / 配置是否已下发）→ buildServiceHostStates
//     （与"本服务下发状态"同源：同一份承载主机与配置态评估）
//   - 解析规则（pipeline 是否存在且与页面一致）→ judgeRulePipeline（与体检 pipelines 层同一个函数）
//   - 数据写入（窗口内有无条数，按 service 收窄）→ queryLogDataFlow（与体检 data_flow 同一查询）
//
// 服务级采集开关不在这里返回：它已经在页面上（log-config 的服务级总开关 + 逐条开关），
// 前端直接用它，多返一份只会多一个可能不一致的来源。
//
// 窗口取 30 分钟，比体检的 15 分钟宽：这是"用户盯着某个服务看"的场景，宁可慢一点报"没在写"，
// 也不要因为一个采集周期刚过就报红。
const logServiceChainWindowMinutes = 30

// GetServiceCollectionChain GET /monitor/log-targets/service-collection-chain/?application_service_id=
func (handler *Handler) GetServiceCollectionChain(context *gin.Context) {
	serviceID := parseID(context.Query("application_service_id"))
	if serviceID < 1 {
		response.BusinessError(context, 400, "application_service_id is required", nil)
		return
	}
	service, err := db.New(handler.db).GetApplicationServiceDetail(context, serviceID)
	if err != nil {
		response.BusinessError(context, 404, "逻辑服务不存在或已被删除", nil)
		return
	}
	managed, unmanaged, _, err := handler.buildServiceHostStates(context, serviceID)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}

	layers := []gin.H{
		hostLayer("agent", "Agent 在线", managed, unmanaged),
		hostLayer("runtime", "采集进程", managed, unmanaged),
		hostLayer("host_configs", "主机配置", managed, unmanaged),
	}
	layers = append(layers, handler.servicePipelineLayer(context, serviceID))
	layers = append(layers, handler.serviceDataFlowLayer(context, service))
	response.Success(context, gin.H{
		"service": gin.H{
			"id": service.ID, "name": service.Name, "code": service.Code,
			// 服务停用 / 服务级采集总开关关掉时，日志本来就不会再采——
			// 这是"查不到日志"最常见也最容易忘的原因，所以随链路一起给出。
			"enabled": service.Enabled, "log_collection_enabled": service.LogCollectionEnabled,
		},
		"layers": layers,
		"hosts": gin.H{
			"items":           managed,
			"unmanaged_items": unmanaged,
			"summary":         summarizeServiceChainHosts(managed),
		},
		"window_minutes": logServiceChainWindowMinutes,
		"checked_at":     time.Now().UTC().Format(time.RFC3339),
	})
}

// serviceChainHostSummary 承载主机各层计数（每台主机一个事实，不是"服务级"结论）。
type serviceChainHostSummary struct {
	Total           int `json:"total"`
	AgentOnline     int `json:"agent_online"`
	FilebeatRunning int `json:"filebeat_running"`
	FilebeatStopped int `json:"filebeat_stopped"`
	FilebeatError   int `json:"filebeat_error"`
	// FilebeatUnknown 从没查过状态（runtime_status 为空）：与"已停止"不同，不能混为一谈——
	// 前者是"不知道"，后者是"确实停了"，处置方式也不一样（前者先刷新一次）。
	FilebeatUnknown int `json:"filebeat_unknown"`
	ConfigSynced    int `json:"config_synced"`
	ConfigPending   int `json:"config_pending"`
}

func summarizeServiceChainHosts(managed []serviceApplyTarget) serviceChainHostSummary {
	summary := serviceChainHostSummary{Total: len(managed)}
	for _, host := range managed {
		if host.AgentOnline {
			summary.AgentOnline++
		}
		switch host.RuntimeStatus {
		case "running":
			summary.FilebeatRunning++
		case "stopped":
			summary.FilebeatStopped++
		case "error":
			summary.FilebeatError++
		default:
			summary.FilebeatUnknown++
		}
		switch host.ConfigState {
		case LogConfigSynced:
			summary.ConfigSynced++
		case LogConfigDrift, LogConfigNever:
			summary.ConfigPending++
		}
	}
	return summary
}

// hostLayer 把主机事实汇总成一层：agent 在线 / Filebeat 进程 / 配置下发。
//
// 三层都用同一份承载主机，只换判据（chainHostFact）。**未纳管的主机在每一层都出现，且状态是 warn**：
// 它的问题不是"配置漂移"（那会让人去点下发，而下发对它无效），而是"这台机器根本不在采集范围内"。
// 但也不能因为未纳管就报 ok——"3 台里 1 台没采"就是要看见的问题。
func hostLayer(key, name string, managed, unmanaged []serviceApplyTarget) gin.H {
	items := []gin.H{}
	statuses := []string{}
	for _, host := range managed {
		status, detail := chainHostFact(key, host)
		statuses = append(statuses, status)
		items = append(items, logHealthItem(chainHostLabel(host), status, detail))
	}
	for _, host := range unmanaged {
		statuses = append(statuses, logHealthWarn)
		items = append(items, logHealthItem(chainHostLabel(host), logHealthWarn, chainUnmanagedNote))
	}
	if len(items) == 0 {
		return logHealthLayer(key, name, logHealthWarn, "该服务还没有绑定部署实例，没有承载主机", items)
	}
	return logHealthLayer(key, name, worstLogHealthStatus(statuses), chainSummary(statuses), items)
}

const chainUnmanagedNote = "未纳管日志采集：该主机上的日志不会被采集，配置也下发不到"

// chainHostFact 一台已纳管主机在某一层上的事实与说明。
func chainHostFact(key string, host serviceApplyTarget) (string, string) {
	switch key {
	case "agent":
		if host.AgentOnline {
			return logHealthOK, "在线"
		}
		return logHealthDrift, "agent 不在线：查状态/启停/下发都做不了"
	case "runtime":
		switch host.RuntimeStatus {
		case "running":
			return logHealthOK, "运行中"
		case "stopped":
			return logHealthDrift, "Filebeat 服务已停止：日志不会写入"
		case "error":
			return logHealthError, "Filebeat 运行异常"
		default:
			// 空值不是"停了"，是"没查过"：处置是刷新一次，不是当故障处理。
			return logHealthWarn, "状态未知，需刷新（刷新会向主机查一次）"
		}
	default: // host_configs
		switch host.ConfigState {
		case LogConfigSynced:
			return logHealthOK, "期望配置与已下发一致"
		case LogConfigDrift:
			return logHealthDrift, "配置已变更未下发：改动还没到主机上"
		case LogConfigNever:
			return logHealthDrift, "从未下发过采集配置"
		default:
			return logHealthWarn, "配置态未知"
		}
	}
}

// chainHostLabel 用"主机实例名（IP）"标识一台主机：只给 IP 认不出是哪台机器，只给名字找不到地址。
func chainHostLabel(host serviceApplyTarget) string {
	label := host.HostInstanceName
	if label == "" {
		label = fmt.Sprintf("host-%d", host.HostID)
	}
	if host.HostIP != "" {
		label += "（" + host.HostIP + "）"
	}
	return label
}

// chainSummary 计数摘要：分母是**全部承载主机**（含未纳管），因为"3 台里 1 台没采"才是真相。
func chainSummary(statuses []string) string {
	total := len(statuses)
	if total == 0 {
		return "没有承载主机"
	}
	ok := 0
	for _, status := range statuses {
		if status == logHealthOK {
			ok++
		}
	}
	if ok == total {
		return fmt.Sprintf("%d/%d 台正常", total, total)
	}
	return fmt.Sprintf("%d/%d 台正常（%d 台待处理）", ok, total, total-ok)
}

// servicePipelineLayer 该服务的日志定义引用的那些规则，在集群上是否已发布（判据见 judgeRulePipeline）。
//
// **按规则自己的集群判**（规则在哪个集群发布，就用哪个集群的前缀与连接）：规则编辑页能选集群，
// 用"默认集群"去判别的集群上的规则会得出"不存在该 pipeline"这种假结论。同集群只读一次连接。
func (handler *Handler) servicePipelineLayer(context *gin.Context, serviceID int64) gin.H {
	rows, err := db.New(handler.db).ListServiceTemplateLogs(context, serviceID)
	if err != nil {
		return logHealthLayer("pipelines", "解析规则", logHealthError, "读取服务的日志定义失败: "+err.Error(), nil)
	}
	items := []gin.H{}
	seen := map[int64]bool{}
	clusters := map[int64]elasticsearchCluster{}
	for _, row := range rows {
		if !row.ProcessingRuleID.Valid || seen[row.ProcessingRuleID.Int64] {
			continue
		}
		seen[row.ProcessingRuleID.Int64] = true
		rule, err := db.New(handler.db).GetLogProcessingRule(context, row.ProcessingRuleID.Int64)
		if err != nil {
			items = append(items, logHealthItem(row.ProcessingRuleName, logHealthError, "规则不存在或已被删除：日志定义还指着它"))
			continue
		}
		cluster, cached := clusters[rule.ClusterID]
		if !cached {
			loaded, loadErr := handler.loadElasticsearchClusterByID(context, rule.ClusterID)
			if loadErr != nil {
				items = append(items, logHealthItem(rule.Name, logHealthError, "规则所属的 Elasticsearch 集群不可用: "+loadErr.Error()))
				continue
			}
			cluster = loaded
			clusters[rule.ClusterID] = loaded
		}
		items = append(items, handler.judgeRulePipeline(context, cluster, rulePipelineInputFromRow(rule)))
	}
	return pipelineLayerFromRuleItems(items, len(rows))
}

// pipelineLayerFromRuleItems 把"服务的规则 → pipeline 判定"收成一层，并处理两种"没有规则"的情形：
// 模板下没有日志定义（warn：还没配）与有日志定义但都没挂解析规则（drift：日志按约定不采集，
// 要人去挂规则）。单独拎出来是为了能直接测——这两种都会被用户当成"这个服务没日志"，
// 但处置方式完全不同，报错方向不能反。
func pipelineLayerFromRuleItems(items []gin.H, definitionCount int) gin.H {
	if len(items) > 0 {
		return logHealthLayerFromItems("pipelines", "解析规则", items, "尚未配置解析规则")
	}
	if definitionCount == 0 {
		return logHealthLayer("pipelines", "解析规则", logHealthWarn, "该服务的部署模板下还没有日志定义", nil)
	}
	return logHealthLayer("pipelines", "解析规则", logHealthDrift,
		"该服务的日志定义都没有关联解析规则：未挂 pipeline 的日志不会被采集", nil)
}

// serviceDataFlowLayer 最近窗口内该服务有没有在写：前几层全绿也可能没数据，这层是链路真通了的证据。
//
// 查的是**默认启用的集群**（Filebeat output 写的就是它，见 defaultElasticsearchCluster），
// 不是 URL 上那个——"数据落在哪"与"管理端在看哪个集群"是两件事。
func (handler *Handler) serviceDataFlowLayer(context *gin.Context, service db.GetApplicationServiceDetailRow) gin.H {
	cluster, err := handler.defaultElasticsearchCluster(context)
	if err != nil {
		reason := "读取默认 Elasticsearch 集群失败: " + err.Error()
		if err == sql.ErrNoRows {
			reason = "还没有启用中的 Elasticsearch 集群"
		}
		return logHealthLayer("data_flow", "数据写入", logHealthError, reason, nil)
	}
	total, _, queryErr := handler.queryLogDataFlow(context, cluster, logHealthPrefix(cluster), service.ID, logServiceChainWindowMinutes)
	if queryErr != nil {
		return logHealthLayer("data_flow", "数据写入", logHealthError, "查询失败: "+truncateElasticsearchError(queryErr), nil)
	}
	if total == 0 {
		return logHealthLayer("data_flow", "数据写入", logHealthWarn,
			fmt.Sprintf("最近 %d 分钟没有新日志写入：先看上面三层是「没采集」还是「采集了没解析」", logServiceChainWindowMinutes), nil)
	}
	return logHealthLayer("data_flow", "数据写入", logHealthOK,
		fmt.Sprintf("最近 %d 分钟写入 %.0f 条", logServiceChainWindowMinutes, total), nil)
}
