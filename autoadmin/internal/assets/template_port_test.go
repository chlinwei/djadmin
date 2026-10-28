package assets

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTemplatePortJSONRoundTrip(t *testing.T) {
	for _, test := range []struct {
		input, stored, output string
	}{
		{`8080`, "8080", `8080`},
		{`"8080"`, "8080", `8080`},
		{`"${HTTP_PORT}"`, "${HTTP_PORT}", `"${HTTP_PORT}"`},
		{`" ${HTTP_PORT} "`, "${HTTP_PORT}", `"${HTTP_PORT}"`},
	} {
		t.Run(test.input, func(t *testing.T) {
			var value TemplatePortValue
			if err := json.Unmarshal([]byte(test.input), &value); err != nil {
				t.Fatal(err)
			}
			if string(value) != test.stored {
				t.Fatalf("stored %q, want %q", value, test.stored)
			}
			raw, err := json.Marshal(value)
			if err != nil || string(raw) != test.output {
				t.Fatalf("output %s want %s err=%v", raw, test.output, err)
			}
		})
	}
}

func TestTemplatePortValidation(t *testing.T) {
	for _, value := range []string{"1", "65535", "${PORT}", "${http_port_2}", " 8080 "} {
		ports := []TemplatePortInput{{Port: TemplatePortValue(value)}}
		if err := validateTemplatePorts(&ports); err != nil {
			t.Errorf("valid port %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "0", "-1", "65536", "1.5", "abc", "${PORT", "${1PORT}", "80${PORT}", "00080"} {
		t.Run(value, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			ports := []TemplatePortInput{{Port: TemplatePortValue(value)}}
			for _, id := range []int64{0, 2} {
				if _, err := NewRepository(pool).SaveDeploymentTemplate(context.Background(), id,
					DeploymentTemplateInput{Ports: &ports}); err == nil {
					t.Fatalf("invalid port %q saved", value)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTemplatePortSaveAndReadPreservesMacro(t *testing.T) {
	for _, creating := range []bool{false, true} {
		t.Run(map[bool]string{false: "edit", true: "create"}[creating], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			id := int64(2)
			mock.ExpectBegin()
			if creating {
				id = 0
				expectTemplatePortCreate(mock, 2)
			} else {
				mock.ExpectExec("UPDATE assets_application_deployment_template").
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec("DELETE FROM assets_application_port").WithArgs(int64(2)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO assets_application_port").
				WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, "HTTP", "tcp", "0.0.0.0",
					"${HTTP_PORT}", true, false, true, int64(2)).
				WillReturnResult(sqlmock.NewResult(3, 1))
			mock.ExpectCommit()
			ports := []TemplatePortInput{{Name: "HTTP", Protocol: "tcp", BindAddress: "0.0.0.0", Port: "${HTTP_PORT}"}}
			repository := NewRepository(pool)
			saved, err := repository.SaveDeploymentTemplate(context.Background(), id,
				DeploymentTemplateInput{Application: 1, Name: "Template", Ports: &ports})
			if err != nil || saved != 2 {
				t.Fatalf("save: id=%d err=%v", saved, err)
			}
			mock.ExpectQuery("FROM assets_application_port WHERE").WithArgs(int64(2)).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "create_time", "update_time", "remark", "name", "protocol", "bind_address",
					"port", "required", "external_access", "check_enabled",
				}).AddRow(3, testRuleUpdatedAt, testRuleUpdatedAt, nil, "HTTP", "tcp", "0.0.0.0",
					"${HTTP_PORT}", true, false, true))
			rows, err := repository.ListTemplatePorts(context.Background(), 2)
			if err != nil || len(rows) != 1 || rows[0].Port != "${HTTP_PORT}" {
				t.Fatalf("macro was lost on read: rows=%+v err=%v", rows, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
