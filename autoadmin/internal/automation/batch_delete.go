package automation

import (
	"database/sql"
	"errors"
	"fmt"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// batchIDs 统一解析批删请求体 {"ids":[...]}，与 monitor.BatchDeleteLogTargets 的范式一致。
func batchIDs(context *gin.Context) ([]int64, bool) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		automationBadRequest(context, "ids must be a non-empty array")
		return nil, false
	}
	return input.IDs, true
}

// respondBatchDelete 逐 id 执行删除并汇总：ok:false 表示不存在或校验未通过，不中断其余 id。
func respondBatchDelete(context *gin.Context, ids []int64, deleteOne func(int64) error) {
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
	response.Success(context, gin.H{"count": okCount, "results": results})
}

// BatchDeletePlaybooks 逐 id 复用原单删逻辑：Agent 安装/更新模板禁止删除。
func (handler *Handler) BatchDeletePlaybooks(context *gin.Context) {
	ids, ok := batchIDs(context)
	if !ok {
		return
	}
	respondBatchDelete(context, ids, func(id int64) error {
		playbook, err := db.New(handler.db).GetAutomationPlaybook(context, id)
		if err != nil {
			return err
		}
		if playbook.Category == playbookCategoryAgent {
			return fmt.Errorf("该模板是 Agent 安装/更新的唯一配置源，禁止删除")
		}
		affected, err := db.New(handler.db).DeleteAutomationPlaybook(context, id)
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func (handler *Handler) BatchDeleteInventories(context *gin.Context) {
	ids, ok := batchIDs(context)
	if !ok {
		return
	}
	respondBatchDelete(context, ids, func(id int64) error {
		affected, err := db.New(handler.db).DeleteAutomationInventory(context, id)
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func (handler *Handler) BatchDeleteTasks(context *gin.Context) {
	ids, ok := batchIDs(context)
	if !ok {
		return
	}
	respondBatchDelete(context, ids, func(id int64) error {
		affected, err := db.New(handler.db).DeleteAutomationTask(context, id)
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
}
