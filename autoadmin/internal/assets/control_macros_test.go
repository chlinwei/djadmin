package assets

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDeploymentControlMacroPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, service, instance, user string
	}{
		{"template", `{}`, `{}`, "template-user"},
		{"service", `{"AOS_APP_USER":"service-user"}`, `{}`, "service-user"},
		{"instance", `{"AOS_APP_USER":"service-user"}`, `{"AOS_APP_USER":"instance-user"}`, "instance-user"},
	} {
		t.Run(test.name, func(t *testing.T) {
			params, err := deploymentControlParams(db.ListDeploymentControlContextsRow{
				ControlType: "command", RunUser: "${AOS_APP_USER}",
				AppHome: "${BASE}/app", WorkDirectory: "${APP_HOME}/bin",
				MacroDefinitions: json.RawMessage(`[{"name":"AOS_APP_USER","value":"template-user"},{"name":"BASE","value":"/opt"}]`),
				MacroValues:      json.RawMessage(test.service), RuntimeVariables: json.RawMessage(test.instance),
			}, []db.ListDeploymentControlActionsRow{{
				Action: "status", Command: `test -f "${APP_HOME}/pid" && echo "${AOS_APP_USER}:${HOME}"`,
				SuccessExitCodes: json.RawMessage(`[0]`),
			}})
			if err != nil {
				t.Fatal(err)
			}
			if params["run_user"] != test.user || params["app_home"] != "/opt/app" || params["work_directory"] != "/opt/app/bin" {
				t.Fatalf("unexpected resolved parameters: %+v", params)
			}
			command := params["control_actions"].(map[string]any)["status"].(map[string]any)["command"]
			want := `test -f "/opt/app/pid" && echo "` + test.user + `:${HOME}"`
			if command != want {
				t.Fatalf("got command %q, want %q", command, want)
			}
		})
	}
}

func TestDeploymentControlInvalidMacros(t *testing.T) {
	for _, test := range []struct{ name, definitions, user string }{
		{"undefined-user", `[]`, "${MISSING}"},
		{"empty-user", `[{"name":"USER","value":""}]`, "${USER}"},
		{"cycle", `[{"name":"A","value":"${B}"},{"name":"B","value":"${A}"}]`, "${A}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := deploymentControlParams(db.ListDeploymentControlContextsRow{
				RunUser: test.user, MacroDefinitions: json.RawMessage(test.definitions),
			}, nil)
			if err == nil {
				t.Fatal("expected explicit macro error before sending to Agent")
			}
		})
	}
}

func TestDeploymentControlRejectsDifferentResolvedTargets(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM assets_application_deployment").
		WithArgs(int64(20)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(20)))
	contexts := deploymentControlRows()
	for _, user := range []string{"service-a", "service-b"} {
		contexts.AddRow("host", int64(10), "command", "${USER}", "", "", "", "system",
			[]byte(`[{"name":"USER","value":"default-user"}]`), "instance",
			[]byte(`{"USER":"`+user+`"}`), []byte(`{}`))
	}
	mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(20)).WillReturnRows(contexts)
	mock.ExpectQuery("SELECT action,command,timeout_seconds,success_exit_codes").
		WithArgs(int64(10)).WillReturnRows(sqlmock.NewRows([]string{"action", "command", "timeout_seconds", "success_exit_codes"}))
	mock.ExpectRollback()
	_, _, err = NewRepository(database).deploymentControl(context.Background(), 20)
	if !errors.Is(err, ErrDeploymentControlConflict) {
		t.Fatalf("expected resolved-control conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentControlPreservesLiteralConfiguration(t *testing.T) {
	params, err := deploymentControlParams(db.ListDeploymentControlContextsRow{
		RunUser: "application", ControlType: "systemd", ServiceName: "artemis", SystemdScope: "system",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if params["run_user"] != "application" || params["service_name"] != "artemis" {
		t.Fatalf("literal configuration changed: %+v", params)
	}
	_, err = deploymentControlParams(db.ListDeploymentControlContextsRow{}, []db.ListDeploymentControlActionsRow{
		{Action: "status", SuccessExitCodes: json.RawMessage(`invalid`)},
	})
	if err == nil || !strings.Contains(err.Error(), "success exit codes") {
		t.Fatalf("invalid exit codes must report an error, got %v", err)
	}
}
