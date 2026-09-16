//go:build postgres

// 按方言编译的测试辅助：见同名的 !postgres 版本。

package baseline

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"time"

	"autoadmin/internal/platform/database"

	"github.com/DATA-DOG/go-sqlmock"
)

// expectCreateReturnsID 的 PostgreSQL 版本：`:one` + RETURNING 走 QueryRow。
func expectCreateReturnsID(mock sqlmock.Sqlmock, fragment string, args []driver.Value, id int64) {
	mock.ExpectQuery(regexp.QuoteMeta(fragment)).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
}

// openSmokeDatabase 连真库（冒烟用例用）：-tags postgres 连 PostgreSQL。
func openSmokeDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		PostgresDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}
