package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
)

func TestSecurityIssuesRequireDedicatedReadPermission(t *testing.T) {
	tokens := identity.NewTokenManager("issues-test", time.Hour)
	engine, err := New(nil, tokens, []string{"*"}, nil, "", "issues-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, permissions := range [][]string{{}, {"baseline:view"}, {"security:issues:view"}} {
		token, err := tokens.Issue(7, "operator", permissions)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{
			"/sys/security/issues/?page=0", "/sys/security/issues/groups/?page=0",
			"/sys/security/issues/invalid/", "/sys/security/issues/invalid/history/",
			"/sys/security/issues/42/history/?scope=invalid",
		} {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("Authorization", token)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			expected := 403
			if len(permissions) > 0 && permissions[0] == "security:issues:view" {
				expected = 400
			}
			if envelope.Code != expected {
				t.Fatalf("%s permissions=%v got %s", path, permissions, recorder.Body.String())
			}
		}
		if len(permissions) != 1 || permissions[0] != "security:issues:view" {
			continue
		}
		request := httptest.NewRequest(http.MethodPost, "/sys/security/issues/42/history/?scope=milestones", nil)
		request.Header.Set("Authorization", token)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound && recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST history unexpectedly routed: status %d, body %s", recorder.Code, recorder.Body.String())
		}
	}
}
