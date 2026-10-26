package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHistoricalStreamOwner(t *testing.T) {
	owner := db.AssetsServiceDeletion{ServiceID: 7, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc"}
	other := owner
	other.ServiceID, other.ServiceCode = 8, "svc-extra"
	for _, tt := range []struct {
		name       string
		fail       bool
		wantReason string
	}{
		{name: "logs-p-b-prod-svc-old-tier"},
		{name: "logs-p-b-prod-svc-extra-hot", fail: true},
		{name: "logs-p-b-prod-svc-", fail: true, wantReason: "missing retention tier"},
		{name: "logs-p-b-prod-hot", fail: true, wantReason: "missing service ownership"},
		{name: "logs-other-b-prod-svc-hot", fail: true, wantReason: "missing service ownership"},
		{name: "logs-p-b-prod-svc-*", fail: true},
		{name: "logs-p-b-prod-svc-hot,other", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			found, tier, err := historicalStreamOwner("logs", tt.name, []db.AssetsServiceDeletion{owner, owner, other})
			if (err != nil) != tt.fail {
				t.Fatalf("owner=%v tier=%q err=%v", found, tier, err)
			}
			if tt.wantReason != "" && !strings.Contains(err.Error(), tt.wantReason) {
				t.Fatalf("error %q should explain %q", err, tt.wantReason)
			}
			if !tt.fail && (found.ServiceID != 7 || tier != "old-tier") {
				t.Fatalf("unexpected ownership: %v %q", found, tier)
			}
		})
	}
}

func TestHistoricalRetentionAge(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"7d": 7 * 24 * time.Hour, "12h": 12 * time.Hour, "30m": 30 * time.Minute,
		"1s": time.Second, "500ms": 500 * time.Millisecond,
	} {
		got, err := parseHistoricalRetentionAge(value)
		if err != nil || got != want {
			t.Fatalf("%s = %v, %v; want %v", value, got, err, want)
		}
	}
	for _, value := range []string{"", "0d", "-1d", "7", "7w", "1.5d", "999999999999999999999d", "999999999d"} {
		if _, err := parseHistoricalRetentionAge(value); err == nil {
			t.Fatalf("accepted invalid retention %q", value)
		}
	}
}

func TestHistoricalStreamHostSafety(t *testing.T) {
	now := time.Now()
	ready := serviceApplyTarget{HostID: 1, HostInstanceName: "host", AgentOnline: true, ConfigState: LogConfigSynced, ConfigFingerprint: "host-config"}
	evidence := db.ListHistoricalStreamHostsRow{
		HostID: 1, InstanceName: sql.NullString{String: "host", Valid: true},
		ManagedEnabled: true, ConfigFingerprint: "host-config", ServiceFingerprints: json.RawMessage(`{"7":"new-config"}`),
		LastAppliedTime: sql.NullTime{Time: now.Add(-time.Minute), Valid: true},
	}
	for _, tt := range []struct {
		name   string
		change func(*serviceApplyTarget, *db.ListHistoricalStreamHostsRow)
	}{
		{"offline", func(h *serviceApplyTarget, _ *db.ListHistoricalStreamHostsRow) { h.AgentOnline = false }},
		{"drift", func(h *serviceApplyTarget, _ *db.ListHistoricalStreamHostsRow) { h.ConfigState = LogConfigDrift }},
		{"never", func(h *serviceApplyTarget, _ *db.ListHistoricalStreamHostsRow) { h.ConfigState = LogConfigNever }},
		{"unknown", func(h *serviceApplyTarget, _ *db.ListHistoricalStreamHostsRow) { h.ConfigState = LogConfigUnknown }},
		{"unmanaged", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) { r.ManagedEnabled = false }},
		{"missing apply time", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) { r.LastAppliedTime.Valid = false }},
		{"reload in progress", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) {
			r.LastAppliedTime.Time = now.Add(-59 * time.Second)
		}},
		{"future apply time", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) {
			r.LastAppliedTime.Time = now.Add(time.Hour)
		}},
		{"host renamed", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) { r.InstanceName.String = "other" }},
		{"config changed during check", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) { r.ConfigFingerprint = "changed" }},
		{"config evidence missing", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) { r.ConfigFingerprint = "" }},
		{"invalid fingerprints", func(_ *serviceApplyTarget, r *db.ListHistoricalStreamHostsRow) {
			r.ServiceFingerprints = json.RawMessage(`bad`)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			host, row := ready, evidence
			tt.change(&host, &row)
			if err := checkHistoricalStreamHosts(now, 7, []serviceApplyTarget{host}, nil, []db.ListHistoricalStreamHostsRow{row}); err == nil {
				t.Fatal("unsafe host accepted")
			}
		})
	}
	if err := checkHistoricalStreamHosts(now, 7, []serviceApplyTarget{ready}, nil, []db.ListHistoricalStreamHostsRow{evidence}); err != nil {
		t.Fatal(err)
	}
	for _, rows := range [][]db.ListHistoricalStreamHostsRow{
		nil,
		{evidence, {HostID: 2, ServiceFingerprints: json.RawMessage(`{"7":"stale"}`)}},
	} {
		if err := checkHistoricalStreamHosts(now, 7, []serviceApplyTarget{ready}, nil, rows); err == nil {
			t.Fatal("missing or formerly associated host was ignored")
		}
	}
	if err := checkHistoricalStreamHosts(now, 7, nil, nil, nil); err == nil {
		t.Fatal("hostless service accepted")
	}
	if err := checkHistoricalStreamHosts(now, 7, []serviceApplyTarget{ready}, []serviceApplyTarget{{HostID: 2}}, []db.ListHistoricalStreamHostsRow{evidence}); err == nil {
		t.Fatal("unmanaged service host accepted")
	}
}

