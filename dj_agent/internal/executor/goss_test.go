package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

func TestCheckGoss_PassAndFail(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("goss run_user 降权用例需要 root")
	}
	executor := New(0)
	passSpec := `file:
  /etc/hosts:
    exists: true
    mode: "0644"
`
	failSpec := `file:
  /etc/hosts:
    exists: false
`
	pass := executor.checkGoss(context.Background(), map[string]any{
		"key": "goss-pass", "type": "goss", "name": "通过", "executor": "goss", "spec": passSpec,
		"run_user": "root",
	})
	if pass.Status != "pass" {
		t.Fatalf("valid spec must pass: %#v", pass)
	}

	fail := executor.checkGoss(context.Background(), map[string]any{
		"key": "goss-fail", "type": "goss", "name": "失败", "executor": "goss", "spec": failSpec,
		"run_user": "root",
	})
	if fail.Status != "fail" {
		t.Fatalf("failing spec must fail: %#v", fail)
	}
	actual, ok := fail.Actual.(map[string]any)
	if !ok || actual["failed_count"] != 1 {
		t.Fatalf("failure details must be reported: %#v", fail.Actual)
	}
}

func TestCheckGoss_EmptySpecIsError(t *testing.T) {
	executor := New(0)
	result := executor.checkGoss(context.Background(), map[string]any{
		"key": "goss-empty", "type": "goss", "name": "空", "executor": "goss", "spec": "  \n",
	})
	if result.Status != "error" {
		t.Fatalf("empty spec must error: %#v", result)
	}
}

func TestCheckGoss_VarsAndEnvironmentInjected(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("goss run_user 降权用例需要 root")
	}
	executor := New(0)
	// vars 通过 --vars-inline 注入；environment 作为进程环境变量注入。
	spec := `command:
  echo ${VAR}:
    exit-status: 0
    stdout:
      - from-env
`
	result := executor.checkGoss(context.Background(), map[string]any{
		"key": "goss-vars", "type": "goss", "name": "变量", "executor": "goss", "spec": spec,
		"run_user": "root",
		"vars":     map[string]any{"VAR": "from-env"},
		"environment": map[string]any{"HOST_IP": "10.0.0.1"},
	})
	if result.Status != "pass" {
		t.Fatalf("vars must be injected into goss spec: %#v", result)
	}
}

func TestRunGossInspectionJob(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("goss run_user 降权用例需要 root")
	}
	executor := New(0)
	job := protocol.Job{
		JobID: "goss-job", Type: protocol.TaskTypeCustom, Action: actionCheckApplicationBaseline,
		Params: map[string]any{
			"control_type": "command",
			"check_plan": map[string]any{
				"schema_version": 1,
				"checks": []any{map[string]any{
					"key": "goss-1", "type": "goss", "name": "套件", "executor": "goss",
					"spec": "file:\n  /etc/hostname:\n    exists: true\n", "run_user": "root",
				}},
			},
		},
	}
	result, err := executor.Run(context.Background(), job)
	if err != nil {
		t.Fatalf("job returned error: %v", err)
	}
	checks, _ := result.Data["checks"].([]applicationCheckResult)
	found := false
	for _, check := range checks {
		if check.Key == "goss-1" {
			found = true
			if check.Status != "pass" {
				t.Fatalf("goss check must pass: %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("goss check missing from results: %#v", result.Data["checks"])
	}
}

func TestEnsureGossBinary_ReleasesAndReuses(t *testing.T) {
	paths, err := ensureGossBinary()
	if err != nil || len(paths) == 0 {
		t.Fatalf("release failed: %v", err)
	}
	path := paths[0]
	if filepath.Base(path) != "goss" {
		t.Fatalf("unexpected binary path: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("released binary must be executable: %v %v", info, err)
	}
	// 第二次调用应命中哈希一致的复用分支。
	again, err := ensureGossBinary()
	if err != nil || len(again) == 0 || again[0] != path {
		t.Fatalf("second release must reuse same path: %v %v", again, err)
	}
}


func TestEnsureGossBinary_FixesMissingExecBit(t *testing.T) {
	paths, err := ensureGossBinary()
	if err != nil || len(paths) == 0 {
		t.Fatalf("release failed: %v", err)
	}
	path := paths[0]
	// 模拟升级通道以 0644 落盘后，复用分支必须补回执行位。
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := ensureGossBinary()
	if err != nil || len(again) == 0 {
		t.Fatalf("reuse failed: %v", err)
	}
	info, err := os.Stat(again[0])
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("reused binary must be executable: %v %v", info, err)
	}
}
