package assets

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

func TestDependencyValidation(t *testing.T) {
	tests := []struct {
		name  string
		input ServiceDependencyInput
	}{
		{"self", ServiceDependencyInput{SourceServiceID: 1, TargetServiceID: 1}},
		{"missing source", ServiceDependencyInput{TargetServiceID: 2}},
		{"missing scope", ServiceDependencyInput{SourceServiceID: 1, TargetServiceID: 2, DependencyDetailsInput: DependencyDetailsInput{InteractionTypeID: 1}}},
		{"bad strength", ServiceDependencyInput{SourceServiceID: 1, TargetServiceID: 2, DependencyDetailsInput: DependencyDetailsInput{InteractionTypeID: 1, FunctionScope: "read", Strength: "medium"}}},
		{"bad reverse", ServiceDependencyInput{SourceServiceID: 1, TargetServiceID: 2, DependencyDetailsInput: DependencyDetailsInput{InteractionTypeID: 1, FunctionScope: "read"}, Reverse: &DependencyDetailsInput{InteractionTypeID: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{}
			if _, err := s.SaveServiceDependency(context.Background(), 0, tt.input); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	details := DependencyDetailsInput{InteractionTypeID: 1, FunctionScope: " read "}
	if err := normalizeDependencyDetails(&details); err != nil || details.Strength != "unknown" || details.FunctionScope != "read" {
		t.Fatalf("default/trim: %+v %v", details, err)
	}
	if _, err := (&Service{}).SaveServiceDependency(context.Background(), 1, ServiceDependencyInput{SourceServiceID: 1, TargetServiceID: 2, Reverse: &details}); err == nil {
		t.Fatal("reverse on update accepted")
	}
}

func TestDependencyScopeRules(t *testing.T) {
	a := db.GetServiceDependencyContextRow{ID: 1, BusinessSystemID: 10, ProjectID: dependencyID(3), EnvironmentID: dependencyID(4)}
	b := a
	b.ID = 2
	b.BusinessSystemID = 20
	if !sameDependencyScope(a, b) {
		t.Fatal("cross-business same-project/environment must be allowed")
	}
	for _, change := range []func(*db.GetServiceDependencyContextRow){
		func(v *db.GetServiceDependencyContextRow) { v.ProjectID = dependencyID(5) },
		func(v *db.GetServiceDependencyContextRow) { v.EnvironmentID = dependencyID(6) },
		func(v *db.GetServiceDependencyContextRow) { v.ProjectID = sql.NullInt64{} },
		func(v *db.GetServiceDependencyContextRow) { v.EnvironmentID = sql.NullInt64{} },
	} {
		b = a
		change(&b)
		if sameDependencyScope(a, b) {
			t.Fatalf("invalid scope accepted: %+v", b)
		}
	}
}

func dependencyTypeRows(enabled bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "create_time", "update_time", "code", "name", "enabled", "description"}).
		AddRow(1, time.Now(), time.Now(), "http", "服务调用", enabled, "")
}

func dependencyRow(id, source, target int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "create_time", "update_time", "source_service_id", "target_service_id",
		"source_business_id", "target_business_id", "project_id", "environment_id", "interaction_type_id", "strength", "protocol", "function_scope", "failure_impact", "fallback_plan"}).
		AddRow(id, time.Now(), time.Now(), source, target, 10, 20, 3, 4, 1, "strong", "HTTPS", "read", "", "")
}

func expectDependencyContext(mock sqlmock.Sqlmock, id, business, project, environment int64, deleting bool) {
	mock.ExpectQuery("SELECT s.id,s.business_system_id,s.environment_id,b.project_id").
		WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "business_system_id", "environment_id", "project_id"}).AddRow(id, business, environment, project))
	q := mock.ExpectQuery("FROM assets_service_deletion WHERE service_id=").WithArgs(id)
	if deleting {
		q.WillReturnRows(sqlmock.NewRows([]string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}).AddRow(id, "p", "b", "prod", "s"))
	} else {
		q.WillReturnError(sql.ErrNoRows)
	}
}

func dependencyGetRows(id, source, target int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "create_time", "update_time", "source_service_id", "target_service_id",
		"source_business_id", "target_business_id", "project_id", "environment_id", "interaction_type_id", "strength", "protocol", "function_scope", "failure_impact", "fallback_plan",
		"source_name", "target_name", "interaction_type_code", "interaction_type_name", "interaction_type_enabled"}).
		AddRow(id, time.Now(), time.Now(), source, target, 10, 20, 3, 4, 1, "strong", "HTTPS", "read", "", "", "A", "B", "http", "服务调用", true)
}

