package assets

import (
	"context"

	db "autoadmin/internal/platform/database/generated"
)

// LogConfigConsistencyChecker 由 logcollect 注入（它才持有渲染与宏展开的实现）。
//
// 职责：把受影响主机的期望采集配置**渲染一遍**，报出"硬问题"——配置本身不自洽、
// 只能回到配置里改的那些（路径展不开、同一主机上两个实例展开成同一路径、首行正则/采集过滤
// 正则编译不过）。返回的 error 要能直接给用户看（400 + 原文），因为这就是"保存被拒的原因"。
//
// 服务、模板、部署实例和服务采集开关/覆盖写入都在保存时检查：渲染遇到这些问题是"跳过并告警"
// （不带病下发），但那是下发那一步——
// 用户要等到某台主机少采了、或者更糟（同主机同路径会让同一条日志进 ES 两次）才发现。
//
// **pool 是事务**：调用点在写库之后、提交之前，校验必须读到刚写进去的状态，
// 所以它拿到的是同一个事务句柄，而不是另开连接。
type LogConfigConsistencyChecker interface {
	CheckServiceLogConfigConsistency(ctx context.Context, pool db.DBTX, serviceID int64) error
	CheckTemplateLogConfigConsistency(ctx context.Context, pool db.DBTX, templateID int64) error
	CheckDeploymentLogConfigConsistency(ctx context.Context, pool db.DBTX, deploymentID int64) error
}

// SetLogConfigConsistencyChecker 注入校验器（router 里注入，与 LogFormatVerifier 同一处）。
// 未注入时不做这项校验（单测/无日志域的部署）。
func (s *Service) SetLogConfigConsistencyChecker(checker LogConfigConsistencyChecker) {
	s.logConfigChecker = checker
}

// checkLogConfigConsistency 供保存路径调用：未注入校验器时直接放行。
func (s *Service) checkLogConfigConsistency(ctx context.Context, pool db.DBTX, serviceID int64) error {
	if s.logConfigChecker == nil {
		return nil
	}
	return s.logConfigChecker.CheckServiceLogConfigConsistency(ctx, pool, serviceID)
}

func (s *Service) checkTemplateLogConfigConsistency(ctx context.Context, pool db.DBTX, templateID int64) error {
	if s.logConfigChecker == nil {
		return nil
	}
	return s.logConfigChecker.CheckTemplateLogConfigConsistency(ctx, pool, templateID)
}

func (s *Service) checkDeploymentLogConfigConsistency(ctx context.Context, pool db.DBTX, deploymentID int64) error {
	if s.logConfigChecker == nil {
		return nil
	}
	return s.logConfigChecker.CheckDeploymentLogConfigConsistency(ctx, pool, deploymentID)
}
