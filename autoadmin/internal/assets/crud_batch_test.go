package assets

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func checkBatch(t *testing.T, recorder *httptest.ResponseRecorder, code, count int, itemCodes ...int) []response.BatchResult {
	t.Helper()
	var body struct {
		Code int
		Data struct {
			Count   int
			Results []response.BatchResult
		}
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != code || body.Data.Count != count || len(body.Data.Results) != len(itemCodes) {
		t.Fatalf("unexpected batch: %s", recorder.Body.String())
	}
	for i, code := range itemCodes {
		if item := body.Data.Results[i]; item.Code != code || item.OK != (code == 200) {
			t.Fatalf("unexpected item: %+v", item)
		}
	}
	return body.Data.Results
}

func TestBatchDeleteAssetsZeroRows(t *testing.T) {
	for _, table := range []string{"assets_application", "assets_host", "assets_credential"} {
		pool, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Close() })
		h := &Handler{service: &Service{repository: NewRepository(pool)}}
		handler := h.DeleteApplications
		atomic := table != "assets_application"
		if atomic {
			mock.ExpectBegin()
			handler = h.DeleteHosts
			if table == "assets_credential" {
				handler = h.DeleteCredentials
			}
		}
		for _, id := range []int64{1, 2} {
			if table == "assets_credential" {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets_hostcredential`).WithArgs(id).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			}
			mock.ExpectExec("DELETE FROM " + table + " WHERE id").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 2-id))
		}
		if atomic {
			mock.ExpectRollback()
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"ids":[1,2]}`))
		handler(ctx)
		if atomic {
			checkBatch(t, recorder, apperror.CodeBatchFailed, 0, apperror.CodeBatchFailed, 404)
		} else {
			checkBatch(t, recorder, apperror.CodeBatchPartial, 1, 200, 404)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCredentialCSVBatchLateFailureRollsBack(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	expectCredentialImport(mock, nil)
	expectCredentialImport(mock, errors.New("private SQL failure"))
	mock.ExpectRollback()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "credentials.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "username,auth_type,port\nfirst,1,22\nsecond,1,22\n"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/", &body)
	ctx.Request.Header.Set("Content-Type", form.FormDataContentType())
	(&Handler{service: &Service{repository: NewRepository(pool)}}).BatchCreateCredentials(ctx)
	results := checkBatch(t, recorder, apperror.CodeBatchFailed, 0, apperror.CodeBatchFailed, apperror.CodeInternal)
	if results[0].ID != 2 || results[1].ID != 3 || results[0].Detail != nil || strings.Contains(recorder.Body.String(), "private SQL") {
		t.Fatalf("uncommitted ID or internal detail returned: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
