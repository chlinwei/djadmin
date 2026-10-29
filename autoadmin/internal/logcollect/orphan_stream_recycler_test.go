package logcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOrphanStreamRetentionAndFinalRecheck(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name       string
		count      int64
		age        time.Duration
		managed    bool
		race       bool
		ownerRace  bool
		wantDelete bool
		wantError  bool
	}{
		{name: "empty", wantDelete: true},
		{name: "empty receives writes", race: true, wantError: true},
		{name: "unmanaged expired", count: 15, age: 8 * 24 * time.Hour, wantDelete: true},
		{name: "unmanaged recent", count: 15, age: 6 * 24 * time.Hour},
		{name: "unmanaged future", count: 15, age: -time.Hour},
		{name: "managed original 30 day retention protects", count: 15, age: 8 * 24 * time.Hour, managed: true},
		{name: "managed original retention expired", count: 15, age: 31 * 24 * time.Hour, managed: true, wantDelete: true},
		{name: "ownership changed before delete", ownerRace: true, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			searches, deletes, restored, preparations := 0, 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := `{"acknowledged":true}`
				switch {
				case strings.HasSuffix(r.URL.Path, "/_search"):
					searches++
					var query map[string]any
					if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
						t.Error(err)
					}
					if query["size"] != float64(10) || query["sort"] == nil || query["aggs"] == nil {
						t.Error("orphan search must return latest ten and prove global max")
					}
					count := tt.count
					if tt.race && searches > 1 {
						count++
					}
					body = historicalTestSearch(count, now.Add(-tt.age))
				case strings.HasSuffix(r.URL.Path, "/_ilm/explain"):
					body = `{ "indices": {".ds-logs-p-b-prod-svc-old-000001":{"managed":false}}}`
					if tt.managed {
						body = fmt.Sprintf(`{"indices":{".ds-logs-p-b-prod-svc-old-000001":{"managed":true,"policy":"original","index_creation_date_millis":%d,"lifecycle_date_millis":%d}}}`, now.Add(-40*24*time.Hour).UnixMilli(), now.Add(-40*24*time.Hour).UnixMilli())
					}
				case r.URL.Path == "/_ilm/policy/original":
					body = `{"original":{"policy":{"phases":{"delete":{"min_age":"30d","actions":{"delete":{}}}}}}}`
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/_settings"):
					body = `{".ds-logs-p-b-prod-svc-old-000001":{"settings":{"index":{"blocks":{"write":"false"}}}}}`
				case strings.HasSuffix(r.URL.Path, "/_block/write"):
					body = `{"acknowledged":true,"shards_acknowledged":true}`
				case strings.HasSuffix(r.URL.Path, "/_refresh"):
					body = `{"_shards":{"failed":0,"total":1,"successful":1}}`
				case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/_settings"):
					restored++
				case r.Method == http.MethodDelete:
					if r.URL.Path != "/_data_stream/logs-p-b-prod-svc-old" {
						t.Errorf("unsafe delete %s", r.URL)
					}
					deletes++
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			deleted, _, err := (&Handler{}).deleteExpiredStream(context.Background(), elasticsearchCluster{
				Hosts: server.URL, IndexPrefix: "logs", Timeout: 5,
			}, historicalTestStream(t), now, true, func() error {
				preparations++
				if tt.ownerRace {
					return fmt.Errorf("ownership changed")
				}
				return nil
			})
			if deleted != tt.wantDelete || (err != nil) != tt.wantError {
				t.Fatalf("deleted=%v err=%v", deleted, err)
			}
			if tt.wantDelete && (deletes != 1 || preparations != 1 || restored != 0) {
				t.Fatalf("deletes=%d preparations=%d restores=%d", deletes, preparations, restored)
			}
			if tt.wantError && (deletes != 0 || restored != 1) {
				t.Fatalf("unsafe failure: deletes=%d restores=%d", deletes, restored)
			}
		})
	}
}

func TestOrphanScanProvesGlobalLatestAndRejectsBadEvidence(t *testing.T) {
	for _, test := range []string{"latest in second page", "malformed timestamp", "missing timestamp", "incomplete scan", "query error", "failed shards"} {
		t.Run(test, func(t *testing.T) {
			calls, cleaned := 0, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					cleaned = true
					_, _ = w.Write([]byte(`{"succeeded":true}`))
					return
				}
				calls++
				if calls == 1 {
					if test == "query error" {
						w.WriteHeader(503)
						_, _ = w.Write([]byte(`{"error":"unavailable"}`))
						return
					}
					// Complete aggregation proves some docs need timestamp parsing.
					_, _ = w.Write([]byte(`{"timed_out":false,"_shards":{"failed":0,"total":1,"successful":1},"hits":{"total":{"value":11,"relation":"eq"}},"aggregations":{"missing":{"doc_count":11},"latest":{"value":null}}}`))
					return
				}
				hits := []any{}
				if calls == 2 {
					for i := 0; i < 10; i++ {
						source := map[string]any{"timestamp": "2026-10-01T00:00:00Z"}
						if test == "malformed timestamp" {
							source["timestamp"] = "not-a-time"
						}
						if test == "missing timestamp" {
							source = map[string]any{"message": "no reliable event time"}
						}
						hits = append(hits, map[string]any{"_source": source})
					}
				}
				if calls == 3 && test != "incomplete scan" {
					hits = append(hits, map[string]any{"_source": map[string]any{"time": "2026-10-28T12:00:00+08:00"}})
				}
				failed := 0
				if test == "failed shards" {
					failed = 1
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"_scroll_id": "cursor", "timed_out": false,
					"_shards": map[string]any{"total": 1, "successful": 1 - failed, "failed": failed},
					"hits":    map[string]any{"total": map[string]any{"value": 11, "relation": "eq"}, "hits": hits},
				})
			}))
			defer server.Close()
			snapshot, err := (&Handler{}).orphanStreamSnapshot(context.Background(), elasticsearchCluster{Hosts: server.URL, Timeout: 5}, "logs-orphan")
			if test == "latest in second page" {
				if err != nil || snapshot.Count != 11 || !snapshot.Latest.Equal(time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)) {
					t.Fatalf("snapshot=%+v err=%v", snapshot, err)
				}
			} else if err == nil {
				t.Fatal("unsafe evidence accepted")
			}
			if test == "query error" && calls != 1 {
				t.Fatal("query errors must defer without guessing")
			}
			if test != "query error" && !cleaned {
				t.Fatal("scroll cursor leaked")
			}
		})
	}
}

