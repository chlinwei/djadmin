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
	ID         int64           `json:"id"`
	MediaID    int64           `json:"media_id"`
	MediaName  string          `json:"media_name"`
	Recipients []string        `json:"recipients"`
	Enabled    bool            `json:"enabled"`
	Scope      json.RawMessage `json:"scope"`
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
		MediaID    int64           `json:"media_id"`
		Recipients []string        `json:"recipients"`
		Enabled    *bool           `json:"enabled"`
		Scope      json.RawMessage `json:"scope"`
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
		scopeJSON, scopeErr := normalizeBindingScope(item.Scope)
		if scopeErr != "" {
			response.BusinessError(context, 400, scopeErr, nil)
			return
		}
		valid = append(valid, validatedBinding{MediaID: item.MediaID, Recipients: recipients, Enabled: enabled, Scope: scopeJSON})
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
	Scope      json.RawMessage
}

// bindingScopeEntry 订阅范围条目：type ∈ service|environment|business|project。
type bindingScopeEntry struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// normalizeBindingScope 归一化订阅范围：nil/空数组 → nil（全局订阅，落库 NULL）；
// 否则校验 type 与 id 后原样返回 JSON。返回值第二个为用户可读错误文案（空串=通过）。
func normalizeBindingScope(raw json.RawMessage) (json.RawMessage, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var entries []bindingScopeEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, "scope 必须是 [{type,id}] 数组"
	}
	if len(entries) > 50 {
		return nil, "scope 条目不能超过 50 个"
	}
	validTypes := map[string]bool{"service": true, "environment": true, "business": true, "project": true}
	normalized := make([]bindingScopeEntry, 0, len(entries))
	for _, entry := range entries {
		if !validTypes[entry.Type] {
			return nil, "scope.type 仅支持 service/environment/business/project"
		}
		if entry.ID <= 0 {
			return nil, "scope.id 必须是正整数"
		}
		normalized = append(normalized, entry)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, "scope 序列化失败"
	}
	return encoded, ""
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
	rows, err = service.repository.Pool().QueryContext(ctx, `SELECT b.id,b.media_id,m.name,b.recipients,b.enabled,b.scope
		FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m ON m.id=b.media_id
		WHERE b.user_id=? ORDER BY b.id`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item alertMediaBindingItem
		var recipientsRaw []byte
		var scopeRaw []byte
		if err = rows.Scan(&item.ID, &item.MediaID, &item.MediaName, &recipientsRaw, &item.Enabled, &scopeRaw); err != nil {
			return nil, nil, err
		}
		item.Scope = json.RawMessage(scopeRaw)
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
		var scopeValue any
		if len(binding.Scope) > 0 {
			scopeValue = string(binding.Scope)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO monitor_user_alert_media_binding(create_time,update_time,remark,recipients,enabled,media_id,user_id,scope) VALUES(?,?,?,?,?,?,?,?)`,
			now, now, nil, string(recipientsJSON), binding.Enabled, binding.MediaID, userID, scopeValue); err != nil {
			return err
		}
	}
	return tx.Commit()
}
