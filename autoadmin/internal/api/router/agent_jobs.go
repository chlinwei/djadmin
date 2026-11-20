package router

import (
	"context"
	"fmt"
	"time"

	"autoadmin/internal/baseline"
	"autoadmin/internal/inspection"
	"autoadmin/internal/job"
)

type AgentJobs struct {
	baseline   *baseline.Handler
	inspection *inspection.Handler
	scope      *job.AgentScope
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
