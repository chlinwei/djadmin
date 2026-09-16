//go:build !postgres

package monitor

import (
	"context"

	db "autoadmin/internal/platform/database/generated"
)

// createMonitorTargetIfAbsent 插入一条监控目标，返回 (目标 id, 是否新建)。
//
// 两侧的"已纳管就跳过"表达方式不同，所以按方言分文件：
//   - MySQL：`INSERT IGNORE`，靠 sql.Result 的 RowsAffected()==0 判断被跳过，主键由 LastInsertId 取；
//   - PostgreSQL：派生把语句改写成 `INSERT … ON CONFLICT DO NOTHING`，被跳过时 RETURNING 不返回行走
//     sql.ErrNoRows（见 target_dialect_postgres.go）。
//
// 语义差异：MySQL 的 INSERT IGNORE 会吞掉外键等其它可忽略错误，PG 的 ON CONFLICT 只处理唯一冲突。
// 这里的唯一键是 (host_id, exporter_type)，调用点此前刚校验过主机存在，所以落在唯一冲突这一类。
func createMonitorTargetIfAbsent(ctx context.Context, queries *db.Queries, arg db.CreateMonitorTargetIfAbsentParams) (int64, bool, error) {
	result, err := queries.CreateMonitorTargetIfAbsent(ctx, arg)
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
