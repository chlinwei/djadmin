package assets

import (
	"strconv"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func dependencyFilter(c *gin.Context) (DependencyFilter, error) {
	var f DependencyFilter
	for name, dest := range map[string]*int64{
		"project": &f.ProjectID, "environment": &f.EnvironmentID, "business_system": &f.BusinessID,
		"service": &f.ServiceID, "source_service": &f.SourceServiceID, "target_service": &f.TargetServiceID,
		"interaction_type": &f.InteractionTypeID,
	} {
		value, err := queryID(c, name)
		if err != nil {
			return f, err
		}
		*dest = value
	}
	f.Strength = c.Query("strength")
	f.Search = c.Query("search")
	if f.Strength != "" && !validDependencyStrength(f.Strength) {
		return f, ErrInvalid
	}
	return f, nil
}

func (h *Handler) ListServiceDependencies(c *gin.Context) {
	p, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	f, err := dependencyFilter(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListServiceDependencies(c.Request.Context(), f, p)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, p.Number, p.Size)
}

func (h *Handler) GetServiceDependency(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	row, err := h.service.GetServiceDependency(c.Request.Context(), id)
	respond(c, row, err)
}

func (h *Handler) CreateServiceDependency(c *gin.Context) {
	input, ok := bind[ServiceDependencyInput](c)
	if !ok {
		return
	}
	rows, err := h.service.SaveServiceDependency(c.Request.Context(), 0, input)
	respond(c, gin.H{"results": rows}, err)
}

func (h *Handler) UpdateServiceDependency(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[ServiceDependencyInput](c)
	if !ok {
		return
	}
	rows, err := h.service.SaveServiceDependency(c.Request.Context(), id, input)
	respond(c, gin.H{"results": rows}, err)
}

func dependencyDeleteIDs(c *gin.Context) ([]int64, bool) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return nil, false
	}
	if len(ids) > 500 {
		response.Error(c, apperror.ErrInvalidRequest)
		return nil, false
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id < 1 || seen[id] {
			response.Error(c, apperror.ErrInvalidRequest)
			return nil, false
		}
		seen[id] = true
	}
	return ids, true
}

func (h *Handler) BatchDeleteServiceDependencies(c *gin.Context) {
	ids, ok := dependencyDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error { return h.service.DeleteServiceDependency(c.Request.Context(), id) })
}

func (h *Handler) GetServiceDependencyTopology(c *gin.Context) {
	var scope DependencyTopologyScope
	for name, dest := range map[string]*int64{"project": &scope.ProjectID, "environment": &scope.EnvironmentID, "business_system": &scope.BusinessID, "service": &scope.ServiceID} {
		value, err := queryID(c, name)
		if err != nil {
			response.Error(c, err)
			return
		}
		*dest = value
	}
	result, err := h.service.GetServiceDependencyTopology(c.Request.Context(), scope)
	respond(c, result, err)
}

func (h *Handler) ListServiceDependencyTypes(c *gin.Context) {
	p, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var enabled *bool
	if c.Request.URL.Query().Has("enabled") {
		flag, err := strconv.ParseBool(c.Query("enabled"))
		if err != nil {
			response.Error(c, ErrInvalid)
			return
		}
		enabled = &flag
	}
	rows, count, err := h.service.ListServiceDependencyTypes(c.Request.Context(), c.Query("search"), enabled, p)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, p.Number, p.Size)
}

func (h *Handler) GetServiceDependencyType(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	row, err := h.service.GetServiceDependencyType(c.Request.Context(), id)
	respond(c, row, err)
}

func (h *Handler) CreateServiceDependencyType(c *gin.Context) {
	input, ok := bind[DependencyTypeInput](c)
	if !ok {
		return
	}
	row, err := h.service.SaveServiceDependencyType(c.Request.Context(), 0, input)
	respond(c, row, err)
}

func (h *Handler) UpdateServiceDependencyType(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[DependencyTypeInput](c)
	if !ok {
		return
	}
	row, err := h.service.SaveServiceDependencyType(c.Request.Context(), id, input)
	respond(c, row, err)
}

func (h *Handler) BatchDeleteServiceDependencyTypes(c *gin.Context) {
	ids, ok := dependencyDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error { return h.service.DeleteServiceDependencyType(c.Request.Context(), id) })
}
