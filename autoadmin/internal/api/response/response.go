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
	// 500 的根因必须落服务端日志，否则前端只看到笼统错误、无从排查。
	// 测试环境下 Request 可能为 nil。
	if context.Request != nil {
		log.Printf("[API-ERROR] %s %s: %v", context.Request.Method, context.Request.URL.Path, errors.Unwrap(appError))
	} else {
		log.Printf("[API-ERROR] %v", errors.Unwrap(appError))
	}
	context.JSON(appError.HTTPStatus(), Envelope{Code: appError.Code(), Msg: appError.Message(), Data: nil})
}

func Paginated(context *gin.Context, results any, count int64, pageNumber int32, pageSize int32) {
	totalPages := int64(0)
	if count > 0 {
		totalPages = (count + int64(pageSize) - 1) / int64(pageSize)
	}
	Success(context, gin.H{
		"results": results, "count": count, "pageNumber": pageNumber,
		"pageSize": pageSize, "totalPages": totalPages,
		"next": nil, "previous": nil,
	})
}
