package logcollect

import (
	"database/sql"
	"fmt"
	"strings"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
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
)

// LogConfigTargetRef 配置态评估的入参：目标主机 + 其已下发的配置指纹。
// 指纹由调用方从自己已经查出的行里带过来，评估过程不再重复查库。
type LogConfigTargetRef struct {
	HostID             int64
	AppliedFingerprint string
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
func (handler *Handler) EvaluateLogConfigStates(context *gin.Context, refs []LogConfigTargetRef) (map[int64]LogConfigState, error) {
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
