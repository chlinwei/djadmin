package sysconfig

import (
	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }
func (h *Handler) List(c *gin.Context) {
	page, err := configPage(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.List(c.Request.Context(), strings.TrimSpace(c.Query("search")), page)
	if err != nil {
		response.Error(c, apperror.WithCause(ErrQueryInternal, err))
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) Get(c *gin.Context) {
	id, ok := configID(c)
	if !ok {
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), id)
	configRespond(c, item, err)
}
func (h *Handler) GetByKey(c *gin.Context) {
	item, err := h.service.GetByKey(c.Request.Context(), c.Param("key"))
	if err != nil {
		configRespond(c, item, err)
		return
	}
	response.Success(c, gin.H{"key": item.Key, "value": item.Value, "name": item.Name})
}
func (h *Handler) Update(c *gin.Context) {
	id, ok := configID(c)
	if !ok {
		return
	}
	var r struct {
		Value        any `json:"value"`
		DefaultValue any `json:"default_value"`
	}
	if c.ShouldBindJSON(&r) != nil {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	claims, _ := identity.ClaimsFromContext(c)
	item, err := h.service.Update(c.Request.Context(), id, r.Value, r.DefaultValue, claims != nil && claims.Username == "admin")
	configRespond(c, item, err)
}
func (h *Handler) UpdateByKey(c *gin.Context) {
	var r struct {
		Value any `json:"value"`
	}
	if c.ShouldBindJSON(&r) != nil {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	item, err := h.service.UpdateByKey(c.Request.Context(), c.Param("key"), r.Value)
	configRespond(c, item, err)
}
func (h *Handler) Reset(c *gin.Context) {
	id, ok := configID(c)
	if !ok {
		return
	}
	item, err := h.service.Reset(c.Request.Context(), id)
	configRespond(c, item, err)
}
func configPage(c *gin.Context) (pagination.Page, error) {
	number, err := strconv.ParseInt(defaultText(c.Query("page"), "1"), 10, 32)
	if err != nil || number < 1 {
		return pagination.Page{}, apperror.ErrPageInvalid
	}
	sizeRaw := c.Query("page_size")
	if sizeRaw == "" {
		sizeRaw = c.Query("size")
	}
	size, err := strconv.ParseInt(defaultText(sizeRaw, "10"), 10, 32)
	if err != nil || size < 1 {
		return pagination.Page{}, apperror.ErrPageSizeInvalid
	}
	return pagination.New(int32(number), int32(size)), nil
}
func defaultText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func configID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.Error(c, apperror.ErrIDInvalid)
		return 0, false
	}
	return id, true
}
func configRespond(c *gin.Context, data any, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, ErrNotFound)
		return
	}
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, data)
}
