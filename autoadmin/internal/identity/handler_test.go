package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoginRequestBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "Vue form encoding", contentType: "application/x-www-form-urlencoded", body: "username=admin&password=admin"},
		{name: "JSON clients", contentType: "application/json", body: `{"username":"admin","password":"admin"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/sys/login", strings.NewReader(test.body))
			context.Request.Header.Set("Content-Type", test.contentType)
			var request loginRequest
			if err := context.ShouldBind(&request); err != nil {
				t.Fatalf("ShouldBind() error = %v", err)
			}
			if request.Username != "admin" || request.Password != "admin" {
				t.Fatalf("unexpected login request: %+v", request)
			}
		})
	}
}

func TestParsePageSupportsLegacySize(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/sys/users/?page=2&size=100", nil)
	page, err := parsePage(context)
	if err != nil {
		t.Fatalf("parsePage() error = %v", err)
	}
	if page.Number != 2 || page.Size != 30 || page.Offset != 30 {
		t.Fatalf("unexpected page: %+v", page)
	}
}
