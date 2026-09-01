package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListAlertMedia(context *gin.Context) {
	page, size := pagination(context)
	clauses, args := []string{"1=1"}, make([]any, 0)
	for query, column := range map[string]string{"media_type": "media_type", "enabled": "enabled"} {
		if value := strings.TrimSpace(context.Query(query)); value != "" {
			clauses = append(clauses, column+"=?")
			args = append(args, value)
		}
	}
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		clauses = append(clauses, "name LIKE ?")
		args = append(args, "%"+search+"%")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM monitor_alert_media`+where, args)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT id,name,media_type,config,enabled,create_time,update_time,remark FROM monitor_alert_media`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		maskConfig(item)
	}
	paginated(context, items, count, page, size)
}

func maskConfig(item gin.H) {
	config, ok := item["config"].(map[string]any)
	if !ok {
		if raw, rawOK := item["config"].(string); rawOK {
			_ = json.Unmarshal([]byte(raw), &config)
		}
	}
	if config == nil {
		config = map[string]any{}
	}
	if stringValue(config["password"]) != "" {
		config["password"] = "********"
	}
	item["config"] = config
}

func (handler *Handler) GetAlertMedia(context *gin.Context) {
	handler.getAlertMedia(context, parseID(context.Param("id")))
}
func (handler *Handler) getAlertMedia(context *gin.Context, id int64) {
	rows, err := handler.db.QueryContext(context, `SELECT id,name,media_type,config,enabled,create_time,update_time,remark FROM monitor_alert_media WHERE id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	if len(items) == 0 {
		response.BusinessError(context, 404, "alert media not found", nil)
		return
	}
	maskConfig(items[0])
	response.Success(context, items[0])
}
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

func (handler *Handler) DeleteAlertMedia(context *gin.Context) {
	id := parseID(context.Param("id"))
	result, err := handler.db.ExecContext(context, `DELETE FROM monitor_alert_media WHERE id=?`, id)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		response.BusinessError(context, 404, "alert media not found", nil)
		return
	}
	response.Success(context, gin.H{"deleted": true})
}

func (handler *Handler) ListAlertRoutes(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	count, err := queries.CountAlertRoutes(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListAlertRoutes(context, db.ListAlertRoutesParams{Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]alertRouteResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, handler.alertRouteResponse(context, row))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}
func (handler *Handler) GetAlertRoute(context *gin.Context) {
	id := parseID(context.Param("id"))
	row, err := db.New(handler.db).GetAlertRoute(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "alert route not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, handler.alertRouteResponse(context, row))
}

type alertRouteResponse struct {
	ID               int64           `json:"id"`
	CreateTime       time.Time       `json:"create_time"`
	UpdateTime       time.Time       `json:"update_time"`
	Remark           *string         `json:"remark"`
	Name             string          `json:"name"`
	Enabled          bool            `json:"enabled"`
	Matchers         json.RawMessage `json:"matchers"`
	NotifyOnFiring   bool            `json:"notify_on_firing"`
	NotifyOnResolved bool            `json:"notify_on_resolved"`
	Media            []int64         `json:"media"`
}

func (handler *Handler) alertRouteResponse(context *gin.Context, row db.MonitorAlertRoute) alertRouteResponse {
	var remark *string
	if row.Remark.Valid {
		remark = &row.Remark.String
	}
	return alertRouteResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: remark,
		Name: row.Name, Enabled: row.Enabled, Matchers: row.Matchers,
		NotifyOnFiring: row.NotifyOnFiring, NotifyOnResolved: row.NotifyOnResolved,
		Media: handler.routeMediaIDs(context, row.ID),
	}
}

func (handler *Handler) routeMediaIDs(context *gin.Context, id int64) []int64 {
	rows, err := handler.db.QueryContext(context, `SELECT alertmedia_id FROM monitor_alert_route_media WHERE alertroute_id=? ORDER BY alertmedia_id`, id)
	if err != nil {
		return []int64{}
	}
	defer rows.Close()
	result := make([]int64, 0)
	for rows.Next() {
		var mediaID int64
		if rows.Scan(&mediaID) == nil {
			result = append(result, mediaID)
		}
	}
	if rows.Err() != nil {
		return []int64{}
	}
	return result
}
func (handler *Handler) CreateAlertRoute(context *gin.Context) { handler.saveAlertRoute(context, 0) }
func (handler *Handler) UpdateAlertRoute(context *gin.Context) {
	handler.saveAlertRoute(context, parseID(context.Param("id")))
}

func (handler *Handler) saveAlertRoute(context *gin.Context, id int64) {
	var input struct {
		Name           string         `json:"name"`
		Enabled        *bool          `json:"enabled"`
		Matchers       map[string]any `json:"matchers"`
		NotifyFiring   *bool          `json:"notify_on_firing"`
		NotifyResolved *bool          `json:"notify_on_resolved"`
		Media          []int64        `json:"media"`
		Remark         string         `json:"remark"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Media) == 0 {
		response.BusinessError(context, 400, "name and at least one media are required", nil)
		return
	}
	firing, resolved := true, true
	if input.NotifyFiring != nil {
		firing = *input.NotifyFiring
	}
	if input.NotifyResolved != nil {
		resolved = *input.NotifyResolved
	}
	if !firing && !resolved {
		response.BusinessError(context, 400, "at least one notification event type is required", nil)
		return
	}
	normalized := map[string]string{}
	for key, value := range input.Matchers {
		k, v := strings.TrimSpace(key), strings.TrimSpace(stringValue(value))
		if k == "" || v == "" {
			response.BusinessError(context, 400, "matcher names and values cannot be empty", nil)
			return
		}
		normalized[k] = v
	}
	matchers, _ := json.Marshal(normalized)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now().UTC()
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	if id == 0 {
		result, execErr := tx.ExecContext(context, `INSERT INTO monitor_alert_route(create_time,update_time,remark,name,enabled,matchers,notify_on_firing,notify_on_resolved) VALUES(?,?,?,?,?,?,?,?)`, now, now, input.Remark, input.Name, enabled, string(matchers), firing, resolved)
		if execErr != nil {
			response.BusinessError(context, 400, execErr.Error(), nil)
			return
		}
		id, _ = result.LastInsertId()
	} else {
		result, execErr := tx.ExecContext(context, `UPDATE monitor_alert_route SET update_time=?,remark=?,name=?,enabled=?,matchers=?,notify_on_firing=?,notify_on_resolved=? WHERE id=?`, now, input.Remark, input.Name, enabled, string(matchers), firing, resolved, id)
		if execErr != nil {
			response.BusinessError(context, 400, execErr.Error(), nil)
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			response.BusinessError(context, 404, "alert route not found", nil)
			return
		}
	}
	if _, err = tx.ExecContext(context, `DELETE FROM monitor_alert_route_media WHERE alertroute_id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	sort.Slice(input.Media, func(i, j int) bool { return input.Media[i] < input.Media[j] })
	for _, mediaID := range input.Media {
		if _, err = tx.ExecContext(context, `INSERT INTO monitor_alert_route_media(alertroute_id,alertmedia_id) VALUES(?,?)`, id, mediaID); err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	context.Params = append(context.Params, gin.Param{Key: "id", Value: fmt.Sprint(id)})
	handler.GetAlertRoute(context)
}
func (handler *Handler) DeleteAlertRoute(context *gin.Context) {
	id := parseID(context.Param("id"))
	result, err := handler.db.ExecContext(context, `DELETE FROM monitor_alert_route WHERE id=?`, id)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		response.BusinessError(context, 404, "alert route not found", nil)
		return
	}
	response.Success(context, gin.H{"deleted": true})
}

var _ = sql.ErrNoRows
