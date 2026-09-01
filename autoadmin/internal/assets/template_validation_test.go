package assets

import (
	"encoding/json"
	"testing"
)

func TestValidateDeploymentTemplateRequiresCommandActions(t *testing.T) {
	actions := []TemplateControlActionInput{{Action: "start", Command: "start"}}
	input := DeploymentTemplateInput{Application: 1, Name: "template", ControlType: "command", MacroDefinitions: json.RawMessage(`[]`), ControlActions: &actions}
	if err := validateDeploymentTemplate(input, true); err == nil {
		t.Fatal("expected command template without stop/status actions to be rejected")
	}
}

func TestValidateDeploymentTemplateRequiresDockerContainer(t *testing.T) {
	input := DeploymentTemplateInput{Application: 1, Name: "template", ControlType: "docker", MacroDefinitions: json.RawMessage(`[]`), DockerConfig: &DockerConfigInput{}}
	if err := validateDeploymentTemplate(input, true); err == nil {
		t.Fatal("expected docker template without container name to be rejected")
	}
}
