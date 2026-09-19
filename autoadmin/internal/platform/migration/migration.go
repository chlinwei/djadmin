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

// Force 把迁移版本表强行置为 version 并清除 dirty 标记（golang-migrate 的 Force）。
//
// 用途：**迁移在真库上失败之后**。golang-migrate 在跑某个版本前先把版本表写成
// `(version=N, dirty=1)`，失败后状态就停在脏标记上，后续 `migrate` 会直接拒绝执行。
// DDL 失败的那一步通常没有落库（ALTER 报错即回滚该语句），所以正确用法是：
//
//	autoadmin migrate force <失败前的版本号>   # 清脏标记并退回上一个已完成版本
//	autoadmin migrate                         # 修好迁移文件后再从头跑那一个版本
//
// 注意它只改版本号、**不动 schema**：把版本号置成与库内实际结构不符的值会让迁移链错位，
// 只在"确认那一步没落库"时使用；不确定就先看 `schema_migrations` 表与库内实际结构。
func Force(sourceURL string, databaseURL string, version int64) error {
	migrator, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Force(int(version)); err != nil {
		return fmt.Errorf("force migration version: %w", err)
	}
	return nil
}
