package monitor

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) CreateAlertMedia(context *gin.Context) { handler.saveAlertMedia(context, 0) }
func (handler *Handler) UpdateAlertMedia(context *gin.Context) {
	handler.saveAlertMedia(context, parseID(context.Param("id")))
}

func (handler *Handler) saveAlertMedia(context *gin.Context, id int64) {
	var input struct {
		Name      string         `json:"name"`
		MediaType string         `json:"media_type"`
		Config    map[string]any `json:"config"`
		Enabled   *bool          `json:"enabled"`
		Remark    string         `json:"remark"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	if input.MediaType != "email" {
		response.BusinessError(context, 400, "only email media is supported", nil)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		response.BusinessError(context, 400, "name is required", nil)
		return
	}
	server := strings.TrimSpace(stringValue(input.Config["smtpServer"]))
	email := strings.TrimSpace(stringValue(input.Config["email"]))
	port := intValue(input.Config["smtpPort"])
	if server == "" || email == "" || port < 1 || port > 65535 {
		response.BusinessError(context, 400, "valid smtpServer, smtpPort, and email are required", nil)
		return
	}
	if password := stringValue(input.Config["password"]); password == "********" && id > 0 {
		current, err := db.New(handler.db).GetAlertMediaConfig(context, id)
		if err == nil {
			var existing map[string]any
			_ = json.Unmarshal(current, &existing)
			input.Config["password"] = existing["password"]
		}
	} else if password != "" {
		encrypted, err := handler.secrets.Encrypt(password)
		if err != nil {
			response.Error(context, err)
			return
		}
		input.Config["password"] = encrypted
	}
	config, _ := json.Marshal(input.Config)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now().UTC()
	queries := db.New(handler.db)
	if id == 0 {
		createdID, err := queries.CreateAlertMedia(context, db.CreateAlertMediaParams{
			CreateTime: now, UpdateTime: now, Remark: sql.NullString{String: input.Remark, Valid: true},
			Name: input.Name, MediaType: input.MediaType, Config: config, Enabled: enabled,
		})
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		id = createdID
	} else {
		affected, err := queries.UpdateAlertMedia(context, db.UpdateAlertMediaParams{
			UpdateTime: now, Remark: sql.NullString{String: input.Remark, Valid: true},
			Name: input.Name, MediaType: input.MediaType, Config: config, Enabled: enabled, ID: id,
		})
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		if affected == 0 {
			response.BusinessError(context, 404, "alert media not found", nil)
			return
		}
	}
	handler.getAlertMedia(context, id)
}

func (handler *Handler) BatchDeleteAlertMedia(context *gin.Context) {
	batchDeleteMonitorRows(context, handler, handler.deleteAlertMediaByID)
}

// deleteAlertMediaByID 找不到行时返回 sql.ErrNoRows，由批删入口记成 ok:false。
func (handler *Handler) deleteAlertMediaByID(context *gin.Context, id int64) error {
	return deleteRowsAffected(db.New(handler.db).DeleteAlertMedia(context, id))
}
