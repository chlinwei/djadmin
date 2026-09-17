//go:build embedansible

package ansiblecmd

import (
	"context"
	"strings"
	"testing"
)

// embed 变体：真的用嵌入的 CPython + ansible-core 跑一次 `ansible-playbook --version`。
// 依赖先跑过 `make ansible-embed` 生成 embeddata（否则本包无法编译）。
func TestEmbeddedAnsiblePlaybookVersion(t *testing.T) {
	command, err := CommandContext(context.Background(), "--version")
	if err != nil {
		t.Fatalf("CommandContext: %v", err)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run embedded ansible-playbook: %v\n%s", err, output)
	}
	text := string(output)
	if !strings.Contains(text, "ansible-playbook [core 2.16") {
		t.Fatalf("期望嵌入的 ansible-core 2.16，实际输出：\n%s", text)
	}
	if strings.Contains(text, "future feature annotations") {
		t.Fatalf("嵌入 ansible 仍包含 3.6 不兼容的 future annotations 语法：\n%s", text)
	}
}
