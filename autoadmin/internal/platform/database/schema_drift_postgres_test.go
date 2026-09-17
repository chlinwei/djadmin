//go:build postgres

package database_test

import (
	"context"
	"database/sql"
)

func init() {
	schemaDialect = func() string { return "postgres" }
	liveSchemaColumns = func(ctx context.Context, db *sql.DB) (map[string]map[string]bool, error) {
		rows, err := db.QueryContext(ctx,
			"SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = current_schema()")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanSchemaRows(rows)
	}
}
