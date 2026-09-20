package logcollect

import (
	"context"
	"strconv"
	"strings"

	"autoadmin/internal/assets"
	db "autoadmin/internal/platform/database/generated"
)

// 一批服务的配置态评估（日志中心层级视图的"待下发"列）。
//
// 与单服务版 `EvaluateServiceLogConfigStates` 判据**完全相同**（同一套渲染 + 同一个
// `serviceConfigStatus`），差别只在算的方式：这里把入参服务涉及的主机**并起来，每台主机只渲染一次**，
// 再用各服务的子指纹分别比对。单服务版按服务调用；层级视图一次要问几十个服务，逐个调用会把
// 同一台主机的配置重复渲染几十遍——渲染是纯函数，但它是这套评估里最贵的一步。
//
// 规模：查询次数 = 服务数（每个服务一次承载主机查询，见下）+ 3（默认集群 1 + 渲染输入 2）。
// 服务数那部分是接口可见上限的来源，扩容路径写在 assets.MaxLogStatusSummaryServices 上。

// serviceAppliedFingerprints 一台主机上某个服务的**已下发**指纹。
// 两个都要：服务级子指纹用来判"本服务的片段到位没"，整机指纹只在"该服务还没有服务级记录"时兜底
// （存量目标 service_fingerprints 为空，但整机配置与期望一致 —— 见 serviceConfigStatus）。
type serviceAppliedFingerprints struct {
	service string
	host    string
}

// hostExpectedFingerprints 一台主机的**期望**指纹（渲染结果）。
type hostExpectedFingerprints struct {
	host    string
	service map[string]string
}

// serviceLogTargets 一个服务的承载主机情况（读库结果）。
type serviceLogTargets struct {
	unmanaged int
	managed   map[int64]serviceAppliedFingerprints
}

// countServicePendingHosts 把"每服务每主机的已下发指纹"与"每主机的期望指纹"折叠成逐服务计数。
// 抽成纯函数是因为这里的归组最容易出错（共享主机上 A 的改动算到 B 头上就是那个老问题），
// 而它不需要数据库或集群就能直测。
func countServicePendingHosts(targets map[int64]serviceLogTargets, expected map[int64]hostExpectedFingerprints) map[int64]assets.ServiceLogPendingCounts {
	counts := make(map[int64]assets.ServiceLogPendingCounts, len(targets))
	for serviceID, target := range targets {
		counts[serviceID] = assets.ServiceLogPendingCounts{
			Hosts: len(target.managed) + target.unmanaged, Managed: len(target.managed), Unmanaged: target.unmanaged,
		}
	}
	for serviceID, target := range targets {
		entry := counts[serviceID]
		key := strconv.FormatInt(serviceID, 10)
		for hostID, applied := range target.managed {
			expect := expected[hostID]
			// 逐 (服务 × 主机) 用同一个判据函数：口径只能有一处，否则层级视图与服务页会给出不同结论。
			switch serviceConfigStatus(expect.service[key], applied.service, expect.host, applied.host) {
			case LogConfigDrift:
				entry.Drift++
			case LogConfigNever:
				entry.Never++
			default:
				entry.Synced++
			}
		}
		counts[serviceID] = entry
	}
	return counts
}

// ServiceLogPendingHosts 实现 assets.ServiceLogPendingEvaluator。
//
// 返回按服务 id 索引的计数。**任何一环算不出来都返回 error**（不返回空 map）：界面宁可显示"-"，
// 也不能把"没算出来"显示成"都已同步"。
func (handler *Handler) ServiceLogPendingHosts(context context.Context, serviceIDs []int64) (map[int64]assets.ServiceLogPendingCounts, error) {
	if len(serviceIDs) == 0 {
		return map[int64]assets.ServiceLogPendingCounts{}, nil
	}
	queries := db.New(handler.db)

	// 每个服务的承载主机 + 已下发指纹；顺便把主机并集收起来给渲染用。
	targets := make(map[int64]serviceLogTargets, len(serviceIDs))
	hostIDs := []int64{}
	seenHost := map[int64]bool{}
	for _, serviceID := range serviceIDs {
		rows, err := queries.ListServiceLogApplyTargets(context, serviceID)
		if err != nil {
			return nil, err
		}
		target := serviceLogTargets{managed: map[int64]serviceAppliedFingerprints{}}
		for _, row := range rows {
			if !row.TargetID.Valid {
				// 未纳管：没有采集目标，配置态不适用（属"下发不到"而不是"待下发"），单列不混进待下发。
				target.unmanaged++
				continue
			}
			target.managed[row.HostID] = serviceAppliedFingerprints{
				service: serviceFingerprintOf(row.ServiceFingerprints, serviceID),
				host:    strings.TrimSpace(row.ConfigFingerprint),
			}
			if !seenHost[row.HostID] {
				seenHost[row.HostID] = true
				hostIDs = append(hostIDs, row.HostID)
			}
		}
		targets[serviceID] = target
	}

	// 主机并集：一次默认集群 + 一次渲染输入（内部两条查询），然后每台主机**渲染一次**。
	// 索引前缀口径与单服务版一致（取默认集群的 index_prefix）。
	cluster, err := queries.GetDefaultEnabledElasticsearchCluster(context)
	if err != nil {
		return nil, err
	}
	outputIdentity, err := filebeatOutputIdentity(cluster.Hosts, cluster.Username, cluster.VerifyTls)
	if err != nil {
		return nil, err
	}
	sets, err := handler.loadHostLogRenderInputs(context, hostIDs)
	if err != nil {
		return nil, err
	}
	expected := make(map[int64]hostExpectedFingerprints, len(hostIDs))
	for _, hostID := range hostIDs {
		set := sets[hostID]
		for index := range set.Entries {
			set.Entries[index].Prefix = cluster.IndexPrefix
		}
		rendered := renderHostLogConfig(set.Entries, set.Instances, outputIdentity)
		expected[hostID] = hostExpectedFingerprints{
			host:    rendered.Fingerprint,
			service: rendered.ServiceFingerprints,
		}
	}
	return countServicePendingHosts(targets, expected), nil
}
