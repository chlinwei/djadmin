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

func TestRun_CustomStartExporter_MissingServiceName_ShouldFail(t *testing.T) {
	e := New(1 * time.Second)
	job := protocol.Job{
		JobID:  "job-start-exporter",
		Type:   protocol.TaskTypeCustom,
		Action: actionStartExporter,
		Params: map[string]any{"exporter_name": "node_exporter"},
	}

	res, err := e.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("run should not return transport error, got: %v", err)
	}
	if res.Action != actionStartExporter {
		t.Fatalf("unexpected action: %s", res.Action)
	}
	if res.Status != protocol.StatusFailed {
		t.Fatalf("expected failed status when service_name missing, got: %s", res.Status)
	}
}

func TestValidateJobByType_CustomStartExporter_RequiresParams(t *testing.T) {
	err := validateJobByType(protocol.Job{
		JobID:  "job-start-validate",
		Type:   protocol.TaskTypeCustom,
		Action: actionStartExporter,
	})
	if err == nil {
		t.Fatalf("expected validation error when params is empty for generic exporter action")
	}
}

func TestResolveServiceName_Valid(t *testing.T) {
	name, err := resolveServiceName(map[string]any{"service_name": "node_exporter.service"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if name != "node_exporter.service" {
		t.Fatalf("unexpected service name: %s", name)
	}
}

func TestResolveServiceName_Missing_ShouldFail(t *testing.T) {
	if _, err := resolveServiceName(map[string]any{}); err == nil {
		t.Fatalf("expected error when service_name is missing")
	}
}

func TestResolveServiceName_InvalidFormat_ShouldFail(t *testing.T) {
	cases := []string{"node_exporter", "node_exporter.service; rm -rf /", "../etc/passwd.service", ""}
	for _, raw := range cases {
		if _, err := resolveServiceName(map[string]any{"service_name": raw}); err == nil {
			t.Fatalf("expected error for invalid service_name %q", raw)
		}
	}
}

func TestRun_CustomCheckExporterStatus_InvalidServiceName_ShouldFail(t *testing.T) {
	e := New(1 * time.Second)
	job := protocol.Job{
		JobID:  "job-status-exporter-invalid",
		Type:   protocol.TaskTypeCustom,
		Action: actionCheckExporterStatus,
		Params: map[string]any{"service_name": "not a service"},
	}

	res, err := e.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("run should not return transport error, got: %v", err)
	}
	if res.Status != protocol.StatusFailed {
		t.Fatalf("expected failed status for invalid service_name, got: %s", res.Status)
	}
}
