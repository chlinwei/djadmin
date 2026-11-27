//go:build postgres

package database

import (
	"database/sql"
	"errors"
)

// InsertIfAbsentResult maps an empty ON CONFLICT DO NOTHING RETURNING result
// to a skipped insert, not a failed lookup.
func InsertIfAbsentResult(id int64, err error) (int64, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
