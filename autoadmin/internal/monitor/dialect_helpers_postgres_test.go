//go:build postgres

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
		PostgresDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}

// setTargetInstallStateFragment 见 mysql 版本。
func setTargetInstallStateFragment() string {
	return `SET install_status=$1, install_message=$2, update_time=$3`
}

// expectAlertNotificationEventInsert 见 mysql 版本：PG 侧派生为
// `INSERT … ON CONFLICT DO NOTHING RETURNING id`（`:one` 走 QueryRow），
// 被唯一键跳过时不返回行 —— sql.ErrNoRows。
func expectAlertNotificationEventInsert(mock sqlmock.Sqlmock, eventType, deduplicationKey string, alertID, id int64, inserted bool) {
	expectation := mock.ExpectQuery("INSERT INTO monitor_alert_notification_event").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), eventType, deduplicationKey, alertID)
	if !inserted {
		expectation.WillReturnError(sql.ErrNoRows)
		return
	}
	expectation.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
}

// expectAlertNotificationDeliveryCreate 见 mysql 版本：PG 侧是
// `ON CONFLICT (event_id, media_id, user_id, address) DO UPDATE SET id=<表>.id RETURNING id`，
// 撞唯一键时返回的仍是既有行 id（`:one` 走 QueryRow）。
func expectAlertNotificationDeliveryCreate(mock sqlmock.Sqlmock, deliveryID int64) {
	mock.ExpectQuery("INSERT INTO monitor_alert_notification_delivery").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "a@b.com", int64(1), int64(2), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(deliveryID))
}
