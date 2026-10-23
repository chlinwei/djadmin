package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

const (
	RefreshInterval  = 5 * time.Second
	executionLimit   = 2
	executionTimeout = 5 * time.Minute
)

type Manager struct {
	repository *Repository
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewManager(repository *Repository) *Manager {
	return &Manager{repository: repository}
}

// Run uses cron only to compute durable cursors. A task lock serializes enqueue,
// execution, and configuration changes across every scheduler/API instance.
func (manager *Manager) Run(ctx context.Context) error {
	manager.mu.Lock()
	if manager.done != nil {
		manager.mu.Unlock()
		return fmt.Errorf("scheduler manager already started")
	}
	ctx, manager.cancel = context.WithCancel(ctx)
	manager.done = make(chan struct{})
	done := manager.done
	manager.mu.Unlock()
	defer close(done)

	// Dispatch may wait for both execution slots, but never occupies the scan
	// loop. The unbuffered channel bounds admission without per-task goroutines.
	work := make(chan int64)
	var workers sync.WaitGroup
	workers.Add(executionLimit + 1)
	go func() {
		defer workers.Done()
		manager.dispatchExecutions(ctx, work)
	}()
	for range executionLimit {
		go func() {
			defer workers.Done()
			manager.runExecutions(ctx, work)
		}()
	}
	defer func() {
		manager.cancel()
		workers.Wait()
	}()

	ticker := time.NewTicker(RefreshInterval)
	defer ticker.Stop()
	for {
		scanCtx, cancel := context.WithTimeout(ctx, RefreshInterval)
		err := manager.scan(scanCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			// Failed scans never become an empty/default configuration. Retry at
			// the normal interval rather than spinning on an unavailable database.
			slog.Error("durable scheduler scan failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (manager *Manager) scan(ctx context.Context) error {
	ids, err := manager.repository.queries.ListSchedulerTaskIDs(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		if err := manager.scanTask(ctx, id, time.Now().UTC()); err != nil {
			failures = append(failures, fmt.Errorf("scan task %d: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

func (manager *Manager) dispatchExecutions(ctx context.Context, work chan<- int64) {
	defer close(work)
	for ctx.Err() == nil {
		readCtx, cancel := context.WithTimeout(ctx, RefreshInterval)
		ids, err := manager.repository.queries.ListSchedulerTaskIDs(readCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Error("list durable scheduler execution candidates", "error", err)
		}
		for _, id := range ids {
			select {
			case work <- id:
			case <-ctx.Done():
				return
			}
		}
		timer := time.NewTimer(RefreshInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (manager *Manager) runExecutions(ctx context.Context, work <-chan int64) {
	for {
		select {
		case <-ctx.Done():
			return
		case id, open := <-work:
			if !open {
				return
			}
			executionCtx, cancel := context.WithTimeout(ctx, executionTimeout)
			err := manager.executeTask(executionCtx, id)
			cancel()
			if err != nil && ctx.Err() == nil {
				slog.Error("durable scheduler execution failed", "task_id", id, "error", err)
			}
		}
	}
}

func (manager *Manager) scanTask(ctx context.Context, id int64, now time.Time) error {
	return manager.repository.transaction(ctx, func(tx *Repository) error {
		task, err := tx.queries.TryLockSchedulerTask(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !IsSupportedTaskCode(task.Code) {
			return nil
		}
		config, err := tx.queries.LockSchedulerConfig(ctx)
		if err != nil {
			return err
		}
		enabled, err := schedulerEnabled(config.Value)
		if err != nil {
			return err
		}
		if !enabled || !task.Enabled {
			return nil
		}
		if task.NextRunTime.Valid && task.NextRunTime.Time.After(now) {
			return nil
		}
		next, err := nextRunAt(task.CronExpression.String, task.ScheduleTimezone, now)
		if err != nil {
			return err
		}
		// A NULL cursor is the explicit cutover/first initialization marker. It
		// starts at now, never backfills an old display snapshot.
		if task.NextRunTime.Valid {
			_, err := tx.queries.GetPendingSchedulerExecution(ctx, id)
			if errors.Is(err, sql.ErrNoRows) {
				if err := tx.enqueue(ctx, id, "automatic", task.NextRunTime.Time, now); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
		// Jump directly past now: any number of missed ticks coalesces into one
		// pending execution, including while a scheduler was offline.
		return tx.queries.SetSchedulerCursor(ctx, db.SetSchedulerCursorParams{NextRunTime: next, ID: id})
	})
}

func (manager *Manager) executeTask(ctx context.Context, id int64) error {
	return manager.repository.transaction(ctx, func(tx *Repository) error {
		task, err := tx.queries.TryLockSchedulerTask(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		execution, err := tx.queries.GetPendingSchedulerExecution(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if execution.TriggerType == "automatic" {
			// Global edits lock every task before changing the switch, so this
			// read is stable while this task is locked without serializing all
			// cleanup transactions on the global configuration row.
			enabled, err := tx.SchedulerEnabled(ctx)
			if err != nil {
				return err
			}
			if !enabled {
				return tx.queries.CancelAutomaticSchedulerExecutions(ctx, db.CancelAutomaticSchedulerExecutionsParams{
					FinishedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, TaskID: id,
				})
			}
		}
		startedAt := time.Now().UTC()
		// sqlc does not generate transaction-control statements. These contain
		// no data SQL; all reads/writes still use transaction-scoped sqlc queries.
		if _, err := tx.tx.ExecContext(ctx, "SAVEPOINT scheduler_cleanup"); err != nil {
			return err
		}
		worker := NewWorker(tx)
		handler := worker.handlers[task.Code]
		var output string
		var executionErr error
		if handler == nil {
			executionErr = fmt.Errorf("%s", UnsupportedTaskNote(task.Name, task.Code))
		} else if !task.Enabled {
			executionErr = ErrTaskDisabled
		} else {
			output, executionErr = handler(ctx)
		}
		status, message, terminal := "成功", "执行成功", "succeeded"
		if executionErr != nil {
			// PostgreSQL aborts a transaction on SQL errors. The savepoint both
			// clears that state and undoes earlier child deletes before logging.
			if _, err := tx.tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT scheduler_cleanup"); err != nil {
				return errors.Join(executionErr, err)
			}
			status, message, terminal = "失败", executionErr.Error(), "failed"
			output = "cleanup rolled back: " + executionErr.Error()
			slog.Error("cleanup failed", "task_id", id, "execution_id", execution.ID, "error", executionErr)
		}
		if _, err := tx.tx.ExecContext(ctx, "RELEASE SAVEPOINT scheduler_cleanup"); err != nil {
			return err
		}
		if err := tx.complete(ctx, id, startedAt, status, message, output); err != nil {
			return err
		}
		// No running claim is committed: task+execution locks fence stale
		// workers, and process death rolls cleanup/log/status back to pending.
		return tx.queries.FinishSchedulerExecution(ctx, db.FinishSchedulerExecutionParams{
			Status: terminal, FinishedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, ID: execution.ID,
		})
	})
}

func (manager *Manager) Shutdown(ctx context.Context) error {
	manager.mu.Lock()
	cancel, done := manager.cancel, manager.done
	manager.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
