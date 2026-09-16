//go:build !postgres

// 按方言编译的测试辅助：两侧生成的代码在 Go 层签名一致，但驱动调用形态与连接参数不同，
// 差异集中在本文件与同名的 postgres 版本里，业务用例不必感知。

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

// expectCreateReturnsID 登记一条"插入并取回自增主键"的期望：MySQL 侧是 `:execlastid`
// （ExecContext + LastInsertId），PG 侧派生为 `:one`+RETURNING（QueryRowContext）。
// 见 internal/platform/database/derive/derive.go 的 appendReturningID。
func expectCreateReturnsID(mock sqlmock.Sqlmock, fragment string, args []driver.Value, id int64) {
	mock.ExpectExec(regexp.QuoteMeta(fragment)).WithArgs(args...).WillReturnResult(sqlmock.NewResult(id, 1))
}

// openSmokeDatabase 连真库（冒烟用例用）：默认构建连 MySQL。
func openSmokeDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}
