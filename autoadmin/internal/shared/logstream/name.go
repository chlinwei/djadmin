// Package logstream 统一构造逻辑服务级 data stream 名，
// 供 monitor（Fluent Bit 渲染 / 流状态解析）与 assets（逻辑服务弹窗预览）共用，
// 任何生成或解析流名的地方禁止自行拼接。
package logstream

import "strings"

// Name 构造 data stream 名：logs-<项目>-<业务系统>-<环境>-<逻辑服务>-<档位>。
// 参数顺序保持 (environment, businessSystem) 与调用方字段一致，拼接顺序为
// 业务系统在前、环境在后（2026-09 调整：层级树按 项目→业务系统→环境 展示，
// 流名段序与层级树保持一致）。
func Name(prefix, project, environment, businessSystem, service, tier string) string {
	base := prefix
	if base == "" {
		base = "logs"
	}
	return strings.Join([]string{base, project, businessSystem, environment, service, tier}, "-")
}
