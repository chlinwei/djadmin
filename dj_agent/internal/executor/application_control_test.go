package executor

import (
	"context"
	"os/exec"
	"os/user"
	"testing"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

func TestApplicationSystemdActionCommand_UserScope(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("failed to get current user: %v", err)
	}
	command, err := applicationSystemdActionCommand(context.Background(), map[string]any{
		"service_name": "tomcat", "systemd_scope": "user", "run_user": currentUser.Username,
	}, "start")
	if err != nil {
		t.Fatalf("build systemd start command: %v", err)
	}
	if len(command.Args) != 4 || command.Args[1] != "--user" || command.Args[2] != "start" || command.Args[3] != "tomcat" {
		t.Fatalf("unexpected systemd command: %#v", command.Args)
	}
}

func TestControlApplication_RejectsUnknownAction(t *testing.T) {
	executor := New(2 * time.Second)
	result, err := executor.Run(context.Background(), protocol.Job{
		JobID: "application-control-invalid", Type: protocol.TaskTypeCustom, Action: actionControlApplication,
		Params: map[string]any{"control_type": "systemd", "control_action": "restart"},
	})
	if err != nil {
		t.Fatalf("builtin action should return a structured result: %v", err)
	}
	if result.Status != protocol.StatusFailed || result.Error == "" {
		t.Fatalf("unknown action must fail: %#v", result)
	}
}

func TestApplicationDockerControlCommand_Status(t *testing.T) {
	command, err := applicationDockerControlCommand(context.Background(), map[string]any{
		"docker_config": map[string]any{"container_name": "order-api"},
	}, "status")
	if err != nil {
		t.Fatalf("build docker status command: %v", err)
	}
	if len(command.Args) != 5 || command.Args[1] != "inspect" || command.Args[4] != "order-api" {
		t.Fatalf("unexpected docker status command: %#v", command.Args)
	}
}

func TestApplicationControlResult_StatusPreservesNonzeroState(t *testing.T) {
	job := protocol.Job{JobID: "application-status", Type: protocol.TaskTypeCustom, Action: actionControlApplication}
	output, commandErr := exec.Command("/bin/sh", "-c", "printf inactive; exit 3").CombinedOutput()
	result := applicationControlResult(job, "status", time.Now(), output, commandErr)
	if result.Status != protocol.StatusSuccess || result.Stdout != "inactive" {
		t.Fatalf("inactive must be returned as an application state: %#v", result)
	}
}

func TestControlApplication_ExternalHAStatusReturnsManagementHint(t *testing.T) {
	executor := New(2 * time.Second)
	result, err := executor.Run(context.Background(), protocol.Job{
		JobID: "external-ha-status", Type: protocol.TaskTypeCustom, Action: actionControlApplication,
		Params: map[string]any{"control_type": "external_ha", "control_action": "status"},
	})
	if err != nil {
		t.Fatalf("external HA status returned transport error: %v", err)
	}
	if result.Status != protocol.StatusSuccess || result.Stdout == "" {
		t.Fatalf("external HA status must return a management hint: %#v", result)
	}
}
