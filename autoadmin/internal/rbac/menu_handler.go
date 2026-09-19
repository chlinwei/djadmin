package rbac

import (
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/binding"
	"database/sql"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
)

type menuRequest struct {
	Name       string  `json:"name"`
	Icon       *string `json:"icon"`
	ParentID   *int32  `json:"parent_id"`
	OrderNum   *int32  `json:"order_num"`
	Path       *string `json:"path"`
	Component  *string `json:"component"`
	MenuType   *string `json:"menu_type"`
	Perms      *string `json:"perms"`
	Remark     *string `json:"remark"`
	Location   int16   `json:"location"`
	IsExpanded *bool   `json:"is_expanded"`
}

// menuBindError 把 ShouldBindJSON 的失败原因带上（含**字段名**）。
//
// 只说"请求参数错误"时谁也定位不到：前端弹一个笼统提示、服务端日志只有一个 `<nil>`。
// 2026-09-19 现场：菜单保存失败，真实原因只是「显示顺序」被当成字符串提交
// （`json: cannot unmarshal string into Go struct field menuRequest.order_num of type int32`），
// 而它必须是数字——这句原文比任何猜测都好用，所以直接透给调用方。
func menuBindError(err error) error {
	return apperror.New(apperror.CodeInvalidArgument, "请求参数错误："+err.Error())
}

func (h *Handler) MenuTree(c *gin.Context) {
	tree, err := h.service.MenuTree(c.Request.Context())
	if err != nil {
		response.Error(c, apperror.WithCause(ErrMenuQueryInternal, err))
		return
	}
	response.Success(c, tree)
}
func (h *Handler) GetMenu(c *gin.Context) {
	id, ok := roleID(c)
	if !ok {
		return
	}
	item, err := h.service.GetMenu(c.Request.Context(), id)
	respond(c, item, err, ErrMenuQueryInternal)
}
func (h *Handler) CreateMenu(c *gin.Context) {
	var r menuRequest
	if err := c.ShouldBindJSON(&r); err != nil {
		response.Error(c, menuBindError(err))
		return
	}
	if strings.TrimSpace(r.Name) == "" {
		response.Error(c, ErrMenuNameRequired)
		return
	}
	expanded := true
	if r.IsExpanded != nil {
		expanded = *r.IsExpanded
	}
	location := r.Location
	if location == 0 {
		location = 1
	}
	item, err := h.service.CreateMenu(c.Request.Context(), MenuInput{Name: strings.TrimSpace(r.Name), Icon: r.Icon, ParentID: r.ParentID, OrderNum: r.OrderNum, Path: r.Path, Component: r.Component, MenuType: r.MenuType, Perms: r.Perms, Remark: r.Remark, Location: location, IsExpanded: expanded})
	respond(c, item, err, ErrMenuCreateInternal)
}
func (h *Handler) UpdateMenu(c *gin.Context) {
	id, ok := roleID(c)
	if !ok {
		return
	}
	current, err := h.service.GetMenu(c.Request.Context(), id)
	if err != nil {
		respond(c, current, err, ErrMenuQueryInternal)
		return
	}
	var r menuRequest
	if err := c.ShouldBindJSON(&r); err != nil {
		// 带出**是哪个字段**：只说"请求参数错误"时，前端与服务端都不知道该改哪里
		// （2026-09-19 现场：菜单保存失败，服务端日志只有一个 `<nil>`，前端只弹出一个 `400`，
		// 真正的原因是「显示顺序」被当成字符串提交，而它必须是数字）。
		response.Error(c, menuBindError(err))
		return
	}
	expanded := current.IsExpanded
	if r.IsExpanded != nil {
		expanded = *r.IsExpanded
	}
	item, err := h.service.UpdateMenu(c.Request.Context(), id, MenuInput{Name: r.Name, Icon: r.Icon, ParentID: r.ParentID, OrderNum: r.OrderNum, Path: r.Path, Component: r.Component, MenuType: r.MenuType, Perms: r.Perms, Remark: r.Remark, Location: r.Location, IsExpanded: expanded})
	respond(c, item, err, ErrMenuUpdateInternal)
}
func (h *Handler) MenuIDsByRole(c *gin.Context) {
	roleIDValue, err := positive(c.Query("role_id"), 0)
	if err != nil || roleIDValue < 1 {
		response.Error(c, ErrRoleIDInvalid)
		return
	}
	ids, err := h.service.MenuIDsByRole(c.Request.Context(), roleIDValue)
	if err != nil {
		response.Error(c, apperror.WithCause(ErrMenuQueryInternal, err))
		return
	}
	response.Success(c, ids)
}
func (h *Handler) GrantMenus(c *gin.Context) {
	var r struct {
		RoleID  int32        `json:"role_id"`
		MenuIDs []binding.ID `json:"menuIds"`
	}
	if c.ShouldBindJSON(&r) != nil || r.RoleID < 1 {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	err := h.service.GrantMenus(c.Request.Context(), r.RoleID, binding.Int32s(r.MenuIDs))
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, ErrRoleNotFound)
		return
	}
	if err != nil {
		response.Error(c, apperror.WithCause(ErrMenuGrantInternal, err))
		return
	}
	response.Success(c, nil)
}
func (h *Handler) DeleteMenu(c *gin.Context) {
	var r struct {
		ID int32 `json:"id"`
	}
	if c.ShouldBindJSON(&r) != nil || r.ID < 1 {
		response.Error(c, apperror.ErrIDInvalid)
		return
	}
	if err := h.service.DeleteMenu(c.Request.Context(), r.ID); err != nil {
		response.Error(c, apperror.WithCause(ErrMenuDeleteInternal, err))
		return
	}
	response.Success(c, nil)
}
