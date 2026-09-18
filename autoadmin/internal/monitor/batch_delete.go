package monitor

import (
	"database/sql"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// 本包自备的批删小工具。同名同义的实现也在 logcollect 里各持一份（仓库既有约定：
// 每个域独立，不为两行函数建公共包）。

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

func batchErrMessage(err error) string {
	if err == sql.ErrNoRows {
		return "resource not found"
	}
	return err.Error()
}

// batchDeleteMonitorRows 无前置校验的简单表通用批删（告警媒介等）。
// 表名不作为字符串传入：sqlc 的语句是编译期固定的，按表分派由调用方给出的 deleteOne 承担。
func batchDeleteMonitorRows(context *gin.Context, handler *Handler, deleteOne func(*gin.Context, int64) error) {
	ids, ok := batchTargetIDs(context)
	if !ok {
		return
	}
	results := make([]gin.H, 0, len(ids))
	okCount := 0
	for _, id := range ids {
		err := deleteOne(context, id)
		if err != nil {
			results = append(results, gin.H{"id": id, "ok": false, "message": batchErrMessage(err)})
			continue
		}
		okCount++
		results = append(results, gin.H{"id": id, "ok": true, "message": ""})
	}
	response.Success(context, gin.H{"count": okCount, "results": results})
}
