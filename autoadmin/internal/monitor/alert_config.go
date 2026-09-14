package monitor

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"autoadmin/internal/api/response"

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
		var current []byte
		if err := handler.db.QueryRowContext(context, `SELECT config FROM monitor_alert_media WHERE id=?`, id).Scan(&current); err == nil {
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
	if id == 0 {
		result, err := handler.db.ExecContext(context, `INSERT INTO monitor_alert_media(create_time,update_time,remark,name,media_type,config,enabled) VALUES(?,?,?,?,?,?,?)`, now, now, input.Remark, input.Name, input.MediaType, string(config), enabled)
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		id, _ = result.LastInsertId()
	} else {
		result, err := handler.db.ExecContext(context, `UPDATE monitor_alert_media SET update_time=?,remark=?,name=?,media_type=?,config=?,enabled=? WHERE id=?`, now, input.Remark, input.Name, input.MediaType, string(config), enabled, id)
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			response.BusinessError(context, 404, "alert media not found", nil)
			return
		}
	}
	handler.getAlertMedia(context, id)
}

func (handler *Handler) BatchDeleteAlertMedia(context *gin.Context) {
	batchDeleteMonitorRows(context, handler, "monitor_alert_media")
}

var _ = sql.ErrNoRows
