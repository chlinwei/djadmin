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
