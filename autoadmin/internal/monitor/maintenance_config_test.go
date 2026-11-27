package monitor

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

const maintenanceLabel = `[{"type":"label","label":"team","operator":"=","value":"ops"}]`

func TestMaintenanceScopes(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `{}`, `[{"type":"label","label":"env","operator":"!=","value":"prod"}]`, `[{"type":"host","id":-1}]`, `[{"type":"label","label":"a","operator":"=~","value":"("}]`, `[{"type":"host","id":2,"hosts":[1,2]}]`, `[{"type":"label","label":"env","operator":"=~","value":".*"}]`} {
		if _, err := maintenanceMatchers([]byte(raw)); err == nil {
			t.Errorf("unsafe scope accepted: %s", raw)
		}
	}
	ms, err := maintenanceMatchers([]byte(`[{"type":"host","id":2},{"type":"host","id":3},{"type":"tree","node_type":"project","id":7},{"type":"tree","node_type":"project","id":9},{"type":"label","label":"team","operator":"=","value":"ops"}]`))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("literal contains", func(t *testing.T) {
		ms, err := maintenanceMatchers([]byte(`[{"type":"label","label":"team","operator":"contains","value":"ops.*"}]`))
		if err != nil {
			t.Fatal(err)
		}
		for value, want := range map[string]bool{"my-ops.*-team": true, "my-ops-team": false, "OPS.*": false, "": false} {
			ok, err := (&Handler{}).maintenanceScopeMatch(t.Context(), ms, &maintenanceAlertFacts{labels: map[string]string{"team": value}})
			if err != nil || ok != want {
				t.Errorf("team=%q: got %v, %v", value, ok, err)
			}
		}
		for _, op := range []string{"!=", "!~", "=~"} {
			if _, err := maintenanceMatchers([]byte(`[{"type":"host","id":3},{"type":"label","label":"team","operator":"` + op + `","value":"ops"}]`)); err == nil {
				t.Errorf("unsupported operator accepted: %s", op)
			}
		}
		if _, err := maintenanceMatchers([]byte(`[{"type":"label","label":"team","operator":"contains","value":" "}]`)); err == nil {
			t.Error("empty substring accepted")
		}
	})
	h := &Handler{}
	for _, tc := range []struct {
		host          int64
		project, team string
		want          bool
	}{{3, "project:9", "ops", true}, {1, "project:9", "ops", true}, {3, "project:2", "ops", true}, {1, "project:2", "ops", false}, {3, "project:2", "dev", false}, {0, "", "ops", false}} {
		ok, err := h.maintenanceScopeMatch(t.Context(), ms, &maintenanceAlertFacts{hostID: tc.host, nodes: map[string]bool{tc.project: true}, labels: map[string]string{"team": tc.team}, assetsLoaded: true})
		if err != nil || ok != tc.want {
			t.Errorf("%+v: %v, %v", tc, ok, err)
		}
	}
}

func maintenanceRows(id int64, status, matchers string, start, end time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "title", "reason", "starts_at", "ends_at", "matchers", "status", "created_by", "updated_by", "created_at", "updated_at"}).AddRow(id, "patch", "upgrade", start, end, []byte(matchers), status, "alice", "alice", start, start)
}

func TestMaintenanceTreeBranches(t *testing.T) {
	if _, err := maintenanceMatchers([]byte(`[{"type":"tree","node_type":"environment","id":9}]`)); err == nil {
		t.Fatal("unscoped environment accepted")
	}
	ms, err := maintenanceMatchers([]byte(`[{"type":"tree","node_type":"project","id":1},{"type":"tree","node_type":"environment","id":9,"business_id":3}]`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		project, business, env int64
		want                   bool
	}{{1, 2, 8, true}, {2, 3, 9, true}, {2, 4, 9, false}, {2, 3, 8, false}} {
		pool, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery("GetMaintenanceHost").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "host"))
		mock.ExpectQuery("ListHostAlertScopeNodes").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"service_id", "business_system_id", "environment_id", "project_id"}).
			AddRow(1, tc.business, tc.env, tc.project).AddRow(2, 4, 9, 2))
		h := Handler{db: pool}
		ok, err := h.maintenanceScopeMatch(t.Context(), ms, &maintenanceAlertFacts{labels: map[string]string{"host_id": "7"}})
		pool.Close()
		if ok != tc.want || err != nil {
			t.Fatalf("%+v: matched=%t err=%v", tc, ok, err)
		}
	}
}

