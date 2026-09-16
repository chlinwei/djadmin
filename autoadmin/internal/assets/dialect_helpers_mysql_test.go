//go:build !postgres

// 按方言编译的测试辅助（与 internal/baseline、internal/inspection 的同名文件同一套路）。

package assets

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"autoadmin/internal/platform/database"

	"github.com/DATA-DOG/go-sqlmock"
)

// openSmokeDatabase 连真库（冒烟用例用）：mysql 侧连接。
func openSmokeDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}

// agentListTargetArgs 给出"按 host_ids 批量取主机"在该方言下的驱动参数个数：
// MySQL 的 IN (?,?) 展开成 N 个参数，PG 侧是单个数组参数（= ANY($1::bigint[])）。
// 具体值由 mock 返回的行断言，这里只钉住个数与 SQL 形态。
func agentListTargetArgs(count int) []driver.Value {
	if false {
		return []driver.Value{sqlmock.AnyArg()}
	}
	args := make([]driver.Value, 0, count)
	for index := 0; index < count; index++ {
		args = append(args, sqlmock.AnyArg())
	}
	return args
}
