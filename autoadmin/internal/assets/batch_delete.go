package assets

import (
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

// batchDeleteIDs 解析统一批删请求体 {"ids":[...]}。
func batchDeleteIDs(c *gin.Context) ([]int64, bool) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.Error(c, apperror.ErrInvalidRequest)
		return nil, false
	}
	return input.IDs, true
}

// respondBatchDelete 逐 id 独立删除，汇总已提交的结果，任何失败都不能返回成功业务码。
func respondBatchDelete(c *gin.Context, ids []int64, deleteOne func(id int64) error) {
	results := make([]response.BatchResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, response.BatchItem(id, deleteOne(id)))
	}
	response.Batch(c, results)
}

func (h *Handler) BatchDeleteProjects(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteProject(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteBusinessSystems(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteBusinessSystem(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteEnvironments(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteEnvironment(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteHostGroups(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteHostGroup(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteVersions(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteVersion(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteProfiles(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteProfile(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteDeploymentTemplates(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteDeploymentTemplate(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteApplicationServices(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteApplicationService(c.Request.Context(), id)
	})
}

func (h *Handler) BatchDeleteApplicationDeployments(c *gin.Context) {
	ids, ok := batchDeleteIDs(c)
	if !ok {
		return
	}
	respondBatchDelete(c, ids, func(id int64) error {
		return h.service.DeleteApplicationDeployment(c.Request.Context(), id)
	})
}
