package logcollect

import (
	"fmt"
	"strconv"
	"strings"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// 本包自备的请求/取值小工具。与 assets、automation、monitor、inspection 等包同名同义的
// 函数各自持有一份，是仓库既有约定（每个域独立、不为两行函数建公共包）。

func parseID(value string) int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return id
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func defaultString(value any, fallback string) string {
	result := stringValue(value)
	if result == "" {
		return fallback
	}
	return result
}

// boolValue 把 PATCH 提交的 JSON 值转成 bool（兼容 true/1/"true" 三种写法）。
func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed == "true" || typed == "1"
	default:
		return false
	}
}

func intValue(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		result, _ := strconv.ParseInt(stringValue(value), 10, 64)
		return result
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// ids 解析批量接口的 body `{"ids":[...]}`。列表型资源只提供批量删除/批量操作，
// 因此这是本包所有批量入口的统一读取方式（与 monitor 的 batchTargetIDs 同一范式）。
func ids(context *gin.Context) ([]int64, bool) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.BusinessError(context, 400, "ids must be a non-empty array", nil)
		return nil, false
	}
	return input.IDs, true
}

// serviceControlResult 判定 agent 的执行结果：Agent 在线且动作已下发时 Execute 的 err 为 nil，
// 但 systemctl 本身可能失败（服务不存在、权限不足等），必须检查状态/退出码，否则会把失败当成功上报。
// 注意 Status 为空也是失败：executor 层直接报错（典型是 agent 版本过旧、不认识动作名）时
// 返回的是零值 JobResult，Status=""、ExitCode=0，只判 "failed" 会被漏过。
func serviceControlResult(result *pb.AutomationExecuteResponse, action, exporterType string) (gin.H, error) {
	if action != "status" && (result.Status != "success" || result.ExitCode != 0) {
		reason := firstNonEmpty(strings.TrimSpace(result.ErrorMessage), strings.TrimSpace(result.Stderr), strings.TrimSpace(result.Stdout))
		return nil, fmt.Errorf("systemctl %s %s.service failed: %s", action, exporterType, reason)
	}
	return gin.H{"job_id": result.JobId, "status": result.Status, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr, "error_message": result.ErrorMessage}, nil
}

// pagination 解析列表接口的 page/page_size（上限 30，与 monitor 的 lists.go 同名同义各持一份）。
func pagination(context *gin.Context) (int, int) {
	page, _ := strconv.Atoi(context.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(context.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 30 {
		size = 30
	}
	return page, size
}
