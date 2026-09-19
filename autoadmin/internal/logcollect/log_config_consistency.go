package logcollect

import (
	"context"
	"fmt"
	"sort"
	"strings"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
)

// 保存逻辑服务时的"采集配置自洽性"校验（2026-09-19）。
//
// 为什么要有它：渲染遇到不自洽的配置是**跳过并告警**（不带病下发），可那已经是下发那一步了——
// 用户要等到"某台主机少采了"，或者更糟（同主机同路径会让日志重复进 ES）才发现。
// 这类问题**只能在配置里改**，所以保存时就要挡住：把该服务各承载主机的期望配置渲染一遍，
// 只要渲染出"硬问题"就拒绝保存（硬问题的定义见 renderedHostLogConfig.Errors）。
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
	targets, err := db.New(pool).ListServiceLogApplyTargets(context, serviceID)
	if err != nil {
		return fmt.Errorf("读取承载主机失败: %w", err)
	}
	hostIDs := make([]int64, 0, len(targets))
	for _, target := range targets {
		hostIDs = append(hostIDs, target.HostID)
	}
	if len(hostIDs) == 0 {
		return nil
	}
	sets, err := loadHostLogRenderInputsPool(context, pool, hostIDs)
	if err != nil {
		return fmt.Errorf("渲染期望配置失败: %w", err)
	}
	messages := collectLogConfigProblems(sets, hostIDs)
	if len(messages) == 0 {
		return nil
	}
	// 返回 400 而不是内部错误：这是"配置要改"，前端要把原因原样显示出来。
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
			problems[problem] = true
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
