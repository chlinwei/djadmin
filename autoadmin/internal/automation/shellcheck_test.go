package automation

import (
	"strings"
	"testing"
)

// 内容形态归一：缺省 = playbook；仅精确的 "shell" 视为 shell；其他值一律回落 playbook。
func TestPlaybookContentFormat(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"", playbookFormatPlaybook},
		{"playbook", playbookFormatPlaybook},
		{"shell", playbookFormatShell},
		{"  shell ", playbookFormatShell},
		{"SHELL", playbookFormatPlaybook},
		{"yaml", playbookFormatPlaybook},
	}
	for _, testCase := range cases {
		if got := playbookContentFormat(testCase.raw); got != testCase.want {
			t.Errorf("playbookContentFormat(%q) = %q, want %q", testCase.raw, got, testCase.want)
		}
	}
}

// 包装渲染：脚本原文原样缩进为唯一 shell 任务，executable 固定 /bin/bash，
// 名称里的单引号被剔除（防 YAML/命令注入面），身份与变量不出现在包装壳里。
func TestWrapShellPlaybook(t *testing.T) {
	script := "#!/bin/bash\nfree -m | awk '/Mem:/{print $3*2}'\necho done\n"
	playbook := wrapShellPlaybook("内存检查 <v1>", script)
	for _, want := range []string{
		"- hosts: all",
		"gather_facts: false",
		"ansible.builtin.shell: |",
		"free -m | awk '/Mem:/{print $3*2}'",
		"executable: /bin/bash",
	} {
		if !strings.Contains(playbook, want) {
			t.Errorf("wrapped playbook missing %q:\n%s", want, playbook)
		}
	}
	// 模板名经 %q 双引号包裹（YAML 安全）；脚本原文里的单引号属于脚本内容，应原样保留。
	if strings.Contains(playbook, "'内存检查") {
		t.Errorf("template name must be double-quoted, not raw:\n%s", playbook)
	}
	if !strings.HasPrefix(playbook, "- hosts: all\n") {
		t.Errorf("wrapped playbook should start with the play:\n%s", playbook)
	}
}

// error 级告警拒绝保存；warning/info/style 放行。
func TestFatalShellcheckFindings(t *testing.T) {	findings := []shellcheckFinding{
		{Line: 1, Level: "style", Code: 2126},
		{Line: 2, Level: "warning", Code: 2086},
		{Line: 3, Level: "error", Code: 1036},
		{Line: 4, Level: "info", Code: 2086},
	}
	fatal := fatalShellcheckFindings(findings)
	if len(fatal) != 1 || fatal[0].Code != 1036 {
		t.Fatalf("fatal = %+v, want only SC1036", fatal)
	}
	if len(fatalShellcheckFindings(nil)) != 0 {
		t.Fatalf("nil findings should have no fatal")
	}
}

// validatePlaybookContent 分流：playbook 形态沿用 YAML 结构校验；shell 形态在组件未就绪时
// 返回带就绪指引的错误（不 panic、不带行号数据）。
func TestValidatePlaybookContentSplit(t *testing.T) {
	handler := &Handler{} // shellcheckDir 为空
	if fatal, warnings := handler.validatePlaybookContent(t.Context(), playbookFormatPlaybook, "not-a-list"); fatal == "" || warnings != nil {
		t.Fatalf("playbook 形态应报 YAML 结构错误: fatal=%q warnings=%v", fatal, warnings)
	}
	if fatal, _ := handler.validatePlaybookContent(t.Context(), playbookFormatPlaybook, "- hosts: all\n  tasks:\n    - name: ok\n      ansible.builtin.debug:\n        msg: hi\n"); fatal != "" {
		t.Fatalf("合法 playbook 不应报错: %q", fatal)
	}

	// 清空 PATH + 无上传目录 → shell 形态必须给出"未就绪"错误而不是静默通过。
	t.Setenv("PATH", t.TempDir())
	fatal, _ := handler.validatePlaybookContent(t.Context(), playbookFormatShell, "echo hi")
	if fatal == "" || !strings.Contains(fatal, "ShellCheck") {
		t.Fatalf("shell 形态未就绪应返回含 ShellCheck 指引的错误，得到 %q", fatal)
	}
}

// shellcheck -f json1 返回对象 {"comments":[...]}（现场 2026-09-21 报错来源）；
// 也要兼容早期的数组形态与空输出，别把"没有告警"解析成失败。
func TestParseShellcheckOutput(t *testing.T) {
	objectOutput := []byte(`{"comments":[{"file":"script.sh","line":3,"endLine":3,"column":6,"level":"warning","code":2086,"message":"Double quote to prevent globbing"}]}`)
	findings, err := parseShellcheckOutput(objectOutput)
	if err != nil {
		t.Fatalf("object output: %v", err)
	}
	if len(findings) != 1 || findings[0].EndLine != 3 || findings[0].Code != 2086 {
		t.Fatalf("object output parsed wrong: %+v", findings)
	}

	arrayOutput := []byte(`[{"line":1,"endLine":1,"level":"error","code":1036,"message":"syntax error"}]`)
	if findings, err := parseShellcheckOutput(arrayOutput); err != nil || len(findings) != 1 || findings[0].Level != "error" {
		t.Fatalf("array output: %v %+v", err, findings)
	}

	for _, empty := range [][]byte{nil, []byte(""), []byte("  "), []byte("{}"), []byte("[]")} {
		findings, err := parseShellcheckOutput(empty)
		if err != nil || len(findings) != 0 {
			t.Fatalf("empty output %q should yield no findings: %v %+v", empty, err, findings)
		}
	}
}
