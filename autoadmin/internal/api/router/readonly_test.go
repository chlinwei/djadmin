package router

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
)

// Collect every registered read permission, so adding another :view/:list grant cannot
// silently exempt a new mutation from the non-admin readonly regression.
func registeredReadPermissions(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "router.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	permissions := []string{}
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(value, ":view") || strings.HasSuffix(value, ":list") {
			permissions = append(permissions, value)
		}
		return true
	})
	return permissions
}

func TestReadonlyCannotMutateAnyBusinessRoute(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	tokens := identity.NewTokenManager("readonly-test", time.Hour)
	engine, err := New(database, tokens, []string{"*"}, nil, "", "readonly-test")
	if err != nil {
		t.Fatal(err)
	}
	rawToken, err := tokens.Issue(7, "view", registeredReadPermissions(t))
	if err != nil {
		t.Fatal(err)
	}

	// Exceptions are deliberately exact routes, never prefixes. Adding another
	// self-service or validation endpoint requires reviewing its actual side effects.
	exceptions := map[string]string{
		"POST /sys/login":                                             "public login",
		"POST /user/changeAvatar":                                     "current user's avatar",
		"POST /sys/usercenter/updateUserInfo/":                        "current user's profile",
		"POST /sys/usercenter/updateUserPassword/":                    "current user's password with old-password verification",
		"POST /sys/usercenter/updateAlertMediaBindings/":              "current user's notification bindings",
		"POST /sys/automation/playbooks/validate/":                    "content validation, not remote execution",
		"POST /monitor/log-targets/:id/check-status/":                 "read service status, no start/stop or config change",
		"POST /monitor/elasticsearch-clusters/:id/pipeline-simulate/": "pipeline simulation without publishing",
		"POST /monitor/alert-webhook/api/v2/alerts":                   "separate machine authentication",
	}
	parameters := regexp.MustCompile(`[:*][^/]+`)
	checked := 0
	for _, route := range engine.Routes() {
		isTerminal := strings.HasSuffix(route.Path, "/webssh/")
		if !isTerminal && route.Method != http.MethodPost && route.Method != http.MethodPut &&
			route.Method != http.MethodPatch && route.Method != http.MethodDelete {
			continue
		}
		if _, excepted := exceptions[route.Method+" "+route.Path]; excepted {
			continue
		}
		checked++
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			if route.Method != http.MethodGet {
				mock.ExpectExec("INSERT INTO audit_operation_log").WillReturnResult(sqlmock.NewResult(1, 1))
			}
			path := parameters.ReplaceAllString(route.Path, "1")
			request := httptest.NewRequest(route.Method, path, strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", rawToken)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("expected permission response, got HTTP %d: %s", recorder.Code, recorder.Body.String())
			}
			if recorder.Code != http.StatusOK || envelope.Code != 403 {
				t.Fatalf("readonly reached business handler: HTTP %d code %d body %s", recorder.Code, envelope.Code, recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if checked < 100 {
		t.Fatalf("unexpectedly narrow mutation coverage: %d routes", checked)
	}
	t.Logf("verified readonly denial on %d mutation/terminal routes", checked)
}

func TestActionPermissionsReachHandlersWithoutAdmin(t *testing.T) {
	tests := []struct {
		method, path string
		permissions  []string
	}{
		{http.MethodGet, "/assets/hosts/invalid/webssh/", []string{"assets:hosts:execute"}},
		{http.MethodGet, "/ws/assets/hosts/invalid/webssh/", []string{"assets:hosts:execute"}},
		{http.MethodPatch, "/sys/scheduler/tasks/invalid/", []string{"system:scheduler:update"}},
		{http.MethodPost, "/sys/scheduler/tasks/invalid/run_now/", []string{"system:scheduler:execute"}},
		{http.MethodPatch, "/sys/configs/invalid/", []string{"sys:config:update"}},
		{http.MethodPost, "/sys/configs/invalid/reset-default/", []string{"sys:config:update"}},
		{http.MethodPost, "/monitor/packages/", []string{"monitor:view", "monitor:create"}},
		{http.MethodPatch, "/monitor/targets/1/", []string{"monitor:view", "monitor:update"}},
		{http.MethodPost, "/monitor/packages/batch-delete/", []string{"monitor:view", "monitor:delete"}},
		{http.MethodPost, "/monitor/targets/batch-create/", []string{"monitor:view", "monitor:create", "monitor:execute"}},
		{http.MethodPost, "/monitor/targets/batch-delete/", []string{"monitor:view", "monitor:delete", "monitor:execute"}},
		{http.MethodPost, "/monitor/log-targets/batch-create/", []string{"monitor:view", "monitor:log_collect:view", "monitor:log_collect:create", "monitor:log_collect:execute"}},
		{http.MethodPost, "/monitor/log-targets/batch-delete/", []string{"monitor:view", "monitor:log_collect:view", "monitor:log_collect:delete", "monitor:log_collect:execute"}},
		{http.MethodPost, "/monitor/log-targets/batch-jobs/", []string{"monitor:view", "monitor:log_collect:view", "monitor:log_collect:execute"}},
		{http.MethodPost, "/monitor/log-datastreams/cleanup/", []string{"monitor:view", "monitor:logs:delete"}},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			tokens := identity.NewTokenManager("action-test", time.Hour)
			engine, _, err := NewWithGateway(database, tokens, []string{"*"}, nil, "", "action-test", agent.NewGateway(nil, nil), LogBatchOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for index := -1; index < len(test.permissions); index++ {
				permissions := append([]string(nil), test.permissions...)
				wantCode := 400
				wantStatus := http.StatusOK
				if strings.HasSuffix(test.path, "/webssh/") {
					wantStatus = http.StatusBadRequest
				}
				if index >= 0 {
					permissions = append(permissions[:index], permissions[index+1:]...)
					wantCode = 403
					wantStatus = http.StatusOK
				}
				rawToken, err := tokens.Issue(7, "operator", permissions)
				if err != nil {
					t.Fatal(err)
				}
				if test.method != http.MethodGet {
					mock.ExpectExec("INSERT INTO audit_operation_log").WillReturnResult(sqlmock.NewResult(1, 1))
				}
				// Malformed IDs/empty bodies safely prove that a complete permission
				// set passes authorization into validation, without doing real writes.
				request := httptest.NewRequest(test.method, test.path, nil)
				request.Header.Set("Authorization", rawToken)
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				var envelope response.Envelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("decode: %v, body %s", err, recorder.Body.String())
				}
				if recorder.Code != wantStatus || envelope.Code != wantCode {
					t.Fatalf("permissions %v: HTTP %d code %d, want code %d; %s", permissions, recorder.Code, envelope.Code, wantCode, recorder.Body.String())
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestConfigReadRequiresReadPermission(t *testing.T) {
	for _, path := range []string{"/sys/configs/1/", "/sys/configs/by-key/sys.example.value/"} {
		for _, allowed := range []bool{false, true} {
			t.Run(path+"/allowed="+strconv.FormatBool(allowed), func(t *testing.T) {
				database, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer database.Close()
				tokens := identity.NewTokenManager("config-read-test", time.Hour)
				engine, err := New(database, tokens, []string{"*"}, nil, "", "config-read-test")
				if err != nil {
					t.Fatal(err)
				}
				permissions := []string{}
				wantCode := 403
				if allowed {
					permissions = append(permissions, "sys:config:list")
					wantCode = 200
					mock.ExpectQuery("FROM sys_config").WithArgs(sqlmock.AnyArg()).
						WillReturnRows(sqlmock.NewRows([]string{
							"id", "create_time", "update_time", "remark", "key", "value",
							"value_type", "name", "description", "is_readonly", "default_value",
						}).AddRow(1, time.Now().UTC(), time.Now().UTC(), nil, "sys.example.value", "true", "bool", "test", nil, false, "true"))
				}
				rawToken, err := tokens.Issue(7, "view", permissions)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("Authorization", rawToken)
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				var envelope response.Envelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if recorder.Code != http.StatusOK || envelope.Code != wantCode {
					t.Fatalf("code %d, want %d: %s", envelope.Code, wantCode, recorder.Body.String())
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
