//go:build !postgres

// 按方言编译的测试辅助（与 internal/monitor、internal/inspection、internal/assets 的同名文件同一套路）。

package logcollect

import (
	"context"
	"database/sql"
	"time"

	"autoadmin/internal/platform/database"
)

// openSmokeDatabase 连真库（冒烟用例用；未设 DSN 时调用方自行 t.Skip）。
func openSmokeDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}
