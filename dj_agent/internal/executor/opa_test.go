package executor

import (
	"context"
	"strings"
	"testing"
)

// ---- parseOpaViolations：opa eval --format json 输出的解析与 undefined 语义 ----

// ---- 全量断言：pass/fail 都进明细，失败项计入 violations ----

func TestCheckOpa_AssertionsContract(t *testing.T) {
	executor := New(0)
	spec := map[string]any{
		"input_commands": []any{
			map[string]any{"key": "value", "exec": "echo 65530", "parse": "raw"},
		},
		"policy": `package baseline

assertions contains assertion if {
	assertion := {"name": "内存锁定限制应为 unlimited", "pass": trim_space(input.value.raw) == "unlimited",
		"expected": "unlimited", "actual": trim_space(input.value.raw)}
}

assertions contains assertion if {
	assertion := {"name": "示例通过项", "pass": true, "expected": "ok", "actual": "ok"}
}`,
	}
	result := executor.checkOpa(context.Background(), map[string]any{
		"key": "opa-assert", "type": "opa", "name": "全量断言",
		"config": spec, "run_user": "root",
	})
	if result.Status != "fail" {
		t.Fatalf("one failing assertion must fail: %#v", result)
	}
	actual := result.Actual.(map[string]any)
	if actual["violation_count"] != 1 {
		t.Fatalf("violation_count = %v, want 1", actual["violation_count"])
	}
	details := actual["details"].([]map[string]any)
	// 关键：pass/fail 全量展示（2 条断言，1 pass 1 fail）
	if len(details) != 2 {
		t.Fatalf("details must contain ALL assertions, got %d: %+v", len(details), details)
	}
	passedRows := 0
	for _, row := range details {
		if row["successful"] == true {
			passedRows++
		}
	}
	if passedRows != 1 {
		t.Fatalf("expected exactly 1 passing row: %+v", details)
	}
}

// ---- opaParsedOutput：三种 parse 语义 ----

func TestOpaParsedOutput(t *testing.T) {
	raw := opaParsedOutput("command", "hello\n", 0, "raw")
	if raw["raw"] != "hello\n" {
		t.Fatalf("raw parse: %+v", raw)
	}

	lines := opaParsedOutput("file", "Port 22\r\nPermitRootLogin no\r\n", 0, "lines")
	parsed, _ := lines["lines"].([]string)
	if len(parsed) != 2 || parsed[1] != "PermitRootLogin no" {
		t.Fatalf("lines parse (CRLF + 去空行): %+v", lines)
	}

	jsonOut := opaParsedOutput("command", `{"version":"8.0"}`, 0, "json")
	data, _ := jsonOut["data"].(map[string]any)
	if data["version"] != "8.0" {
		t.Fatalf("json parse: %+v", jsonOut)
	}

	// JSON 解析失败保留原文并带错误标记，不整体失败
	badJSON := opaParsedOutput("command", `not-json`, 0, "json")
	if badJSON["raw"] != "not-json" || badJSON["json_error"] == nil {
		t.Fatalf("bad json parse: %+v", badJSON)
	}
}

// ---- opaInputKey：key/source/parse 校验与保留字冲突 ----

func TestOpaInputKey(t *testing.T) {
	if _, _, _, err := opaInputKey(map[string]any{"key": "a", "exec": "ls"}); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
	if _, _, _, err := opaInputKey(map[string]any{"key": "a", "path": "/etc/hosts"}); err != nil {
		t.Fatalf("valid file rejected: %v", err)
	}
	// 缺 key / 缺来源
	if _, _, _, err := opaInputKey(map[string]any{"exec": "ls"}); err == nil {
		t.Fatalf("missing key must error")
	}
	if _, _, _, err := opaInputKey(map[string]any{"key": "a"}); err == nil {
		t.Fatalf("missing source must error")
	}
	// 保留字段冲突
	if _, _, _, err := opaInputKey(map[string]any{"key": "host", "exec": "ls"}); err == nil {
		t.Fatalf("reserved key must error")
	}
	// parse 枚举
	if _, _, _, err := opaInputKey(map[string]any{"key": "a", "exec": "ls", "parse": "yaml"}); err == nil {
		t.Fatalf("unsupported parse must error")
	}
}

// ---- checkOpa 端到端：真实 opa 二进制求值（不依赖 root/降权）。 ----

