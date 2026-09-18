//go:build !postgres

package logcollect

import (
	"context"
	"database/sql"

	db "autoadmin/internal/platform/database/generated"
)

// createLogCollectionTargetIfAbsent 建纳管目标，返回 (目标 id, 是否新建)。
//
// monitor_log_collection_target 上 host_id 是唯一键，"已纳管就跳过"两种方言的表达方式不同，
// 因此按方言分文件（与 monitor 的 target_dialect_*.go、alert_event_dialect_*.go 同一手法）：
//   - MySQL：`INSERT IGNORE`，靠 sql.Result 的 RowsAffected()==0 判断被跳过，主键由 LastInsertId 取；
//   - PostgreSQL：派生把语句改写成 `INSERT … ON CONFLICT DO NOTHING RETURNING id`，被跳过时
//     不返回行走 sql.ErrNoRows（见 collect_target_dialect_postgres.go）。
//
// 语义差异：MySQL 的 INSERT IGNORE 会吞掉外键等其它可忽略错误，PG 的 ON CONFLICT 只处理唯一冲突。
// 这里的唯一键就是 host_id，因此落在唯一冲突这一类。
func createLogCollectionTargetIfAbsent(ctx context.Context, queries *db.Queries, arg db.CreateLogCollectionTargetIfAbsentParams) (int64, bool, error) {
	result, err := queries.CreateLogCollectionTargetIfAbsent(ctx, arg)
	return insertIgnoreOutcome(result, err)
}

// insertIgnoreOutcome 把「INSERT IGNORE 的 sql.Result」翻成 (id, 是否新建, err)：
// 影响行数为 0 即撞唯一键被跳过。
func insertIgnoreOutcome(result sql.Result, err error) (int64, bool, error) {
	if err != nil {
		return 0, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if affected == 0 {
		return 0, false, nil
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
