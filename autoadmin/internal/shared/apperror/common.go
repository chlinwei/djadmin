package apperror

import "net/http"

const (
	CodeInvalidCredentials = 300
	CodeTokenInvalid       = 301
	CodeInvalidArgument    = 400
	CodePermissionDenied   = 403
	CodeNotFound           = 404
	CodeInternal           = 600
)

var (
	ErrInvalidRequest   = New(CodeInvalidArgument, "请求参数错误")
	ErrIDInvalid        = New(CodeInvalidArgument, "ID 无效")
	ErrPageInvalid      = New(CodeInvalidArgument, "page 必须是正整数")
	ErrPageSizeInvalid  = New(CodeInvalidArgument, "page_size 或 size 必须是正整数")
	ErrTokenInvalid     = New(CodeTokenInvalid, "Token验证失败！")
	ErrTokenExpired     = New(CodeTokenInvalid, "Token过期，请重新登录！")
	ErrPermissionDenied = New(CodePermissionDenied, "无权限访问")
	ErrResourceNotFound = New(CodeNotFound, "资源不存在")
	ErrInternal         = NewWithHTTP(CodeInternal, "服务器内部错误", http.StatusInternalServerError)
)
