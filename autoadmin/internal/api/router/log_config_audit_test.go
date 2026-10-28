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

func TestLogConfigAuditRequiresLogCollectPermission(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "forbidden", true: "allowed"}[allowed], func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			tokens := identity.NewTokenManager("test-secret", time.Hour)
			engine, err := New(database, tokens, []string{"*"}, nil, "", "test-secret")
			if err != nil {
				t.Fatal(err)
			}
			permissions := []string{"monitor:view"}
			wantCode := 403
			if allowed {
				permissions = append(permissions, "monitor:log_collect:view")
				wantCode = 200
				mock.ExpectBegin()
				mock.ExpectQuery("name: ListLogConfigAuditHosts").WillReturnRows(
					sqlmock.NewRows([]string{"host_id", "host_ip", "host_instance_name"}))
				mock.ExpectCommit()
			}
			token, err := tokens.Issue(7, "audit-user", permissions)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/monitor/log-targets/config-audit/", nil)
			request.Header.Set("Authorization", token)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || envelope.Code != wantCode {
				t.Fatalf("unexpected permission result: %s", recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
