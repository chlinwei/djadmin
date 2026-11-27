//go:build !postgres

package database

import "database/sql"

// InsertIfAbsentResult reports whether INSERT IGNORE inserted a row. A skipped
// insert has no new ID; don't interpret LastInsertId before checking RowsAffected.
func InsertIfAbsentResult(result sql.Result, err error) (int64, bool, error) {
	if err != nil {
		return 0, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return 0, false, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