func historicalTestStream(t *testing.T) historicalDataStream {
	t.Helper()
	var stream historicalDataStream
	if err := json.Unmarshal([]byte(`{"name":"logs-p-b-prod-svc-old","indices":[{"index_name":".ds-logs-p-b-prod-svc-old-000001"}]}`), &stream); err != nil {
		t.Fatal(err)
	}
	return stream
}

func historicalTestSearch(count int64, latest time.Time) string {
	value := "null"
	if count > 0 {
		value = fmt.Sprint(latest.UnixMilli())
	}
	return fmt.Sprintf(`{"timed_out":false,"_shards":{"failed":0,"total":1,"successful":1},"hits":{"total":{"value":%d,"relation":"eq"}},"aggregations":{"latest":{"value":%s},"missing":{"doc_count":0}}}`, count, value)
}

func TestHistoricalStreamDeletionSafety(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	old := now.Add(-8 * 24 * time.Hour)
	for _, tt := range []struct {
		name         string
		count        int64
		latest       time.Time
		search       string
		finalSearch  string
		explain      string
		policy       string
		block        string
		blockStatus  int
		settings     string
		refresh      string
		deleteBody   string
		deleteStatus int
		restoreBody  string
		wantDelete   bool
		wantError    bool
		wantBlocked  bool
		wantRestore  bool
	}{
		{name: "empty old stream", wantDelete: true, wantBlocked: true},
		{name: "nonempty expired exactly at deadline", count: 3, latest: now.Add(-7 * 24 * time.Hour), wantDelete: true, wantBlocked: true},
		{name: "not yet expired by one millisecond", count: 3, latest: now.Add(-7*24*time.Hour + time.Millisecond)},
		{name: "latest log protects recently written old index", count: 3, latest: now.Add(-time.Hour)},
		{name: "future timestamp retained", count: 3, latest: now.Add(time.Hour)},
		{name: "hour-based original retention", count: 3, latest: now.Add(-12 * time.Hour), policy: `{"old-policy":{"policy":{"phases":{"delete":{"min_age":"12h","actions":{"delete":{}}}}}}}`, wantDelete: true, wantBlocked: true},
		{name: "recent rollover protects older logs", count: 3, latest: old,
			explain: fmt.Sprintf(`{"indices":{".ds-logs-p-b-prod-svc-old-000001":{"managed":true,"policy":"old-policy","index_creation_date_millis":%d,"lifecycle_date_millis":%d}}}`, old.UnixMilli(), now.Add(-24*time.Hour).UnixMilli())},
		{name: "unmanaged nonempty stream", count: 3, latest: old, explain: `{"indices":{".ds-logs-p-b-prod-svc-old-000001":{"managed":false}}}`, wantError: true},
		{name: "unknown ILM response", count: 3, latest: old, explain: `{}`, wantError: true},
		{name: "missing deletion policy", count: 3, latest: old, policy: `{}`, wantError: true},
		{name: "no delete action", count: 3, latest: old, policy: `{"old-policy":{"policy":{"phases":{"delete":{"min_age":"7d","actions":{}}}}}}`, wantError: true},
		{name: "partial search rejected", search: `{"timed_out":false,"_shards":{"failed":1}}`, wantError: true},
		{name: "timed out search rejected", search: `{"timed_out":true,"_shards":{"failed":0}}`, wantError: true},
		{name: "incomplete response rejected", search: `{}`, wantError: true},
		{name: "missing timestamp rejected", search: `{"timed_out":false,"_shards":{"failed":0,"total":1,"successful":1},"hits":{"total":{"value":3,"relation":"eq"}},"aggregations":{"latest":{"value":1},"missing":{"doc_count":1}}}`, wantError: true},
		{name: "approximate count rejected", search: `{"timed_out":false,"_shards":{"failed":0,"total":1,"successful":1},"hits":{"total":{"value":10000,"relation":"gte"}},"aggregations":{"latest":{"value":1},"missing":{"doc_count":0}}}`, wantError: true},
		{name: "preexisting write block preserved", settings: `{".ds-index":{"settings":{"index":{"blocks":{"write":"true"}}}}}`, wantError: true},
		{name: "block must be acknowledged", block: `{"acknowledged":false}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "shard block must be acknowledged", block: `{"acknowledged":true,"shards_acknowledged":false}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "refresh failure", refresh: `{"_shards":{"failed":1}}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "unrefreshed writes prevent deletion", finalSearch: historicalTestSearch(1, now), wantError: true, wantBlocked: true, wantRestore: true},
		{name: "delete acknowledgement required", deleteBody: `{}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "delete failure restores writes", deleteStatus: 403, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "restore failure is explicit", deleteStatus: 403, restoreBody: `{"acknowledged":false}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "concurrently deleted stream", deleteStatus: 404, wantDelete: true, wantBlocked: true},
		{name: "stream disappeared before write block", blockStatus: 404, wantDelete: true, wantBlocked: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			searchCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				body := `{"acknowledged":true}`
				switch {
				case strings.HasSuffix(r.URL.Path, "/_search"):
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request["track_total_hits"] != true || request["size"] != float64(0) {
						t.Errorf("search must obtain an exact count: %v, %v", request, err)
					}
					body = historicalTestSearch(tt.count, tt.latest)
					if tt.search != "" {
						body = tt.search
					}
					if searchCalls > 0 && tt.finalSearch != "" {
						body = tt.finalSearch
					}
					searchCalls++
				case strings.HasSuffix(r.URL.Path, "/_ilm/explain"):
					body = fmt.Sprintf(`{"indices":{".ds-logs-p-b-prod-svc-old-000001":{"managed":true,"policy":"old-policy","index_creation_date_millis":%d,"lifecycle_date_millis":%d,"step":"check-rollover-ready"}}}`, old.UnixMilli(), old.UnixMilli())
					if tt.explain != "" {
						body = tt.explain
					}
				case strings.HasPrefix(r.URL.Path, "/_ilm/policy/"):
					body = `{"old-policy":{"policy":{"phases":{"delete":{"min_age":"7d","actions":{"delete":{}}}}}}}`
					if tt.policy != "" {
						body = tt.policy
					}
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/_settings"):
					body = `{}`
					if tt.settings != "" {
						body = tt.settings
					}
				case strings.HasSuffix(r.URL.Path, "/_block/write"):
					body = `{"acknowledged":true,"shards_acknowledged":true}`
					if tt.block != "" {
						body = tt.block
					}
					if tt.blockStatus != 0 {
						w.WriteHeader(tt.blockStatus)
						body = `{"error":{"type":"index_not_found_exception","reason":"no such index [logs-p-b-prod-svc-old]"},"status":404}`
					}
				case strings.HasSuffix(r.URL.Path, "/_refresh"):
					body = `{"_shards":{"failed":0,"total":1,"successful":1}}`
					if tt.refresh != "" {
						body = tt.refresh
					}
				case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/_settings"):
					if tt.restoreBody != "" {
						body = tt.restoreBody
					}
				case r.Method == http.MethodDelete && r.URL.Path == "/_data_stream/logs-p-b-prod-svc-old":
					if tt.deleteBody != "" {
						body = tt.deleteBody
					}
					if tt.deleteStatus != 0 {
						w.WriteHeader(tt.deleteStatus)
					}
				default:
					t.Errorf("unexpected ES request %s %s", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			handler := &Handler{}
			deleted, _, err := handler.deleteExpiredHistoricalStream(context.Background(), elasticsearchCluster{
				Hosts: server.URL, IndexPrefix: "logs", Timeout: 5,
			}, historicalTestStream(t), now)
			if deleted != tt.wantDelete || (err != nil) != tt.wantError {
				t.Fatalf("deleted=%v err=%v; want deleted=%v error=%v", deleted, err, tt.wantDelete, tt.wantError)
			}
			blocked, restored := false, false
			for _, call := range calls {
				blocked = blocked || strings.HasSuffix(call, "/_block/write")
				restored = restored || call == "PUT /logs-p-b-prod-svc-old/_settings"
				if strings.Contains(call, "_delete_by_query") || strings.Contains(call, "DELETE") && strings.ContainsAny(call, "*,") {
					t.Fatalf("unsafe deletion %q", call)
				}
			}
			if blocked != tt.wantBlocked || restored != tt.wantRestore {
				t.Fatalf("calls=%v; want blocked=%v restored=%v", calls, tt.wantBlocked, tt.wantRestore)
			}
			if tt.wantDelete && tt.deleteStatus != 404 && tt.blockStatus != 404 {
				tail := []string{
					"PUT /logs-p-b-prod-svc-old/_block/write",
					"POST /logs-p-b-prod-svc-old/_refresh",
					"POST /logs-p-b-prod-svc-old/_search",
					"DELETE /_data_stream/logs-p-b-prod-svc-old",
				}
				if !reflect.DeepEqual(calls[len(calls)-4:], tail) {
					t.Fatalf("unsafe delete order: %v", calls)
				}
			}
		})
	}
}

func TestIsElasticsearchIndexNotFound(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "missing index", err: fmt.Errorf(`elasticsearch 404 Not Found: {"error":{"type":"index_not_found_exception"}}`), want: true},
		{name: "missing policy", err: fmt.Errorf(`elasticsearch 404 Not Found: {"error":{"type":"resource_not_found_exception"}}`)},
		{name: "other status", err: fmt.Errorf(`elasticsearch 403 Forbidden: {"error":{"type":"index_not_found_exception"}}`)},
		{name: "nil"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isElasticsearchIndexNotFound(test.err); got != test.want {
				t.Fatalf("isElasticsearchIndexNotFound(%v)=%v, want %v", test.err, got, test.want)
			}
		})
	}
}

func TestHistoricalStreamRechecksCurrentTierUnderLocks(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("current stream must never be touched: %s %s", r.Method, r.URL)
	}))
	defer server.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster WHERE").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM monitor_elasticsearch_cluster WHERE id =").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled",
	}).AddRow(1, server.URL, "", "", false, "", "logs", 5, true))
	mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
		WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT s.id").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT ld.id").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
	mock.ExpectQuery("SELECT id FROM assets_application_service_log_setting").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
	mock.ExpectQuery("SELECT id FROM monitor_log_retention_tier").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
	mock.ExpectQuery("SELECT d.id").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(13))
	mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
		WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM assets_service_deletion ORDER BY").
		WillReturnRows(sqlmock.NewRows([]string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}))
	mock.ExpectQuery("SELECT DISTINCT p.code AS project_code").
		WillReturnRows(sqlmock.NewRows([]string{
			"project_code", "environment_code", "business_system_code", "service_id", "service_code",
			"tier_code", "service_enabled", "log_collection_enabled",
		}).AddRow("p", "prod", "b", 7, "svc", "old", true, true))
	mock.ExpectQuery("SELECT DISTINCT s.id AS service_id").
		WillReturnRows(sqlmock.NewRows([]string{"service_id", "service_code", "tier_code"}).AddRow(7, "svc", "old"))
	mock.ExpectRollback()
	handler := &Handler{db: pool}
	err = handler.recycleHistoricalStream(context.Background(), elasticsearchCluster{ID: 1},
		historicalTestStream(t), db.AssetsServiceDeletion{
			ServiceID: 7, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc",
		}, "old")
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalStreamRecyclerSkipsServiceDeletionTombstone(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster WHERE").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM monitor_elasticsearch_cluster WHERE id =").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled",
	}).AddRow(1, "http://unused", "", "", false, "", "logs", 5, true))
	mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
		WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{
		"service_id", "project_code", "business_system_code", "environment_code", "service_code",
	}).AddRow(7, "p", "b", "prod", "svc"))
	mock.ExpectRollback()

	handler := &Handler{db: pool}
	err = handler.recycleHistoricalStream(context.Background(), elasticsearchCluster{ID: 1},
		historicalTestStream(t), db.AssetsServiceDeletion{
			ServiceID: 7, ProjectCode: "p", BusinessSystemCode: "b", EnvironmentCode: "prod", ServiceCode: "svc",
		}, "old")
	if err != nil {
		t.Fatalf("service deletion workflow owns tombstoned service streams: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
