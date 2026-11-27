package response

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestBatchResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	protected := apperror.New(400, "资产仍被其他记录引用，无法删除")
	for _, tt := range []struct {
		name    string
		errors  []error
		code    int
		message string
		count   int
	}{
		{"success", []error{nil, nil}, 200, "success", 2},
		{"single failure", []error{protected}, 400, protected.Message(), 0},
		{"domain error wraps missing row", []error{apperror.WithCause(protected, sql.ErrNoRows)}, 400, protected.Message(), 0},
		{"partial", []error{nil, protected}, apperror.CodeBatchPartial, "部分操作失败", 1},
		{"same code", []error{protected, apperror.New(400, "其他原因")}, 400, "操作失败", 0},
		{"mixed codes", []error{protected, sql.ErrNoRows}, apperror.CodeBatchFailed, "操作失败", 0},
		{"unknown error", []error{errors.New("secret database detail")}, apperror.CodeInternal, "服务器内部错误", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			results := make([]BatchResult, 0, len(tt.errors))
			for i, err := range tt.errors {
				results = append(results, BatchItem(int64(i+1), err))
			}
			Batch(ctx, results)
			var body struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
				Data struct {
					Count   int           `json:"count"`
					Results []BatchResult `json:"results"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != 200 || body.Code != tt.code || body.Data.Count != tt.count ||
				len(body.Data.Results) != len(tt.errors) || !strings.Contains(body.Msg, tt.message) {
				t.Fatalf("unexpected response: %s", recorder.Body.String())
			}
			for i, result := range body.Data.Results {
				if result.ID != int64(i+1) || result.OK != (tt.errors[i] == nil) ||
					(result.OK && result.Code != 200) || (!result.OK && result.Code == 200) {
					t.Fatalf("unexpected item: %+v", result)
				}
			}
			if strings.Contains(recorder.Body.String(), "secret database detail") {
				t.Fatal("internal error leaked")
			}
		})
	}
}
