package automation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
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
			return translateDeleteReference(err, "模板仍被其他资源引用，请先调整关联配置后再删除")
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
			return translateDeleteReference(err, "Inventory 仍被自动化任务引用，请先在任务中更换或解除该 Inventory，或删除关联任务后再重试")
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
		return handler.deleteTask(context, id)
	})
}

func translateDeleteReference(err error, message string) error {
	var mysqlError *mysql.MySQLError
	var stateError interface{ SQLState() string }
	// 只在 DELETE 失败路径翻译引用约束，不把其他数据库故障伪装成业务限制。
	if (errors.As(err, &mysqlError) && mysqlError.Number == 1451) ||
		(errors.As(err, &stateError) && stateError.SQLState() == "23503") {
		return errors.New(message)
	}
	return err
}

func (handler *Handler) deleteTask(ctx context.Context, id int64) error {
	transaction, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	queries := db.New(transaction)
	// 先锁父行：外键插入也要获取父行锁，避免检查结束后又提交新作业。
	if _, err := queries.LockAutomationTaskForDeletion(ctx, id); err != nil {
		return err
	}
	jobs, err := queries.LockAutomationTaskJobsForDeletion(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if !isTerminalJobStatus(job.Status) {
			return fmt.Errorf("任务存在未结束的作业 #%d（%s），请等待作业结束或取消后再删除", job.ID, job.Status)
		}
	}
	// 作业内容、名称和运行参数已有快照，只解除关联，不删除历史和日志。
	if err := queries.DetachAutomationTaskJobs(ctx, sql.NullInt64{Int64: id, Valid: true}); err != nil {
		return err
	}
	affected, err := queries.DeleteAutomationTask(ctx, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return transaction.Commit()
}
