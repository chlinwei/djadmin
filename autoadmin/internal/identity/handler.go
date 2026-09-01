package identity

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type loginRequest struct {
	Username string `json:"username" form:"username" binding:"required"`
	Password string `json:"password" form:"password" binding:"required"`
}

func (handler *Handler) Login(context *gin.Context) {
	var request loginRequest
	if err := context.ShouldBind(&request); err != nil {
		response.Error(context, ErrLoginFieldsRequired)
		return
	}
	result, err := handler.service.Login(
		context.Request.Context(), request.Username, request.Password,
		clientIP(context), truncate(context.GetHeader("User-Agent"), 255),
	)
	if errors.Is(err, ErrInvalidCredentials) {
		response.Error(context, ErrInvalidCredentials)
		return
	}
	if errors.Is(err, ErrUserDisabled) {
		response.Error(context, ErrUserDisabled)
		return
	}
	if err != nil {
		response.Error(context, apperror.WithCause(ErrLoginInternal, err))
		return
	}
	response.Success(context, result)
}

func (handler *Handler) Current(context *gin.Context) {
	claims, ok := ClaimsFromContext(context)
	if !ok {
		response.Error(context, apperror.ErrTokenInvalid)
		return
	}
	user, err := handler.service.Current(context.Request.Context(), claims.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(context, ErrUserNotFound)
		return
	}
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	response.Success(context, user)
}

func (handler *Handler) ListUsers(context *gin.Context) {
	page, err := parsePage(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	users, count, err := handler.service.ListUsers(context.Request.Context(), strings.TrimSpace(context.Query("search")), page)
	if err != nil {
		response.Error(context, apperror.WithCause(ErrUserListInternal, err))
		return
	}
	totalPages := int64(0)
	if count > 0 {
		totalPages = (count + int64(page.Size) - 1) / int64(page.Size)
	}
	response.Success(context, gin.H{
		"results": users, "count": count, "pageNumber": page.Number,
		"pageSize": page.Size, "totalPages": totalPages,
		"next": nil, "previous": nil,
	})
}

func parsePage(context *gin.Context) (pagination.Page, error) {
	pageNumber, err := positiveQueryInt(context.Query("page"), 1)
	if err != nil {
		return pagination.Page{}, apperror.ErrPageInvalid
	}
	sizeValue := context.Query("page_size")
	if sizeValue == "" {
		sizeValue = context.Query("size")
	}
	pageSize, err := positiveQueryInt(sizeValue, pagination.DefaultSize)
	if err != nil {
		return pagination.Page{}, apperror.ErrPageSizeInvalid
	}
	return pagination.New(pageNumber, pageSize), nil
}

func positiveQueryInt(value string, defaultValue int32) (int32, error) {
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 1 {
		return 0, apperror.ErrInvalidRequest
	}
	return int32(parsed), nil
}

func clientIP(context *gin.Context) string {
	if forwarded := context.GetHeader("X-Forwarded-For"); forwarded != "" {
		return truncate(strings.TrimSpace(strings.Split(forwarded, ",")[0]), 64)
	}
	return truncate(context.ClientIP(), 64)
}

func truncate(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes]
}
