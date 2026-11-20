package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

type inspectionDispatch struct {
	Task    runTask
	Targets []runTarget
}

// RecoverDispatches runs before this single API instance accepts new work.
func (handler *Handler) RecoverDispatches(ctx context.Context) (func(), error) {
	rows, err := db.New(handler.db).ListRecoverableInspectionDispatches(ctx)
	if err != nil {
		return nil, err
	}
	type pending struct {
		id int64
		inspectionDispatch
	}
	jobs := make([]pending, 0, len(rows))
	for _, row := range rows {
		job := pending{id: row.ID}
		missing := len(row.DispatchSnapshot) == 0
		if !missing {
			if err := json.Unmarshal(row.DispatchSnapshot, &job.inspectionDispatch); err != nil {
				return nil, fmt.Errorf("decode inspection execution %d dispatch: %w", row.ID, err)
			}
			missing = len(job.Targets) == 0
		}
		if err := handler.interruptDispatch(ctx, row.ID, missing, true); err != nil {
			return nil, fmt.Errorf("recover inspection execution %d: %w", row.ID, err)
		}
		jobs = append(jobs, job)
	}
	return func() {
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			handler.enqueueExecution(job.id, job.Task, job.Targets)
		}
	}, nil
}

func (handler *Handler) interruptDispatch(ctx context.Context, id int64, includePending, reset bool) error {
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := db.New(tx)
	if reset {
		if err := queries.ResetInspectionDispatch(ctx, id); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	if err := queries.InterruptInspectionDispatchTargets(ctx, db.InterruptInspectionDispatchTargetsParams{
		ExecutionID: id, MissingSnapshot: includePending, UpdateTime: now,
		EndTime: sql.NullTime{Time: now, Valid: true}, ErrorMessage: "巡检执行中断，结果未确认，请按需重新发起",
	}); err != nil {
		return err
	}
	return tx.Commit()
}
