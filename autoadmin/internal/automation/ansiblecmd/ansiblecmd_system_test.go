//go:build !embedansible

package ansiblecmd

import (
	"context"
	"testing"
)

// 默认变体：命令头必须是 PATH 上的 ansible-playbook。
func TestSystemCommandUsesHostBinary(t *testing.T) {
	command, err := CommandContext(context.Background(), "-i", "inventory.ini", "playbook.yml")
	if err != nil {
		t.Fatalf("CommandContext: %v", err)
	}
	if len(command.Args) == 0 || command.Args[0] != "ansible-playbook" {
		t.Fatalf("args[0] = %q, want ansible-playbook", command.Args)
	}
}
