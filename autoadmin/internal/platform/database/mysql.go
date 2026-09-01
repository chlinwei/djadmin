package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

type MySQLConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func OpenMySQL(ctx context.Context, configuration MySQLConfig) (*sql.DB, error) {
	driverConfig, err := mysql.ParseDSN(configuration.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse MySQL DSN: %w", err)
	}
	driverConfig.ParseTime = true
	driverConfig.Loc = time.UTC

	connector, err := mysql.NewConnector(driverConfig)
	if err != nil {
		return nil, fmt.Errorf("create MySQL connector: %w", err)
	}

	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(configuration.MaxOpenConns)
	database.SetMaxIdleConns(configuration.MaxIdleConns)
	database.SetConnMaxLifetime(configuration.ConnMaxLifetime)

	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping MySQL: %w", err)
	}
	return database, nil
}
