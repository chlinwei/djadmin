package executor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

func TestRun_Success(t *testing.T) {
	e := New(2 * time.Second)
	job := protocol.Job{
		JobID:   "job-success",
		Type:    protocol.TaskTypeCommand,
		Action:  "shell",
		Command: "sh",
		Args:    []string{"-c", "echo hello"},
	}

	res, err := e.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Status != protocol.StatusSuccess {
		t.Fatalf("unexpected status: %s", res.Status)
	}
	if strings.TrimSpace(res.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
}

func TestRun_Failed(t *testing.T) {
	e := New(2 * time.Second)
	job := protocol.Job{
		JobID:   "job-failed",
		Type:    protocol.TaskTypeCommand,
		Action:  "shell",
		Command: "sh",
		Args:    []string{"-c", "echo boom 1>&2; exit 7"},
	}

	res, err := e.Run(context.Background(), job)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if res.Status != protocol.StatusFailed {
		t.Fatalf("unexpected status: %s", res.Status)
	}
	if res.ExitCode != 7 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "boom") {
		t.Fatalf("unexpected stderr: %q", res.Stderr)
	}
}

func TestRun_Timeout(t *testing.T) {
	e := New(200 * time.Millisecond)
	job := protocol.Job{
		JobID:   "job-timeout",
		Type:    protocol.TaskTypeCommand,
		Action:  "shell",
		Command: "sh",
		Args:    []string{"-c", "sleep 2"},
	}

	res, err := e.Run(context.Background(), job)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if res.Status != protocol.StatusTimeout {
		t.Fatalf("unexpected status: %s", res.Status)
	}
}

func TestResolveAutomationWorkDir_ExplicitWorkDir(t *testing.T) {
	resolved := resolveAutomationWorkDir("/custom/workdir")
	if resolved != "/custom/workdir" {
		t.Fatalf("expected explicit workdir, got: %s", resolved)
	}
}

func TestResolveAutomationWorkDir_DefaultToTmp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	resolved := resolveAutomationWorkDir("")
	if resolved != fallbackAutomationWorkDir {
		t.Fatalf("expected /tmp default workdir, got: %s", resolved)
	}
	if stat, err := os.Stat(resolved); err != nil || !stat.IsDir() {
		t.Fatalf("resolved default workdir is not a directory: %s", resolved)
	}
}

func TestResolveNodeExporterPort_Default(t *testing.T) {
	port, err := resolveNodeExporterPort(map[string]any{})
	if err != nil {
		t.Fatalf("resolveNodeExporterPort returned error: %v", err)
	}
	if port != defaultNodeExporterPort {
		t.Fatalf("expected default port %d, got %d", defaultNodeExporterPort, port)
	}
}

func TestResolveNodeExporterPort_Invalid(t *testing.T) {
	_, err := resolveNodeExporterPort(map[string]any{"exporter_port": 70000})
	if err == nil {
		t.Fatalf("expected invalid port error")
	}
}

func TestResolveNodeExporterVersion_FromParams(t *testing.T) {
	version := resolveNodeExporterVersion(map[string]any{"version": "1.7.0"})
	if version != "1.7.0" {
		t.Fatalf("expected version 1.7.0, got %s", version)
	}
}

func TestResolveNodeExporterVersion_StripsVPrefix(t *testing.T) {
	version := resolveNodeExporterVersion(map[string]any{"version": "v1.8.2"})
	if version != "1.8.2" {
		t.Fatalf("expected version 1.8.2, got %s", version)
	}
}

func TestRun_CustomInstallNodeExporter_ShouldHandleBuiltin(t *testing.T) {
	e := New(1 * time.Second)
	job := protocol.Job{
		JobID:  "job-install-node-exporter",
		Type:   protocol.TaskTypeCustom,
		Action: actionInstallNodeExporter,
		Params: map[string]any{"exporter_port": 0},
	}

	res, err := e.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("run should not return transport error, got: %v", err)
	}
	if res.Action != actionInstallNodeExporter {
		t.Fatalf("unexpected action: %s", res.Action)
	}
	if res.Status != protocol.StatusFailed {
		t.Fatalf("expected failed status for invalid params, got: %s", res.Status)
	}
}

func TestValidateJobByType_CustomInstallNodeExporter_NoParamsAllowed(t *testing.T) {
	err := validateJobByType(protocol.Job{
		JobID:  "job-install-validate",
		Type:   protocol.TaskTypeCustom,
		Action: actionInstallNodeExporter,
	})
	if err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}
