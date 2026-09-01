package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/pbkdf2"
)

type APITokenHandler struct{ queries *db.Queries }

func NewAPITokenHandler(queries *db.Queries) *APITokenHandler {
	return &APITokenHandler{queries: queries}
}
func (handler *APITokenHandler) List(context *gin.Context) {
	items, err := handler.queries.ListAPITokens(context.Request.Context())
	if err != nil {
		response.Error(context, fmt.Errorf("list api tokens: %w", err))
		return
	}
	response.Success(context, gin.H{"results": items, "count": len(items)})
}
func tokenPlain() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func djangoHash(value string) (string, error) {
	saltRaw := make([]byte, 12)
	if _, err := rand.Read(saltRaw); err != nil {
		return "", err
	}
	salt := hex.EncodeToString(saltRaw)
	derived := pbkdf2.Key([]byte(value), []byte(salt), 600000, 32, sha256.New)
	return fmt.Sprintf("pbkdf2_sha256$600000$%s$%s", salt, base64.StdEncoding.EncodeToString(derived)), nil
}
func parseExpiry(raw string) (interface{}, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	if !value.After(time.Now()) {
		return nil, fmt.Errorf("expires_at must be in the future")
	}
	return value, nil
}
func (handler *APITokenHandler) Create(context *gin.Context) {
	var input struct {
		AgentID   string `json:"agent_id"`
		BindMode  string `json:"bind_mode"`
		Name      string `json:"name"`
		ExpiresAt string `json:"expires_at"`
		Remark    string `json:"remark"`
	}
	if err := context.ShouldBindJSON(&input); err != nil {
		response.Error(context, ErrAPITokenRequestInvalid)
		return
	}
	mode := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(input.BindMode), "-", "_"))
	if mode == "" {
		mode = "api"
	}
	if mode != "api" && mode != "agent" {
		response.Error(context, ErrAPITokenBindModeInvalid)
		return
	}
	agentID := strings.TrimSpace(input.AgentID)
	if mode == "agent" {
		agentID = "global"
	} else if agentID == "" {
		response.Error(context, ErrAPITokenAgentIDRequired)
		return
	} else if agentID == "global" {
		response.Error(context, ErrAPITokenAgentIDReserved)
		return
	}
	if count, err := handler.queries.CountAPITokensByAgentID(context, agentID); err != nil {
		response.Error(context, apperror.WithCause(ErrAPITokenCreateInternal, err))
		return
	} else if count > 0 {
		response.Error(context, ErrAPITokenAgentIDExists)
		return
	}
	expiry, err := parseExpiry(strings.TrimSpace(input.ExpiresAt))
	if err != nil {
		response.Error(context, err)
		return
	}
	plain, err := tokenPlain()
	if err != nil {
		response.Error(context, err)
		return
	}
	hash, err := djangoHash(plain)
	if err != nil {
		response.Error(context, err)
		return
	}
	claims, _ := ClaimsFromContext(context)
	var userID sql.NullInt32
	if claims != nil {
		userID = sql.NullInt32{Int32: claims.UserID, Valid: true}
	}
	result, err := handler.queries.CreateAPIToken(context, db.CreateAPITokenParams{AgentID: agentID, TokenHash: hash, Name: apiTokenNullableString(input.Name), IsActive: true, ExpiresAt: nullableTime(expiry), Remark: apiTokenNullableString(input.Remark), CreateTime: time.Now().UTC(), UpdateTime: time.Now().UTC(), CreatedByID: userID, BindMode: mode})
	if err != nil {
		response.Error(context, apperror.WithCause(ErrAPITokenCreateInternal, err))
		return
	}
	id, _ := result.LastInsertId()
	response.Success(context, gin.H{"id": id, "agent_id": agentID, "bind_mode": mode, "token": plain, "expires_at": expiry, "is_active": true})
}
func (handler *APITokenHandler) Rotate(context *gin.Context) {
	id := context.PostForm("id")
	if id == "" {
		var input struct {
			ID int32 `json:"id"`
		}
		if context.ShouldBindJSON(&input) != nil || input.ID < 1 {
			response.Error(context, fmt.Errorf("id不能为空"))
			return
		}
		id = strconv.Itoa(int(input.ID))
	}
	numeric, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		response.Error(context, err)
		return
	}
	record, err := handler.queries.GetAPITokenByID(context, int32(numeric))
	if err != nil {
		response.Error(context, err)
		return
	}
	if !record.IsActive {
		response.Error(context, fmt.Errorf("ApiToken已禁用"))
		return
	}
	plain, err := tokenPlain()
	if err != nil {
		response.Error(context, err)
		return
	}
	hash, err := djangoHash(plain)
	if err != nil {
		response.Error(context, err)
		return
	}
	if err = handler.queries.RotateAPIToken(context, db.RotateAPITokenParams{TokenHash: hash, UpdateTime: time.Now().UTC(), ID: int32(numeric)}); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": record.ID, "agent_id": record.AgentID, "bind_mode": record.BindMode, "token": plain})
}
func (handler *APITokenHandler) Disable(context *gin.Context) { handler.change(context, false) }
func (handler *APITokenHandler) Delete(context *gin.Context) {
	var input struct {
		ID int32 `json:"id"`
	}
	if context.ShouldBindJSON(&input) != nil || input.ID < 1 {
		response.Error(context, fmt.Errorf("id不能为空"))
		return
	}
	if err := handler.queries.DeleteAPIToken(context, input.ID); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, nil)
}
func (handler *APITokenHandler) change(context *gin.Context, active bool) {
	var input struct {
		ID int32 `json:"id"`
	}
	if context.ShouldBindJSON(&input) != nil || input.ID < 1 {
		response.Error(context, fmt.Errorf("id不能为空"))
		return
	}
	if err := handler.queries.DisableAPIToken(context, db.DisableAPITokenParams{UpdateTime: time.Now().UTC(), ID: input.ID}); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, nil)
}
func apiTokenNullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
func nullableTime(value interface{}) sql.NullTime {
	parsed, ok := value.(time.Time)
	return sql.NullTime{Time: parsed, Valid: ok}
}
