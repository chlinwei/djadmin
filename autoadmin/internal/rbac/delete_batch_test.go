package rbac

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"autoadmin/internal/shared/batchwrite"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBatchDeleteRolesRollsBackOnMissingRole(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	for _, id := range []int32{1, 2} {
		mock.ExpectExec("DELETE FROM sys_user_role WHERE role_id").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM sys_role_menu WHERE role_id").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM sys_role WHERE id").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, int64(2-id)))
	}
	mock.ExpectRollback()
	err = NewRepository(pool).DeleteRoles(context.Background(), []int32{1, 2})
	var item *batchwrite.ItemError
	if !errors.As(err, &item) || item.ID != 2 || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want missing role 2, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
