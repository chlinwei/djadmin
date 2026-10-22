package assets

import (
	"context"
	"testing"

	"autoadmin/internal/shared/pagination"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestHostOrderingReachesSQLBeforePagination(t *testing.T) {
	for _, ordering := range []string{"", "create_time", "-create_time"} {
		t.Run(ordering, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectQuery("SELECT COUNT").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(25))
			mock.ExpectQuery(`(?s)ORDER BY.*CASE WHEN.*create_time.*ASC.*CASE WHEN.*create_time.*DESC.*h.id DESC LIMIT`).
				WithArgs(int64(0), int64(0), int64(0), int64(0),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					ordering, ordering, int32(10), int32(10)).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			_, count, err := NewRepository(database).ListHosts(context.Background(), "", 0, 0, pagination.New(2, 10), ordering)
			if err != nil {
				t.Fatal(err)
			}
			if count != 25 {
				t.Fatalf("count = %d, want 25", count)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
