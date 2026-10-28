package logcollect

import (
	"context"
	"database/sql"
	"fmt"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

type logConfigAuditHost struct {
	HostID           int64    `json:"host_id"`
	HostIP           string   `json:"host_ip"`
	HostInstanceName string   `json:"host_instance_name"`
	Errors           []string `json:"errors"`
}

type logConfigAuditResult struct {
	HostsChecked    int                  `json:"hosts_checked"`
	HostsWithErrors int                  `json:"hosts_with_errors"`
	ProblemCount    int                  `json:"problem_count"`
	Results         []logConfigAuditHost `json:"results"`
}

func (handler *Handler) AuditLogConfigs(c *gin.Context) {
	result, err := handler.auditLogConfigs(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (handler *Handler) auditLogConfigs(ctx context.Context) (logConfigAuditResult, error) {
	result := logConfigAuditResult{Results: []logConfigAuditHost{}}
	// 主机清单和每批配置必须来自同一快照，避免并发编辑使审计拼接出不存在的配置。
	// 只读检查不调用 Agent/ES，也不改变历史配置或已部署的 Filebeat。
	tx, err := handler.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return logConfigAuditResult{}, fmt.Errorf("开始日志配置检查失败: %w", err)
	}
	defer tx.Rollback()
	hosts, err := db.New(tx).ListLogConfigAuditHosts(ctx)
	if err != nil {
		return logConfigAuditResult{}, fmt.Errorf("读取日志配置检查主机失败: %w", err)
	}
	result.HostsChecked = len(hosts)
	const batchSize = 200
	for start := 0; start < len(hosts); start += batchSize {
		batch := hosts[start:min(start+batchSize, len(hosts))]
		ids := make([]int64, 0, len(batch))
		for _, host := range batch {
			ids = append(ids, host.HostID)
		}
		sets, err := loadHostLogRenderInputsPool(ctx, tx, ids)
		if err != nil {
			// 查询失败时整次检查失败，不把已读到的部分清单伪装成完整的通过结果。
			return logConfigAuditResult{}, fmt.Errorf("读取日志配置检查输入失败: %w", err)
		}
		for _, host := range batch {
			problems := collectLogConfigProblems(sets, []int64{host.HostID})
			if len(problems) == 0 {
				continue
			}
			result.Results = append(result.Results, logConfigAuditHost{
				HostID: host.HostID, HostIP: host.HostIp, HostInstanceName: host.HostInstanceName,
				Errors: problems,
			})
			result.ProblemCount += len(problems)
		}
	}
	result.HostsWithErrors = len(result.Results)
	if err := tx.Commit(); err != nil {
		return logConfigAuditResult{}, fmt.Errorf("完成日志配置检查失败: %w", err)
	}
	return result, nil
}
