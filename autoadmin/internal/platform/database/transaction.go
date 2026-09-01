package database

import (
	"context"
	"database/sql"
	"fmt"

	db "autoadmin/internal/platform/database/generated"
)

func InTransaction(ctx context.Context, pool *sql.DB, operation func(*db.Queries) error) error {
	transaction, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer transaction.Rollback()

	if err := operation(db.New(transaction)); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
