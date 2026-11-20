package automation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestHostOptionsSerializesNullableInstanceNames(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}

	defer pool.Close()
	mock.ExpectQuery("FROM sys_config").WithArgs(hostOptionsPageSizeKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_host h").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery("SELECT h.id, h.instance_name, s.hostname, h.ip, h.group_id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "instance_name", "hostname", "ip", "group_id"}).
			AddRow(201, "hak-app-201", "os-host-201", "10.25.66.201", 1).
			AddRow(202, nil, nil, "10.25.66.202", nil).
			AddRow(203, "", "", "10.25.66.203", 1))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/sys/automation/playbooks/host-options/", nil)
	handler := &Handler{db: pool}
	handler.HostOptions(ctx)

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Count      int `json:"count"`
			PageNumber int `json:"pageNumber"`
			PageSize   int `json:"pageSize"`
			Results    []struct {
				ID           int64   `json:"id"`
				InstanceName *string `json:"instance_name"`
				Hostname     *string `json:"hostname"`
				IP           string  `json:"ip"`
				GroupID      *int64  `json:"group_id"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("host option names must be strings or null: %s: %v", recorder.Body.String(), err)
	}
	if recorder.Code != http.StatusOK || envelope.Code != 200 || envelope.Data.Count != 3 ||
		envelope.Data.PageNumber != 1 || envelope.Data.PageSize != 10 || len(envelope.Data.Results) != 3 {
		t.Fatalf("unexpected pagination response: %s", recorder.Body.String())
	}
	first, missing, empty := envelope.Data.Results[0], envelope.Data.Results[1], envelope.Data.Results[2]
	if first.ID != 201 || first.InstanceName == nil || *first.InstanceName != "hak-app-201" ||
		first.Hostname == nil || *first.Hostname != "os-host-201" || first.IP != "10.25.66.201" ||
		first.GroupID == nil || *first.GroupID != 1 {
		t.Fatalf("unexpected host fields: %s", recorder.Body.String())
	}
	if missing.InstanceName != nil || missing.Hostname != nil || missing.GroupID != nil ||
		empty.InstanceName == nil || *empty.InstanceName != "" || empty.Hostname == nil || *empty.Hostname != "" {
		t.Fatalf("nullable names lost null/empty distinction: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHostOptionsUsesConfiguredPageSizeLimit(t *testing.T) {
	for _, tt := range []struct {
		name, setting, query string
		size, offset, pages  int
	}{
		{"default maximum", "", "?page=2&page_size=1500", 1000, 1000, 2},
		{"configured maximum", "250", "?page=2&page_size=1000", 250, 250, 6},
		{"smaller request", "1200", "?page=2&page_size=500", 500, 500, 3},
		{"raised maximum", "1200", "?page=2&page_size=1500", 1200, 1200, 2},
		{"invalid setting", "0", "?page=2&page_size=1500", 1000, 1000, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()

			configRows := sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "key", "value", "value_type", "name", "description", "is_readonly", "default_value"})
			if tt.setting != "" {
				now := time.Now().UTC()
				configRows.AddRow(1, now, now, nil, hostOptionsPageSizeKey, tt.setting, "int", "page size", nil, false, "1000")
			}
			mock.ExpectQuery("FROM sys_config").WithArgs(hostOptionsPageSizeKey).WillReturnRows(configRows)
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_host h").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1500))
			mock.ExpectQuery("SELECT h.id, h.instance_name, s.hostname, h.ip, h.group_id").
				WithArgs(nil, nil, nil, nil, tt.size, tt.offset).
				WillReturnRows(sqlmock.NewRows([]string{"id", "instance_name", "hostname", "ip", "group_id"}))

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/sys/automation/playbooks/host-options/"+tt.query, nil)
			(&Handler{db: pool}).HostOptions(ctx)

			var envelope struct {
				Code int `json:"code"`
				Data struct {
					PageNumber int `json:"pageNumber"`
					PageSize   int `json:"pageSize"`
					TotalPages int `json:"totalPages"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || envelope.Code != 200 || envelope.Data.PageNumber != 2 ||
				envelope.Data.PageSize != tt.size || envelope.Data.TotalPages != tt.pages {
				t.Fatalf("unexpected pagination response: %s", recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGroupOptionsAreIndependentGroups(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT id, name FROM assets_hostgroup ORDER BY id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "production").AddRow(2, "testing"))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/sys/automation/playbooks/group-tree/", nil)
	(&Handler{db: pool}).GroupTree(ctx)
	var envelope struct {
		Code int              `json:"code"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || envelope.Code != 200 || len(envelope.Data) != 2 {
		t.Fatalf("unexpected flat groups: %s", recorder.Body.String())
	}
	for _, group := range envelope.Data {
		if _, found := group["parent_id"]; found || len(group["children"].([]any)) != 0 {
			t.Fatalf("group nesting must not be exposed: %#v", group)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
