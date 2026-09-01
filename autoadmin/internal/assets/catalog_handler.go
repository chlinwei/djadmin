package assets

import (
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListApplications(c *gin.Context) {
	p, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListApplications(c.Request.Context(), c.Query("search"), p)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, p.Number, p.Size)
}
func (h *Handler) GetApplication(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetApplication(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateApplication(c *gin.Context) {
	input, ok := bind[ApplicationInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateApplication(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateApplication(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[ApplicationInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateApplication(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteApplications(c *gin.Context) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	for _, id := range input.IDs {
		if err := h.service.DeleteApplication(c.Request.Context(), id); err != nil {
			respond(c, nil, err)
			return
		}
	}
	response.Success(c, nil)
}
func (h *Handler) ListVersions(c *gin.Context) {
	p, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	app, err := queryID(c, "application")
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListVersions(c.Request.Context(), app, c.Query("search"), p)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, p.Number, p.Size)
}
func (h *Handler) GetVersion(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetVersion(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateVersion(c *gin.Context) {
	input, ok := bind[ApplicationVersionInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateVersion(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateVersion(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[ApplicationVersionInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateVersion(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteVersion(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteVersion(c.Request.Context(), id))
}
func (h *Handler) ListProfiles(c *gin.Context) {
	p, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	app, err := queryID(c, "application")
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListProfiles(c.Request.Context(), app, c.Query("search"), p)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, p.Number, p.Size)
}
func (h *Handler) GetProfile(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetProfile(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateProfile(c *gin.Context) {
	input, ok := bind[ClusterProfileInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateProfile(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateProfile(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[ClusterProfileInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateProfile(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteProfile(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteProfile(c.Request.Context(), id))
}
