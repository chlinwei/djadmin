package logcollect

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	db "autoadmin/internal/platform/database/generated"
)

// 采集目标的**配置态**：后端实时渲染出"期望指纹"，与主机上已下发的 `config_fingerprint` 比对。
// 与运行态（agent/filebeat 是否在跑）正交，见 docs/plans/LOG_COLLECTION_LIFECYCLE.md §2。
const (
	// LogConfigSynced 期望配置与已下发的一致。
	LogConfigSynced = "synced"
	// LogConfigDrift 下发过，但配置已变更（路径/宏/档位/日志定义开关，或默认集群的输出段改了）。
	LogConfigDrift = "drift"
	// LogConfigNever 从未下发过（指纹为空）。
	LogConfigNever = "never"
	// LogConfigUnknown 期望配置算不出来（例如没有启用的默认集群），此时不谎报"已同步"。
	LogConfigUnknown = "unknown"
)

// LogConfigTargetRef 配置态评估的入参：目标主机 + 其已下发的配置指纹。
// 指纹由调用方从自己已经查出的行里带过来，评估过程不再重复查库。
type LogConfigTargetRef struct {
	HostID             int64
	AppliedFingerprint string
	// AppliedServiceFingerprint 该主机上**本服务**上次下发的子指纹（服务视图判状态用）；
	// 主机视图评估留空、不参与。
	AppliedServiceFingerprint string
}

// LogConfigState 单台主机的配置态评估结果。
type LogConfigState struct {
	HostID              int64    `json:"host_id"`
	Status              string   `json:"status"`
	ExpectedFingerprint string   `json:"expected_fingerprint"`
	AppliedFingerprint  string   `json:"applied_fingerprint"`
	ServiceNum          int      `json:"service_num"`
	Warnings            []string `json:"warnings,omitempty"`
}

// EvaluateLogConfigStates 批量评估若干主机的配置态。
//
// 算力约束（500–1000 台规模下的硬要求，见计划 §2.4）：整个评估只做
// **1 次默认集群查询 + 2 次渲染输入查询**，查询次数与主机数无关；渲染是纯函数、无 IO。
// 因此调用方可以按页传（展示场景）或一次传全部（筛选/统计场景）。
//
// 只对**已纳管**的采集目标评估：未纳管的主机没有日志目标，配置态不适用（属监控域的"未纳管"概念）。
//
// 入参是 context.Context 而非 *gin.Context：批量作业的执行发生在 worker 的批量 runner 里，
// 没有 HTTP 请求可依附（*gin.Context 也不能带出请求生命周期——请求一结束它就被取消了）。
func (handler *Handler) EvaluateLogConfigStates(context context.Context, refs []LogConfigTargetRef) (map[int64]LogConfigState, error) {
	states := map[int64]LogConfigState{}
	if len(refs) == 0 {
		return states, nil
	}
	cluster, err := db.New(handler.db).GetDefaultEnabledElasticsearchCluster(context)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("没有已启用的默认 Elasticsearch 集群，请先在日志存储里配置")
	}
	if err != nil {
		return nil, err
	}
	outputIdentity, err := filebeatOutputIdentity(cluster.Hosts, cluster.Username, cluster.VerifyTls)
	if err != nil {
		return nil, err
	}

	hostIDs := make([]int64, 0, len(refs))
	for _, ref := range refs {
		hostIDs = append(hostIDs, ref.HostID)
	}
	sets, err := handler.loadHostLogRenderInputs(context, hostIDs)
	if err != nil {
		return nil, err
	}

	for _, ref := range refs {
		set := sets[ref.HostID]
		// 与下发路径保持同一口径：索引前缀/流名/pipeline 名都取默认集群的 index_prefix。
		for index := range set.Entries {
			set.Entries[index].Prefix = cluster.IndexPrefix
		}
		rendered := renderHostLogConfig(set.Entries, set.Instances, outputIdentity)

		status := LogConfigSynced
		applied := strings.TrimSpace(ref.AppliedFingerprint)
		switch {
		case applied == "":
			status = LogConfigNever
		case applied != rendered.Fingerprint:
			status = LogConfigDrift
		}
		states[ref.HostID] = LogConfigState{
			HostID: ref.HostID, Status: status,
			ExpectedFingerprint: rendered.Fingerprint, AppliedFingerprint: applied,
			ServiceNum: rendered.ServiceNum, Warnings: rendered.Warnings,
		}
	}
	return states, nil
}

// EvaluateServiceLogConfigStates 是服务视图的配置态评估：只比较**本服务**的子指纹。
//
// 与主机级 EvaluateLogConfigStates 的区别只有一个：期望与已下发都取 `ServiceFingerprints[serviceID]`，
// 而不是整机指纹。这样共享主机上改服务 B 只影响 B 的状态，服务 A 保持 synced——A 不再被 B 的改动
// 带成"待下发"（现场问题）。用 id 而不是 code：服务编码允许跨业务/环境重复。
//
// 语义：expected 为空（本服务在该主机没有片段，如停采）时——已下发也为空 → synced（无需采集，已一致）；
// 已下发非空 → drift（主机上还残留该服务的片段，需下发一次清理）。
func (handler *Handler) EvaluateServiceLogConfigStates(context context.Context, serviceID int64, refs []LogConfigTargetRef) (map[int64]LogConfigState, error) {
	states := map[int64]LogConfigState{}
	if len(refs) == 0 {
		return states, nil
	}
	cluster, err := db.New(handler.db).GetDefaultEnabledElasticsearchCluster(context)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("没有已启用的默认 Elasticsearch 集群，请先在日志存储里配置")
	}
	if err != nil {
		return nil, err
	}
	outputIdentity, err := filebeatOutputIdentity(cluster.Hosts, cluster.Username, cluster.VerifyTls)
	if err != nil {
		return nil, err
	}
	hostIDs := make([]int64, 0, len(refs))
	for _, ref := range refs {
		hostIDs = append(hostIDs, ref.HostID)
	}
	sets, err := handler.loadHostLogRenderInputs(context, hostIDs)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		set := sets[ref.HostID]
		for index := range set.Entries {
			set.Entries[index].Prefix = cluster.IndexPrefix
		}
		rendered := renderHostLogConfig(set.Entries, set.Instances, outputIdentity)
		expected := rendered.ServiceFingerprints[strconv.FormatInt(serviceID, 10)]
		applied := strings.TrimSpace(ref.AppliedServiceFingerprint)
		status := serviceConfigStatus(expected, applied, rendered.Fingerprint, strings.TrimSpace(ref.AppliedFingerprint))
		states[ref.HostID] = LogConfigState{
			HostID: ref.HostID, Status: status,
			ExpectedFingerprint: expected, AppliedFingerprint: applied,
			ServiceNum: rendered.ServiceNum, Warnings: rendered.Warnings,
		}
	}
	return states, nil
}

// serviceConfigStatus 服务视图的状态判定（抽成纯函数便于直测）。
//
// expectedService/appliedService 是本服务的子指纹；expectedHost/appliedHost 是整机指纹，只在
// "本服务还没有服务级记录"时作兜底：存量目标 service_fingerprints 为空，但整机配置与期望一致，
// 说明本服务的片段其实已经下发，不该被误报成"从未下发"。
func serviceConfigStatus(expectedService, appliedService, expectedHost, appliedHost string) string {
	switch {
	case appliedService != "" && appliedService != expectedService:
		return LogConfigDrift
	case appliedService == "" && expectedService != "" && appliedHost != expectedHost:
		return LogConfigNever
	default:
		return LogConfigSynced
	}
}
