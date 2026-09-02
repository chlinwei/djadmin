package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

// 个人中心（/sys/usercenter）：修改资料、修改密码、告警媒介绑定。
// 语义与 Django user/views.py 的 updateUserInfo/updateUserPassword/
// alertMediaBindings/updateAlertMediaBindings 保持一致。

type updateUserInfoRequest struct {
	Phonenumber string `json:"phonenumber"`
}

func (handler *Handler) UpdateUserInfo(context *gin.Context) {
	claims, ok := ClaimsFromContext(context)
	if !ok {
		response.Error(context, apperror.ErrTokenInvalid)
		return
	}
	var input updateUserInfoRequest
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	user, err := handler.service.UpdatePhonenumber(context.Request.Context(), claims.UserID, strings.TrimSpace(input.Phonenumber))
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	// 不回传密码哈希
	user.Password = ""
	response.Success(context, gin.H{"user": user})
}

type updateUserPasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (handler *Handler) UpdateUserPassword(context *gin.Context) {
	claims, ok := ClaimsFromContext(context)
	if !ok {
		response.Error(context, apperror.ErrTokenInvalid)
		return
	}
	var input updateUserPasswordRequest
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	if strings.TrimSpace(input.NewPassword) == "" {
		response.BusinessError(context, 400, "新密码不能为空", nil)
		return
	}
	user, err := handler.service.GetByID(context.Request.Context(), claims.UserID)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	if !VerifyPassword(user.Password, input.OldPassword) {
		response.BusinessError(context, 400, "旧密码错误", nil)
		return
	}
	hashed, err := HashPassword(input.NewPassword)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	if err = handler.service.UpdatePassword(context.Request.Context(), claims.UserID, hashed); err != nil {
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	response.Success(context, nil)
}

// ---- 告警媒介绑定 ----

type alertMediaBindingItem struct {
	ID         int64    `json:"id"`
	MediaID    int64    `json:"media_id"`
	MediaName  string   `json:"media_name"`
	Recipients []string `json:"recipients"`
	Enabled    bool     `json:"enabled"`
}

func (handler *Handler) AlertMediaBindings(context *gin.Context) {
	claims, ok := ClaimsFromContext(context)
	if !ok {
		response.Error(context, apperror.ErrTokenInvalid)
		return
	}
	options, selected, err := handler.service.ListAlertMediaBindings(context.Request.Context(), claims.UserID)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	response.Success(context, gin.H{"options": options, "selected_bindings": selected})
}

type updateAlertMediaBindingsRequest struct {
	Bindings []struct {
		MediaID    int64    `json:"media_id"`
		Recipients []string `json:"recipients"`
		Enabled    *bool    `json:"enabled"`
	} `json:"bindings"`
}

func (handler *Handler) UpdateAlertMediaBindings(context *gin.Context) {
	claims, ok := ClaimsFromContext(context)
	if !ok {
		response.Error(context, apperror.ErrTokenInvalid)
		return
	}
	var input updateAlertMediaBindingsRequest
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	valid := make([]validatedBinding, 0, len(input.Bindings))
	for _, item := range input.Bindings {
		if item.MediaID <= 0 {
			response.BusinessError(context, 400, "media_id 必须是正整数", nil)
			return
		}
		recipients := make([]string, 0, len(item.Recipients))
		for _, recipient := range item.Recipients {
			email := strings.TrimSpace(recipient)
			if email != "" && strings.Contains(email, "@") {
				duplicated := false
				for _, existing := range recipients {
					if existing == email {
						duplicated = true
						break
					}
				}
				if !duplicated {
					recipients = append(recipients, email)
				}
			}
		}
		if len(recipients) == 0 {
			response.BusinessError(context, 400, "每个媒介至少需要配置一个收件人邮箱", nil)
			return
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		valid = append(valid, validatedBinding{MediaID: item.MediaID, Recipients: recipients, Enabled: enabled})
	}
	if err := handler.service.ReplaceAlertMediaBindings(context.Request.Context(), claims.UserID, valid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.BusinessError(context, 400, "媒介不存在或已禁用", nil)
			return
		}
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	response.Success(context, gin.H{"message": "告警媒介绑定已更新"})
}

// ---- Service ----

type validatedBinding struct {
	MediaID    int64
	Recipients []string
	Enabled    bool
}

type alertMediaOption struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Enabled   bool   `json:"enabled"`
}

func (service *Service) UpdatePhonenumber(ctx context.Context, userID int32, phonenumber string) (db.SysUser, error) {
	user, err := service.repository.GetByID(ctx, userID)
	if err != nil {
		return db.SysUser{}, err
	}
	if err = service.repository.UpdatePhonenumber(ctx, db.UpdateUserPhonenumberParams{
		Phonenumber: sql.NullString{String: phonenumber, Valid: phonenumber != ""},
		UpdateTime:  sql.NullTime{Time: time.Now().UTC(), Valid: true},
		ID:          userID,
	}); err != nil {
		return db.SysUser{}, err
	}
	user.Phonenumber = sql.NullString{String: phonenumber, Valid: phonenumber != ""}
	return user, nil
}

func (service *Service) GetByID(ctx context.Context, userID int32) (db.SysUser, error) {
	return service.repository.GetByID(ctx, userID)
}

func (service *Service) UpdatePassword(ctx context.Context, userID int32, hashed string) error {
	return service.repository.UpdatePassword(ctx, db.UpdateUserPasswordParams{
		Password:   hashed,
		UpdateTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		ID:         userID,
	})
}

func (service *Service) ListAlertMediaBindings(ctx context.Context, userID int32) ([]alertMediaOption, []alertMediaBindingItem, error) {
	options := make([]alertMediaOption, 0)
	rows, err := service.repository.Pool().QueryContext(ctx, `SELECT id,name,media_type,enabled FROM monitor_alert_media WHERE enabled=TRUE ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var option alertMediaOption
		if err = rows.Scan(&option.ID, &option.Name, &option.MediaType, &option.Enabled); err != nil {
			return nil, nil, err
		}
		options = append(options, option)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	selected := make([]alertMediaBindingItem, 0)
	rows, err = service.repository.Pool().QueryContext(ctx, `SELECT b.id,b.media_id,m.name,b.recipients,b.enabled
		FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m ON m.id=b.media_id
		WHERE b.user_id=? ORDER BY b.id`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item alertMediaBindingItem
		var recipientsRaw []byte
		if err = rows.Scan(&item.ID, &item.MediaID, &item.MediaName, &recipientsRaw, &item.Enabled); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal(recipientsRaw, &item.Recipients)
		if item.Recipients == nil {
			item.Recipients = []string{}
		}
		selected = append(selected, item)
	}
	return options, selected, rows.Err()
}

func (service *Service) ReplaceAlertMediaBindings(ctx context.Context, userID int32, bindings []validatedBinding) error {
	// 逐个校验媒介存在且启用（Django: AlertMedia.objects.filter(id=.., enabled=True)），
	// 然后整表替换该用户的绑定。
	for _, binding := range bindings {
		var count int
		if err := service.repository.Pool().QueryRowContext(ctx, `SELECT COUNT(*) FROM monitor_alert_media WHERE id=? AND enabled=TRUE`, binding.MediaID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return sql.ErrNoRows
		}
	}
	tx, err := service.repository.Pool().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM monitor_user_alert_media_binding WHERE user_id=?`, userID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, binding := range bindings {
		recipientsJSON, err := json.Marshal(binding.Recipients)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO monitor_user_alert_media_binding(create_time,update_time,remark,recipients,enabled,media_id,user_id) VALUES(?,?,?,?,?,?,?)`,
			now, now, nil, string(recipientsJSON), binding.Enabled, binding.MediaID, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
