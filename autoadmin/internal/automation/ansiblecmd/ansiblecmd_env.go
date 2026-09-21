package ansiblecmd

import "os"

// ansibleEnv 在继承进程环境的基础上补齐默认项；用户已显式设置的变量不覆盖。
//
// 目前只补一个：ANSIBLE_STDOUT_CALLBACK=minimal。ansible 默认回调只打印每台主机的
// ok/changed 状态行，**不显示 shell/command 任务的成功 stdout**，表现为"脚本明明有输出、
// 作业日志里看不到"；minimal 把 stdout/stderr 直接打在 `host | CHANGED | rc=0 >>` 之后，
// 既有输出又不引入 -v 那种整份任务结果字典的噪声（2026-09-21 现场）。
// 两个构建变体（宿主 PATH / 嵌入 ansible）共用，保证行为一致。
func ansibleEnv() []string {
	env := os.Environ()
	if os.Getenv("ANSIBLE_STDOUT_CALLBACK") == "" {
		env = append(env, "ANSIBLE_STDOUT_CALLBACK=minimal")
	}
	return env
}
