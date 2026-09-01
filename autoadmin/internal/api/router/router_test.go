package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/identity"
)

func TestLoginPreflight(t *testing.T) {
	tokens := identity.NewTokenManager("test-secret", time.Hour)
	engine, err := New(nil, tokens, []string{"*"}, nil, "", "test-secret")
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/sys/login", nil)
	request.Header.Set("Origin", "http://10.25.66.150:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type,authorization")

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, expected %d", recorder.Code, http.StatusNoContent)
	}
	if origin := recorder.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Fatalf("allow origin = %q, expected *", origin)
	}
	if headers := recorder.Header().Get("Access-Control-Allow-Headers"); headers == "" {
		t.Fatal("expected Access-Control-Allow-Headers")
	}
}
