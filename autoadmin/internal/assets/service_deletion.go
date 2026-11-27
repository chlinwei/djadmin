package assets

import (
	"context"
	"database/sql"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/batchwrite"
)

// The tombstone commits before external work. Failures retain both identity and membership,
// so retries always address the same hosts and ES ownership scope, even after an API restart.
func (r *Repository) beginServiceDeletion(ctx context.Context, id int64) error {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.LockApplicationServiceForDeletion(ctx, id); err != nil {
		return err
	}
	// Check before tombstoning or external cleanup, sharing the endpoint lock with dependency writes.
	count, err := q.CountServiceDependenciesForService(ctx, db.CountServiceDependenciesForServiceParams{SourceServiceID: id, TargetServiceID: id})
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrServiceHasDependencies
	}
	businessID, err := q.GetServiceDeletionBusinessSystem(ctx, id)
	if err != nil {
		return err
	}
	if _, err = q.LockServiceIdentityDomain(ctx, businessID); err != nil {
		return err
	}
	if _, err = q.GetServiceDeletionTombstone(ctx, id); err == sql.ErrNoRows {
		dims, err := q.GetApplicationServiceStreamDims(ctx, id)
		if err != nil {
			return err
		}
		if err = q.CreateServiceDeletionTombstone(ctx, db.CreateServiceDeletionTombstoneParams{
			ServiceID: id, ProjectCode: dims.ProjectCode, BusinessSystemCode: dims.BusinessSystemCode,
			EnvironmentCode: dims.EnvironmentCode, ServiceCode: dims.ServiceCode,
		}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err = q.UpdateApplicationServiceLogCollection(ctx, db.UpdateApplicationServiceLogCollectionParams{
		ID: id, UpdateTime: time.Now().UTC(), LogCollectionEnabled: false,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) finishServiceDeletion(ctx context.Context, id int64, revoke func() error) error {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.LockApplicationServiceForDeletion(ctx, id); err != nil {
		return err
	}
	// Hold the service lock through external cleanup and commit. A delayed concurrent
	// retry must not delete streams belonging to a newly created service with the same codes.
	if revoke != nil {
		if err = revoke(); err != nil {
			return err
		}
	}
	deployments, err := q.ListServiceDeletionDeployments(ctx, id)
	if err != nil {
		return err
	}
	if err = q.DeleteServiceLogSettings(ctx, id); err != nil {
		return err
	}
	if err = q.DeleteServiceDeployments(ctx, id); err != nil {
		return err
	}
	for _, deploymentID := range deployments {
		// FK checks and deployment row locks serialize concurrent attachments; shared rows survive.
		if err = q.DeleteUnsharedServiceDeployment(ctx, deploymentID); err != nil {
			return err
		}
	}
	if err = batchwrite.RequireRow(q.DeleteApplicationService(ctx, id)); err != nil {
		return err
	}
	return tx.Commit()
}
