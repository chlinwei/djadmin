//go:build !postgres

package monitor

import (
	"context"
	"database/sql"

	db "autoadmin/internal/platform/database/generated"
)

// createAlertNotificationEventIfAbsent 建一条通知事件，返回 (事件 id, 是否新建)。
//
// 与 createMonitorTargetIfAbsent 同一手法（见 target_dialect_mysql.go）：deduplication_key 是唯一键，
// 两侧的"已入队就跳过"表达方式不同，所以按方言分文件：
//   - MySQL：`INSERT IGNORE`，靠 sql.Result 的 RowsAffected()==0 判断被跳过，主键由 LastInsertId 取；
//   - PostgreSQL：派生把语句改写成 `INSERT … ON CONFLICT DO NOTHING`，被跳过时 RETURNING 不返回行走
//     sql.ErrNoRows（见 alert_event_dialect_postgres.go）。
//
// 语义差异：MySQL 的 INSERT IGNORE 会吞掉外键等其它可忽略错误，PG 的 ON CONFLICT 只处理唯一冲突。
// 这里的唯一键就是 deduplication_key，调用点此前已按 alert_id 读/建告警行，所以落在唯一冲突这一类。
func createAlertNotificationEventIfAbsent(ctx context.Context, queries *db.Queries, arg db.CreateAlertNotificationEventIfAbsentParams) (int64, bool, error) {
	result, err := queries.CreateAlertNotificationEventIfAbsent(ctx, arg)
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
