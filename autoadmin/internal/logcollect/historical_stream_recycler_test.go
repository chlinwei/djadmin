package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHistoricalStreamRecyclerStartupRecheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks, retry := make(chan time.Time), make(chan time.Time, 2)
	calls := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		runHistoricalStreamRecycler(ctx, func(sweep context.Context) error {
			if _, ok := sweep.Deadline(); !ok {
				t.Error("sweep must have a bounded deadline")
			}
			calls <- struct{}{}
			return nil
		}, ticks, retry)
	}()
	await := func() {
		t.Helper()
		select {
		case <-calls:
		case <-time.After(time.Second):
			t.Fatal("collector was not invoked")
		}
	}
	await()
	retry <- time.Now()
	await()
	// 一次性的启动补查不能变成高频扫描；之后仍只响应正常周期。
	retry <- time.Now()
	ticks <- time.Now()
	await()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collector did not stop after cancellation")
	}
}

func TestHistoricalStreamRecyclerCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runHistoricalStreamRecycler(ctx, func(context.Context) error {
		t.Fatal("must not collect after shutdown")
		return nil
	}, nil, nil)
}

func TestHistoricalStreamWaitsForInFlightWrites(t *testing.T) {
	for _, tt := range []struct {
		name       string
		list       string
		listStatus int
		state      string
		stateCode  int
		wantWait   bool
		wantError  bool
	}{
		{name: "no writes", list: `{"tasks":[]}`},
		{name: "wait for existing bulk", list: `{"tasks":[{"node":"node","id":7}]}`, state: `{"completed":true}`, wantWait: true},
		{name: "completed task removed", list: `{"tasks":[{"node":"node","id":7}]}`, stateCode: 404, wantWait: true},
		{name: "write still running", list: `{"tasks":[{"node":"node","id":7}]}`, state: `{"completed":false}`, wantWait: true, wantError: true},
		{name: "failed task lookup", list: `{"tasks":[{"node":"node","id":7}]}`, stateCode: 503, wantWait: true, wantError: true},
		{name: "missing task list", list: `{}`, wantError: true},
		{name: "partial node evidence", list: `{"tasks":[],"node_failures":[{}]}`, wantError: true},
		{name: "partial task evidence", list: `{"tasks":[],"task_failures":[{}]}`, wantError: true},
		{name: "unknown task node", list: `{"tasks":[{"id":7}]}`, wantError: true},
		{name: "negative task id", list: `{"tasks":[{"node":"node","id":-1}]}`, wantError: true},
		{name: "task monitoring forbidden", listStatus: 403, list: `{}`, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			waited := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("task observation must be GET: %s", r.Method)
				}
				if r.URL.Path == "/_tasks" {
					if r.URL.Query().Get("group_by") != "none" || r.URL.Query().Get("actions") != "indices:data/write/*" {
						t.Error("must capture the complete flat write task list")
					}
					if tt.listStatus != 0 {
						w.WriteHeader(tt.listStatus)
					}
					fmt.Fprint(w, tt.list)
					return
				}
				if r.URL.Path != "/_tasks/node:7" || r.URL.Query().Get("wait_for_completion") != "true" || r.URL.Query().Get("timeout") != "5s" {
					t.Errorf("must wait for the captured write task: %s", r.URL)
				}
				waited = true
				if tt.stateCode != 0 {
					w.WriteHeader(tt.stateCode)
				}
				fmt.Fprint(w, tt.state)
			}))
			defer server.Close()
			err := (&Handler{}).waitForHistoricalStreamWrites(context.Background(), elasticsearchCluster{Hosts: server.URL, Timeout: 5})
			if (err != nil) != tt.wantError || waited != tt.wantWait {
				t.Fatalf("error=%v waited=%t; want error=%t waited=%t", err, waited, tt.wantError, tt.wantWait)
			}
		})
	}
}

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
		stillExists  bool
		rollover     bool
		badConfirm   bool
		multiBacking bool
		wantDelete   bool
		wantError    bool
		wantBlocked  bool
		wantRestore  bool
	}{
		{name: "empty old stream", wantDelete: true, wantBlocked: true},
		{name: "all backing indices blocked explicitly", multiBacking: true, wantDelete: true, wantBlocked: true},
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
		{name: "refresh failure", refresh: `{"_shards":{"failed":1}}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "unrefreshed writes prevent deletion", finalSearch: historicalTestSearch(1, now), wantError: true, wantBlocked: true, wantRestore: true},
		{name: "delete acknowledgement required", deleteBody: `{}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "delete failure restores writes", deleteStatus: 403, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "restore failure is explicit", deleteStatus: 403, restoreBody: `{"acknowledged":false}`, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "concurrently deleted stream", deleteStatus: 404, wantDelete: true, wantBlocked: true},
		{name: "index 404 must not masquerade as deleted stream", blockStatus: 404, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "delete acknowledgement cannot hide surviving stream", stillExists: true, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "rollover after blocking prevents deletion", rollover: true, wantError: true, wantBlocked: true, wantRestore: true},
		{name: "missing deletion evidence is not success", badConfirm: true, wantError: true, wantBlocked: true, wantRestore: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			searchCalls := 0
			deleteCalls := 0
			stream := historicalTestStream(t)
			backingPath := "/" + stream.Indices[0].Name
			if tt.multiBacking {
				extra := stream.Indices[0]
				extra.Name = ".ds-logs-p-b-prod-svc-old-000002"
				stream.Indices = append(stream.Indices, extra)
				backingPath += "," + extra.Name
			}
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
				case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/_settings"):
					if r.URL.Path != backingPath+"/_settings" {
						t.Errorf("write settings must target concrete backing index: %s", r.URL.Path)
					}
					var settings map[string]bool
					if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
						t.Error(err)
					}
					block := settings["index.blocks.write"]
					calls[len(calls)-1] += fmt.Sprintf(" write=%t", block)
					if !block {
						if tt.restoreBody != "" {
							body = tt.restoreBody
						}
						break
					}
					if tt.block != "" {
						body = tt.block
					}
					if tt.blockStatus != 0 {
						w.WriteHeader(tt.blockStatus)
						body = `{"error":{"type":"index_not_found_exception","reason":"no such index [logs-p-b-prod-svc-old]"},"status":404}`
					}
				case r.URL.Path == "/_tasks":
					if r.Method != http.MethodGet || r.URL.Query().Get("actions") != "indices:data/write/*" {
						t.Error("must observe pending writes after the settings barrier")
					}
					body = `{"tasks":[]}`
				case strings.HasSuffix(r.URL.Path, "/_refresh"):
					body = `{"_shards":{"failed":0,"total":1,"successful":1}}`
					if tt.refresh != "" {
						body = tt.refresh
					}
				case r.Method == http.MethodDelete && r.URL.Path == "/_data_stream/logs-p-b-prod-svc-old":
					deleteCalls++
					if tt.deleteBody != "" {
						body = tt.deleteBody
					}
					if tt.deleteStatus != 0 {
						w.WriteHeader(tt.deleteStatus)
					}
				case r.Method == http.MethodGet && r.URL.Path == "/_data_stream/logs-p-b-prod-svc-old":
					if deleteCalls > 0 && !tt.stillExists {
						if tt.badConfirm {
							body = `{}`
						} else {
							w.WriteHeader(http.StatusNotFound)
							body = `{"error":{"type":"resource_not_found_exception"},"status":404}`
						}
						break
					}
					current := stream
					current.Indices = slices.Clone(stream.Indices)
					if tt.rollover {
						current.Indices[0].Name += "-changed"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data_streams": []historicalDataStream{current}})
					return
				default:
					t.Errorf("unexpected ES request %s %s", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			handler := &Handler{}
			deleted, _, err := handler.deleteExpiredHistoricalStream(context.Background(), elasticsearchCluster{
				Hosts: server.URL, IndexPrefix: "logs", Timeout: 5,
			}, stream, now)
			if deleted != tt.wantDelete || (err != nil) != tt.wantError {
				t.Fatalf("deleted=%v err=%v; want deleted=%v error=%v", deleted, err, tt.wantDelete, tt.wantError)
			}
			blocked, restored := false, false
			for _, call := range calls {
				blocked = blocked || strings.HasSuffix(call, " write=true")
				restored = restored || strings.HasSuffix(call, " write=false")
				if strings.Contains(call, "_delete_by_query") || strings.Contains(call, "DELETE") && strings.ContainsAny(call, "*,") {
					t.Fatalf("unsafe deletion %q", call)
				}
			}
			if blocked != tt.wantBlocked || restored != tt.wantRestore {
				t.Fatalf("calls=%v; want blocked=%v restored=%v", calls, tt.wantBlocked, tt.wantRestore)
			}
			if tt.wantDelete && tt.deleteStatus != 404 && tt.blockStatus != 404 {
				tail := []string{
					"PUT " + backingPath + "/_settings write=true",
					"GET /_tasks",
					"POST /logs-p-b-prod-svc-old/_refresh",
					"POST /logs-p-b-prod-svc-old/_search",
					"GET /_data_stream/logs-p-b-prod-svc-old",
					"DELETE /_data_stream/logs-p-b-prod-svc-old",
					"GET /_data_stream/logs-p-b-prod-svc-old",
				}
				if !reflect.DeepEqual(calls[len(calls)-len(tail):], tail) {
					t.Fatalf("unsafe delete order: %v", calls)
				}
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
