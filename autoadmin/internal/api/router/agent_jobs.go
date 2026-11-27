package router

import (
	"context"
	"fmt"
	"time"

	"autoadmin/internal/baseline"
	"autoadmin/internal/inspection"
	"autoadmin/internal/job"
	"autoadmin/internal/monitor"
)

type AgentJobs struct {
	baseline   *baseline.Handler
	inspection *inspection.Handler
	scope      *job.AgentScope
	monitor    *monitor.Handler
}

func (jobs *AgentJobs) Start(ctx context.Context) error {
	resumeBaseline, err := jobs.baseline.RecoverDispatches(ctx)
	if err != nil {
		return fmt.Errorf("recover baseline jobs: %w", err)
	}
	resumeInspection, err := jobs.inspection.RecoverDispatches(ctx)
	if err != nil {
		return fmt.Errorf("recover inspection jobs: %w", err)
	}
	// 只在 API 角色显式启动时恢复通知，而不是在构造路由（含测试和工具）时发信。
	jobs.scope.Go(func(context.Context) { jobs.monitor.RunMaintenanceDelivery(jobs.scope.Stopping()) })
	jobs.scope.Go(func(context.Context) {
		// Avoid treating all persisted targets as offline while agents reconnect.
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-jobs.scope.Stopping().Done():
			return
		case <-timer.C:
		}
		resumeBaseline()
		resumeInspection()
		jobs.inspection.RunScheduler(jobs.scope.Stopping())
	})
	return nil
}

func (jobs *AgentJobs) Shutdown(ctx context.Context) error {
	return jobs.scope.Shutdown(ctx)
}
