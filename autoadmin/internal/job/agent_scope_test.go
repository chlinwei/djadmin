package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAgentScopeStopsNewClaimsAndDrainsActiveWork(t *testing.T) {
	scope := NewAgentScope()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	scope.Go(func(work context.Context) {
		started <- work
		select {
		case <-release:
		case <-work.Done():
		}
	})
	work := <-started
	waiting := make(chan bool, 1)
	full := make(chan struct{}, 1)
	full <- struct{}{}
	go func() { waiting <- AcquireAgentSlot(ctx, scope.Stopping(), full) }()

	drained := make(chan error, 1)
	go func() { drained <- scope.Shutdown(ctx) }()
	select {
	case <-scope.Stopping().Done():
	case <-ctx.Done():
		t.Fatal("admission did not close")
	}
	if <-waiting {
		t.Fatal("a queued target acquired a slot during shutdown")
	}
	if AcquireAgentSlot(ctx, scope.Stopping(), make(chan struct{}, 1)) {
		t.Fatal("an available slot allowed a new claim during shutdown")
	}
	if work.Err() != nil {
		t.Fatal("shutdown canceled in-flight work before the drain deadline")
	}
	select {
	case err := <-drained:
		t.Fatalf("drain returned before work finished: %v", err)
	default:
	}
	close(release)
	if err := <-drained; err != nil {
		t.Fatalf("drain after work finished: %v", err)
	}
}

func TestAgentScopeCancelsActiveWorkAtShutdownDeadline(t *testing.T) {
	scope := NewAgentScope()
	finished := make(chan struct{})
	scope.Go(func(work context.Context) {
		<-work.Done()
		close(finished)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := scope.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown = %v; want deadline exceeded", err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight work was not canceled")
	}
}
