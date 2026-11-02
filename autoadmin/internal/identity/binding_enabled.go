package identity

import (
	"database/sql"
	"strconv"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/binding"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) UpdateOwnAlertMediaBindingEnabled(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		response.Error(c, apperror.ErrTokenInvalid)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	value, err := binding.EnabledOnly(c)
	if err != nil || value == nil {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	tx, err := handler.service.repository.pool.BeginTx(c.Request.Context(), nil)
	if err != nil {
		response.Error(c, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	defer tx.Rollback()
	q := db.New(tx)
	// Scope both the lock and write to the authenticated user; never trust a user ID supplied by the browser.
	current, err := q.LockOwnAlertMediaBindingEnabled(c, db.LockOwnAlertMediaBindingEnabledParams{ID: id, UserID: claims.UserID})
	if err == sql.ErrNoRows {
		response.BusinessError(c, 404, "媒介绑定不存在", nil)
		return
	}
	if err != nil {
		response.Error(c, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	if *value && !current.MediaEnabled {
		response.BusinessError(c, 400, "媒介已停用，不能启用该绑定", nil)
		return
	}
	if err = q.UpdateOwnAlertMediaBindingEnabled(c, db.UpdateOwnAlertMediaBindingEnabledParams{
		Enabled: *value, UpdateTime: time.Now().UTC(), ID: id, UserID: claims.UserID,
	}); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		response.Error(c, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	response.Success(c, gin.H{"id": id, "enabled": *value})
}
