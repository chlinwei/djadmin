package inspection

import (
	"database/sql"
	"errors"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// batchDeleteInspection 解析 {"ids":[...]} 并逐 id 执行 deleteOne，逐条记录
// ok/message；不存在的 id 记 ok:false，不中断其余 id（与 BatchDeleteLogTargets 范式一致）。
func batchDeleteInspection(context *gin.Context, handler *Handler, deleteOne func(*gin.Context, int64) error) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.BusinessError(context, 400, "ids must be a non-empty array", nil)
		return
	}
	results := make([]gin.H, 0, len(input.IDs))
	okCount := 0
	for _, id := range input.IDs {
		if err := deleteOne(context, id); err != nil {
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
	response.Success(context, gin.H{"count": okCount, "results": results})
}
