package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
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

func TestWriteRoutesRequireConfiguredPermission(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	tokens := identity.NewTokenManager("test-secret", time.Hour)
	engine, err := New(database, tokens, []string{"*"}, nil, "", "test-secret")
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	token, err := tokens.Issue(7, "limited-user", []string{"unrelated:permission"})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "monitor package create", method: http.MethodPost, path: "/monitor/packages/"},
		{name: "scheduler task update", method: http.MethodPatch, path: "/sys/scheduler/tasks/1/"},
		{name: "inspection task create", method: http.MethodPost, path: "/sys/inspection/tasks/"},
		{name: "asset host update", method: http.MethodPatch, path: "/assets/hosts/1/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock.ExpectExec("INSERT INTO audit_operation_log").
				WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("Authorization", token)
			engine.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("HTTP status = %d, want %d", recorder.Code, http.StatusOK)
			}
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if envelope.Code != 403 {
				t.Fatalf("response code = %d, want 403", envelope.Code)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("database expectations: %v", err)
			}
		})
	}
}
