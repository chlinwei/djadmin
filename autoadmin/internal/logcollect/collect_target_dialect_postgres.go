//go:build postgres

package logcollect

import (
	"context"
	"database/sql"
	"errors"

	db "autoadmin/internal/platform/database/generated"
)

// createLogCollectionTargetIfAbsent 见同名的 mysql 版本：host_id 唯一键冲突时 PG 不返回行。
func createLogCollectionTargetIfAbsent(ctx context.Context, queries *db.Queries, arg db.CreateLogCollectionTargetIfAbsentParams) (int64, bool, error) {
	id, err := queries.CreateLogCollectionTargetIfAbsent(ctx, arg)
	return insertIgnoreOutcome(id, err)
}

// insertIgnoreOutcome 把「INSERT … ON CONFLICT DO NOTHING RETURNING id」的结果翻成
// (id, 是否新建, err)：被跳过时 sqlc 的 :one 走 QueryRow，返回 sql.ErrNoRows。
func insertIgnoreOutcome(id int64, err error) (int64, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
