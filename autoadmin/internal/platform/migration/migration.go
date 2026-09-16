package migration

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// 数据库驱动按构建标签在 driver_mysql.go / driver_postgres.go 里注册。

func Up(sourceURL string, databaseURL string) error {
	migrator, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
