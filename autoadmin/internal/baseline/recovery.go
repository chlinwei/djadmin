package baseline

import (
	"context"
	"encoding/json"
	"fmt"

	db "autoadmin/internal/platform/database/generated"
)

type dispatchHost struct {
	Host         scanHost
	InstanceName string
}

type baselineDispatch struct {
	Items []db.ListBaselineItemsRow
	Hosts []dispatchHost
}

// RecoverDispatches runs before this single API instance accepts new work.
func (handler *Handler) RecoverDispatches(ctx context.Context) (func(), error) {
	rows, err := db.New(handler.db).ListRecoverableBaselineDispatches(ctx)
	if err != nil {
		return nil, err
	}
	type pending struct {
		id    int64
		items []db.ListBaselineItemsRow
		hosts []scanHost
	}
	jobs := make([]pending, 0, len(rows))
	for _, row := range rows {
		var payload baselineDispatch
		missing := len(row.DispatchSnapshot) == 0
		if !missing {
			if err := json.Unmarshal(row.DispatchSnapshot, &payload); err != nil {
				return nil, fmt.Errorf("decode baseline scan %d dispatch: %w", row.ID, err)
			}
			missing = len(payload.Items) == 0 || len(payload.Hosts) == 0
		}
		if err := handler.interruptDispatch(ctx, row.ID, missing, true); err != nil {
			return nil, fmt.Errorf("recover baseline scan %d: %w", row.ID, err)
		}
		job := pending{id: row.ID, items: payload.Items}
		if !missing {
			for _, entry := range payload.Hosts {
				entry.Host.InstanceName = entry.InstanceName
				job.hosts = append(job.hosts, entry.Host)
			}
		}
		jobs = append(jobs, job)
	}
	return func() {
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			handler.enqueueScan(job.id, job.items, job.hosts)
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
		if err := queries.ResetBaselineDispatch(ctx, id); err != nil {
			return err
		}
	}
	if err := queries.InterruptBaselineDispatchTargets(ctx, db.InterruptBaselineDispatchTargetsParams{
		ScanID: id, MissingSnapshot: includePending, ErrorMessage: "基线扫描中断，结果未确认，请按需重新发起",
	}); err != nil {
		return err
	}
	return tx.Commit()
}