func TestMaintenanceRuntimeLookup(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	host := `[{"type":"host","id":7}]`
	for _, tc := range []struct {
		name, rules  string
		err, boolHit bool
	}{
		{"label_without_asset", maintenanceLabel, false, true}, {"asset_read_error", host, true, false}, {"corrupt_stored_conditions", `[]`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, m, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer pool.Close()
			m.ExpectQuery("ListActiveMaintenanceWindows").WithArgs(now, now).WillReturnRows(maintenanceRows(1, "open", tc.rules, now, now.Add(time.Hour)))
			if tc.rules == host {
				m.ExpectQuery("GetMaintenanceHost").WithArgs(int64(7)).WillReturnError(errors.New("read interrupted"))
			}
			h := &Handler{db: pool}
			hits, e := h.matchedMaintenanceWindows(t.Context(), alertNotificationTarget{labels: map[string]any{"host_id": "7", "team": "ops"}}, now)
			if (e != nil) != tc.err || (len(hits) == 1) != tc.boolHit {
				t.Fatalf("hits=%v err=%v", hits, e)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaintenanceInstanceIP(t *testing.T) {
	for _, tc := range []struct {
		instance, want string
	}{
		{"10.25.66.177:9100", "10.25.66.177"},
		{"10.25.66.177", "10.25.66.177"},
		{"[2001:db8::1]:9100", "2001:db8::1"},
		{"2001:db8::1", "2001:db8::1"},
		{"server.example:9100", ""},
		{"http://10.25.66.177:9100", ""},
		{"", ""},
	} {
		if got := maintenanceInstanceIP(tc.instance); got != tc.want {
			t.Errorf("instance %q: got %q, want %q", tc.instance, got, tc.want)
		}
	}
}

func TestMaintenanceResolveInstanceHost(t *testing.T) {
	for _, tc := range []struct {
		name, instance, label, queryIP string
		ids                            []int64
		wantID                         int64
		wantErr                        string
	}{
		{name: "IPv4", instance: "10.25.66.177:9100", queryIP: "10.25.66.177", ids: []int64{222}, wantID: 222},
		{name: "IPv6", instance: "[2001:db8::1]:9100", queryIP: "2001:db8::1", ids: []int64{7}, wantID: 7},
		{name: "unknown IP", instance: "192.0.2.1:9100", queryIP: "192.0.2.1"},
		{name: "ambiguous IP", instance: "192.0.2.1:9100", queryIP: "192.0.2.1", ids: []int64{1, 2}, wantErr: "对应多个主机"},
		{name: "hostname", instance: "server:9100"},
		{name: "invalid ID does not fall back", instance: "10.25.66.177:9100", label: "invalid", wantErr: "无效的告警 host_id"},
		{name: "host ID takes precedence", instance: "10.25.66.177:9100", label: "7", wantID: 7},
		{name: "unknown host ID does not fall back", instance: "10.25.66.177:9100", label: "99"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			if tc.queryIP != "" {
				rows := sqlmock.NewRows([]string{"id", "name"})
				for _, id := range tc.ids {
					rows.AddRow(id, "host")
				}
				mock.ExpectQuery("ListMaintenanceHostsByIP").WithArgs(tc.queryIP).WillReturnRows(rows)
			}
			if tc.label == "7" {
				mock.ExpectQuery("GetMaintenanceHost").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "host"))
			}
			if tc.label == "99" {
				mock.ExpectQuery("GetMaintenanceHost").WithArgs(int64(99)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
			}
			if tc.wantID != 0 {
				mock.ExpectQuery("ListHostAlertScopeNodes").WithArgs(tc.wantID).WillReturnRows(sqlmock.NewRows([]string{"service_id", "business_system_id", "environment_id", "project_id"}))
			}
			h := &Handler{db: pool}
			facts := newMaintenanceFacts(alertNotificationTarget{instance: tc.instance, labels: map[string]any{"host_id": tc.label}})
			err = h.loadMaintenanceAssets(t.Context(), facts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
			} else if err != nil || facts.hostID != tc.wantID {
				t.Fatalf("host ID = %d, error = %v", facts.hostID, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaintenanceTreeMatchesInstanceHost(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("ListMaintenanceHostsByIP").WithArgs("10.25.66.177").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(222, "177"))
	mock.ExpectQuery("ListHostAlertScopeNodes").WithArgs(int64(222)).
		WillReturnRows(sqlmock.NewRows([]string{"service_id", "business_system_id", "environment_id", "project_id"}).AddRow(1, 2, 3, 4))
	matchers, err := maintenanceMatchers([]byte(`[{"type":"tree","node_type":"project","id":4}]`))
	if err != nil {
		t.Fatal(err)
	}
	facts := newMaintenanceFacts(alertNotificationTarget{instance: "10.25.66.177:9100"})
	matched, err := (&Handler{db: pool}).maintenanceScopeMatch(t.Context(), matchers, facts)
	if err != nil || !matched {
		t.Fatalf("matched=%t err=%v", matched, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenancePreviewResolvesInstanceHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"alerts":[{"labels":{"alertname":"HighCpuUsage","instance":"10.25.66.177:9100"},"state":"firing"}]}}`))
	}))
	defer server.Close()
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("GetMaintenanceHost").WithArgs(int64(222)).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(222, "177"))
	mock.ExpectQuery("SELECT value FROM sys_config").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(server.URL))
	mock.ExpectQuery("ListMaintenanceHostsByIP").WithArgs("10.25.66.177").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(222, "177"))
	mock.ExpectQuery("ListHostAlertScopeNodes").WithArgs(int64(222)).WillReturnRows(sqlmock.NewRows([]string{"service_id", "business_system_id", "environment_id", "project_id"}))
	router := gin.New()
	router.GET("/", (&Handler{db: pool, client: server.Client()}).PreviewMaintenanceWindow)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?matchers="+url.QueryEscape(`[{"type":"host","id":222}]`), nil))
	var result struct {
		Code int `json:"code"`
		Data struct {
			Count   int `json:"count"`
			Results []struct {
				HostID int64 `json:"host_id"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.Code != 200 || result.Data.Count != 1 || len(result.Data.Results) != 1 || result.Data.Results[0].HostID != 222 {
		t.Fatalf("preview = %s, error = %v", recorder.Body.String(), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifiedMaintenanceHostUsesHistoryInstance(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("ListMaintenanceHostsByIP").WithArgs("10.25.66.177").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(222, "177"))
	mock.ExpectQuery("ListHostAlertScopeNodes").WithArgs(int64(222)).
		WillReturnRows(sqlmock.NewRows([]string{"service_id", "business_system_id", "environment_id", "project_id"}))
	got := (&Handler{db: pool}).verifiedMaintenanceHost(t.Context(), 314, "10.25.66.177:9100", map[string]any{})
	if got == nil || *got != 222 {
		t.Fatalf("verified host = %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceHalfOpen(t *testing.T) {
	now := time.Now().UTC()
	w := db.MonitorMaintenanceWindow{Status: "open", StartsAt: now, EndsAt: now.Add(time.Hour)}
	for at, want := range map[time.Time]string{now.Add(-time.Microsecond): "scheduled", now: "active", w.EndsAt.Add(-time.Microsecond): "active", w.EndsAt: "expired"} {
		got, err := maintenanceState(w, at)
		if err != nil || got != want {
			t.Fatalf("%v: %s %v", at, got, err)
		}

	}
}

func TestMaintenanceCreateEditEnd(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, scenario := range []string{"create", "update", "active_update", "end"} {
		t.Run(scenario, func(t *testing.T) {
			action := scenario
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			start, end := now.Add(time.Hour), now.Add(3*time.Hour)
			in := maintenanceInput{Title: "patch", Reason: "upgrade", StartsAt: start, EndsAt: end, Matchers: json.RawMessage(maintenanceLabel)}
			status, wantState := "open", "scheduled"
			if scenario == "active_update" {
				action, wantState, start = "update", "active", now.Add(-time.Hour)
				in.StartsAt = start
			}
			mock.ExpectBegin()
			if action == "create" {
				expectCreatedMaintenanceWindow(mock, 7)
			} else {
				in.ID = 7
				if action == "end" {
					start = now.Add(-time.Hour)
				}
				mock.ExpectQuery("LockMaintenanceWindow").WithArgs(int64(7)).WillReturnRows(maintenanceRows(7, "open", maintenanceLabel, start, end))
				if action == "end" {
					mock.ExpectExec("CloseMaintenanceWindow").WithArgs("ended", sqlmock.AnyArg(), "alice", sqlmock.AnyArg(), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
					status, wantState, end = "ended", "ended", now
				} else {
					mock.ExpectExec("EditMaintenanceWindow").WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			mock.ExpectQuery("GetMaintenanceWindow").WithArgs(int64(7)).WillReturnRows(maintenanceRows(7, status, maintenanceLabel, start, end))
			if action == "end" {
				mock.ExpectQuery("ListMaintenanceWindowSuppressionCounts").WillReturnRows(sqlmock.NewRows([]string{"window_id", "suppressed_count"}).AddRow(7, 2))
			}
			mock.ExpectExec("AddMaintenanceWindowAudit").WithArgs(int64(7), action, "alice", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			r := gin.New()
			r.POST("/", func(c *gin.Context) {
				c.Set(identity.ClaimsContextKey, &identity.Claims{Username: "alice"})
				(&Handler{db: pool}).mutateMaintenance(c, action)
			})
			body, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body))))
			var res struct {
				Code int
				Data maintenanceWindowItem
			}
			if err = json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Code != 200 || res.Data.Status != wantState || (action == "end" && res.Data.SuppressedCount != 2) {
				t.Fatal(w.Body.String(), err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaintenanceActiveDeleteAndStartLock(t *testing.T) {
	for _, action := range []string{"delete", "update"} {
		t.Run(action, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			now := time.Now().UTC().Truncate(time.Microsecond)
			start, end := now.Add(-time.Hour), now.Add(time.Hour)
			mock.ExpectBegin()
			mock.ExpectQuery("LockMaintenanceWindow").WithArgs(int64(7)).WillReturnRows(maintenanceRows(7, "open", maintenanceLabel, start, end))
			h := Handler{db: pool}
			handle, body, want := h.BatchDeleteMaintenanceWindows, `{"ids":[7]}`, 200
			if action == "delete" {
				mock.ExpectExec("CloseMaintenanceWindow").WithArgs("deleted", end, "alice", sqlmock.AnyArg(), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("AddMaintenanceWindowAudit").WithArgs(int64(7), "delete", "alice", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			} else {
				handle, want = h.UpdateMaintenanceWindow, 409
				raw, err := json.Marshal(maintenanceInput{ID: 7, Title: "test", Reason: "patch", StartsAt: now, EndsAt: end, Matchers: json.RawMessage(maintenanceLabel)})
				if err != nil {
					t.Fatal(err)
				}
				body = string(raw)
				mock.ExpectRollback()
			}
			r := gin.New()
			r.POST("/", func(c *gin.Context) {
				c.Set(identity.ClaimsContextKey, &identity.Claims{Username: "alice"})
				handle(c)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
			var res struct{ Code int }
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Code != want {
				t.Fatal(w.Body.String(), err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaintenanceCancelTransaction(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		active, auditFailure bool
		code                 int
	}{{"scheduled", false, false, 200}, {"audit_rollback", false, true, 600}, {"active_forbidden", true, false, 409}} {
		t.Run(tc.name, func(t *testing.T) {
			pool, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			start := time.Now().UTC().Add(time.Hour)
			if tc.active {
				start = start.Add(-2 * time.Hour)
			}
			end := start.Add(3 * time.Hour)
			m.ExpectBegin()
			m.ExpectQuery("LockMaintenanceWindow").WithArgs(int64(7)).WillReturnRows(maintenanceRows(7, "open", maintenanceLabel, start, end))
			if tc.active {
				m.ExpectRollback()
			} else {
				m.ExpectExec("CloseMaintenanceWindow").WithArgs("cancelled", end, "operator", sqlmock.AnyArg(), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectQuery("GetMaintenanceWindow").WithArgs(int64(7)).WillReturnRows(maintenanceRows(7, "cancelled", maintenanceLabel, start, end))
				audit := m.ExpectExec("AddMaintenanceWindowAudit").WithArgs(int64(7), "cancel", "operator", sqlmock.AnyArg(), maintenanceAuditSnapshot{})
				if tc.auditFailure {
					audit.WillReturnError(errors.New("audit unavailable"))
					m.ExpectRollback()
				} else {
					audit.WillReturnResult(sqlmock.NewResult(0, 1))
					m.ExpectCommit()
				}
			}
			r := gin.New()
			r.POST("/", func(c *gin.Context) {
				c.Set(identity.ClaimsContextKey, &identity.Claims{Username: "operator"})
				(&Handler{db: pool}).CancelMaintenanceWindow(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id":7}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var res struct{ Code int }
			if e := json.Unmarshal(w.Body.Bytes(), &res); e != nil || res.Code != tc.code {
				t.Fatalf("%v %s", e, w.Body.String())
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type maintenanceAuditSnapshot struct{}

func (maintenanceAuditSnapshot) Match(v driver.Value) bool {
	raw, ok := v.([]byte)
	if !ok {
		return false
	}
	var snap struct{ Before, After db.MonitorMaintenanceWindow }
	return json.Unmarshal(raw, &snap) == nil && snap.Before.Status == "open" && snap.After.Status == "cancelled" && snap.After.ID == 7 && snap.After.Reason == "upgrade"
}

func TestMaintenanceDetailScopeNames(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const matchers = `[{"type":"host","id":7},{"type":"tree","node_type":"project","id":9}]`
	now := time.Now().UTC()
	mock.ExpectQuery("GetMaintenanceWindow").WithArgs(int64(3)).WillReturnRows(maintenanceRows(3, "open", matchers, now, now.Add(time.Hour)))
	mock.ExpectQuery("ListMaintenanceWindowSuppressionCounts").WillReturnRows(sqlmock.NewRows([]string{"window_id", "suppressed_count"}))
	mock.ExpectQuery("GetHostTargetIdentity").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"instance_name", "ip", "is_deleted_in_cloud"}).AddRow("db", "192.0.2.7", false))
	mock.ExpectQuery("GetProjectNameByID").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"name"}))
	r := gin.New()
	r.GET("/", (&Handler{db: pool}).DetailMaintenanceWindow)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?id=3", nil))
	var res struct {
		Code int
		Data struct {
			ID    int64
			Names map[string]string `json:"scope_names"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Code != 200 || res.Data.ID != 3 || res.Data.Names["host:7"] != "db(192.0.2.7)" || !strings.Contains(res.Data.Names["project:9"], "不存在") {
		t.Fatal(w.Body.String(), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenancePreview(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code, n    int
	}{
		{"matches", `{"alerts":[{"labels":{"alertname":"A","team":"ops"},"state":"firing"},{"labels":{"alertname":"B"}}]}`, 200, 1},
		{"empty", `{"alerts":[]}`, 200, 0}, {"invalid_upstream", `{}`, 502, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/alerts" {
					t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL)
				}
				_, _ = w.Write([]byte(`{"status":"success","data":` + tc.body + `}`))
			}))
			defer server.Close()
			pool, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			m.ExpectQuery("SELECT value FROM sys_config").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(server.URL))
			r := gin.New()
			r.GET("/", (&Handler{db: pool, client: server.Client()}).PreviewMaintenanceWindow)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?matchers="+url.QueryEscape(maintenanceLabel), nil))
			var res struct {
				Code int
				Data struct {
					Count   int
					Results []struct {
						Name    string
						Matched []policyMatcher `json:"matched_by"`
					}
				}
			}
			if err = json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Code != tc.code || res.Data.Count != tc.n || len(res.Data.Results) != tc.n || (tc.n == 1 && (res.Data.Results[0].Name != "A" || len(res.Data.Results[0].Matched) != 1)) {
				t.Fatal(w.Body.String(), err)
			}
			if err = m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
