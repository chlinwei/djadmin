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

func TestReadOnlyRoutesUseGET(t *testing.T) {
	tokens := identity.NewTokenManager("test-secret", time.Hour)
	engine, err := New(nil, tokens, []string{"*"}, nil, "", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/sys/security/baseline/scan-hosts/",
		"/sys/automation/inventories/:id/precheck-limit/",
		"/sys/automation/tasks/:id/precheck/",
		"/monitor/targets/:id/check-service-status/",
		"/monitor/targets/prometheus/proxy/*apiPath",
		"/assets/application-services/log-status-summary/",
		"/assets/service-dependencies/topology/",
		"/assets/hosts/:id/refresh-info/",
		"/assets/hosts/refresh-info/",
		"/assets/application-services/:id/refresh-runtime-status/",
		"/assets/application-deployments/refresh-runtime-status/",
		"/monitor/log-targets/:id/check-status/",
		"/monitor/elasticsearch-clusters/:id/test-connection/",
	} {
		t.Run(path, func(t *testing.T) {
			found := false
			for _, route := range engine.Routes() {
				if route.Path != path {
					continue
				}
				if route.Method != http.MethodGet {
					t.Errorf("read-only route accepts %s", route.Method)
				}
				found = found || route.Method == http.MethodGet
			}
			if !found {
				t.Fatal("GET route missing")
			}
		})
	}
}

func TestObservationQueriesDoNotKeepPOSTRoutes(t *testing.T) {
	tokens := identity.NewTokenManager("test-secret", time.Hour)
	engine, err := New(nil, tokens, []string{"*"}, nil, "", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/assets/hosts/1/refresh-info/",
		"/assets/hosts/refresh-info/",
		"/assets/application-services/1/refresh-runtime-status/",
		"/assets/application-deployments/refresh-runtime-status/",
		"/monitor/log-targets/1/check-status/",
		"/monitor/elasticsearch-clusters/1/test-connection/",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
			// Gin may normalize the slash of an unmatched POST; it must still
			// reject that method after the redirect, not restore a query route.
			if recorder.Code == http.StatusTemporaryRedirect {
				location := recorder.Header().Get("Location")
				recorder = httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, location, nil))
			}
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("removed POST query returned %d, want 404", recorder.Code)
			}
		})
	}
}

func TestContentValidationAndSimulationRemainPOST(t *testing.T) {
	tokens := identity.NewTokenManager("test-secret", time.Hour)
	engine, err := New(nil, tokens, []string{"*"}, nil, "", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/sys/automation/playbooks/validate/",
		"/monitor/elasticsearch-clusters/:id/pipeline-simulate/",
	} {
		found := false
		for _, route := range engine.Routes() {
			if route.Path != path {
				continue
			}
			if route.Method != http.MethodPost {
				t.Errorf("%s accepts %s, want POST only", path, route.Method)
			}
			found = true
		}
		if !found {
			t.Errorf("%s is not registered", path)
		}
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
		{name: "dependency create", method: http.MethodPost, path: "/assets/service-dependencies/"},
		{name: "dependency update", method: http.MethodPut, path: "/assets/service-dependencies/1/"},
		{name: "dependency delete", method: http.MethodPost, path: "/assets/service-dependencies/batch-delete/"},
		{name: "dependency type create", method: http.MethodPost, path: "/assets/service-dependency-types/"},
		{name: "dependency type update", method: http.MethodPut, path: "/assets/service-dependency-types/1/"},
		{name: "dependency type delete", method: http.MethodPost, path: "/assets/service-dependency-types/batch-delete/"},
		{name: "other user update", method: http.MethodPatch, path: "/sys/users/99/"},
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
