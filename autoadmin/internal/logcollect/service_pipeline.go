package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"autoadmin/internal/assets"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 服务级后处理：包装 pipeline（2026-09-28）。
//
// 问题：解析规则的 pipeline 按规则共享（Filebeat 片段 `pipeline:` 指向同一条 id），在规则里
// 写 drop_fields 会影响所有引用该规则的服务；而"只有某几个服务要额外处理"是真实需求。
//
// 方案（ES 侧串联）：配置了 extra_processors 的服务，平台为它生成一条**包装 pipeline**：
//
//	<基础名>@s<服务id> = { processors: [ {"pipeline": {"name": <基础名>}}, <extra_processors…> ] }
//
// 该服务的 Filebeat 片段 `pipeline:` 改指包装（log_config_render.go），其他服务不变。
// 处理仍然全部发生在 ES 侧、方言与规则一致、`_simulate` 可完整串联模拟；代价是包装的生命
// 周期要平台自己管——本文件就是它的全部生命周期：
//
//	入口                          动作
//	------------------------------  ----------------------------------------------
//	日志中心保存服务级后处理器       assets 触发 SyncServiceLogPipelines（按服务对账）
//	解析规则改名/换应用/换集群       publishProcessingRuleBeforeSave 里清旧名包装 + republish
//	链路体检                        log_health.go 按同一命名与签名比对包装
//
// 删除解析规则不需要入口：规则被日志定义引用时拒绝删除，而包装行必然引用着规则——
// 能删的规则身上不可能有包装。

// serviceExtraPipelineName 包装 pipeline id：<基础名>@s<服务id>。`s` 前缀把服务 id 与其他
// 后缀形式隔开；id 段都经过 safeOptionalSegment 清洗（与 processingPipelineName 同一套），
// '@'/'s'/数字对 ES 的 pipeline id 都是合法字符。
func serviceExtraPipelineName(prefix, application, rule string, serviceID int64) string {
	return processingPipelineName(prefix, application, rule) + "@s" + strconv.FormatInt(serviceID, 10)
}

// buildServiceExtraPipelineBody 生成包装 pipeline 的 body：先以 pipeline 处理器调基础
// （忽略失败与否保持 ES 默认 = 失败即中断，与"基础 pipeline 出错就不该继续处理"一致），
// 再顺序执行 extra_processors。extra 已在保存时经过 assets.ValidateServiceExtraProcessors
// （禁嵌套 pipeline、单个处理器键、数量/体积上限），这里防御性地再走一遍同一校验。
func buildServiceExtraPipelineBody(baseName string, extra json.RawMessage) (map[string]any, error) {
	normalized, err := assets.ValidateServiceExtraProcessors(extra)
	if err != nil {
		return nil, err
	}
	var items []any
	if err := json.Unmarshal(normalized, &items); err != nil {
		return nil, fmt.Errorf("服务级后处理器解析失败: %w", err)
	}
	processors := make([]any, 0, len(items)+1)
	processors = append(processors, gin.H{"pipeline": gin.H{"name": baseName}})
	processors = append(processors, items...)
	return gin.H{
		"processors": processors,
		"description": fmt.Sprintf("服务级后处理（平台生成）：先执行解析规则 %s，再执行本服务的额外处理器；"+
			"修改请回到日志中心的服务级后处理配置，不要直接改集群上的这份", baseName),
	}, nil
}

// extraPipelineRow 对账查询的行：一个服务的覆盖行 × 该行日志定义引用的解析规则。
// 规则被删/不再被引用时 RuleID/RuleName/RuleClusterID 为零值，该行的包装属于"不再期望"。
type extraPipelineRow struct {
	ServiceID        int64
	LogDefinitionID  int64
	ExtraProcessors  json.RawMessage
	ApplicationCode  string
	RuleID           int64
	RuleName         string
	RuleClusterID    int64
	ruleClusterValid bool
}

func extraPipelineRowsFromService(rows []db.ListServiceExtraPipelineRowsRow) []extraPipelineRow {
	out := make([]extraPipelineRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, extraPipelineRow{
			ServiceID: row.ServiceID, LogDefinitionID: row.LogDefinitionID,
			ExtraProcessors: row.ExtraProcessors, ApplicationCode: row.ApplicationCode,
			RuleID: row.RuleID.Int64, RuleName: row.RuleName, RuleClusterID: row.RuleClusterID.Int64,
			ruleClusterValid: row.RuleClusterID.Valid,
		})
	}
	return out
}

func extraPipelineRowsFromRule(rows []db.ListRuleExtraPipelineRowsRow) []extraPipelineRow {
	out := make([]extraPipelineRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, extraPipelineRow{
			ServiceID: row.ServiceID, LogDefinitionID: row.LogDefinitionID,
			ExtraProcessors: row.ExtraProcessors, ApplicationCode: row.ApplicationCode,
			RuleID: row.RuleID.Int64, RuleName: row.RuleName, RuleClusterID: row.RuleClusterID.Int64,
			ruleClusterValid: row.RuleClusterID.Valid,
		})
	}
	return out
}

// SyncServiceLogPipelines 实现 assets.ServiceLogPipelineSyncer：按服务对账包装 pipeline。
//
// 期望集 = 该服务下"extra_processors 非空且解析规则有效"的覆盖行；不在期望集里的包装
// （清空了后处理、规则解绑）按精确 id 删除。只删**本服务**的包装（id 后缀 @s<本服务id>），
// 其他服务共享同一条基础 pipeline、各自的包装互不波及。
func (handler *Handler) SyncServiceLogPipelines(ctx context.Context, serviceID int64) error {
	if serviceID < 1 {
		return assets.ErrInvalid
	}
	rows, err := db.New(handler.db).ListServiceExtraPipelineRows(ctx, serviceID)
	if err != nil {
		return err
	}
	return handler.reconcileExtraPipelines(ctx, extraPipelineRowsFromService(rows))
}

