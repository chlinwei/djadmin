package automation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestBindPrecheckQuery(t *testing.T) {
	for _, tc := range []struct {
		query string
		ids   []int64
		valid bool
	}{
		{"", nil, true},
		{"host_ids=", nil, true},
		{"host_ids=10,11", []int64{10, 11}, true},
		{"host_ids=10,10", []int64{10, 10}, true},
		{"host_ids=10,", nil, false},
		{"host_ids=abc", nil, false},
		{"host_ids=0", nil, false},
		{"host_ids=-1", nil, false},
		{"host_ids=9223372036854775808", nil, false},
		{"host_ids=10&host_ids=11", nil, false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/?limit=host-*&"+tc.query, nil)
			input, valid := bindPrecheckQuery(ctx)
			if valid != tc.valid || valid && (!reflect.DeepEqual(input.HostIDs, tc.ids) || input.Limit != "host-*") {
				t.Fatalf("input=%+v valid=%v", input, valid)
			}
			if !valid && !json.Valid(recorder.Body.Bytes()) {
				t.Fatalf("missing error response: %s", recorder.Body.String())
			}
		})
	}
}

func TestPrecheckGETQueries(t *testing.T) {
	for _, task := range []bool{false, true} {
		t.Run(map[bool]string{false: "inventory", true: "task default limit"}[task], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			now := time.Now()
			if task {
				mock.ExpectQuery("FROM automation_task t").WithArgs(int64(7)).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "create_time", "update_time", "remark", "name", "env_vars", "enabled", "inventory_id",
						"default_limit", "execution_timeout_seconds", "playbook_template_id", "run_as_user", "run_as_group",
						"work_directory", "template_name", "template_content", "template_content_format", "inventory_name",
					}).AddRow(7, now, now, nil, "task", []byte(`{}`), true, 9, "host:host-b", 600, nil, "", "", "", "", "", "", "inventory"))
			}
			mock.ExpectQuery("FROM automation_inventory").WithArgs(int64(9)).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "create_time", "update_time", "remark", "name", "selected_host_ids", "enabled",
					"last_sync_host_count", "last_sync_message", "last_sync_status", "last_sync_time",
					"update_cache_timeout", "update_on_launch",
				}).AddRow(9, now, now, nil, "inventory", []byte(`[]`), true, 0, "", "", nil, 300, false))
			mock.ExpectQuery("FROM assets_host h").WithArgs(int64(10), int64(11)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "instance_name", "ip", "group_id", "group_name", "agent_online"}).
					AddRow(10, "host-a", "10.0.0.1", nil, "", false).
					AddRow(11, "host-b", "10.0.0.2", nil, "", false))
			query := url.Values{"host_ids": {"10,11"}}
			if !task {
				query.Set("limit", "host:host-b")
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil)
			ctx.Params = gin.Params{{Key: "id", Value: map[bool]string{false: "9", true: "7"}[task]}}
			handler := &Handler{db: pool}
			if task {
				handler.PrecheckTask(ctx)
			} else {
				handler.PrecheckInventoryLimit(ctx)
			}
			var envelope struct {
				Code int `json:"code"`
				Data struct {
					Hosts []struct {
						ID int64 `json:"host_id"`
					} `json:"matched_hosts_preview"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || envelope.Code != 200 ||
				len(envelope.Data.Hosts) != 1 || envelope.Data.Hosts[0].ID != 11 {
				t.Fatalf("unexpected precheck: %s, error=%v", recorder.Body.String(), err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
