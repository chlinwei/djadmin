package audit

import (
	"net/http"

	"autoadmin/internal/shared/apperror"
)

var (
	ErrMethodInvalid = apperror.New(apperror.CodeInvalidArgument, "method 参数无效")
	ErrStatusInvalid = apperror.New(apperror.CodeInvalidArgument, "status_code 参数无效")
	ErrQueryInternal = apperror.NewWithHTTP(apperror.CodeInternal, "查询操作审计失败", http.StatusInternalServerError)
)