func TestCheckOpa_EndToEnd(t *testing.T) {
	executor := New(0)
	passSpec := map[string]any{
		"input_commands": []any{
			map[string]any{"key": "greeting", "exec": "echo hello", "parse": "raw"},
		},
		"policy": `package baseline

assertions contains assertion if {
	assertion := {"name": "greeting 应为 hello", "pass": trim_space(input.greeting.raw) == "hello",
		"expected": "hello", "actual": trim_space(input.greeting.raw)}
}`,
	}
	pass := executor.checkOpa(context.Background(), map[string]any{
		"key": "opa-pass", "type": "opa", "name": "通过",
		"config": passSpec, "run_user": "root",
	})
	if pass.Status != "pass" {
		t.Fatalf("satisfied policy must pass: %#v", pass)
	}

	failSpec := map[string]any{
		"input_commands": []any{
			map[string]any{"key": "greeting", "exec": "echo world", "parse": "raw"},
		},
		"policy": `package baseline

assertions contains assertion if {
	assertion := {"name": "greeting 应为 hello", "pass": trim_space(input.greeting.raw) == "hello",
		"expected": "hello", "actual": trim_space(input.greeting.raw)}
}`,
	}
	fail := executor.checkOpa(context.Background(), map[string]any{
		"key": "opa-fail", "type": "opa", "name": "失败",
		"config": failSpec, "run_user": "root",
	})
	if fail.Status != "fail" {
		t.Fatalf("violating policy must fail: %#v", fail)
	}
	actual, ok := fail.Actual.(map[string]any)
	if !ok {
		t.Fatalf("actual must be object: %#v", fail.Actual)
	}
	if actual["violation_count"] != 1 {
		t.Fatalf("violation_count = %v, want 1", actual["violation_count"])
	}
	violations, _ := actual["violations"].([]map[string]any)
	if len(violations) != 1 {
		t.Fatalf("violations = %#v", actual["violations"])
	}
	// 关键回归锚点：violation.item.actual 是策略回传的真实实际值（对比 goss 的 bytes.Reader）。
	item, _ := violations[0]["item"].(map[string]any)
	if item["actual"] != "world" {
		t.Fatalf("item.actual = %q, want 真实输出 world", item["actual"])
	}
}

func TestCheckOpa_CollectFailureIsError(t *testing.T) {
	executor := New(0)
	spec := map[string]any{
		"input_commands": []any{
			map[string]any{"key": "boom", "exec": "exit 3"},
		},
		"policy": "package baseline\nviolations contains msg if { true }",
	}
	result := executor.checkOpa(context.Background(), map[string]any{
		"key": "opa-err", "type": "opa", "name": "采集失败",
		"config": spec, "run_user": "root",
	})
	// 采集失败 → 整体 error，不进策略求值（输入不完整不出结论）。
	if result.Status != "error" {
		t.Fatalf("collect failure must be error: %#v", result)
	}

	// 文件采集失败同样整体 error。
	specFile := map[string]any{
		"input_files": []any{
			map[string]any{"key": "gone", "path": "/nonexistent/opa-collect-missing"},
		},
		"policy": "package baseline\nviolations contains msg if { true }",
	}
	result = executor.checkOpa(context.Background(), map[string]any{
		"key": "opa-file-err", "type": "opa", "name": "文件采集失败",
		"config": specFile, "run_user": "root",
	})
	if result.Status != "error" {
		t.Fatalf("file collect failure must be error: %#v", result)
	}
}

func TestCheckOpa_InvalidConfig(t *testing.T) {
	executor := New(0)
	cases := []map[string]any{
		{},
		{"policy": "package baseline\nviolations contains msg if { true }"},
		{"input_commands": []any{map[string]any{"key": "a", "exec": "ls"}}},
		{"input_commands": []any{map[string]any{"key": "host", "exec": "ls"}}, "policy": "package baseline\nviolations contains msg if { true }"},
	}
	for index, config := range cases {
		result := executor.checkOpa(context.Background(), map[string]any{
			"key": "x", "type": "opa", "name": "配置", "config": config,
		})
		if result.Status != "error" {
			t.Fatalf("case %d must be error: %#v", index, result)
		}
	}
}

func TestCheckOpa_PolicyRefsUncollectedKey(t *testing.T) {
	executor := New(0)
	spec := map[string]any{
		"input_commands": []any{
			map[string]any{"key": "es_pid", "exec": "echo 123"},
		},
		// 引用未采集的 es_port：旧数据没过保存校验时，这里必须兜底成计划级 error，
		// 而不是求值时断言静默消失。
		"policy": "package baseline\nassertions contains a if {\n\tinput.es_port.raw != \"\"\n\ta := {\"name\": \"n\", \"pass\": true, \"expected\": \"e\", \"actual\": \"a\"}\n}",
	}
	result := executor.checkOpa(context.Background(), map[string]any{
		"key": "opa-missing-key", "type": "opa", "name": "引用未采集字段", "config": spec,
	})
	if result.Status != "error" {
		t.Fatalf("policy referencing uncollected key must be error: %#v", result)
	}
	if !strings.Contains(result.Message, "es_port") {
		t.Fatalf("error must name the missing key: %#v", result)
	}
}

func TestEvalOpaQueryDirect(t *testing.T) {
	policy := `package baseline

assertions contains a if {
	trim_space(input.greeting.raw) != "hello"
	a := {"name": "greeting 应为 hello", "pass": false}
}`
	input := map[string]any{
		"greeting": map[string]any{"source": "command", "exit_code": 0, "raw": "world\n"},
		"host":     map[string]any{"ip": "", "name": ""},
		"vars":     map[string]any{},
	}
	items, defined, err := evalOpaQuery(context.Background(), "data.baseline.assertions", policy, input)
	t.Logf("err=%v defined=%v items=%#v", err, defined, items)
	// defined=true 表示结果集有内容（与 undefined=空集区分）。
	if err != nil || !defined || len(items) != 1 {
		t.Fatalf("expected 1 violation, got err=%v defined=%v items=%d", err, defined, len(items))
	}
}