// republishRuleExtraPipelines 规则**发布钩子**用：规则改名/换应用/换集群时，包装 id 里嵌着
// 基础名、挂在规则所在集群——全部要重建。与 SyncServiceLogPipelines 的差别是：包装名从**本次
// 提交的**应用段/规则名/集群构建，而不是回读库（beforeSave 阶段库里还是旧值）。
func (handler *Handler) republishRuleExtraPipelines(ctx context.Context, ruleID int64, clusterID int64, applicationSegment, ruleName string) error {
	rows, err := db.New(handler.db).ListRuleExtraPipelineRows(ctx, sql.NullInt64{Int64: ruleID, Valid: ruleID > 0})
	if err != nil {
		return err
	}
	normalized := extraPipelineRowsFromRule(rows)
	for index := range normalized {
		normalized[index].RuleName = ruleName
		normalized[index].RuleClusterID = clusterID
		normalized[index].ruleClusterValid = true
		// 应用段以本次提交为准（换应用时 DB 里还是旧应用的 code）。
		normalized[index].ApplicationCode = applicationSegment
	}
	return handler.reconcileExtraPipelines(ctx, normalized)
}

// reconcileExtraPipelines 对账核心：期望集逐个 PUT（幂等，内容变了覆盖即生效），
// 期望外（规则无效的行除外——算不出 id，见下）的精确删除。
//
// 规则无效（被删/解绑）的行：包装 id 依赖规则的集群与名字，行上已取不到，精确删除无从谈起。
// 这类行只出现在数据不一致的场合（能删的规则身上不该有包装），跳过并交给体检的包装比对暴露。
func (handler *Handler) reconcileExtraPipelines(ctx context.Context, rows []extraPipelineRow) error {
	// 同一 (集群, 基础名) 只加载一次集群行；wrapper 期望表按精确 id 收敛。
	expected := map[string]gin.H{}
	touched := map[string]elasticsearchCluster{}
	for _, row := range rows {
		if !row.ruleClusterValid || row.RuleClusterID == 0 || strings.TrimSpace(row.RuleName) == "" {
			continue
		}
		if !assets.ExtraProcessorsEnabled(row.ExtraProcessors) {
			continue
		}
		cluster, err := handler.loadElasticsearchClusterByID(ctx, row.RuleClusterID)
		if err != nil {
			return fmt.Errorf("读取 Elasticsearch 集群失败: %w", err)
		}
		base := processingPipelineName(cluster.IndexPrefix, row.ApplicationCode, row.RuleName)
		body, err := buildServiceExtraPipelineBody(base, row.ExtraProcessors)
		if err != nil {
			return err
		}
		id := serviceExtraPipelineName(cluster.IndexPrefix, row.ApplicationCode, row.RuleName, row.ServiceID)
		expected[id] = body
		touched[id] = cluster
	}
	for id, body := range expected {
		cluster := touched[id]
		if _, err := handler.elasticsearchRequest(ctx, cluster, http.MethodPut, "/_ingest/pipeline/"+id, body); err != nil {
			return fmt.Errorf("发布包装 pipeline %s 失败: %w", id, err)
		}
	}
	// 期望外删除：按行反向构造精确 id（不需要通配列举；规则改名场景的旧名残留
	// 由 publishProcessingRuleBeforeSave 的 deleteExtraPipelinesByBase 通配清理）。
	seen := map[string]bool{}
	for _, row := range rows {
		if !row.ruleClusterValid || row.RuleClusterID == 0 || strings.TrimSpace(row.RuleName) == "" {
			continue
		}
		if assets.ExtraProcessorsEnabled(row.ExtraProcessors) {
			continue
		}
		cluster, err := handler.loadElasticsearchClusterByID(ctx, row.RuleClusterID)
		if err != nil {
			return fmt.Errorf("读取 Elasticsearch 集群失败: %w", err)
		}
		id := serviceExtraPipelineName(cluster.IndexPrefix, row.ApplicationCode, row.RuleName, row.ServiceID)
		if expected[id] != nil || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := handler.elasticsearchRequest(ctx, cluster, http.MethodDelete, "/_ingest/pipeline/"+id, nil); err != nil && !isElasticsearchNotFound(err) {
			return fmt.Errorf("清理包装 pipeline %s 失败: %w", id, err)
		}
	}
	return nil
}

// deleteExtraPipelinesByBase 通配删除一个基础 pipeline 名下的**全部**服务包装
// （id 形如 <基础名>@s*）：规则改名/换集群后，旧名字的包装永远不会再出现在期望集里，
// 只能按通配找出来删。找不到（通配无命中）不算错误。
func (handler *Handler) deleteExtraPipelinesByBase(ctx context.Context, clusterID int64, baseName string) {
	if clusterID == 0 || strings.TrimSpace(baseName) == "" {
		return
	}
	cluster, err := handler.loadElasticsearchClusterByID(ctx, clusterID)
	if err != nil || !cluster.Enabled {
		return
	}
	// 基础名各段都经过 safeOptionalSegment 清洗（小写字母/数字/_/-），'@'/'s'/'*' 为通配语法，
	// 直接拼路径即可（与 publishProcessingPipeline 的拼法同一取舍）。
	payload, err := handler.elasticsearchRequest(ctx, cluster, http.MethodGet, "/_ingest/pipeline/"+baseName+"@s*", nil)
	if err != nil {
		return
	}
	for id := range payload {
		if !strings.HasPrefix(id, baseName+"@s") {
			continue
		}
		_, _ = handler.elasticsearchRequest(ctx, cluster, http.MethodDelete, "/_ingest/pipeline/"+id, nil)
	}
}
