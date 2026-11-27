package identity

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestBatchDeleteUserRelationCountsRequireCommit(t *testing.T) {
	for _, missing := range []bool{false, true} {
		pool, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Close() })
		mock.ExpectBegin()
		for _, id := range []int32{1, 2} {
			affected := int64(1)
			if missing && id == 2 {
				affected = 0
			}
			mock.ExpectExec("DELETE FROM sys_user_role WHERE user_id").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 3))
			mock.ExpectExec("DELETE FROM sys_user WHERE id").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, affected))
		}
		wantCount, wantRoles, wantCode := 2, 6, 200
		if missing {
			mock.ExpectRollback()
			wantCount, wantRoles, wantCode = 0, 0, apperror.CodeBatchFailed
		} else {
			mock.ExpectCommit()
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"ids":[1,1,2]}`))
		NewHandler(&Service{repository: NewRepository(pool)}).BatchDeleteUsers(ctx)
		var body struct {
			Code int
			Data struct {
				Count   int
				Roles   int `json:"user_role_deleted_count"`
				Users   int `json:"user_deleted_count"`
				Results []response.BatchResult
			}
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != wantCode || body.Data.Count != wantCount || body.Data.Users != wantCount ||
			body.Data.Roles != wantRoles || len(body.Data.Results) != 2 || (missing && body.Data.Results[1].Code != 404) {
			t.Fatal(recorder.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}
