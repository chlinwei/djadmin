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
	if c.ShouldBindJSON(&r) != nil || strings.TrimSpace(r.Name) == "" {
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
	if c.ShouldBindJSON(&r) != nil {
		response.Error(c, apperror.ErrInvalidRequest)
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
