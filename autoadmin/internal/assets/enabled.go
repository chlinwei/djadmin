package assets

import (
	"context"
	"database/sql"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/binding"

	"github.com/gin-gonic/gin"
)

func (h *Handler) enabledPatch(c *gin.Context, id int64, resource string, detail gin.HandlerFunc) bool {
	value, err := binding.EnabledOnly(c)
	if err != nil {
		respond(c, nil, err)
		return true
	}
	if value == nil {
		return false
	}
	if err := h.service.setEnabled(c.Request.Context(), resource, id, *value); err != nil {
		respond(c, nil, translate(err))
		return true
	}
	detail(c)
	return true
}

func (s *Service) setEnabled(ctx context.Context, resource string, id int64, value bool) error {
	// Match full-edit isolation and validation without rewriting configuration or nested relationships.
	var tx *sql.Tx
	var err error
	if resource == "project" || resource == "business-system" || resource == "environment" || resource == "service" {
		tx, err = s.repository.beginServiceIdentityWrite(ctx)
	} else {
		tx, err = beginLogConfigWrite(ctx, s.repository.pool)
	}
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := time.Now().UTC()
	switch resource {
	case "project":
		if _, err = q.LockProjectEnabled(ctx, id); err == nil {
			err = q.UpdateProjectEnabled(ctx, db.UpdateProjectEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
	case "business-system":
		if _, err = q.LockBusinessSystemEnabled(ctx, id); err == nil {
			err = q.UpdateBusinessSystemEnabled(ctx, db.UpdateBusinessSystemEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
	case "environment":
		if _, err = q.LockBusinessEnvironmentEnabled(ctx, id); err == nil {
			err = q.UpdateBusinessEnvironmentEnabled(ctx, db.UpdateBusinessEnvironmentEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
	case "application":
		if _, err = q.LockApplicationEnabled(ctx, id); err == nil {
			err = q.UpdateApplicationEnabled(ctx, db.UpdateApplicationEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
	case "version":
		if _, err = q.LockApplicationVersionEnabled(ctx, id); err == nil {
			err = q.UpdateApplicationVersionEnabled(ctx, db.UpdateApplicationVersionEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
	case "profile":
		var current db.LockClusterProfileEnabledRow
		current, err = q.LockClusterProfileEnabled(ctx, id)
		if err == nil && current.ProfileType == "builtin" {
			err = ErrDeleteProtected
		}
		if err == nil {
			err = q.UpdateClusterProfileEnabled(ctx, db.UpdateClusterProfileEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
	case "template":
		if _, err = q.LockDeploymentTemplateEnabled(ctx, id); err == nil {
			err = q.UpdateDeploymentTemplateEnabled(ctx, db.UpdateDeploymentTemplateEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
		if err == nil {
			err = s.checkTemplateLogConfigConsistency(ctx, tx, id)
		}
	case "service":
		if _, err = q.LockApplicationServiceEnabled(ctx, id); err == nil {
			// Share the row lock and tombstone check with deletion; an enable request must not revive a deleting service.
			if _, tombstoneErr := q.GetServiceDeletionTombstone(ctx, id); tombstoneErr == nil {
				err = ErrServiceLogIdentityDeleting
			} else if tombstoneErr != sql.ErrNoRows {
				err = tombstoneErr
			}
		}
		if err == nil {
			err = q.UpdateApplicationServiceEnabled(ctx, db.UpdateApplicationServiceEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
		if err == nil {
			err = s.checkLogConfigConsistency(ctx, tx, id)
		}
	case "deployment":
		if _, err = q.LockApplicationDeploymentEnabled(ctx, id); err == nil {
			err = q.UpdateApplicationDeploymentEnabled(ctx, db.UpdateApplicationDeploymentEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
		if err == nil {
			err = s.checkDeploymentLogConfigConsistency(ctx, tx, id)
		}
	case "dependency-type":
		if _, err = q.LockServiceDependencyTypeEnabled(ctx, id); err == nil {
			err = q.UpdateServiceDependencyTypeEnabled(ctx, db.UpdateServiceDependencyTypeEnabledParams{Enabled: value, UpdateTime: now, ID: id})
		}
	default:
		return ErrInvalid
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
