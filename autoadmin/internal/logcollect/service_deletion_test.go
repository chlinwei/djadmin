package logcollect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/assets"
	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestServiceDeletionScopeUsesAllOwnershipDimensions(t *testing.T) {
	raw, _ := json.Marshal(serviceDeletionScope(db.AssetsServiceDeletion{
		ServiceID: 7, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "SVC",
	}))
	for _, field := range []string{"project", "business_system", "environment", "service"} {
		if !strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("missing ownership dimension %s: %s", field, raw)
		}
	}
	if strings.Contains(string(raw), "match_all") {
		t.Fatal("purge must never delete an entire shared index")
	}
	for _, field := range []string{"project", "business_system", "environment", "service"} {
		if !strings.Contains(serviceDeletionCondition, "ctx."+field+" == owner."+field) {
			t.Fatalf("guard must compare %s without interpolating user input", field)
		}
	}
}

func TestServicePurgeRequiresSynchronousCompleteSuccess(t *testing.T) {
	for _, raw := range []string{
		`{"task":"pending"}`, `{}`, `{"total":2,"deleted":1}`,
		`{"total":1,"deleted":1,"timed_out":true}`,
		`{"total":1,"deleted":1,"version_conflicts":1}`,
		`{"total":1,"deleted":1,"failures":[{"reason":"denied"}]}`,
	} {
		var value map[string]any
		_ = json.Unmarshal([]byte(raw), &value)
		if checkServicePurgeResult(value) == nil {
			t.Fatalf("false success: %s", raw)
		}
	}
	for _, value := range []map[string]any{
		{"total": 0, "deleted": 0}, {"total": 2, "deleted": 2, "failures": []any{}},
	} {
		if err := checkServicePurgeResult(value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestServiceDeletionFencesRefreshesThenPurgesEvenWithOfflineHost(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("rejects unguarded future indices", func(t *testing.T) {
		for _, body := range []string{
			`{"index_templates":[]}`,
			`{"index_templates":[{"index_template":{"index_patterns":["logs-*"],"template":{"settings":{}}}}]}`,
			`{"index_templates":[{"index_template":{"index_patterns":["logs-prod-*"],"template":{"settings":{"index.final_pipeline":"logs-mapping-guard"}}}}]}`,
		} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			handler := &Handler{}
			err := handler.checkServiceDeletionTemplates(context.Background(), elasticsearchCluster{
				ID: 1, Hosts: server.URL, IndexPrefix: "logs", Timeout: 5,
			})
			server.Close()
			if err == nil {
				t.Fatalf("unguarded rollover must not report safe deletion: %s", body)
			}
		}
	})
	defer pool.Close()
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/_index_template/") {
			_, _ = w.Write([]byte(`{"index_templates":[{"index_template":{"index_patterns":["logs-*"],"template":{"settings":{"index":{"final_pipeline":"logs-mapping-guard"}}}}}]}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/_delete_by_query") {
			if r.URL.Query().Get("wait_for_completion") != "true" || r.URL.Query().Get("refresh") != "true" {
				t.Error("purge must be synchronous and refreshed")
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			raw, _ := json.Marshal(body)
			if !strings.Contains(string(raw), `"business_system"`) {
				t.Error("purge lost ownership filter")
			}
			_, _ = w.Write([]byte(`{"total":3,"deleted":3,"failures":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"acknowledged":true}`))
	}))
	defer server.Close()
	columns := []string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}
	mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows(columns).AddRow(7, "p", "b", "prod", "SVC"))
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster ORDER BY id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM monitor_elasticsearch_cluster WHERE id =").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled",
	}).AddRow(1, server.URL, "", "", false, "", "logs", 5, false))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster WHERE").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM assets_service_deletion ORDER BY").
		WillReturnRows(sqlmock.NewRows(columns).AddRow(7, "p", "b", "prod", "SVC"))
	mock.ExpectCommit()
	mock.ExpectQuery("FROM assets_application_service_deployment").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{
		"host_id", "host_ip", "host_instance_name", "target_id", "config_fingerprint",
		"runtime_status", "service_fingerprints", "service_id", "service_code",
	}).AddRow(11, "127.0.0.1", "offline-host", 1, "", "running", []byte(`{}`), 7, "SVC"))
	secrets, err := assets.NewSecretEncryptor("", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{db: pool, secrets: secrets}
	if err = handler.RevokeServiceLogs(context.Background(), 7); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("offline host must retain retryable service: %v", err)
	}
	want := []string{
		"PUT /_ingest/pipeline/logs-mapping-guard", "PUT /logs-*/_settings",
		"GET /_index_template/logs-*",
		"POST /logs-*/_refresh", "POST /logs-*/_delete_by_query",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unsafe execution order: %v", paths)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
