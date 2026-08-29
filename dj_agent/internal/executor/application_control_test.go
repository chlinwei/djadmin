package executor

import (
	"context"
	"os"
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
	if len(command.Args) != 3 || command.Args[0] != "/bin/bash" || command.Args[1] != "-lc" || command.Args[2] != "systemctl --user start tomcat" {
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
	if os.Geteuid() != 0 {
		if err == nil || command != nil {
			t.Fatalf("non-root Docker control must be rejected: command=%#v error=%v", command, err)
		}
		return
	}
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

func TestControlApplication_ExternalHAStatusRunsConfiguredCommand(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("failed to get current user: %v", err)
	}
	executor := New(2 * time.Second)
	result, err := executor.Run(context.Background(), protocol.Job{
		JobID: "external-ha-status", Type: protocol.TaskTypeCustom, Action: actionControlApplication,
		Params: map[string]any{
			"control_type": "external_ha", "control_action": "status", "run_user": currentUser.Username,
			"control_actions": map[string]any{
				"status": map[string]any{"command": "printf running"},
			},
		},
	})
	if err != nil {
		t.Fatalf("external HA status returned transport error: %v", err)
	}
	if result.Status != protocol.StatusSuccess || result.Stdout != "running" || result.ExitCode != 0 {
		t.Fatalf("external HA status must execute configured command: %#v", result)
	}
}
