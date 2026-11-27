package monitor

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestBatchDeletePoliciesReportsRootAndKeepsOtherResults(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT COALESCE\\(parent_id, 0\\) FROM monitor_notification_policy").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(0))
	mock.ExpectQuery("SELECT COALESCE\\(parent_id, 0\\) FROM monitor_notification_policy").
		WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(1))
	mock.ExpectExec("DELETE FROM monitor_notification_policy").WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/test", strings.NewReader(`{"ids":[1,2]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	(&Handler{db: pool}).BatchDeleteNotificationPolicies(ctx)
	var body struct {
		Code int `json:"code"`
		Data struct {
			Count   int `json:"count"`
			Results []struct {
				Code int  `json:"code"`
				OK   bool `json:"ok"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != apperror.CodeBatchPartial || body.Data.Count != 1 || len(body.Data.Results) != 2 ||
		body.Data.Results[0].Code != 400 || body.Data.Results[0].OK || !body.Data.Results[1].OK {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
