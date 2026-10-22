package assets

import (
	"context"
	"errors"
	"testing"

	"autoadmin/internal/agent/pb"

	"github.com/DATA-DOG/go-sqlmock"
)

func deploymentControlRows(templates ...int64) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"instance_name", "template_id", "control_type", "run_user", "work_directory",
		"app_home", "service_name", "systemd_scope", "macro_definitions", "deployment_instance_name", "macro_values", "runtime_variables",
	})
	for _, template := range templates {
		rows.AddRow("host-105", template, "systemd", "root", "", "", "artemis", "system", []byte(`[]`), "artemis-105", []byte(`{}`), []byte(`{}`))
	}
	return rows
}

func expectDeploymentControlRows(mock sqlmock.Sqlmock, templates ...int64) {
	mock.ExpectQuery("SELECT id FROM assets_application_deployment").
		WithArgs(int64(20)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(20)))
	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(int64(20)).WillReturnRows(deploymentControlRows(templates...))
}

func TestDeploymentControlRejectsConflictingTemplates(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectBegin()
	expectDeploymentControlRows(mock, 16, 17, 10)
	mock.ExpectRollback()
	_, _, err = NewRepository(database).deploymentControl(context.Background(), 20)
	if !errors.Is(err, ErrDeploymentTemplateConflict) {
		t.Fatalf("expected explicit template conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentControlAllowsSharedIdenticalTemplate(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectBegin()
	expectDeploymentControlRows(mock, 10, 10)
	mock.ExpectQuery("SELECT action,command,timeout_seconds,success_exit_codes").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"action", "command", "timeout_seconds", "success_exit_codes"}))
	mock.ExpectCommit()
	host, params, err := NewRepository(database).deploymentControl(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if host != "host-105" || params["service_name"] != "artemis" {
		t.Fatalf("unexpected control context: %s %+v", host, params)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveServiceValidatesDeploymentTemplateWithinTransaction(t *testing.T) {
	for _, replaceMembers := range []bool{false, true} {
		for _, conflict := range []bool{false, true} {
			t.Run(map[bool]string{false: "template-edit", true: "member-edit"}[replaceMembers]+
				map[bool]string{false: "/shared", true: "/conflict"}[conflict], func(t *testing.T) {
				database, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer database.Close()
				mock.ExpectBegin()
				expectServiceSaveFence(mock, 18)
				mock.ExpectExec("UPDATE assets_application_service").WillReturnResult(sqlmock.NewResult(0, 1))
				input := ApplicationServiceInput{DeploymentTemplate: 10}
				if replaceMembers {
					members := []ServiceMemberInput{{Deployment: 20}}
					input.MemberConfigs = &members
					mock.ExpectExec("DELETE FROM assets_application_service_deployment").WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("INSERT INTO assets_application_service_deployment").WillReturnResult(sqlmock.NewResult(1, 1))
				}
				mock.ExpectQuery("SELECT deployment_id FROM assets_application_service_deployment").
					WithArgs(int64(18)).
					WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}).AddRow(int64(20)))
				otherTemplate := int64(10)
				if conflict {
					otherTemplate = 16
				}
				expectDeploymentControlRows(mock, 10, otherTemplate)
				if conflict {
					mock.ExpectRollback()
				} else {
					mock.ExpectCommit()
				}
				_, err = NewRepository(database).SaveApplicationService(context.Background(), 18, input, nil)
				if conflict && !errors.Is(err, ErrDeploymentTemplateConflict) {
					t.Fatalf("expected template conflict, got %v", err)
				}
				if !conflict && err != nil {
					t.Fatal(err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestDeploymentSystemdRuntimeStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		status string
		code   int32
		output string
		want   string
	}{
		{"active", "success", 0, "active\n", "running"},
		{"inactive", "success", 3, "inactive\n", "stopped"},
		{"failed-unit", "success", 3, "failed\n", "stopped"},
		{"missing-unit", "success", 4, "unknown\n", "error"},
		{"bus-error", "success", 1, "", "error"},
		{"permission-error", "success", 3, "Failed to connect to bus", "error"},
		{"execution-failed", "failed", 0, "", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := deploymentControlRuntimeStatus("status", "systemd", &pb.AutomationExecuteResponse{
				Status: test.status, ExitCode: test.code, Stdout: test.output,
			})
			if got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}

		})
	}
}

func TestDeploymentDuplicateNameError(t *testing.T) {
	err := duplicateError(errors.New("duplicate key violates unique constraint unique_host_application_instance"))
	if !errors.Is(err, ErrDeploymentNameDuplicate) {
		t.Fatalf("expected actionable deployment name error, got %v", err)
	}
}
