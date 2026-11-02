package assets

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEnabledUpdatesWriteOnlyStatus(t *testing.T) {
	for _, tt := range []struct {
		resource, table string
		identity        bool
	}{
		{"project", "assets_project", true},
		{"business-system", "assets_business_system", true},
		{"environment", "assets_business_environment", true},
		{"application", "assets_application", false},
		{"version", "assets_application_version", false},
		{"profile", "assets_cluster_profile", false},
		{"template", "assets_application_deployment_template", false},
		{"service", "assets_application_service", true},
		{"deployment", "assets_application_deployment", false},
		{"dependency-type", "assets_service_dependency_type", false},
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(tt.resource+map[bool]string{true: "/enable", false: "/disable"}[enabled], func(t *testing.T) {
				pool, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				mock.ExpectBegin()
				if tt.identity {
					mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster ORDER BY id").
						WillReturnRows(sqlmock.NewRows([]string{"id"}))
				}
				rows := sqlmock.NewRows([]string{"enabled"}).AddRow(!enabled)
				if tt.resource == "profile" {
					rows = sqlmock.NewRows([]string{"enabled", "profile_type"}).AddRow(!enabled, "custom")
				}
				mock.ExpectQuery("SELECT enabled.*FROM " + tt.table + " WHERE id = .* FOR UPDATE").WithArgs(int64(7)).WillReturnRows(rows)
				if tt.resource == "service" {
					mock.ExpectQuery("FROM assets_service_deletion").WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
				}
				mock.ExpectExec("UPDATE "+tt.table+" SET enabled = .* update_time = .* WHERE id =").
					WithArgs(enabled, sqlmock.AnyArg(), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
				service := &Service{repository: NewRepository(pool)}
				if err := service.setEnabled(context.Background(), tt.resource, 7, enabled); err != nil {
					t.Fatal(err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestEnabledUpdatesKeepProtectionAndRollback(t *testing.T) {
	for _, tt := range []struct{ name, resource, table string }{
		{"missing", "application", "assets_application"},
		{"builtin", "profile", "assets_cluster_profile"},
		{"deleting", "service", "assets_application_service"},
		{"database failure", "version", "assets_application_version"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectBegin()
			if tt.resource == "service" {
				mock.ExpectQuery("SELECT id FROM monitor_elasticsearch_cluster ORDER BY id").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			}
			query := mock.ExpectQuery("SELECT enabled.*FROM " + tt.table + " WHERE id = .* FOR UPDATE").WithArgs(int64(7))
			switch tt.name {
			case "missing":
				query.WillReturnError(sql.ErrNoRows)
			case "builtin":
				query.WillReturnRows(sqlmock.NewRows([]string{"enabled", "profile_type"}).AddRow(true, "builtin"))
			case "deleting":
				query.WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))
				mock.ExpectQuery("FROM assets_service_deletion").WithArgs(int64(7)).
					WillReturnRows(sqlmock.NewRows([]string{"service_id", "project_code", "business_system_code", "environment_code", "service_code"}).AddRow(7, "p", "b", "e", "s"))
			default:
				query.WillReturnError(errors.New("database unavailable"))
			}
			mock.ExpectRollback()
			service := &Service{repository: NewRepository(pool)}
			if err := service.setEnabled(context.Background(), tt.resource, 7, true); err == nil {
				t.Fatal("expected explicit failure")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type enabledConsistencyProbe struct{ called bool }

func (p *enabledConsistencyProbe) CheckServiceLogConfigConsistency(context.Context, db.DBTX, int64) error {
	p.called = true
	return errors.New("configuration conflict")
}
func (p *enabledConsistencyProbe) CheckTemplateLogConfigConsistency(ctx context.Context, pool db.DBTX, id int64) error {
	return p.CheckServiceLogConfigConsistency(ctx, pool, id)
}
func (p *enabledConsistencyProbe) CheckDeploymentLogConfigConsistency(ctx context.Context, pool db.DBTX, id int64) error {
	return p.CheckServiceLogConfigConsistency(ctx, pool, id)
}
func TestEnabledUpdatesValidateLogConfigurationBeforeCommit(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT enabled FROM assets_application_deployment WHERE id").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(false))
	mock.ExpectExec("UPDATE assets_application_deployment SET enabled").WithArgs(true, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	probe := &enabledConsistencyProbe{}
	service := &Service{repository: NewRepository(pool), logConfigChecker: probe}
	if err := service.setEnabled(context.Background(), "deployment", 7, true); err == nil || !probe.called {
		t.Fatalf("configuration consistency must reject and roll back: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
