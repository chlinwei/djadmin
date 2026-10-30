package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestMonitorHostGroupsAreFlatWithDirectCounts(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT g.id, g.name, COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "host_count", "managed_count"}).
			AddRow(1, "production", 2, 1).AddRow(2, "testing", 3, 2))
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count", "managed_total", "ungrouped"}).AddRow(5, 3, 0))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/monitor/targets/host-group-tree/", nil)
	(&Handler{db: pool}).HostGroupTree(ctx)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Groups []map[string]any `json:"groups"`
			Total  int64            `json:"total_host_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || len(envelope.Data.Groups) != 2 || envelope.Data.Total != 5 {
		t.Fatalf("unexpected group response: %s", recorder.Body.String())
	}
	for i, group := range envelope.Data.Groups {
		if _, found := group["parent_id"]; found || len(group["children"].([]any)) != 0 ||
			group["host_count"] != float64(i+2) || group["managed_count"] != float64(i+1) {
			t.Fatalf("groups must have direct counts without nesting: %#v", group)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorHostFilterUsesOnlyRequestedGroup(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets_host h`).
		WithArgs("", "", "", "7", int64(7), sqlmock.AnyArg(), "", "", "", "", "", "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT h.id, h.instance_name").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/monitor/targets/host-overview/?group_id=7", nil)
	(&Handler{db: pool}).HostOverview(ctx)
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 {
		t.Fatalf("exact group filtering failed: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorRejectsInvalidGroupFilter(t *testing.T) {
	for _, value := range []string{"0", "-1", "abc", "1,2"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/monitor/targets/host-overview/?group_id="+value, nil)
		(&Handler{}).HostOverview(ctx)
		var envelope struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Code != 400 {
			t.Fatalf("invalid group %q must be rejected before database access: %s", value, recorder.Body.String())
		}
	}
}
