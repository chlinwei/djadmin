package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestAuthenticateExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := identity.NewTokenManager("test-secret", -time.Minute)
	rawToken, err := tokens.Issue(1, "operator", nil)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.GET("/protected", Authenticate(tokens), func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", rawToken)
	engine.ServeHTTP(recorder, request)

	var envelope response.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 301 || envelope.Msg != "Token过期，请重新登录！" {
		t.Fatalf("unexpected expired-token response: %+v", envelope)
	}
}

func TestRequirePermissionFailsClosedAndAllowsAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		claims       *identity.Claims
		permission   string
		expectedCode int
	}{
		{name: "missing permission", claims: &identity.Claims{Username: "operator", Perms: []string{"assets:hosts:view"}}, permission: "assets:hosts:delete", expectedCode: 403},
		{name: "matching permission", claims: &identity.Claims{Username: "operator", Perms: []string{"assets:hosts:view"}}, permission: "assets:hosts:view", expectedCode: 200},
		{name: "admin bypass", claims: &identity.Claims{Username: "admin"}, permission: "any:permission", expectedCode: 200},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Set(identity.ClaimsContextKey, test.claims)
			RequirePermission(test.permission)(context)

			actualCode := 200
			if recorder.Body.Len() > 0 {
				var envelope response.Envelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				actualCode = envelope.Code
			}
			if actualCode != test.expectedCode {
				t.Fatalf("application code = %d, expected %d", actualCode, test.expectedCode)
			}
		})
	}
}
