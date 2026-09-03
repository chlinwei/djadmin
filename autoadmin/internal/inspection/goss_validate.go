package inspection

import (
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

type gossValidateRequest struct {
	Spec string `json:"spec"`
}

// ValidateGossSpec 校验 goss YAML 是否符合官方 schema，供前端编辑器即时校验按钮调用。
// 与保存检查项时的校验完全同源（validateGossSpec），避免两处语义漂移。
func (handler *Handler) ValidateGossSpec(context *gin.Context) {
	var input gossValidateRequest
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	if input.Spec == "" {
		response.BusinessError(context, 400, "goss spec 不能为空", nil)
		return
	}
	if err := validateGossSpec(input.Spec); err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, gin.H{"valid": true})
}
