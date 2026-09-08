package opapolicy

import "testing"

func baseConfig(policy string) map[string]any {
	return map[string]any{
		"policy":         policy,
		"input_commands": []any{map[string]any{"key": "es_pid", "exec": "pgrep -o -f elasticsearch"}},
		"input_files":    []any{map[string]any{"key": "es_limits", "path": "/proc/1/limits", "parse": "lines"}},
	}
}

func TestValidate_MissingInputKeys(t *testing.T) {
	// 引用未采集的 key → 保存时必须报错，不能等运行时静默消失。
	policy := "package baseline\nassertions contains a if {\n\tinput.es_pid.raw != \"\"\n\tinput.es_port.raw != \"\"\n}"
	if message := Validate(baseConfig(policy)); message == "" {
		t.Fatal("missing input key must be rejected")
	}

	// 注释里的引用不算。
	policy = "package baseline\n# input.nope.raw 不存在\nassertions contains a if {\n\tinput.es_pid.raw != \"\"\n\ta := {\"name\": \"n\", \"pass\": true, \"expected\": \"e\", \"actual\": \"a\"}\n}"
	if message := Validate(baseConfig(policy)); message != "" {
		t.Fatalf("comment refs must be ignored: %s", message)
	}

	// 保留字段 host/vars 放行。
	policy = "package baseline\nassertions contains a if {\n\tinput.host.ip != \"\"\n\tinput.vars.APP_HOME != \"\"\n\ta := {\"name\": \"n\", \"pass\": true, \"expected\": \"e\", \"actual\": \"a\"}\n}"
	if message := Validate(baseConfig(policy)); message != "" {
		t.Fatalf("reserved keys must be allowed: %s", message)
	}

	// 采集齐全 → 通过。
	policy = "package baseline\nassertions contains a if {\n\tinput.es_pid.raw != \"\"\n\tinput.es_limits.lines != _\n\ta := {\"name\": \"n\", \"pass\": true, \"expected\": \"e\", \"actual\": \"a\"}\n}"
	if message := Validate(baseConfig(policy)); message != "" {
		t.Fatalf("complete refs must pass: %s", message)
	}
}
