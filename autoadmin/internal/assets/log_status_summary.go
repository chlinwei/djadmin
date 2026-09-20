package assets

import (
	"context"
	"sort"
)

// 多服务日志状态汇总（架构文档 §9.5「非服务节点 = 层级下钻视图」）。
//
// 为什么需要它：日志中心的服务树在"项目/业务系统/环境"这些非服务层级上，原来没有任何内容
// ——「日志查询」与「日志配置」都硬性要求一个具体的逻辑服务。要让这些层级有东西可看，就必须
// "一次拿到这一批服务的日志状态"。现在的数据分散在两处，各自都只有单服务入口：
//   - 日志定义条数与格式认证状态：`ListServiceTemplateLogs`（**只此一处**算 format_state 的指纹）；
//   - 配置态（待下发）：logcollect 的 `EvaluateServiceLogConfigStates`。
// 于是本接口做两件事：在 assets 侧按服务聚合第一类（复用同一个函数，不复制指纹逻辑），
// 第二类通过 **注入的评估器**（见 ServiceLogPendingEvaluator）拿到——方向与
// LogFormatVerifier / LogGlobPreviewer 一致：接口定义在消费方 assets，实现注入自 logcollect。

// MaxLogStatusSummaryServices 一次汇总的服务数上限。
//
// 为什么要有上限：本实现的查询次数随服务数线性（每个服务一次 `ListServiceTemplateLogs` +
// 一次承载主机查询），而这是日志中心"一个层级下的服务清单"的展示接口——正常规模是几条到几十条。
// 上限把"误传全库服务"挡在 400，而不是让一个页面打开就打几百条查询。
//
// 扩容路径（真要支撑上千服务时）：把"承载主机 + 已下发子指纹"的查询改成
// `sd.service_id IN (sqlc.slice(service_ids))` 的批量版（`db/queries/mysql/monitor.sql`），
// 日志定义的读取同样按 `s.id IN (...)` 批量取后再在 Go 里按服务分组——那时整批只剩 4 条查询。
// 注意口径不能变：`format_state` 必须仍然由 logFormatFingerprintOf 判定。
const MaxLogStatusSummaryServices = 200

// ServiceLogPendingCounts 一个服务的配置态计数（与 `GET /monitor/log-targets/service-config-state/`
// 的 summary 同口径，只是去掉了 Unknown：整批拿不到集群/渲染输入时本接口整体报错，
// 不会把某台主机静默记成"状态未知"）。
//
// 计数单位是 **(服务 × 主机)**：同一台主机承载多个服务时，每个服务各算一次——这正是"下发这件事"
// 的粒度（agent 侧按主机全量重下发，但一个服务的配置是否到位由它的子指纹决定）。
type ServiceLogPendingCounts struct {
	// Hosts 该服务的承载主机总数（已纳管 + 未纳管）。
	Hosts int `json:"hosts"`
	// Managed 已纳管日志采集、可下发的主机数。
	Managed int `json:"managed"`
	// Unmanaged 只在资产里绑了实例、还没纳管日志采集的主机数（下发不到它们，单列不混进待下发）。
	Unmanaged int `json:"unmanaged"`
	// Synced 配置一致的已纳管主机数。
	Synced int `json:"synced"`
	// Drift 下发过但配置已变（子指纹不一致）的主机数。
	Drift int `json:"drift"`
	// Never 从未下发过的主机数。
	Never int `json:"never"`
}

// ServiceLogPendingEvaluator 由 logcollect 注入（见 logcollect/log_service_status.go）。
//
// 入参是服务 id 列表，返回按服务 id 索引的配置态计数。**未能评估必须返回 error**，
// 不能返回空 map——那会让界面把"没算出来"显示成"都已同步"。
type ServiceLogPendingEvaluator interface {
	ServiceLogPendingHosts(ctx context.Context, serviceIDs []int64) (map[int64]ServiceLogPendingCounts, error)
}

// SetServiceLogPendingEvaluator 注入配置态评估器。未注入时汇总照常返回日志与认证状态，
// `pending` 为 null（界面显示"-"）——宁可说"没这项数据"也不谎报"没有待下发"。
func (s *Service) SetServiceLogPendingEvaluator(evaluator ServiceLogPendingEvaluator) {
	s.logPendingEvaluator = evaluator
}

// ServiceLogStatusItem 一个服务的日志状态汇总。
//
// 刻意**不含**服务级采集总开关与默认档位：那两个字段在 `GET /assets/application-services/`
// 的服务行上已经是权威值，层级视图的清单本来就带着那些行。放在两处必然出现"清单上写着开、
// 点进去是关"的不一致，所以这里只给服务行给不了的：日志定义派生出来的计数 + 配置态。
type ServiceLogStatusItem struct {
	ServiceID int64 `json:"service_id"`
	// Logs 该服务模板下的日志定义条数（也就是服务页那张表的行数）。
	Logs int `json:"logs"`
	// Verified / NeedsRecheck / Unverified 是日志定义按 format_state 的分桶（与页面同口径，
	// 含义见 §4.8；未挂解析规则的日志同样按未认证计入，另有 NoRule 单独给出）。
	Verified     int `json:"verified"`
	NeedsRecheck int `json:"needs_recheck"`
	Unverified   int `json:"unverified"`
	// DisabledLogs 被这个服务逐条关掉采集的日志条数（覆盖值 false）。
	DisabledLogs int `json:"disabled_logs"`
	// NoRule 没挂解析规则的日志条数：它们不会被采集（页面显示"未配置（不会采集）"），
	// 对层级视图来说是"这个服务有东西没配完"的信号，所以单独给一个数。
	NoRule int `json:"no_rule"`
	// Pending 配置态计数；**未注入评估器或评估失败时为 null**（界面显示"-"）。
	Pending *ServiceLogPendingCounts `json:"pending"`
}

