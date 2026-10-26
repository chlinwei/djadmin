package logcollect

import (
	"context"
	"errors"
	"strings"
	"testing"

	"autoadmin/internal/agent/pb"
	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func pendingSharedStopRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"source_id", "generation", "stopped", "last_error", "host_name"}).
		AddRow(5, 7, false, "waiting", "host-01")
}

func TestSharedSourceStopRetriesAfterAgentReconnect(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	handler := &Handler{db: pool}
	mock.ExpectQuery("FROM monitor_shared_log_source_revocation v").WillReturnRows(pendingSharedStopRows())
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("UPDATE monitor_shared_log_source_revocation").
		WithArgs(false, "主机 host-01 Agent 离线，等待重连后撤销采集", int64(5), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err = handler.reconcileSharedSourceStops(context.Background()); err != nil {
		t.Fatal(err)
	}

	handler.gateway = startTestAgent(t, func(frame *pb.ServerFrame) *pb.AgentFrame {
		request := frame.GetAutomationExecuteRequest()
		if request == nil {
			return nil
		}
		if request.Action != "revoke_shared_log_source" ||
			!strings.Contains(request.ParamsJson, `"shared_source_generation":"7"`) ||
			!strings.Contains(request.ParamsJson, `"shared_source_id":"5"`) {
			t.Errorf("unexpected shared-source revocation: %+v", request)
		}
		return &pb.AgentFrame{Payload: &pb.AgentFrame_AutomationExecuteResponse{
			AutomationExecuteResponse: &pb.AutomationExecuteResponse{
				RequestId: request.RequestId, JobId: request.JobId, Status: "success",
			},
		}}
	})
	mock.ExpectQuery("FROM monitor_shared_log_source_revocation v").WillReturnRows(pendingSharedStopRows())
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("UPDATE monitor_shared_log_source_revocation").
		WithArgs(true, "", int64(5), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err = handler.reconcileSharedSourceStops(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedSourceStopPersistsBarrierFailure(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("FROM monitor_shared_log_source_revocation v").WillReturnRows(pendingSharedStopRows())
	mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster").WillReturnError(errors.New("cluster lookup failed"))
	mock.ExpectExec("UPDATE monitor_shared_log_source_revocation").
		WithArgs(false, "ES 写入屏障未完成: cluster lookup failed", int64(5), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err = (&Handler{db: pool}).reconcileSharedSourceStops(context.Background()); err == nil {
		t.Fatal("barrier failure must not be reported as a completed stop")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedHistoricalStreamRetainsActiveOrUnrevokedSource(t *testing.T) {
	for _, active := range []bool{true, false} {
		t.Run(map[bool]string{true: "current tier", false: "pending revocation"}[active], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectBegin()
			tx, err := pool.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			mock.ExpectQuery("FROM monitor_shared_log_source WHERE id=").WithArgs(int64(5)).
				WillReturnRows(sharedSourceModelRows())
			mock.ExpectQuery("SELECT id FROM monitor_log_retention_tier").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
			mock.ExpectQuery("FROM assets_service_deletion").
				WillReturnRows(sqlmock.NewRows([]string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}))
			mock.ExpectQuery("-- name: ListServiceStreamDims :many").
				WillReturnRows(sqlmock.NewRows([]string{"project_code", "environment_code", "business_system_code",
					"service_id", "service_code", "tier_code", "service_enabled", "log_collection_enabled"}).
					AddRow("p", "prod", "b", -5, "shared-source-5", "hot", true, true))
			activeRows := sqlmock.NewRows([]string{"service_id", "service_code", "tier_code"})
			if active {
				activeRows.AddRow(-5, "shared-source-5", "hot")
			}
			mock.ExpectQuery("-- name: ListServiceActiveStreamTiers :many").WillReturnRows(activeRows)
			if !active {
				mock.ExpectQuery("FROM monitor_shared_log_source_revocation v").WillReturnRows(pendingSharedStopRows())
			}
			mock.ExpectRollback()
			owner := db.AssetsServiceDeletion{ServiceID: -5, ProjectCode: "p", BusinessSystemCode: "b",
				EnvironmentCode: "prod", ServiceCode: "shared-source-5"}
			err = (&Handler{db: pool}).recycleSharedHistoricalStream(context.Background(), tx, db.New(tx),
				elasticsearchCluster{IndexPrefix: "logs"}, historicalDataStream{Name: "logs-p-b-prod-shared-source-5-hot"}, owner, "hot")
			if active && err != nil {
				t.Fatal(err)
			}
			if !active && (err == nil || !strings.Contains(err.Error(), "waiting for collection revocation")) {
				t.Fatalf("pending source must be protected from retention cleanup: %v", err)
			}
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSharedSourceUnlinkIsIdempotent(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("FROM monitor_shared_log_source WHERE id=").WithArgs(int64(5)).
		WillReturnRows(sharedSourceModelRows())
	mock.ExpectQuery("SELECT service_id FROM monitor_shared_log_source_service").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"service_id"}).AddRow(33))
	mock.ExpectCommit()
	if err = (&Handler{db: pool}).unlinkSharedSourceServices(context.Background(), 5, []int64{32}); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
