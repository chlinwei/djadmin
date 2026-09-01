package scheduler

import "context"

func (service *Service) SetSchedulerEnabled(ctx context.Context, enabled bool) error {
	return service.repository.SetSchedulerEnabled(ctx, enabled)
}
