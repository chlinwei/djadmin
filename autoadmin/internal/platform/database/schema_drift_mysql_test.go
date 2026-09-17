//go:build !postgres

package database_test

import (
	"context"
	"database/sql"
)

func init() {
	schemaDialect = func() string { return "mysql" }
	liveSchemaColumns = func(ctx context.Context, db *sql.DB) (map[string]map[string]bool, error) {
		rows, err := db.QueryContext(ctx,
			"SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE()")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanSchemaRows(rows)
	}
}
