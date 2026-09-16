// Package logstream 统一构造逻辑服务级 data stream 名，
// 供 monitor（Fluent Bit 渲染 / 流状态解析）与 assets（逻辑服务弹窗预览）共用，
// 任何生成或解析流名的地方禁止自行拼接。
package logstream

import "strings"

// Name 构造 data stream 名：logs-<项目>-<环境>-<业务系统>-<逻辑服务>-<档位>。
func Name(prefix, project, environment, businessSystem, service, tier string) string {
	base := prefix
	if base == "" {
		base = "logs"
	}
	return strings.Join([]string{base, project, environment, businessSystem, service, tier}, "-")
}
