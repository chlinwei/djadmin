//go:build !embedansible

package ansiblecmd

import (
	"context"
	"os/exec"
)

// CommandContext 返回"用宿主 PATH 上的 ansible-playbook 执行"的命令（默认变体）。
//
// args 是 ansible-playbook 之后的参数，与直接 exec 一致；调用方仍可设置 Dir、
// SysProcAttr、Cancel、WaitDelay 等。找不到 ansible-playbook 时在 Run 阶段报错。
func CommandContext(ctx context.Context, args ...string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, "ansible-playbook", args...), nil
}
