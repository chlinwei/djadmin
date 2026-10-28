package logcollect

import (
	"context"
	"fmt"
	"sort"
	"strings"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
)

// 影响最终采集输入的保存操作统一使用这套"采集配置自洽性"校验。
//
// 为什么要有它：渲染遇到不自洽的配置会拒绝整机下发，可那已经是下发那一步了——
// 用户要等到"某台主机少采了"，或者更糟（同主机同路径会让日志重复进 ES）才发现。
// 这类问题**只能在配置里改**，所以服务、模板、实例及采集开关/覆盖保存时都要挡住：
// 把受影响主机的完整期望配置渲染一遍，只要渲染出"硬问题"就拒绝保存
// （硬问题的定义见 renderedHostLogConfig.Errors）。
//
// 与"软告警"的边界：没挂解析规则所以不采集、主机还没纳管采集目标——这些是**按约定允许存在**的
// 状态（先配模板、后纳管是正常顺序），不能因为它保存不了。
//
// 校验发生在**同一个事务里**（pool 就是那个事务）：保存是"先写库、再校验、失败就回滚"，
// 所以校验读到的必须是刚提交的那套配置，不能另开连接读旧状态。
func (handler *Handler) CheckServiceLogConfigConsistency(context context.Context, pool db.DBTX, serviceID int64) error {
	if serviceID < 1 {
		return nil
	}
	targets, err := listServiceLogApplyTargets(context, pool, serviceID)
	if err != nil {
		return fmt.Errorf("读取承载主机失败: %w", err)
	}
	hostIDs := make([]int64, 0, len(targets))
	for _, target := range targets {
		hostIDs = append(hostIDs, target.HostID)
	}
	return handler.checkHostLogConfigConsistency(context, pool, hostIDs)
}

// 模板变更会同时影响所有引用该模板的服务；按受影响服务收集主机后统一渲染，
// 这样模板日志增删、路径及模板宏变更都与服务保存走同一判定。
func (handler *Handler) CheckTemplateLogConfigConsistency(context context.Context, pool db.DBTX, templateID int64) error {
	if templateID < 1 {
		return nil
	}
	services, err := db.New(pool).ListApplicationServicesByTemplate(context, templateID)
	if err != nil {
		return fmt.Errorf("读取模板关联服务失败: %w", err)
	}
	hostIDs := make([]int64, 0)
	for _, service := range services {
		targets, targetErr := listServiceLogApplyTargets(context, pool, service.ID)
		if targetErr != nil {
			return fmt.Errorf("读取承载主机失败: %w", targetErr)
		}
		for _, target := range targets {
			hostIDs = append(hostIDs, target.HostID)
		}
	}
	return handler.checkHostLogConfigConsistency(context, pool, hostIDs)
}

// 实例主机、实例宏或实例名变更后，校验新主机上的完整配置。
func (handler *Handler) CheckDeploymentLogConfigConsistency(context context.Context, pool db.DBTX, deploymentID int64) error {
	if deploymentID < 1 {
		return nil
	}
	deployment, err := db.New(pool).GetApplicationDeploymentDetail(context, deploymentID)
	if err != nil {
		return fmt.Errorf("读取部署实例失败: %w", err)
	}
	return handler.checkHostLogConfigConsistency(context, pool, []int64{deployment.HostID})
}

func (handler *Handler) checkHostLogConfigConsistency(context context.Context, pool db.DBTX, hostIDs []int64) error {
	if len(hostIDs) == 0 {
		return nil
	}
	uniqueHostIDs := make(map[int64]struct{}, len(hostIDs))
	dedupedHostIDs := make([]int64, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		if _, exists := uniqueHostIDs[hostID]; exists {
			continue
		}
		uniqueHostIDs[hostID] = struct{}{}
		dedupedHostIDs = append(dedupedHostIDs, hostID)
	}
	sets, err := loadHostLogRenderInputsPool(context, pool, dedupedHostIDs)
	if err != nil {
		return fmt.Errorf("渲染期望配置失败: %w", err)
	}
	messages := collectLogConfigProblems(sets, dedupedHostIDs)
	if len(messages) == 0 {
		return nil
	}
	// 返回参数业务错误而不是内部错误，前端要把拒绝保存的原因原样显示出来。
	return apperror.New(apperror.CodeInvalidArgument,
		"日志采集配置不自洽，已拒绝保存（请先修好这些问题）："+strings.Join(messages, "；"))
}

// collectLogConfigProblems 逐主机渲染（与下发同一函数）并汇总"硬问题"，去重后排序返回。
// 抽成纯函数是为了能手搓输入直测判据，不必为此铺一整套 sqlmock。
func collectLogConfigProblems(sets map[int64]renderInputSet, hostIDs []int64) []string {
	problems := map[string]bool{}
	for _, hostID := range hostIDs {
		set := sets[hostID]
		if len(set.Entries) == 0 {
			continue
		}
		rendered := renderHostLogConfig(set.Entries, set.Instances, "")
		for _, problem := range rendered.Errors {
			// 同一路径冲突可能同时发生在多台主机，不能去重后丢失受影响的主机身份。
			problems[fmt.Sprintf("主机 %d：%s", hostID, problem)] = true
		}
	}
	if len(problems) == 0 {
		return nil
	}
	messages := make([]string, 0, len(problems))
	for problem := range problems {
		messages = append(messages, problem)
	}
	sort.Strings(messages)
	return messages
}
