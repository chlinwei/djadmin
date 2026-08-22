package executor

import (
	"context"
	"os"
	"os/user"
	"strconv"
	"testing"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

func TestCheckApplicationBaseline_PathAndCommandControl(t *testing.T) {
	tempDirectory := t.TempDir()
	if err := os.Chmod(tempDirectory, 0o700); err != nil {
		t.Fatalf("failed to set test directory mode: %v", err)
	}
	executor := New(2 * time.Second)
	job := protocol.Job{
		JobID:  "application-baseline-path",
		Type:   protocol.TaskTypeCustom,
		Action: actionCheckApplicationBaseline,
		Params: map[string]any{
			"control_type": "command",
			"paths": []any{
				map[string]any{"name": "home", "path": tempDirectory, "expected_mode": "0700"},
			},
		},
	}

	result, err := executor.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("baseline check returned transport error: %v", err)
	}
	if result.Status != protocol.StatusSuccess {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	checks, ok := result.Data["checks"].([]applicationCheckResult)
	if !ok || len(checks) != 2 {
		t.Fatalf("unexpected checks: %#v", result.Data["checks"])
	}
	if checks[0].Status != "skipped" {
		t.Fatalf("command control must be skipped, got: %#v", checks[0])
	}
	if checks[1].Status != "pass" {
		mode := ""
		if info, statErr := os.Stat(tempDirectory); statErr == nil {
			mode = info.Mode().Perm().String()
		}
		t.Fatalf("expected path check to pass (mode=%s), got: %#v", mode, checks[1])
	}
}

func TestCheckApplicationBaseline_RejectsUnsafeSystemdName(t *testing.T) {
	executor := New(2 * time.Second)
	job := protocol.Job{
		JobID:  "application-baseline-systemd",
		Type:   protocol.TaskTypeCustom,
		Action: actionCheckApplicationBaseline,
		Params: map[string]any{
			"control_type": "systemd",
			"service_name": "demo.service;touch /tmp/unsafe",
		},
	}

	result, err := executor.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("baseline check returned transport error: %v", err)
	}
	checks := result.Data["checks"].([]applicationCheckResult)
	if checks[0].Status != "error" {
		t.Fatalf("unsafe service name must be rejected, got: %#v", checks[0])
	}
}

func TestCheckApplicationLogs_ExactGlobAndMissing(t *testing.T) {
	tempDirectory := t.TempDir()
	logPath := tempDirectory + "/catalina.out"
	if err := os.WriteFile(logPath, []byte("started\n"), 0o600); err != nil {
		t.Fatalf("failed to create test log: %v", err)
	}
	directoryWithLogSuffix := tempDirectory + "/archive.log"
	if err := os.Mkdir(directoryWithLogSuffix, 0o700); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	checks := checkApplicationLogs(map[string]any{
		"logs": []any{
			map[string]any{"name": "exact", "path_pattern": logPath},
			map[string]any{"name": "glob", "path_pattern": tempDirectory + "/*.out"},
			map[string]any{"name": "missing", "path_pattern": tempDirectory + "/missing.log"},
			map[string]any{"name": "relative", "path_pattern": "logs/*.log"},
			map[string]any{"name": "directory", "path_pattern": directoryWithLogSuffix},
		},
	})

	if len(checks) != 5 {
		t.Fatalf("unexpected log checks: %#v", checks)
	}
	if checks[0].Status != "pass" || checks[1].Status != "pass" {
		t.Fatalf("existing exact and glob logs must pass: %#v", checks)
	}
	if checks[2].Status != "fail" || checks[3].Status != "error" {
		t.Fatalf("missing log must fail and relative pattern must error: %#v", checks)
	}
	if checks[4].Status != "fail" {
		t.Fatalf("directory must not satisfy a log file check: %#v", checks[4])
	}
}

func TestApplicationSystemdCommand_SystemScope(t *testing.T) {
	command, err := applicationSystemdCommand(context.Background(), map[string]any{
		"service_name":  "tomcat.service",
		"systemd_scope": "system",
	})
	if err != nil {
		t.Fatalf("system scope returned error: %v", err)
	}
	if len(command.Args) != 3 || command.Args[1] != "is-active" || command.Args[2] != "tomcat.service" {
		t.Fatalf("unexpected systemctl command: %#v", command.Args)
	}
}

func TestApplicationSystemdCommand_UserScopeSetsRuntimeEnvironment(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("failed to get current user: %v", err)
	}
	command, err := applicationSystemdCommand(context.Background(), map[string]any{
		"service_name":  "tomcat",
		"systemd_scope": "user",
		"run_user":      currentUser.Username,
	})
	if err != nil {
		t.Fatalf("user scope returned error: %v", err)
	}
	if len(command.Args) != 4 || command.Args[1] != "--user" || command.Args[2] != "is-active" || command.Args[3] != "tomcat" {
		t.Fatalf("unexpected systemctl command: %#v", command.Args)
	}
	expectedRuntimeDir := "XDG_RUNTIME_DIR=/run/user/" + currentUser.Uid
	foundRuntimeDir := false
	for _, entry := range command.Env {
		if entry == expectedRuntimeDir {
			foundRuntimeDir = true
			break
		}
	}
	if !foundRuntimeDir {
		t.Fatalf("missing target user runtime directory in command environment: %s", expectedRuntimeDir)
	}

	currentUID, parseErr := strconv.ParseUint(currentUser.Uid, 10, 32)
	if parseErr != nil {
		t.Fatalf("invalid current user UID: %v", parseErr)
	}
	if uint64(os.Geteuid()) != currentUID && command.SysProcAttr == nil {
		t.Fatal("cross-user systemctl command must set process credentials")
	}
}
