package logcollect

import (
	"context"
	"database/sql"
	"strconv"

	db "autoadmin/internal/platform/database/generated"
)

const sharedSourceCodePrefix = "shared-source-"

func sharedSourceCode(id int64) string {
	return sharedSourceCodePrefix + strconv.FormatInt(id, 10)
}

func retireSharedSourceGeneration(ctx context.Context, q *db.Queries, source db.MonitorSharedLogSource) error {
	if err := q.CreateSharedLogSourceRevocation(ctx, db.CreateSharedLogSourceRevocationParams{
		SourceID: source.ID, Generation: source.Generation,
	}); err != nil {
		return err
	}
	// Keep old revocation epochs fenced so queued legacy config cannot restart a stopped source.
	return q.AdvanceSharedLogSourceGeneration(ctx, source.ID)
}

func (handler *Handler) detachServiceSharedSources(ctx context.Context, serviceID int64) error {
	ids, err := db.New(handler.db).ListSharedLogServiceSourceIDs(ctx, serviceID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = handler.unlinkSharedSourceServices(ctx, id, []int64{serviceID}); err != nil {
			return err
		}
	}
	return nil
}

func containsServiceID(ids []int64, id int64) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func (handler *Handler) unlinkSharedSourceServices(ctx context.Context, sourceID int64, serviceIDs []int64) error {
	tx, err := handler.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	source, err := q.LockSharedLogSource(ctx, sourceID)
	if err != nil {
		return err
	}
	members, err := q.ListSharedLogSourceServiceIDs(ctx, sourceID)
	if err != nil {
		return err
	}
	removed := 0
	for _, serviceID := range serviceIDs {
		if !containsServiceID(members, serviceID) {
			continue
		}
		if err = q.DeleteSharedLogSourceServiceLink(ctx, db.DeleteSharedLogSourceServiceLinkParams{SourceID: sourceID, ServiceID: serviceID}); err != nil {
			return err
		}
		removed++
	}
	if removed > 0 {
		if removed == len(members) {
			err = retireSharedSourceGeneration(ctx, q, source)
		} else {
			err = q.TouchSharedLogSource(ctx, sourceID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