func TestOrphanOwnershipRejectsKnownSharedAndAmbiguousNames(t *testing.T) {
	owners := []db.AssetsServiceDeletion{
		{ServiceID: 7, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc"},
		{ServiceID: 8, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc-extra"},
	}
	for _, name := range []string{"logs-p-b-prod-svc-hot", "logs-p-b-prod-svc-extra-hot", "logs-p-b-prod-shared-source-99-hot", "logs-p-b-prod-shared-source:99-hot", "logs-*", "other-orphan"} {
		if err := orphanStreamUnowned("logs", name, owners); err == nil {
			t.Fatalf("unsafe orphan %q", name)
		}
	}
	if err := orphanStreamUnowned("logs", "logs-p-b-prod-unknown-hot", owners); err != nil {
		t.Fatal(err)
	}
}

func TestOrphanFencePersistsDeletedStreamsAndReleasesKnownOwners(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprint(malformed), func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectQuery("FROM assets_service_deletion ORDER BY").
				WillReturnRows(sqlmock.NewRows([]string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}))
			mock.ExpectQuery("SELECT DISTINCT p.code AS project_code").
				WillReturnRows(sqlmock.NewRows([]string{"project_code", "environment_code", "business_system_code", "service_id", "service_code", "tier_code", "service_enabled", "log_collection_enabled"}).
					AddRow("p", "prod", "b", 7, "svc", "hot", true, true))
			puts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					body := `{"logs-mapping-guard-orphans":{"_meta":{"streams":["logs-p-b-prod-gone-old","logs-p-b-prod-svc-hot"]}}}`
					if malformed {
						body = `{"logs-mapping-guard-orphans":{}}`
					}
					_, _ = w.Write([]byte(body))
					return
				}
				puts++
				var body struct {
					Meta struct {
						Streams []string `json:"streams"`
					} `json:"_meta"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if strings.Join(body.Meta.Streams, ",") != "logs-p-b-prod-gone-old,logs-p-b-prod-orphan-old" {
					t.Errorf("unexpected fences: %v", body.Meta.Streams)
				}
				_, _ = w.Write([]byte(`{"acknowledged":true}`))
			}))
			defer server.Close()
			err = (&Handler{db: pool}).updateOrphanStreamFence(context.Background(), db.New(pool),
				elasticsearchCluster{Hosts: server.URL, IndexPrefix: "logs", Timeout: 5}, "logs-p-b-prod-orphan-old", "")
			if (err != nil) != malformed || puts != map[bool]int{false: 1, true: 0}[malformed] {
				t.Fatalf("puts=%d err=%v", puts, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecycleOrphanStreamRechecksOwnershipUnderClusterLock(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/_ingest/pipeline/logs-mapping-guard" {
			t.Errorf("owned stream must not be touched: %s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"acknowledged":true}`))
	}))
	defer server.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster WHERE").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM assets_service_deletion ORDER BY").
		WillReturnRows(sqlmock.NewRows([]string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}))
	mock.ExpectQuery("FROM monitor_shared_log_source_revocation").
		WillReturnRows(sqlmock.NewRows([]string{"source_id"}))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster WHERE").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM monitor_elasticsearch_cluster WHERE id =").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled",
	}).AddRow(1, server.URL, "", "", false, "", "logs", 5, true))
	mock.ExpectQuery("FROM assets_service_deletion ORDER BY").
		WillReturnRows(sqlmock.NewRows([]string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}))
	mock.ExpectQuery("SELECT DISTINCT p.code AS project_code").
		WillReturnRows(sqlmock.NewRows([]string{"project_code", "environment_code", "business_system_code", "service_id", "service_code", "tier_code", "service_enabled", "log_collection_enabled"}).
			AddRow("p", "prod", "b", 7, "svc", "old", true, true))
	mock.ExpectRollback()
	err = (&Handler{db: pool}).recycleOrphanStream(context.Background(), elasticsearchCluster{
		ID: 1, Hosts: server.URL, IndexPrefix: "logs", Timeout: 5,
	}, historicalTestStream(t))
	if err == nil || !strings.Contains(err.Error(), "owned") {
		t.Fatalf("ownership race not rejected: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
