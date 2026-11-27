package installhistory

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
)

type Source int

const (
	History Source = iota
	Exporter
	Filebeat
)

// Cancel locks the latest history before changing either record. A request from
// the history page must not reset a target now owned by a newer installation.
func Cancel(ctx context.Context, pool *sql.DB, source Source, id int64) error {
	if id <= 0 {
		return apperror.New(400, "invalid id")
	}
	return database.InTransaction(ctx, pool, func(q *db.Queries) error {
		targetID := sql.NullInt64{Int64: id, Valid: true}
		kind := source
		if source == History {
			h, err := q.GetInstallHistoryForUpdate(ctx, id)
			if errors.Is(err, sql.ErrNoRows) {
				return apperror.ErrResourceNotFound
			}
			if err != nil {
				return err
			}
			switch {
			case h.TargetID.Valid:
				kind, targetID = Exporter, h.TargetID
			case h.LogCollectionTargetID.Valid:
				kind, targetID = Filebeat, h.LogCollectionTargetID
			default:
				return apperror.New(400, "task has no managed target")
			}
		}
		var historyID int64
		var status string
		var start sql.NullTime
		var err error
		switch kind {
		case Exporter:
			var row db.GetLatestTargetInstallHistoryRow
			row, err = q.GetLatestTargetInstallHistory(ctx, targetID)
			historyID, status, start = row.ID, row.Status, row.StartTime
		case Filebeat:
			var row db.GetLatestLogTargetInstallHistoryForUpdateRow
			row, err = q.GetLatestLogTargetInstallHistoryForUpdate(ctx, targetID)
			historyID, status, start = row.ID, row.Status, sql.NullTime{Time: row.CreateTime, Valid: true}
		default:
			return errors.New("invalid install history source")
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err != nil || (source == History && historyID != id) || (status != "pending" && status != "running") {
			return apperror.New(400, "current task has ended and does not need cancellation")
		}
		now := time.Now().UTC()
		duration := sql.NullFloat64{}
		if start.Valid {
			duration = sql.NullFloat64{Float64: now.Sub(start.Time).Seconds(), Valid: true}
		}
		if err := q.CancelInstallHistory(ctx, db.CancelInstallHistoryParams{
			ID: historyID, EndTime: sql.NullTime{Time: now, Valid: true}, DurationSeconds: duration, UpdateTime: now,
		}); err != nil {
			return err
		}
		if kind == Filebeat {
			return q.CancelLogTargetInstallState(ctx, db.CancelLogTargetInstallStateParams{ID: targetID.Int64, UpdateTime: now})
		}
		return q.CancelMonitorTargetInstallState(ctx, db.CancelMonitorTargetInstallStateParams{ID: targetID.Int64, UpdateTime: now})
	})
}
