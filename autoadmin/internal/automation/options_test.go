package automation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestHostOptionsSerializesNullableInstanceNames(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}

	defer pool.Close()
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
