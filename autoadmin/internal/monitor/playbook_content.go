package monitor

import (
	"database/sql"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// playbookContent 取 Playbook 模板内容（安装派发时把内容快照进作业行）。
// logcollect 侧另有一份同名实现（各自域独立）。
func playbookContent(context *gin.Context, pool *sql.DB, playbookID int64) (string, error) {
	// 复用 automation 域已有的取模板查询（内容是唯一需要的列）。
	playbook, err := db.New(pool).GetAutomationPlaybook(context, playbookID)
	if err != nil {
		return "", err
	}
	return playbook.Content, nil
}
