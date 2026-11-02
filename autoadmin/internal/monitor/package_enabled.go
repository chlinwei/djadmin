package monitor

import (
	"database/sql"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/binding"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) packageEnabledPatch(c *gin.Context, id int64) bool {
	value, err := binding.EnabledOnly(c)
	if err != nil {
		response.Error(c, err)
		return true
	}
	if value == nil {
		return false
	}
	tx, err := handler.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		response.Error(c, err)
		return true
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.LockSoftwarePackageEnabled(c, id); err == nil {
		err = q.UpdateSoftwarePackageEnabled(c, db.UpdateSoftwarePackageEnabledParams{Enabled: *value, UpdateTime: time.Now().UTC(), ID: id})
	}
	if err == sql.ErrNoRows {
		response.BusinessError(c, 404, "software package not found", nil)
		return true
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		response.Error(c, err)
		return true
	}
	handler.respondSoftwarePackage(c, id)
	return true
}
