package binding

import (
	"encoding/json"
	"net/http"

	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

// EnabledOnly recognizes a state-only PATCH and caches the body for full-edit handlers.
func EnabledOnly(c *gin.Context) (*bool, error) {
	if c.Request.Method != http.MethodPatch {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := c.ShouldBindBodyWithJSON(&fields); err != nil {
		return nil, apperror.ErrInvalidRequest
	}
	raw, exists := fields["enabled"]
	if len(fields) != 1 || !exists {
		return nil, nil
	}
	var enabled *bool
	if err := json.Unmarshal(raw, &enabled); err != nil || enabled == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "enabled 必须为布尔值")
	}
	return enabled, nil
}
