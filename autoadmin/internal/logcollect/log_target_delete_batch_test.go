package logcollect

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestBatchDeleteLogTargetReportsEnabledFailure(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT l.id, l.host_id, l.managed_enabled, l.install_status").
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "host_id", "enabled", "status", "name", "ip", "os", "like", "version"}).
			AddRow(12, 4, true, "success", "host-a", "127.0.0.1", "", "", ""))
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/test", strings.NewReader(`{"ids":[12]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	(&Handler{db: pool}).BatchDeleteLogTargets(ctx)
	var body struct {
		Code int `json:"code"`
		Data struct {
			Count   int `json:"count"`
			Results []struct {
				Code int    `json:"code"`
				OK   bool   `json:"ok"`
				Host string `json:"host"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 400 || body.Data.Count != 0 || len(body.Data.Results) != 1 ||
		body.Data.Results[0].Code != 400 || body.Data.Results[0].OK || body.Data.Results[0].Host != "host-a" {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
