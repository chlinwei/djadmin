package logcollect

import (
	"database/sql"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/binding"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) enabledPatch(c *gin.Context, resource string, detail func(*gin.Context, int64)) bool {
	value, err := binding.EnabledOnly(c)
	if err != nil {
		response.Error(c, err)
		return true
	}
	if value == nil {
		return false
	}
	id := parseID(c.Param("id"))
	if id < 1 {
		response.BusinessError(c, 400, "invalid resource id", nil)
		return true
	}
	// Lock the target before updating only its status; do not merge and rewrite secrets or rule configuration.
	tx, err := handler.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		response.Error(c, err)
		return true
	}
	defer tx.Rollback()
	q := db.New(tx)
	now := time.Now().UTC()
	switch resource {
	case "cluster":
		if _, err = q.LockElasticsearchClusterEnabled(c, id); err == nil {
			err = q.UpdateElasticsearchClusterEnabled(c, db.UpdateElasticsearchClusterEnabledParams{Enabled: *value, UpdateTime: now, ID: id})
		}
	case "retention":
		if _, err = q.LockLogRetentionTierEnabled(c, id); err == nil {
			err = q.UpdateLogRetentionTierEnabled(c, db.UpdateLogRetentionTierEnabledParams{Enabled: *value, UpdateTime: now, ID: id})
		}
	case "filter":
		if _, err = q.LockLogCollectionFilterRuleEnabled(c, id); err == nil {
			err = q.UpdateLogCollectionFilterRuleEnabled(c, db.UpdateLogCollectionFilterRuleEnabledParams{Enabled: *value, UpdateTime: now, ID: id})
		}
	default:
		response.BusinessError(c, 400, "invalid resource", nil)
		return true
	}
	if err == sql.ErrNoRows {
		response.BusinessError(c, 404, "resource not found", nil)
		return true
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		response.Error(c, err)
		return true
	}
	detail(c, id)
	return true
}
