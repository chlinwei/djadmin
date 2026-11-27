package monitor

import (
	"database/sql"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func deleteRowsAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// batchDeleteMonitorRows 无前置校验的简单表通用批删（告警媒介等）。
// 表名不作为字符串传入：sqlc 的语句是编译期固定的，按表分派由调用方给出的 deleteOne 承担。
func batchDeleteMonitorRows(context *gin.Context, handler *Handler, deleteOne func(*gin.Context, int64) error) {
	ids, ok := batchTargetIDs(context)
	if !ok {
		return
	}
	results := make([]response.BatchResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, response.BatchItem(id, deleteOne(context, id)))
	}
	response.Batch(context, results)
}