func TestDependencyBidirectionalAtomicWrite(t *testing.T) {
	for _, failReverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback_reverse_duplicate"}[failReverse], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("FROM assets_service_dependency_type WHERE id=.*FOR UPDATE").WithArgs(int64(1)).WillReturnRows(dependencyTypeRows(true))
			expectDependencyContext(mock, 1, 10, 3, 4, false)
			expectDependencyContext(mock, 2, 20, 3, 4, false)
			expectDependencyInsert(mock, []driver.Value{sqlmock.AnyArg(), sqlmock.AnyArg(), int64(2), int64(1), int64(20), int64(10), int64(3), int64(4), int64(1), "strong", "HTTPS", "read", "", ""}, 11, nil)
			var reverseErr error
			if failReverse {
				reverseErr = &mysql.MySQLError{Number: 1062, Message: "assets_dependency_scope_uniq"}
			}
			expectDependencyInsert(mock, []driver.Value{sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1), int64(2), int64(10), int64(20), int64(3), int64(4), int64(1), "weak", "HTTPS", "notify", "", "retry"}, 12, reverseErr)
			if failReverse {
				mock.ExpectRollback()
			} else {
				mock.ExpectQuery("FROM assets_service_dependency d").WithArgs(int64(11)).WillReturnRows(dependencyGetRows(11, 2, 1))
				mock.ExpectQuery("FROM assets_service_dependency d").WithArgs(int64(12)).WillReturnRows(dependencyGetRows(12, 1, 2))
				mock.ExpectCommit()
			}
			rows, err := (&Service{repository: NewRepository(pool)}).SaveServiceDependency(context.Background(), 0, ServiceDependencyInput{
				SourceServiceID: 2, TargetServiceID: 1, DependencyDetailsInput: DependencyDetailsInput{InteractionTypeID: 1, Strength: "strong", Protocol: "HTTPS", FunctionScope: "read"},
				Reverse: &DependencyDetailsInput{InteractionTypeID: 1, Strength: "weak", Protocol: "HTTPS", FunctionScope: "notify", FallbackPlan: "retry"},
			})
			if failReverse {
				if !errors.Is(err, ErrDependencyDuplicate) {
					t.Fatalf("expected duplicate, got %v", err)
				}
			} else if err != nil || len(rows) != 2 {
				t.Fatalf("rows %v err %v", rows, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDependencyWriteRejectsUnavailableScope(t *testing.T) {
	for _, mode := range []string{"disabled_type", "deleting_service", "different_project", "different_environment"} {
		t.Run(mode, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("FROM assets_service_dependency_type WHERE id=.*FOR UPDATE").WithArgs(int64(1)).WillReturnRows(dependencyTypeRows(mode != "disabled_type"))
			if mode != "disabled_type" {
				expectDependencyContext(mock, 1, 10, 3, 4, mode == "deleting_service")
				if mode != "deleting_service" {
					project, environment := int64(3), int64(4)
					if mode == "different_project" {
						project = 5
					}
					if mode == "different_environment" {
						environment = 6
					}
					expectDependencyContext(mock, 2, 20, project, environment, false)
				}
			}
			mock.ExpectRollback()
			_, err = (&Service{repository: NewRepository(pool)}).SaveServiceDependency(context.Background(), 0, ServiceDependencyInput{SourceServiceID: 1, TargetServiceID: 2, DependencyDetailsInput: DependencyDetailsInput{InteractionTypeID: 1, FunctionScope: "read"}})
			if err == nil {
				t.Fatal("write should be rejected")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDependencyProtectsServiceBeforeTombstone(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM assets_application_service WHERE").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_service_dependency").WithArgs(int64(7), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	exists, err := NewRepository(pool).beginServiceDeletion(context.Background(), 7)
	if exists || !errors.Is(err, ErrServiceHasDependencies) {
		t.Fatalf("exists %v err %v", exists, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyTopologySnapshots(t *testing.T) {
	now := time.Now().UTC()
	nodes := []db.ListDependencyTopologyNodesRow{{ID: 1, BusinessSystemID: 10}, {ID: 2, BusinessSystemID: 20}, {ID: 3, BusinessSystemID: 10}}
	edges := []db.ListServiceDependenciesRow{{SourceServiceID: 1, TargetServiceID: 2}}
	instances := []db.ListDependencyTopologyInstancesRow{
		{ServiceID: 1, ID: 11, RuntimeStatus: "running", LastStatusCheckTime: sql.NullTime{Time: now, Valid: true}},
		{ServiceID: 1, ID: 12, RuntimeStatus: "stopped", LastStatusCheckTime: sql.NullTime{Time: now, Valid: true}},
		{ServiceID: 1, ID: 13, RuntimeStatus: "error", LastStatusCheckTime: sql.NullTime{Time: now, Valid: true}},
		{ServiceID: 1, ID: 14, RuntimeStatus: "running"},
		{ServiceID: 2, ID: 11, RuntimeStatus: "running", LastStatusCheckTime: sql.NullTime{Time: now, Valid: true}},
	}
	result := assembleDependencyTopology(nodes, edges, instances, DependencyTopologyScope{ServiceID: 1})
	if len(result) != 2 || result[0].ExternalToSelection || !result[1].ExternalToSelection {
		t.Fatalf("boundary nodes %+v", result)
	}
	if result[0].RuntimeSummary != (DependencyRuntimeSummary{Total: 4, Running: 1, Stopped: 1, Error: 1, Unknown: 1}) {
		t.Fatalf("summary %+v", result[0].RuntimeSummary)
	}
	if result[0].Instances[3].LastStatusCheckTime != nil || result[0].Instances[3].RuntimeStatus != "unknown" {
		t.Fatal("unchecked instance must be unknown")
	}
	if result[1].RuntimeSummary.Running != 1 {
		t.Fatal("shared instance should be represented per service")
	}
	all := assembleDependencyTopology(nodes, nil, nil, DependencyTopologyScope{})
	if len(all) != 3 || all[2].Instances == nil || all[2].RuntimeSummary.Total != 0 {
		t.Fatal("isolated services and empty arrays must survive")
	}
}

func TestDependencyTypeDeleteReferenced(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("FROM assets_service_dependency_type WHERE id=.*FOR UPDATE").WithArgs(int64(1)).WillReturnRows(dependencyTypeRows(false))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_service_dependency WHERE interaction_type_id=").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectRollback()
	if err := (&Service{repository: NewRepository(pool)}).DeleteServiceDependencyType(context.Background(), 1); err == nil {
		t.Fatal("referenced dictionary type deleted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyUpdateKeepsDisabledTypeAndOnlyOneDirection(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("FROM assets_service_dependency WHERE id=.*FOR UPDATE").WithArgs(int64(11)).WillReturnRows(dependencyRow(11, 1, 2))
	mock.ExpectQuery("FROM assets_service_dependency_type WHERE id=.*FOR UPDATE").WithArgs(int64(1)).WillReturnRows(dependencyTypeRows(false))
	expectDependencyContext(mock, 1, 10, 3, 4, false)
	expectDependencyContext(mock, 2, 20, 3, 4, false)
	mock.ExpectExec("UPDATE assets_service_dependency SET").WithArgs(
		sqlmock.AnyArg(), int64(1), int64(2), int64(10), int64(20), int64(3), int64(4), int64(1),
		"weak", "HTTPS", "read", "slow", "retry", int64(11),
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM assets_service_dependency d").WithArgs(int64(11)).WillReturnRows(dependencyGetRows(11, 1, 2))
	mock.ExpectCommit()
	rows, err := (&Service{repository: NewRepository(pool)}).SaveServiceDependency(context.Background(), 11, ServiceDependencyInput{
		SourceServiceID: 1, TargetServiceID: 2, DependencyDetailsInput: DependencyDetailsInput{
			InteractionTypeID: 1, Strength: "weak", Protocol: "HTTPS", FunctionScope: "read", FailureImpact: "slow", FallbackPlan: "retry",
		},
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %v err %v", rows, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyDeleteOneDoesNotDeleteReverse(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("FROM assets_service_dependency WHERE id=.*FOR UPDATE").WithArgs(int64(11)).WillReturnRows(dependencyRow(11, 1, 2))
	mock.ExpectExec("DELETE FROM assets_service_dependency WHERE id=").WithArgs(int64(11)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := (&Service{repository: NewRepository(pool)}).DeleteServiceDependency(context.Background(), 11); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyTypeEditPreservesDisabledStateAndCode(t *testing.T) {
	for _, code := range []string{"http", "other"} {
		t.Run(code, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectBegin()
			mock.ExpectQuery("FROM assets_service_dependency_type WHERE id=.*FOR UPDATE").WithArgs(int64(1)).WillReturnRows(dependencyTypeRows(false))
			if code == "http" {
				mock.ExpectExec("UPDATE assets_service_dependency_type SET").
					WithArgs(sqlmock.AnyArg(), "http", "服务调用", false, "description", int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery("FROM assets_service_dependency_type WHERE id=").WithArgs(int64(1)).WillReturnRows(dependencyTypeRows(false))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			row, err := (&Service{repository: NewRepository(pool)}).SaveServiceDependencyType(context.Background(), 1, DependencyTypeInput{Code: code, Name: "服务调用", Description: "description"})
			if code == "http" {
				if err != nil || row.Enabled {
					t.Fatalf("row %+v err %v", row, err)
				}
			} else if err == nil {
				t.Fatal("stable type code changed")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
