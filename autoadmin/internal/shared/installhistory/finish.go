package installhistory

import (
	"context"
	"database/sql"
	"errors"

	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
)

// Finish claims this specific history before touching the target. Otherwise a
// late result from a cancelled job can consume the next job's pending state.
func Finish(ctx context.Context, pool *sql.DB, history db.FinishTargetInstallHistoryParams, updateTarget func(*db.Queries) (int64, error)) (bool, error) {
	var claimed bool
	err := database.InTransaction(ctx, pool, func(q *db.Queries) error {
		n, err := q.FinishTargetInstallHistory(ctx, history)
		if err != nil || n == 0 {
			return err
		}
		n, err = updateTarget(q)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("install target is no longer pending")
		}
		claimed = true
		return nil
	})
	return claimed && err == nil, err
}
