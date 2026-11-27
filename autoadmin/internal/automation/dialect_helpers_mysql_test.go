//go:build !postgres

// 按方言编译的测试辅助：两侧生成的代码在 Go 层签名一致，但连接参数不同，
// 差异集中在本文件与同名的 postgres 版本里，业务用例不必感知。
// （与 internal/baseline、internal/inspection 的同名文件同一套路。）

package automation

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"autoadmin/internal/platform/database"
)

func hostIDArgs(ids ...int64) []driver.Value {
	args := make([]driver.Value, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// openSmokeDatabase 连真库（冒烟用例用）：默认构建连 MySQL。
func openSmokeDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}
