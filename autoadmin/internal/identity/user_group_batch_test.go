package identity

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestBatchDeleteUserGroupsReportsMissingAlongsideSuccess(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectExec("DELETE FROM sys_user_group WHERE id").WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM sys_user_group WHERE id").WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 0))
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/test", strings.NewReader(`{"ids":[1,2]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	NewHandler(&Service{repository: NewRepository(pool)}).BatchDeleteUserGroups(ctx)
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
		body.Data.Results[1].Code != 404 || body.Data.Results[1].OK {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
