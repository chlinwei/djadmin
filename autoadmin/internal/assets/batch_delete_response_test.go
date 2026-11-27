package assets

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/api/response"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestAssetBatchDeleteBusinessFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	now := time.Now().UTC()
	mock.ExpectQuery("FROM assets_project p WHERE p.id").WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "name", "code", "owner", "enabled", "business_system_names", "business_system_ids"}).
			AddRow(1, now, now, nil, "测试项目", "test", "", true, "服务", "2"))
	mock.ExpectQuery("COUNT\\(\\*\\) FROM assets_business_system WHERE project_id").WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/assets/projects/batch-delete/", strings.NewReader(`{"ids":[1]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	NewHandler(&Service{repository: NewRepository(pool)}, nil, "").BatchDeleteProjects(ctx)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Count   int                    `json:"count"`
			Results []response.BatchResult `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || body.Code != 400 || body.Msg != ErrDeleteProtected.Message() ||
		body.Data.Count != 0 || len(body.Data.Results) != 1 || body.Data.Results[0].ID != 1 ||
		body.Data.Results[0].OK || body.Data.Results[0].Code != 400 || body.Data.Results[0].Message != body.Msg {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
