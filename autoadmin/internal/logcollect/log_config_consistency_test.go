package logcollect

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/assets"
	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectServiceConflictTargets(mock sqlmock.Sqlmock, serviceID int64) {
	columns := []string{"host_id", "host_ip", "host_instance_name", "target_id",
		"config_fingerprint", "runtime_status", "service_fingerprints", "service_id", "service_code"}
	mock.ExpectQuery("FROM assets_application_service_deployment sd").
		WithArgs(serviceID).WillReturnRows(sqlmock.NewRows(columns).
		AddRow(42, "10.0.0.42", "host-42", nil, "", "", []byte(`{}`), serviceID, "new-service"))
	mock.ExpectQuery("FROM monitor_shared_log_source_service").
		WithArgs(serviceID).WillReturnRows(sqlmock.NewRows(columns))
}

func expectServiceConflictInputs(mock sqlmock.Sqlmock, secondPath string) {
	mock.ExpectQuery("FROM assets_application_service_deployment").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "service_id", "service_code", "instance_name",
			"runtime_variables", "app_home", "host_ip"}).
			AddRow(42, 10, "existing-service", "existing-instance", []byte(`{}`), "/data/app", "10.0.0.42").
			AddRow(42, 20, "new-service", "new-instance", []byte(`{}`), "/data/app", "10.0.0.42"))
	mock.ExpectQuery("FROM assets_application_service s").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "project_code", "environment_code", "business_system_code",
			"service_code", "service_id", "application_code", "tier_code", "pipeline_name",
			"log_name", "path_pattern", "macro_values", "macro_definitions", "multiline_enabled",
			"start_pattern", "flush_timeout", "filter_include_rule_id", "filter_exclude_rule_id",
			"collection_filter_rule_id", "collection_exclude_filter_rule_id", "extra_processors"}).
			AddRow(42, "p", "prod", "b", "existing-service", 10, "app", "std", "rule",
				"app.log", "/data/app/logs/app.log", []byte(`{}`), []byte(`[]`), false, "", 2000,
				nil, nil, nil, nil, []byte(`[]`)).
			AddRow(42, "p", "prod", "b", "new-service", 20, "app", "std", "rule",
				"other-name.log", secondPath, []byte(`{}`), []byte(`[]`), false, "", 2000,
				nil, nil, nil, nil, []byte(`[]`)))
}

func requireServicePathConflict(t *testing.T, err error) {
	t.Helper()
	var businessError *apperror.Error
	if !errors.As(err, &businessError) || businessError.Code() != apperror.CodeInvalidArgument {
		t.Fatalf("want invalid-argument business error, got %v", err)
	}
	for _, text := range []string{"已拒绝保存", "主机 42", "existing-service", "new-service",
		"existing-instance", "new-instance", "/data/app/logs/app.log"} {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("conflict error missing %q: %v", text, err)
		}
	}
}

