package response

import (
	"errors"
	"log"
	"net/http"

	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

func Success(context *gin.Context, data any) {
	context.JSON(http.StatusOK, Envelope{Code: 200, Msg: "success", Data: data})
}

func BusinessError(context *gin.Context, code int, message string, data any) {
	context.JSON(http.StatusOK, Envelope{Code: code, Msg: message, Data: data})
}

func Error(context *gin.Context, err error) {
	appError, ok := apperror.As(err)
	if !ok {
		appError = apperror.WithCause(apperror.ErrInternal, err)
	}
	// 服务端日志必须同时带上「给用户看的那句话」与根因：
	// 只打 `errors.Unwrap(appError)`（cause）时，用 apperror.New 造出来的错误
	//（请求参数错误 / 无权限 / 资源不存在 / token 失效…）**没有 cause**，日志里就只剩一句 `<nil>`。
	// 2026-09-19 现场：菜单保存失败，服务端只留下 `[API-ERROR] PATCH /sys/menus/169/: <nil>`，
	// 等于什么都没说——排查只能靠猜。cause 有值时必须补在后面（500 的根因是排查关键）。
	// 测试环境下 Request 可能为 nil。
	location := ""
	if context.Request != nil {
		location = context.Request.Method + " " + context.Request.URL.Path
	}
	if cause := errors.Unwrap(appError); cause != nil {
		log.Printf("[API-ERROR] %s: [%d] %s: %v", location, appError.Code(), appError.Message(), cause)
	} else {
		log.Printf("[API-ERROR] %s: [%d] %s", location, appError.Code(), appError.Message())
	}
	context.JSON(appError.HTTPStatus(), Envelope{Code: appError.Code(), Msg: appError.Message(), Data: nil})
}

func Paginated(context *gin.Context, results any, count int64, pageNumber int32, pageSize int32) {
	PaginatedWith(context, results, count, pageNumber, pageSize, nil)
}

// PaginatedWith 在标准分页信封上追加页面级字段（如"本页某列为何无法计算"的说明）。
// extra 里的键不会覆盖信封本身的键。
func PaginatedWith(context *gin.Context, results any, count int64, pageNumber int32, pageSize int32, extra gin.H) {
	totalPages := int64(0)
	if count > 0 {
		totalPages = (count + int64(pageSize) - 1) / int64(pageSize)
	}
	payload := gin.H{
		"results": results, "count": count, "pageNumber": pageNumber,
		"pageSize": pageSize, "totalPages": totalPages,
		"next": nil, "previous": nil,
	}
	for key, value := range extra {
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
	Success(context, payload)
}
