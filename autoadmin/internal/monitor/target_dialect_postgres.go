//go:build postgres

package monitor

import (
	"context"
	"database/sql"
	"errors"

	db "autoadmin/internal/platform/database/generated"
)

// createMonitorTargetIfAbsent 见同名的 mysql 版本：PG 侧 INSERT 被派生为
// `INSERT … ON CONFLICT DO NOTHING RETURNING id`，被跳过时不返回行（ErrNoRows）。
func createMonitorTargetIfAbsent(ctx context.Context, queries *db.Queries, arg db.CreateMonitorTargetIfAbsentParams) (int64, bool, error) {
	id, err := queries.CreateMonitorTargetIfAbsent(ctx, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