func TestCreateServiceRejectsExistingHostLogPath(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM assets_business_system WHERE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM assets_service_deletion").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("FROM monitor_log_retention_tier").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "code", "name",
			"daily_size_gb", "retention_value", "retention_unit", "rollover_min_index_age",
			"enabled", "is_default", "remark"}).
			AddRow(3, time.Now(), time.Now(), "std", "Standard", 1, 30, "d", "1d", true, false, ""))
	expectCreatedConflictService(mock, 20)
	mock.ExpectExec("DELETE FROM assets_application_service_deployment").
		WithArgs(int64(20)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO assets_application_service_deployment").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT deployment_id FROM assets_application_service_deployment").
		WithArgs(int64(20)).WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(21))
	mock.ExpectQuery("SELECT id FROM assets_application_deployment WHERE").
		WithArgs(int64(21)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(21))
	mock.ExpectQuery("name: ListDeploymentControlContexts").WithArgs(int64(21)).WillReturnRows(
		sqlmock.NewRows([]string{"instance_name", "template_id", "control_type", "run_user",
			"work_directory", "app_home", "service_name", "systemd_scope", "macro_definitions",
			"deployment_instance_name", "macro_values", "runtime_variables"}).
			AddRow("host-42", 22, "systemd", "", "", "/data/app", "app", "system",
				[]byte(`[]`), "new-instance", []byte(`{}`), []byte(`{}`)))
	expectServiceConflictTargets(mock, 20)
	expectServiceConflictInputs(mock, "${APP_HOME}/logs/app.log")
	mock.ExpectRollback()

	handler := &Handler{db: database}
	tier := int64(3)
	enabled := true
	id, err := assets.NewRepository(database).SaveApplicationService(context.Background(), 0,
		assets.ApplicationServiceInput{
			Name: "New service", Code: "new-service", Application: 1, BusinessSystem: 1,
			ApplicationVersion: 1, DeploymentTemplate: 22, LogRetentionTier: &tier,
			LogCollectionEnabled: &enabled,
			MemberConfigs:        &[]assets.ServiceMemberInput{{Deployment: 21}},
		}, handler.CheckServiceLogConfigConsistency)
	requireServicePathConflict(t, err)
	if id != 0 {
		t.Fatalf("rejected creation must not return a saved ID: %d", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateLogAdditionChecksExistingHostPaths(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name, path := "distinct-path", "/data/app/logs/other.log"
		if conflict {
			name, path = "conflicting-path", "${APP_HOME}/logs/app.log"
		}
		t.Run(name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE assets_application_deployment_template").
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("FROM assets_application_log_definition").
				WithArgs(int64(22)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectExec("INSERT INTO assets_application_log_definition").
				WillReturnResult(sqlmock.NewResult(30, 1))
			mock.ExpectQuery("WHERE s.deployment_template_id").WithArgs(int64(22)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "enabled",
					"business_system_id", "environment_id", "project_name", "business_system_name", "environment_name"}).
					AddRow(20, "New service", "new-service", true, 1, 1, "Project", "Business", "Production"))
			expectServiceConflictTargets(mock, 20)
			expectServiceConflictInputs(mock, path)
			if conflict {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			handler := &Handler{db: database}
			id, err := assets.NewRepository(database).SaveDeploymentTemplate(context.Background(), 22,
				assets.DeploymentTemplateInput{
					Name: "Template", ControlType: "systemd", AppHome: "/data/app",
					Logs: &[]assets.TemplateLogInput{{Name: "other-name.log", PathPattern: path}},
				}, handler.CheckTemplateLogConfigConsistency)
			if conflict {
				requireServicePathConflict(t, err)
				if id != 0 {
					t.Fatalf("rejected template save returned ID %d", id)
				}
			} else if err != nil || id != 22 {
				t.Fatalf("distinct path must save: id=%d err=%v", id, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLogConfigConflictRetainsEachHost(t *testing.T) {
	set := renderInputSet{
		Entries: []hostLogRenderInput{
			{Application: "app", Service: "a", ServiceID: 1, Tier: "std", Pipeline: "rule",
				LogName: "app.log", ResolvedPath: "/logs/app.log"},
			{Application: "app", Service: "b", ServiceID: 2, Tier: "std", Pipeline: "rule",
				LogName: "app.log", ResolvedPath: "/logs/app.log"},
		},
		Instances: []hostInstanceInput{{ServiceID: 1, Instance: "a-1"}, {ServiceID: 2, Instance: "b-1"}},
	}

	problems := collectLogConfigProblems(map[int64]renderInputSet{42: set, 43: set}, []int64{43, 42, 42})
	if len(problems) != 2 || !strings.Contains(problems[0], "主机 42") || !strings.Contains(problems[1], "主机 43") {
		t.Fatalf("each affected host must appear once in stable order: %v", problems)
	}
	problems = collectLogConfigProblems(map[int64]renderInputSet{
		42: {Entries: set.Entries[:1], Instances: set.Instances[:1]},
		43: {Entries: set.Entries[1:], Instances: set.Instances[1:]},
	}, []int64{42, 43})
	if len(problems) != 0 {
		t.Fatalf("same path on different hosts must be allowed: %v", problems)
	}
}

func TestReenableServiceCollectionRejectsExistingHostLogPath(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, err := assets.NewService(assets.NewRepository(database), "", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	service.SetLogConfigConsistencyChecker(&Handler{db: database})
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE assets_application_service").
		WithArgs(sqlmock.AnyArg(), true, int64(20)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectServiceConflictTargets(mock, 20)
	expectServiceConflictInputs(mock, "${APP_HOME}/logs/app.log")
	mock.ExpectRollback()
	enabled, err := service.SetServiceLogCollection(context.Background(), 20, true)
	requireServicePathConflict(t, err)
	if enabled {
		t.Fatal("rejected re-enable must not report collection enabled")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReenableLogOverridesRejectExistingHostLogPath(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "batch"}[batch], func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO assets_application_service_log_setting").
				WillReturnResult(sqlmock.NewResult(0, 1))
			expectServiceConflictTargets(mock, 20)
			expectServiceConflictInputs(mock, "${APP_HOME}/logs/app.log")
			mock.ExpectRollback()
			repository, handler := assets.NewRepository(database), &Handler{db: database}
			enabled := true
			input := assets.ServiceLogOverrideInput{LogDefinition: 30, CollectionEnabled: &enabled}
			if batch {
				err = repository.UpsertServiceLogOverrides(context.Background(), 20,
					[]assets.ServiceLogOverrideInput{input}, handler.CheckServiceLogConfigConsistency)
			} else {
				err = repository.UpsertServiceLogOverride(context.Background(), 20, input,
					handler.CheckServiceLogConfigConsistency)
			}
			requireServicePathConflict(t, err)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
