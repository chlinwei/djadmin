package router

import (
	"context"
	"fmt"
	"time"

	"autoadmin/internal/baseline"
	"autoadmin/internal/inspection"
)

type AgentJobs struct {
	baseline   *baseline.Handler
	inspection *inspection.Handler
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
	go func() {
		// Avoid treating all persisted targets as offline while agents reconnect.
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		resumeBaseline()
		resumeInspection()
		jobs.inspection.StartScheduler(ctx)
	}()
	return nil
}
