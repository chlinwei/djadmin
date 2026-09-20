package assets

import "context"

// 日志路径通配展开（按需）：界面「解析后」列对含 `*` / `?` / `[` 的路径给出「展开」能力，
// 调 agent 列出该服务各承载实例上真实匹配到的文件。
//
// 为什么不进 log-config 响应：那是一个纯读库、供表格一次性加载的接口；通配展开要逐台主机调
// agent，慢且依赖主机在线，塞进去会让整张表等主机。所以拆成一个独立的按需接口：
// 只有用户点了「展开」才发一次请求，一次只展开一条 (服务 × 日志定义)。
//
// 分层与格式认证相同：接口定义在 assets（消费方），实现放 logcollect（它持有 agent 文件通道）；
// 反向依赖会成环（见 log_format_verify.go 的说明）。

// LogGlobPreviewRequest 通配展开的输入。
type LogGlobPreviewRequest struct {
	ServiceID       int64
	LogDefinitionID int64
}

// LogGlobInstanceMatches 一台承载实例上某条日志路径展开出的文件清单。
//
// Pattern 是该实例**展开宏后**的路径（仍含通配）；Matches 是 agent 匹配到的全部普通文件
// （按路径排序）。某台主机离线/路径匹配不到时，Error 写明原因，其余实例照常返回——
// 不能因为一台机器失败就让整个展开没有结果。
type LogGlobInstanceMatches struct {
	HostInstanceName       string   `json:"host_instance_name"`
	DeploymentInstanceName string   `json:"deployment_instance_name"`
	Pattern                string   `json:"pattern"`
	Matches                []string `json:"matches"`
	Error                  string   `json:"error,omitempty"`
}

// LogGlobPreview 一条日志定义的展开结果：按承载实例分组。
//
// 前端把它渲染在「解析后」列的同一格里（一条日志 = 一个整体），实例名作分组标题。
type LogGlobPreview struct {
	PathPattern string                   `json:"path_pattern"`
	Instances   []LogGlobInstanceMatches `json:"instances"`
}

// LogGlobPreviewer 由 logcollect 实现：展开某条日志定义在各承载实例上的通配路径。
type LogGlobPreviewer interface {
	PreviewLogGlob(ctx context.Context, request LogGlobPreviewRequest) (LogGlobPreview, error)
}

// SetLogGlobPreviewer 注入通配展开执行器；未注入时 PreviewServiceLogGlob 返回不可用。
func (s *Service) SetLogGlobPreviewer(previewer LogGlobPreviewer) {
	s.logGlobPreviewer = previewer
}

// PreviewServiceLogGlob 展开一条 (逻辑服务 × 日志定义) 的通配路径。
func (s *Service) PreviewServiceLogGlob(ctx context.Context, serviceID, logDefinitionID int64) (LogGlobPreview, error) {
	if s.logGlobPreviewer == nil {
		return LogGlobPreview{}, ErrLogGlobPreviewUnavailable
	}
	return s.logGlobPreviewer.PreviewLogGlob(ctx, LogGlobPreviewRequest{
		ServiceID: serviceID, LogDefinitionID: logDefinitionID,
	})
}
