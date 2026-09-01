package identity

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/binding"

	"github.com/gin-gonic/gin"
)

type userRequest struct {
	Username    string  `json:"username"`
	Phonenumber *string `json:"phonenumber"`
	Status      *int16  `json:"status"`
	Remark      *string `json:"remark"`
	Timezone    string  `json:"timezone"`
}

func (handler *Handler) UserDetail(context *gin.Context) {
	id, ok := pathID(context)
	if !ok {
		return
	}
	user, err := handler.service.UserDetail(context.Request.Context(), id)
	respondEntity(context, user, err, "查询用户失败")
}

func (handler *Handler) CreateUser(context *gin.Context) {
	var request userRequest
	if err := context.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Username) == "" {
		response.Error(context, ErrUsernameRequired)
		return
	}
	status := int16(1)
	if request.Status != nil {
		status = *request.Status
	}
	timezone := request.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	user, err := handler.service.CreateUser(context.Request.Context(), UserInput{
		Username: strings.TrimSpace(request.Username), Phonenumber: request.Phonenumber,
		Status: status, Remark: request.Remark, Timezone: timezone,
	})
	respondEntity(context, user, err, "新增用户失败")
}

func (handler *Handler) UpdateUser(context *gin.Context) {
	id, ok := pathID(context)
	if !ok {
		return
	}
	existing, err := handler.service.Current(context.Request.Context(), id)
	if err != nil {
		respondEntity(context, existing, err, "查询用户失败")
		return
	}
	var request userRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	status := existing.Status
	if request.Status != nil {
		status = *request.Status
	}
	user, err := handler.service.UpdateUser(context.Request.Context(), id, UserInput{
		Phonenumber: request.Phonenumber, Status: status, Remark: request.Remark, Timezone: request.Timezone,
	})
	respondEntity(context, user, err, "更新用户失败")
}

func (handler *Handler) CheckUsername(context *gin.Context) {
	username := strings.TrimSpace(context.Query("username"))
	if username == "" {
		response.Error(context, ErrUsernameRequired)
		return
	}
	exists, err := handler.service.UsernameExists(context.Request.Context(), username)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUsernameCheckInternal, err))
		return
	}
	response.Success(context, gin.H{"exists": exists})
}

func (handler *Handler) BatchDeleteUsers(context *gin.Context) {
	var request struct {
		UserIDs []int32 `json:"user_ids"`
	}
	if context.ShouldBindJSON(&request) != nil || len(request.UserIDs) == 0 {
		response.Error(context, ErrUserIDsEmpty)
		return
	}
	if err := handler.service.DeleteUsers(context.Request.Context(), request.UserIDs); err != nil {
		response.Error(context, apperror.WithCause(ErrUserDeleteInternal, err))
		return
	}
	response.Success(context, gin.H{"user_role_deleted_count": len(request.UserIDs), "user_deleted_count": len(request.UserIDs)})
}

func (handler *Handler) ResetPassword(context *gin.Context) {
	var request struct {
		ID int32 `json:"id"`
	}
	if context.ShouldBindJSON(&request) != nil || request.ID < 1 {
		response.Error(context, ErrUserNotFound)
		return
	}
	if err := handler.service.ResetPassword(context.Request.Context(), request.ID); err != nil {
		respondEntity(context, nil, err, "重置密码失败")
		return
	}
	response.Success(context, gin.H{"password": "123456"})
}

func (handler *Handler) AssignRoles(context *gin.Context) {
	var request struct {
		UserID  int32        `json:"user_id"`
		RoleIDs []binding.ID `json:"roleIds"`
	}
	if context.ShouldBindJSON(&request) != nil || request.UserID < 1 {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	if err := handler.service.repository.ReplaceRoles(context.Request.Context(), request.UserID, binding.Int32s(request.RoleIDs)); err != nil {
		response.Error(context, apperror.WithCause(ErrRoleAssignInternal, err))
		return
	}
	response.Success(context, nil)
}

func (handler *Handler) ChangeUserStatus(context *gin.Context) {
	var request struct {
		UserID int32 `json:"user_id"`
		Status int16 `json:"status"`
	}
	if context.ShouldBindJSON(&request) != nil || request.UserID < 1 || (request.Status != 0 && request.Status != 1) {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	if err := handler.service.ChangeStatus(context.Request.Context(), request.UserID, request.Status); err != nil {
		respondEntity(context, nil, err, "修改用户状态失败")
		return
	}
	response.Success(context, nil)
}

func (handler *Handler) GetUserRoles(context *gin.Context) {
	id64, err := strconv.ParseInt(context.Query("user_id"), 10, 32)
	if err != nil || id64 < 1 {
		response.Error(context, ErrUserIDInvalid)
		return
	}
	roles, err := handler.service.UserRoles(context.Request.Context(), int32(id64))
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUserRoleQueryInternal, err))
		return
	}
	response.Success(context, gin.H{"roleList": roles})
}

func pathID(context *gin.Context) (int32, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 32)
	if err != nil || id < 1 {
		response.Error(context, apperror.ErrIDInvalid)
		return 0, false
	}
	return int32(id), true
}

func respondEntity(context *gin.Context, data any, err error, message string) {
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(context, apperror.ErrResourceNotFound)
		return
	}
	if err != nil {
		response.Error(context, apperror.WithCause(apperror.NewWithHTTP(apperror.CodeInternal, message, http.StatusInternalServerError), err))
		return
	}
	response.Success(context, data)
}
