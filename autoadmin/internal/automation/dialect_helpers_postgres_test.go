//go:build postgres

// 按方言编译的测试辅助：两侧生成的代码在 Go 层签名一致，但连接参数不同，
// 差异集中在本文件与同名的 mysql 版本里，业务用例不必感知。
// （与 internal/baseline、internal/inspection 的同名文件同一套路。）

package automation

import (
	"context"
	"database/sql"
	"time"

	"autoadmin/internal/platform/database"
)

// openSmokeDatabase 连真库（冒烟用例用）：-tags postgres 连 PostgreSQL。
func openSmokeDatabase(ctx context.Context, dsn string) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		PostgresDSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
}
