package identity

import (
	"net/http"

	"autoadmin/internal/shared/apperror"
)

const CodeUserDisabled = 1006

var (
	ErrInvalidCredentials      = apperror.New(apperror.CodeInvalidCredentials, "账号或者密码输入错误")
	ErrLoginFieldsRequired     = apperror.New(apperror.CodeInvalidArgument, "用户名和密码不能为空")
	ErrUsernameRequired        = apperror.New(apperror.CodeInvalidArgument, "用户名不能为空")
	ErrUserIDInvalid           = apperror.New(apperror.CodeInvalidArgument, "user_id 无效")
	ErrUserIDsEmpty            = apperror.New(apperror.CodeInvalidArgument, "用户id数组不能为空")
	ErrUserNotFound            = apperror.New(apperror.CodeNotFound, "用户不存在")
	ErrUserDisabled            = apperror.New(CodeUserDisabled, "用户已被禁用，无法登录")
	ErrAPITokenRequestInvalid  = apperror.New(apperror.CodeInvalidArgument, "API Token 请求参数错误")
	ErrAPITokenBindModeInvalid = apperror.New(apperror.CodeInvalidArgument, "bind_mode仅支持api或agent")
	ErrAPITokenAgentIDRequired = apperror.New(apperror.CodeInvalidArgument, "api模式下agent_id不能为空")
	ErrAPITokenAgentIDReserved = apperror.New(apperror.CodeInvalidArgument, "agent_id不能为global保留字")
	ErrAPITokenAgentIDExists   = apperror.New(apperror.CodeInvalidArgument, "agent_id已存在")

	ErrLoginInternal          = apperror.NewWithHTTP(apperror.CodeInternal, "登录失败", http.StatusInternalServerError)
	ErrUserQueryInternal      = apperror.NewWithHTTP(apperror.CodeInternal, "查询用户失败", http.StatusInternalServerError)
	ErrUserListInternal       = apperror.NewWithHTTP(apperror.CodeInternal, "查询用户列表失败", http.StatusInternalServerError)
	ErrUsernameCheckInternal  = apperror.NewWithHTTP(apperror.CodeInternal, "检查用户名失败", http.StatusInternalServerError)
	ErrUserDeleteInternal     = apperror.NewWithHTTP(apperror.CodeInternal, "删除用户失败", http.StatusInternalServerError)
	ErrRoleAssignInternal     = apperror.NewWithHTTP(apperror.CodeInternal, "分配角色失败", http.StatusInternalServerError)
	ErrUserRoleQueryInternal  = apperror.NewWithHTTP(apperror.CodeInternal, "查询用户角色失败", http.StatusInternalServerError)
	ErrUserStatusInternal     = apperror.NewWithHTTP(apperror.CodeInternal, "修改用户状态失败", http.StatusInternalServerError)
	ErrPasswordResetInternal  = apperror.NewWithHTTP(apperror.CodeInternal, "重置密码失败", http.StatusInternalServerError)
	ErrAPITokenCreateInternal = apperror.NewWithHTTP(apperror.CodeInternal, "创建 API Token 失败", http.StatusInternalServerError)
)
