package executor

import (
	"context"
	"testing"
)

func TestCheckApplicationPlanForState_DeliversOpaChecks(t *testing.T) {
	executor := New(0)
	plan := map[string]any{
		"schema_version": 1,
		"checks": []any{
			map[string]any{
				"key": "es-x", "type": "opa", "name": "es 检查", "requires_running": false,
				"run_user": "root", "config": map[string]any{
					"input_commands": []any{map[string]any{"key": "greeting", "exec": "echo hello", "parse": "raw"}},
					"policy": `package baseline

assertions contains a if {
	a := {"name": "问候", "pass": trim_space(input.greeting.raw) == "hello", "expected": "hello", "actual": trim_space(input.greeting.raw)}
}`,
				},
			},
		},
	}
	results := executor.checkApplicationPlanForState(context.Background(), map[string]any{"check_plan": plan}, true)
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Status != "pass" {
		t.Fatalf("check must pass: %#v", results[0])
	}
}

func TestCheckApplicationPlanForState_RequiresRunningSkips(t *testing.T) {
	executor := New(0)
	plan := map[string]any{
		"schema_version": 1,
		"checks": []any{
			map[string]any{"key": "x", "type": "opa", "name": "x", "requires_running": true, "config": map[string]any{}},
		},
	}
	results := executor.checkApplicationPlanForState(context.Background(), map[string]any{"check_plan": plan}, false)
	if len(results) != 1 || results[0].Status != "skipped" {
		t.Fatalf("requires_running + not running must skip: %#v", results)
	}
}

func TestCheckApplicationPlanForState_RejectsUnknownCapability(t *testing.T) {
	executor := New(0)
	plan := map[string]any{
		"schema_version": 1,
		"required_capabilities": []any{"goss:v1"},
		"checks":               []any{},
	}
	results := executor.checkApplicationPlanForState(context.Background(), map[string]any{"check_plan": plan}, true)
	if len(results) != 1 || results[0].Status != "error" {
		t.Fatalf("unknown capability must error: %#v", results)
	}
}

func TestCheckApplicationPlanForState_RejectsInvalidPlanVersion(t *testing.T) {
	executor := New(0)
	plan := map[string]any{"schema_version": 99, "checks": []any{}}
	results := executor.checkApplicationPlanForState(context.Background(), map[string]any{"check_plan": plan}, true)
	if len(results) != 1 || results[0].Status != "error" {
		t.Fatalf("invalid version must error: %#v", results)
	}
}
