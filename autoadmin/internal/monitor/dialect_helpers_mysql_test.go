//go:build !postgres

// 按方言编译的测试辅助（与 internal/inspection、internal/assets 的同名文件同一套路）。

package monitor

import (
	"context"
	"database/sql"
	"time"

	"autoadmin/internal/platform/database"

	"github.com/DATA-DOG/go-sqlmock"
)

// openSmokeDatabase 连真库（冒烟用例用）。
func openSmokeDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}

// setTargetInstallStateFragment 给 sqlmock 断言用的 SQL 片段：占位符风格按方言切换
// （sqlc 生成的两侧只差 `?` / `$n`）。
func setTargetInstallStateFragment() string {
	return `SET install_status=?, install_message=?, update_time=?`
}

// expectAlertNotificationEventInsert 登记"建通知事件"的期望。
// MySQL 侧是 `INSERT IGNORE` + `:execresult`：走 Exec，靠影响行数判断是否被唯一键跳过。
func expectAlertNotificationEventInsert(mock sqlmock.Sqlmock, eventType, deduplicationKey string, alertID, id int64, inserted bool) {
	affected := int64(0)
	if inserted {
		affected = 1
	}
	mock.ExpectExec("INSERT IGNORE INTO monitor_alert_notification_event").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), eventType, deduplicationKey, alertID).
		WillReturnResult(sqlmock.NewResult(id, affected))
}

// expectAlertNotificationDeliveryCreate 登记"单地址投递的 get-or-create"期望。
// MySQL 侧是 `:execlastid` + `ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`：走 Exec，
// 主键由 LastInsertId 取回（撞唯一键时返回既有行 id）。
func expectAlertNotificationDeliveryCreate(mock sqlmock.Sqlmock, deliveryID int64) {
	mock.ExpectExec("INSERT INTO monitor_alert_notification_delivery").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "a@b.com", int64(1), int64(2), int64(5)).
		WillReturnResult(sqlmock.NewResult(deliveryID, 1))
}
