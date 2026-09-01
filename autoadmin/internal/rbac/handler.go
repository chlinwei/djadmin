package rbac

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

type roleRequest struct {
	Name   *string `json:"name"`
	Code   *string `json:"code"`
	Remark *string `json:"remark"`
}

func (handler *Handler) List(context *gin.Context) {
	page, err := parsePage(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, count, err := handler.service.List(context.Request.Context(), strings.TrimSpace(context.Query("search")), page)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrRoleQueryInternal, err))
		return
	}
	response.Paginated(context, rows, count, page.Number, page.Size)
}
func (handler *Handler) Get(context *gin.Context) {
	id, ok := roleID(context)
	if !ok {
		return
	}
	row, err := handler.service.Get(context.Request.Context(), id)
	respond(context, row, err, ErrRoleQueryInternal)
}
func (handler *Handler) Create(context *gin.Context) {
	var request roleRequest
	if context.ShouldBindJSON(&request) != nil || request.Name == nil || request.Code == nil {
		response.Error(context, ErrRoleNameCodeRequired)
		return
	}
	row, err := handler.service.Create(context.Request.Context(), RoleInput{Name: request.Name, Code: request.Code, Remark: request.Remark})
	respond(context, row, err, ErrRoleCreateInternal)
}
func (handler *Handler) Update(context *gin.Context) {
	id, ok := roleID(context)
	if !ok {
		return
	}
	var request roleRequest
	if context.ShouldBindJSON(&request) != nil {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	row, err := handler.service.Update(context.Request.Context(), id, RoleInput{Name: request.Name, Code: request.Code, Remark: request.Remark})
	respond(context, row, err, ErrRoleUpdateInternal)
}
func (handler *Handler) DeleteMany(context *gin.Context) {
	var request struct {
		RoleIDs []int32 `json:"role_ids"`
	}
	if context.ShouldBindJSON(&request) != nil || len(request.RoleIDs) == 0 {
		response.Error(context, ErrRoleIDsEmpty)
		return
	}
	if err := handler.service.repository.DeleteRoles(context.Request.Context(), request.RoleIDs); err != nil {
		response.Error(context, apperror.WithCause(ErrRoleDeleteInternal, err))
		return
	}
	response.Success(context, nil)
}
func (handler *Handler) Current(context *gin.Context) {
	claims, ok := identity.ClaimsFromContext(context)
	if !ok {
		response.Error(context, apperror.ErrTokenInvalid)
		return
	}
	rows, err := handler.service.CurrentUserRoles(context.Request.Context(), claims.UserID)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrCurrentRoleQueryInternal, err))
		return
	}
	response.Success(context, gin.H{"roleList": rows})
}
func parsePage(context *gin.Context) (pagination.Page, error) {
	number, err := positive(context.Query("page"), 1)
	if err != nil {
		return pagination.Page{}, err
	}
	sizeRaw := context.Query("page_size")
	if sizeRaw == "" {
		sizeRaw = context.Query("size")
	}
	size, err := positive(sizeRaw, pagination.DefaultSize)
	if err != nil {
		return pagination.Page{}, err
	}
	return pagination.New(number, size), nil
}
func positive(raw string, fallback int32) (int32, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 1 {
		return 0, apperror.ErrPageInvalid
	}
	return int32(value), nil
}
func roleID(context *gin.Context) (int32, bool) {
	value, err := strconv.ParseInt(context.Param("id"), 10, 32)
	if err != nil || value < 1 {
		response.Error(context, apperror.ErrIDInvalid)
		return 0, false
	}
	return int32(value), true
}
func respond(context *gin.Context, data any, err error, internalError *apperror.Error) {
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(context, apperror.ErrResourceNotFound)
		return
	}
	if err != nil {
		response.Error(context, apperror.WithCause(internalError, err))
		return
	}
	response.Success(context, data)
}
