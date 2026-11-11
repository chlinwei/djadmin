package assets

import (
	"encoding/json"
	"testing"
)

func TestValidateDeploymentTemplateCommandActions(t *testing.T) {
	for _, creating := range []bool{true, false} {
		for _, test := range []struct {
			name    string
			actions []TemplateControlActionInput
			valid   bool
		}{
			{name: "status only", actions: []TemplateControlActionInput{{Action: "status", Command: "status"}}, valid: true},
			{name: "start and status", actions: []TemplateControlActionInput{{Action: "start", Command: "start"}, {Action: "status", Command: "status"}}, valid: true},
			{name: "stop and status", actions: []TemplateControlActionInput{{Action: "stop", Command: "stop"}, {Action: "status", Command: "status"}}, valid: true},
			{name: "all commands", actions: []TemplateControlActionInput{{Action: "start", Command: "start"}, {Action: "stop", Command: "stop"}, {Action: "status", Command: "status"}}, valid: true},
			{name: "missing status", actions: []TemplateControlActionInput{{Action: "start", Command: "start"}, {Action: "stop", Command: "stop"}}},
			{name: "empty actions", actions: []TemplateControlActionInput{}},
			{name: "empty status", actions: []TemplateControlActionInput{{Action: "status"}}},
			{name: "duplicate status", actions: []TemplateControlActionInput{{Action: "status", Command: "status"}, {Action: "status", Command: "status"}}},
		} {
			mode := "edit"
			if creating {
				mode = "create"
			}
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				input := DeploymentTemplateInput{Application: 1, Name: "template", ControlType: "command", MacroDefinitions: json.RawMessage(`[]`), ControlActions: &test.actions}
				if err := validateDeploymentTemplate(input, creating); (err == nil) != test.valid {
					t.Fatalf("valid=%v, got error %v", test.valid, err)
				}
			})
		}
	}
}

func TestValidateDeploymentTemplateRequiresDockerContainer(t *testing.T) {
	input := DeploymentTemplateInput{Application: 1, Name: "template", ControlType: "docker", MacroDefinitions: json.RawMessage(`[]`), DockerConfig: &DockerConfigInput{}}
	if err := validateDeploymentTemplate(input, true); err == nil {
		t.Fatal("expected docker template without container name to be rejected")
	}
}
