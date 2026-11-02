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

func TestInventoryScopeQueriesUseReadPermission(t *testing.T) {
	for _, path := range []string{
		"/sys/automation/playbooks/host-options/",
		"/sys/automation/playbooks/group-tree/",
	} {
		for _, permission := range []string{"", "automation:inventory:view", "automation:jobs:create"} {
			t.Run(path+"/"+permission, func(t *testing.T) {
				database, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer database.Close()
				tokens := identity.NewTokenManager("inventory-read-test", time.Hour)
				engine, err := New(database, tokens, []string{"*"}, nil, "", "inventory-read-test")
				if err != nil {
					t.Fatal(err)
				}
				wantCode := 403
				if permission == "automation:inventory:view" {
					wantCode = 200
					if path == "/sys/automation/playbooks/host-options/" {
						mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_host h").
							WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
						mock.ExpectQuery("SELECT h.id, h.instance_name, s.hostname, h.ip, h.group_id").
							WillReturnRows(sqlmock.NewRows([]string{"id", "instance_name", "hostname", "ip", "group_id"}).
								AddRow(1, "host-1", "os-host-1", "10.0.0.1", 1))
					} else {
						mock.ExpectQuery("SELECT id, name FROM assets_hostgroup ORDER BY id").
							WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "production"))
					}
				}
				permissions := []string{}
				if permission != "" {
					permissions = append(permissions, permission)
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
					t.Fatalf("permission %q: HTTP %d code %d, want code %d; %s",
						permission, recorder.Code, envelope.Code, wantCode, recorder.Body.String())
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
