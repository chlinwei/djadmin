package batchwrite

import "database/sql"

// RequireRow distinguishes a committed mutation from a successful no-op.
func RequireRow(affected int64, err error) error {
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

type ItemError struct {
	ID    int64
	Cause error
}

func (e *ItemError) Error() string { return e.Cause.Error() }
func (e *ItemError) Unwrap() error { return e.Cause }

func UniqueIDs[T ~int32 | ~int64](ids []T) []T {
	seen := make(map[T]struct{}, len(ids))
	unique := make([]T, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	return unique
}

// Each stops at the first failure so its caller can roll back the transaction.
func Each[T ~int32 | ~int64](ids []T, write func(T) error) error {
	for _, id := range ids {
		if err := write(id); err != nil {
			return &ItemError{int64(id), err}
		}
	}
	return nil
}
