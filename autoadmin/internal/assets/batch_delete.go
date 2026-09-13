package assets

import (
	"database/sql"
	"errors"

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

// respondBatchDelete 逐 id 执行删除并汇总：不存在的 id（sql.ErrNoRows）记 ok:false，
// 其余失败也只记录 message，不中断整个批次（与 BatchDeleteLogTargets 范式一致）。
func respondBatchDelete(c *gin.Context, ids []int64, deleteOne func(id int64) error) {
	results := make([]gin.H, 0, len(ids))
	okCount := 0
	for _, id := range ids {
		if err := deleteOne(id); err != nil {
			message := err.Error()
			if errors.Is(err, sql.ErrNoRows) {
				message = "resource not found"
			}
			results = append(results, gin.H{"id": id, "ok": false, "message": message})
			continue
		}
		okCount++
		results = append(results, gin.H{"id": id, "ok": true, "message": ""})
	}
	response.Success(c, gin.H{"count": okCount, "results": results})
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
