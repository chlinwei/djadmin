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

func TestServiceDeletionGuardUsesAllOwnershipDimensions(t *testing.T) {
	for _, field := range []string{"project", "business_system", "environment", "service"} {
		if !strings.Contains(serviceDeletionCondition, "ctx."+field+" == owner."+field) {
			t.Fatalf("guard must compare %s without interpolating user input", field)
		}
	}
}

func TestServiceDeletionFencesThenDeletesDataStreamsEvenWithOfflineHost(t *testing.T) {
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
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/_data_stream/") {
			_, _ = w.Write([]byte(`{"data_streams":[{"name":"logs-p-b-prod-SVC-hot"},{"name":"logs-p-b-prod-SVC-retired-tier"}]}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/_ingest/pipeline/") {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			raw, _ := json.Marshal(body)
			if !strings.Contains(string(raw), `"business_system"`) {
				t.Error("write barrier lost ownership filter")
			}
		}
		_, _ = w.Write([]byte(`{"acknowledged":true}`))
	}))
	defer server.Close()
	columns := []string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}
	mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows(columns).AddRow(7, "p", "b", "prod", "SVC"))
	mock.ExpectQuery("SELECT DISTINCT p.code AS project_code").
		WillReturnRows(sqlmock.NewRows([]string{
			"project_code", "environment_code", "business_system_code", "service_id", "service_code",
			"tier_code", "service_enabled", "log_collection_enabled",
		}).AddRow("p", "prod", "b", 7, "SVC", "hot", false, false))
	mock.ExpectQuery("FROM assets_service_deletion ORDER BY").
		WillReturnRows(sqlmock.NewRows(columns).AddRow(7, "p", "b", "prod", "SVC"))
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
		"GET /_data_stream/logs-p-b-prod-SVC-*",
		"DELETE /_data_stream/logs-p-b-prod-SVC-hot",
		"DELETE /_data_stream/logs-p-b-prod-SVC-retired-tier",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unsafe execution order: %v", paths)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteServiceDataStreams(t *testing.T) {
	scope := db.AssetsServiceDeletion{
		ServiceID: 7, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc",
	}
	for _, tt := range []struct {
		name         string
		listStatus   int
		listBody     string
		deleteStatus int
		deleteBody   string
		owners       []db.AssetsServiceDeletion
		wantError    bool
		wantDeletes  int
	}{
		{name: "all tiers including empty and removed tiers", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-hot"},{"name":"logs-p-b-prod-svc-old-tier"}]}`, wantDeletes: 2},
		{name: "empty list", listBody: `{"data_streams":[]}`},
		{name: "absent streams", listStatus: 404},
		{name: "list failure", listStatus: 403, wantError: true},
		{name: "incomplete response", listBody: `{}`, wantError: true},
		{name: "null response", listBody: `{"data_streams":null}`, wantError: true},
		{name: "malformed response", listBody: `{"data_streams":"invalid"}`, wantError: true},
		{name: "missing name", listBody: `{"data_streams":[{}]}`, wantError: true},
		{name: "foreign service", listBody: `{"data_streams":[{"name":"logs-p-b-prod-other-hot"}]}`, wantError: true},
		{name: "same service code in another project", listBody: `{"data_streams":[{"name":"logs-other-b-prod-svc-hot"}]}`, wantError: true},
		{name: "wildcard name", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-*"}]}`, wantError: true},
		{name: "missing tier", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-"}]}`, wantError: true},
		{name: "validate before any delete", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-hot"},{"name":"logs-p-b-prod-other-hot"}]}`, wantError: true},
		{
			name: "overlapping service code", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-extra-hot"}]}`, wantError: true,
			owners: []db.AssetsServiceDeletion{{ServiceID: 8, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc-extra"}},
		},
		{
			name: "same identity claimed by another service", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-hot"}]}`, wantError: true,
			owners: []db.AssetsServiceDeletion{{ServiceID: 8, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc"}},
		},
		{name: "delete failure", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-hot"}]}`, deleteStatus: 500, wantError: true, wantDeletes: 1},
		{name: "unacknowledged delete", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-hot"}]}`, deleteBody: `{"acknowledged":false}`, wantError: true, wantDeletes: 1},
		{name: "incomplete delete", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-hot"}]}`, deleteBody: `{}`, wantError: true, wantDeletes: 1},
		{name: "concurrent deletion is idempotent", listBody: `{"data_streams":[{"name":"logs-p-b-prod-svc-hot"}]}`, deleteStatus: 404, wantDeletes: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deletes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if r.URL.Path != "/_data_stream/logs-p-b-prod-svc-*" || r.URL.Query().Get("expand_wildcards") != "all" {
						t.Errorf("unexpected discovery request: %s", r.URL)
					}
					if tt.listStatus != 0 {
						w.WriteHeader(tt.listStatus)
					}
					_, _ = w.Write([]byte(tt.listBody))
					return
				}
				if r.Method != http.MethodDelete || strings.ContainsAny(r.URL.Path, "*,?") || !strings.HasPrefix(r.URL.Path, "/_data_stream/logs-p-b-prod-svc-") {
					t.Errorf("not an exact data stream deletion: %s %s", r.Method, r.URL)
				}
				deletes++
				if tt.deleteStatus != 0 {
					w.WriteHeader(tt.deleteStatus)
				}
				body := tt.deleteBody
				if body == "" {
					body = `{"acknowledged":true}`
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			handler := &Handler{}
			err := handler.deleteServiceDataStreams(context.Background(), elasticsearchCluster{
				ID: 1, Hosts: server.URL, IndexPrefix: "logs", Timeout: 5,
			}, scope, tt.owners)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tt.wantError)
			}
			if deletes != tt.wantDeletes {
				t.Fatalf("deletes = %d, want %d", deletes, tt.wantDeletes)
			}
		})
	}
}
