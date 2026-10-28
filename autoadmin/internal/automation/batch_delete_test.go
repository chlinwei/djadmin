package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestDeleteTaskPreservesHistory(t *testing.T) {
	for _, status := range []string{"success", "failed", "cancelled", "none"} {
		t.Run(status, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT id FROM automation_task .* FOR UPDATE`).WithArgs(int64(34)).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(34))
			jobs := sqlmock.NewRows([]string{"id", "status"})
			if status != "none" {
				jobs.AddRow(100, status)
			}
			mock.ExpectQuery(`(?s)SELECT id, status FROM automation_execution_job.*FOR UPDATE`).WithArgs(int64(34)).
				WillReturnRows(jobs)
			mock.ExpectExec(`UPDATE automation_execution_job SET task_id = NULL`).WithArgs(int64(34)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`DELETE FROM automation_task`).WithArgs(int64(34)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := (&Handler{db: pool}).deleteTask(context.Background(), 34); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeleteTaskRollsBack(t *testing.T) {
	for _, scenario := range []string{"pending", "running", "unknown", "missing", "read-error", "detach-error", "delete-error", "delete-zero", "commit-error"} {
		t.Run(scenario, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			dbError := errors.New("database failure")
			mock.ExpectBegin()
			lock := mock.ExpectQuery(`SELECT id FROM automation_task .* FOR UPDATE`).WithArgs(int64(34))
			if scenario == "missing" {
				lock.WillReturnError(sql.ErrNoRows)
			} else {
				lock.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(34))
				read := mock.ExpectQuery(`(?s)SELECT id, status FROM automation_execution_job.*FOR UPDATE`).WithArgs(int64(34))
				if scenario == "read-error" {
					read.WillReturnError(dbError)
				} else {
					status := "success"
					if scenario == "pending" || scenario == "running" || scenario == "unknown" {
						status = scenario
					}
					read.WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(100, status))
					if status == "success" {
						detach := mock.ExpectExec(`UPDATE automation_execution_job SET task_id = NULL`).WithArgs(int64(34))
						if scenario == "detach-error" {
							detach.WillReturnError(dbError)
						} else {
							detach.WillReturnResult(sqlmock.NewResult(0, 1))
							deletion := mock.ExpectExec(`DELETE FROM automation_task`).WithArgs(int64(34))
							switch scenario {
							case "delete-error":
								deletion.WillReturnError(dbError)
							case "delete-zero":
								deletion.WillReturnResult(sqlmock.NewResult(0, 0))
							default:
								deletion.WillReturnResult(sqlmock.NewResult(0, 1))
							}
						}
					}
				}
			}
			if scenario == "commit-error" {
				mock.ExpectCommit().WillReturnError(dbError)
			} else {
				mock.ExpectRollback()
			}
			err = (&Handler{db: pool}).deleteTask(context.Background(), 34)
			if err == nil {
				t.Fatal("expected deletion failure")
			}
			if scenario == "pending" || scenario == "running" || scenario == "unknown" {
				if !strings.Contains(err.Error(), "#100") || !strings.Contains(err.Error(), scenario) {
					t.Fatalf("missing job diagnostic: %v", err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBatchDeleteReportsPartialFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	respondBatchDelete(ctx, []int64{34, 35}, func(id int64) error {
		if id == 34 {
			return errors.New("作业仍在运行")
		}
		return nil
	})
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Count   int `json:"count"`
			Results []struct {
				ID      int64  `json:"id"`
				OK      bool   `json:"ok"`
				Message string `json:"message"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || payload.Code != 200 || payload.Data.Count != 1 || len(payload.Data.Results) != 2 {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	if payload.Data.Results[0].OK || payload.Data.Results[0].Message == "" || !payload.Data.Results[1].OK {
		t.Fatalf("incorrect item results: %s", recorder.Body.String())
	}
}
