package assets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestResolveConfiguredPort(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		values           map[string]string
		want, problem    string
	}{
		{"literal", "8080", nil, "8080", ""},
		{"service value", "${PORT}", map[string]string{"PORT": "1414"}, "1414", ""},
		{"nested", "${PORT}", map[string]string{"PORT": "${BASE}", "BASE": "1415"}, "1415", ""},
		{"missing", "${PORT}", nil, "${PORT}", "未定义"},
		{"cycle", "${PORT}", map[string]string{"PORT": "${OTHER}", "OTHER": "${PORT}"}, "${PORT}", "循环引用"},
		{"empty", "${PORT}", map[string]string{"PORT": ""}, "", "1–65535"},
		{"out of range", "${PORT}", map[string]string{"PORT": "65536"}, "65536", "1–65535"},
		{"noninteger", "${PORT}", map[string]string{"PORT": "abc"}, "abc", "1–65535"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, problem := resolveConfiguredPort(test.expression, test.values)
			if got != test.want || (test.problem == "" && problem != "") ||
				(test.problem != "" && !strings.Contains(problem, test.problem)) {
				t.Fatalf("got=%q problem=%q", got, problem)
			}
		})
	}
}

func TestServicePortsResolveTemplateServiceAndInstanceValues(t *testing.T) {
	for _, test := range []struct {
		name                                   string
		service, firstInstance, secondInstance string
		want                                   string
		wantInstances                          bool
	}{
		{"template default", `{}`, `{}`, `{}`, "1414", false},
		{"service override", `{"PORT":"1515"}`, `{}`, `{}`, "1515", false},
		{"instance override", `{"PORT":"1515"}`, `{"PORT":"1616"}`, `{}`, "1515", true},
		{"invalid instance", `{"PORT":"1515"}`, `{"PORT":"70000"}`, `{}`, "1515", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectQuery("SELECT t.macro_definitions, t.app_home").WithArgs(int64(21)).
				WillReturnRows(sqlmock.NewRows([]string{
					"macro_definitions", "app_home", "deployment_id", "instance_name", "runtime_variables",
				}).AddRow([]byte(`[{"name":"PORT","value":"1414"}]`), "/app", 31, "instance-a", []byte(test.firstInstance)).
					AddRow([]byte(`[{"name":"PORT","value":"1414"}]`), "/app", 32, "instance-b", []byte(test.secondInstance)))
			ports := []TemplatePort{{Port: "${PORT}"}}
			err = NewRepository(pool).resolveServicePorts(context.Background(), ApplicationService{
				ID: 21, MacroValues: json.RawMessage(test.service),
			}, ports)
			if err != nil || ports[0].Port != "${PORT}" || ports[0].ResolvedPort != test.want {
				t.Fatalf("ports=%+v err=%v", ports, err)
			}
			if test.wantInstances {
				if len(ports[0].InstancePorts) != 2 || ports[0].InstancePorts[1].ResolvedPort != test.want {
					t.Fatalf("all instance outcomes must be shown: %+v", ports)
				}
				if test.name == "invalid instance" && ports[0].InstancePorts[0].ResolutionError == "" {
					t.Fatal("invalid instance port must be explicitly reported")
				}
			} else if len(ports[0].InstancePorts) != 0 {
				t.Fatal("identical inherited values must not produce redundant instance rows")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServicePortContextFailureIsNotHidden(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT t.macro_definitions, t.app_home").
		WillReturnError(context.DeadlineExceeded)
	err = NewRepository(pool).resolveServicePorts(context.Background(), ApplicationService{ID: 21},
		[]TemplatePort{{Port: "${PORT}"}})
	if err != context.DeadlineExceeded {
		t.Fatalf("error must propagate: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
