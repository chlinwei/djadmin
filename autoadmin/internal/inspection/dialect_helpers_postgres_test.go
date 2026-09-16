//go:build postgres

// 按方言编译的测试辅助：两侧生成的代码在 Go 层签名一致，但驱动调用形态与连接参数不同，
// 差异集中在本文件与同名的 mysql 版本里，业务用例不必感知。
// （与 internal/baseline 的同名文件同一套路。）

package inspection

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"time"

	"autoadmin/internal/platform/database"

	"github.com/DATA-DOG/go-sqlmock"
)

// expectCreateReturnsID 登记一条"插入并取回自增主键"的期望：PG 侧 INSERT 被派生为
// `:one` + RETURNING id，走 QueryRowContext 而不是 Exec（pgx 不实现 LastInsertId）。
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
