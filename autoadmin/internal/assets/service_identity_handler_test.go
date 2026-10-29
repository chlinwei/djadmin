package assets

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestSaveServiceReusesCompletedDeletionIdentity(t *testing.T) {
	for _, editing := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "edit"}[editing], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			id := int64(0)
			const savedID = int64(15)
			mock.ExpectBegin()
			expectServiceStreamClusterLocks(mock)
			if editing {
				id = savedID
				mock.ExpectQuery("SELECT id FROM assets_application_service WHERE").
					WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
			}
			mock.ExpectQuery("SELECT id FROM assets_business_system WHERE").
				WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
			if editing {
				mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
					WithArgs(id).WillReturnError(sql.ErrNoRows)
			}
			// The join is essential: a tombstone without a live service must not reserve its codes.
			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets_service_deletion t\s+JOIN assets_application_service deleting ON deleting.id=t.service_id`).
				WithArgs(int64(2), int64(3), "old-service").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery("FROM monitor_log_retention_tier").WithArgs(int64(3)).
				WillReturnRows(serviceRetentionTierRows("std", true))
			if editing {
				mock.ExpectExec("UPDATE assets_application_service").WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				expectServiceIdentityCreate(mock, savedID)
			}
			mock.ExpectQuery("SELECT deployment_id FROM assets_application_service_deployment").
				WithArgs(savedID).WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}))
			mock.ExpectCommit()
			saved, err := NewRepository(pool).SaveApplicationService(context.Background(), id, ApplicationServiceInput{
				Name: "service", Code: "old-service", BusinessSystem: 2,
				Environment: new(int64(3)), LogRetentionTier: new(int64(3)),
			}, nil)
			if err != nil || saved != savedID {
				t.Fatalf("completed deletion must allow reusing codes: saved=%d err=%v", saved, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSaveServiceDeletingLogIdentityReturnsBusinessError(t *testing.T) {
	for _, editing := range []bool{false, true} {
		name := "create"
		if editing {
			name = "edit"
		}
		t.Run(name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			handler := NewHandler(newTestService(t, NewRepository(pool)), nil, "")
			mock.ExpectBegin()
			expectServiceStreamClusterLocks(mock)
			if editing {
				mock.ExpectQuery("SELECT id FROM assets_application_service WHERE").
					WithArgs(int64(15)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(15))
			}
			mock.ExpectQuery("SELECT id FROM assets_business_system WHERE").
				WithArgs(int64(2)).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
			if editing {
				mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").
					WithArgs(int64(15)).WillReturnError(sql.ErrNoRows)
			}
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_service_deletion").
				WithArgs(int64(2), int64(3), "old-service").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectRollback()

			body := []byte(`{"name":"服务","code":"old-service","application":1,"business_system":2,"environment":3,"application_version":4,"deployment_template":5,"log_retention_tier":6,"member_configs":[{"deployment":7}]}`)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/assets/application-services/", bytes.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			if editing {
				ctx.Params = gin.Params{{Key: "id", Value: "15"}}
				handler.UpdateApplicationService(ctx)
			} else {
				handler.CreateApplicationService(ctx)
			}
			var result struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || result.Code != apperror.CodeInvalidArgument ||
				result.Msg != ErrServiceLogIdentityDeleting.Message() {
				t.Fatalf("reserved identity must return actionable business error: HTTP %d %s", recorder.Code, recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
