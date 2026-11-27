package inspection

import (
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// batchDeleteInspection 逐 id 独立删除，保留已提交的结果和每项错误码。
func batchDeleteInspection(context *gin.Context, handler *Handler, deleteOne func(*gin.Context, int64) error) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.BusinessError(context, 400, "ids must be a non-empty array", nil)
		return
	}
	results := make([]response.BatchResult, 0, len(input.IDs))
	for _, id := range input.IDs {
		results = append(results, response.BatchItem(id, deleteOne(context, id)))
	}
	response.Batch(context, results)
}
