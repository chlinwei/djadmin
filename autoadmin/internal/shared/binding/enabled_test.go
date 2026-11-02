package binding

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEnabledOnly(t *testing.T) {
	for _, tt := range []struct {
		body                 string
		only, value, invalid bool
	}{
		{`{"enabled":true}`, true, true, false},
		{`{"enabled":false}`, true, false, false},
		{`{"enabled":null}`, false, false, true},
		{`{"enabled":"false"}`, false, false, true},
		{`{"enabled":0}`, false, false, true},
		{`{"enabled":[]}`, false, false, true},
		{`{"enabled":{}}`, false, false, true},
		{`{`, false, false, true},
		{`[]`, false, false, true},
		{`{}`, false, false, false},
		{`{"name":"test","enabled":false}`, false, false, false},
	} {
		t.Run(tt.body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("PATCH", "/", strings.NewReader(tt.body))
			value, err := EnabledOnly(c)
			if (err != nil) != tt.invalid || (value != nil) != tt.only || value != nil && *value != tt.value {
				t.Fatalf("value=%v error=%v", value, err)
			}
			if !tt.invalid {
				var fields map[string]any
				if err := c.ShouldBindBodyWithJSON(&fields); err != nil {
					t.Fatalf("full-edit handler lost request body: %v", err)
				}
			}
		})
	}
}