// ServiceLogStatusTotals 整批的合计。合计由服务端给（而不是让前端各自累加），
// 否则"指标条"与"清单"迟早会算出两个不同的数。
type ServiceLogStatusTotals struct {
	Services     int `json:"services"`
	Logs         int `json:"logs"`
	Verified     int `json:"verified"`
	NeedsRecheck int `json:"needs_recheck"`
	Unverified   int `json:"unverified"`
	DisabledLogs int `json:"disabled_logs"`
	NoRule       int `json:"no_rule"`
	// 下面是配置态的合计，单位同样是 (服务 × 主机)。PendingError 非空时这几个数无意义。
	Hosts     int `json:"hosts"`
	Managed   int `json:"managed"`
	Unmanaged int `json:"unmanaged"`
	Synced    int `json:"synced"`
	Drift     int `json:"drift"`
	Never     int `json:"never"`
}

// ServiceLogStatusSummary 汇总响应体。
type ServiceLogStatusSummary struct {
	Items  []ServiceLogStatusItem `json:"items"`
	Totals ServiceLogStatusTotals `json:"totals"`
	// PendingError 非空 = 配置态这一项没算出来（例如没有启用的默认集群、评估器未注入），
	// 此时每个 item 的 pending 都是 null。**如实报出来**，界面对应列显示"-"而不是 0。
	PendingError string `json:"pending_error"`
}

// LogStatusSummary 汇总给定的这批服务的日志状态（日志定义分桶 + 配置态）。
//
// 服务 id 会去重、排序后处理；不存在的服务也会返回一条全 0 的 item —— 本接口是"给我这些 id 的
// 状态"，不做存在性校验（调用方传的是它自己刚列出来的服务）。空列表或超过上限报 ErrInvalid。
func (s *Service) LogStatusSummary(ctx context.Context, serviceIDs []int64) (ServiceLogStatusSummary, error) {
	ids := normalizedServiceIDs(serviceIDs)
	if len(ids) == 0 || len(ids) > MaxLogStatusSummaryServices {
		return ServiceLogStatusSummary{}, ErrInvalid
	}
	summary := ServiceLogStatusSummary{Items: make([]ServiceLogStatusItem, 0, len(ids))}
	for _, serviceID := range ids {
		// 复用列表接口的同一个读取函数：format_state 的指纹比对只在这一处实现，
		// 这里若另算一遍，界面上的"未认证"与层级视图上的数字迟早对不上。
		rows, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
		if err != nil {
			return ServiceLogStatusSummary{}, translate(err)
		}
		item := ServiceLogStatusItem{ServiceID: serviceID, Logs: len(rows)}
		for _, row := range rows {
			switch row.FormatState {
			case formatStateVerified:
				item.Verified++
			case formatStateNeedsRecheck:
				item.NeedsRecheck++
			default:
				item.Unverified++
			}
			if row.CollectionEnabled != nil && !*row.CollectionEnabled {
				item.DisabledLogs++
			}
			if row.TemplateProcessingRuleID == nil {
				item.NoRule++
			}
		}
		summary.Items = append(summary.Items, item)
	}

	// 配置态：评估器未注入（单测/最小部署）或评估失败都不让整个接口失败——日志状态本身仍然有用，
	// 把原因写在 pending_error 里，界面这一列显示"-"。
	if s.logPendingEvaluator != nil {
		pending, err := s.logPendingEvaluator.ServiceLogPendingHosts(ctx, ids)
		if err != nil {
			summary.PendingError = err.Error()
		} else {
			for index := range summary.Items {
				counts, ok := pending[summary.Items[index].ServiceID]
				if !ok {
					continue
				}
				summary.Items[index].Pending = &counts
			}
		}
	}
	summary.Totals = summarizeLogStatusItems(summary.Items, summary.PendingError == "")
	return summary, nil
}

// summarizeLogStatusItems 把逐服务结果累加成合计。抽成纯函数便于直测（合计错了整个指标条就错了）。
func summarizeLogStatusItems(items []ServiceLogStatusItem, withPending bool) ServiceLogStatusTotals {
	totals := ServiceLogStatusTotals{Services: len(items)}
	for _, item := range items {
		totals.Logs += item.Logs
		totals.Verified += item.Verified
		totals.NeedsRecheck += item.NeedsRecheck
		totals.Unverified += item.Unverified
		totals.DisabledLogs += item.DisabledLogs
		totals.NoRule += item.NoRule
		if !withPending || item.Pending == nil {
			continue
		}
		totals.Hosts += item.Pending.Hosts
		totals.Managed += item.Pending.Managed
		totals.Unmanaged += item.Pending.Unmanaged
		totals.Synced += item.Pending.Synced
		totals.Drift += item.Pending.Drift
		totals.Never += item.Pending.Never
	}
	return totals
}

// normalizedServiceIDs 去重 + 只留正数 + 排序，保证同一批入参（哪怕顺序不同）结果稳定可比。
func normalizedServiceIDs(serviceIDs []int64) []int64 {
	seen := make(map[int64]bool, len(serviceIDs))
	ids := make([]int64, 0, len(serviceIDs))
	for _, id := range serviceIDs {
		if id < 1 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}
