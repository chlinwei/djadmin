package job

import (
	"context"
	"sync"
)

// AgentScope stops accepting work before canceling in-flight Agent requests.
type AgentScope struct {
	stopping context.Context
	stop     context.CancelFunc
	work     context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	active   sync.WaitGroup
}

func NewAgentScope() *AgentScope {
	stopping, stop := context.WithCancel(context.Background())
	work, cancel := context.WithCancel(context.Background())
	return &AgentScope{stopping: stopping, stop: stop, work: work, cancel: cancel}
}

func (s *AgentScope) Stopping() context.Context { return s.stopping }

func (s *AgentScope) Go(run func(context.Context)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping.Err() != nil {
		return
	}
	s.active.Go(func() { run(s.work) })
}

func (s *AgentScope) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stop()
	s.mu.Unlock()
	defer s.cancel()

	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func AcquireAgentSlot(ctx, stopping context.Context, slots chan struct{}) bool {
	select {
	case <-ctx.Done():
		return false
	case <-stopping.Done():
		return false
	case slots <- struct{}{}:
		if ctx.Err() == nil && stopping.Err() == nil {
			return true
		}
		<-slots
		return false
	}
}
