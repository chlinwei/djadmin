package rbac

import (
	"net/http"

	"autoadmin/internal/shared/apperror"
)

var (
	ErrRoleNameCodeRequired = apperror.New(apperror.CodeInvalidArgument, "角色名称和权限字符不能为空")
	ErrRoleIDsEmpty         = apperror.New(apperror.CodeInvalidArgument, "角色id数组不能为空")
	ErrRoleNotFound         = apperror.New(apperror.CodeNotFound, "角色不存在")
	ErrMenuNameRequired     = apperror.New(apperror.CodeInvalidArgument, "菜单名称不能为空")
	ErrMenuSelfParent       = apperror.New(apperror.CodeInvalidArgument, "菜单不能以自身作为父节点")
	ErrRoleIDInvalid        = apperror.New(apperror.CodeInvalidArgument, "role_id 无效")

	ErrRoleQueryInternal        = apperror.NewWithHTTP(apperror.CodeInternal, "查询角色失败", http.StatusInternalServerError)
	ErrRoleCreateInternal       = apperror.NewWithHTTP(apperror.CodeInternal, "新增角色失败", http.StatusInternalServerError)
	ErrRoleUpdateInternal       = apperror.NewWithHTTP(apperror.CodeInternal, "更新角色失败", http.StatusInternalServerError)
	ErrRoleDeleteInternal       = apperror.NewWithHTTP(apperror.CodeInternal, "删除角色失败", http.StatusInternalServerError)
	ErrCurrentRoleQueryInternal = apperror.NewWithHTTP(apperror.CodeInternal, "查询当前用户角色失败", http.StatusInternalServerError)
	ErrMenuQueryInternal        = apperror.NewWithHTTP(apperror.CodeInternal, "查询菜单失败", http.StatusInternalServerError)
	ErrMenuCreateInternal       = apperror.NewWithHTTP(apperror.CodeInternal, "新增菜单失败", http.StatusInternalServerError)
	ErrMenuUpdateInternal       = apperror.NewWithHTTP(apperror.CodeInternal, "更新菜单失败", http.StatusInternalServerError)
	ErrMenuDeleteInternal       = apperror.NewWithHTTP(apperror.CodeInternal, "删除菜单失败", http.StatusInternalServerError)
	ErrMenuGrantInternal        = apperror.NewWithHTTP(apperror.CodeInternal, "分配菜单失败", http.StatusInternalServerError)
)
