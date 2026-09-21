package ansiblecmd

import (
	"testing"
)

// 默认注入 minimal stdout 回调（否则 shell 任务的成功 stdout 不显示）；用户显式设置时不覆盖。
func TestAnsibleEnvStdoutCallback(t *testing.T) {
	t.Setenv("ANSIBLE_STDOUT_CALLBACK", "")
	env := ansibleEnv()
	if !hasEnv(env, "ANSIBLE_STDOUT_CALLBACK=minimal") {
		t.Fatalf("默认应注入 ANSIBLE_STDOUT_CALLBACK=minimal，实际 env 未包含")
	}

	t.Setenv("ANSIBLE_STDOUT_CALLBACK", "yaml")
	env = ansibleEnv()
	if hasEnv(env, "ANSIBLE_STDOUT_CALLBACK=minimal") {
		t.Fatalf("用户已设置回调时不应再注入 minimal")
	}
	if !hasEnv(env, "ANSIBLE_STDOUT_CALLBACK=yaml") {
		t.Fatalf("用户设置的回调应保留在继承环境里")
	}
}

func hasEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
