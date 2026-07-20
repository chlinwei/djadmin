package executor

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
)

// mapToEnv 将 map[string]string 转换为环境变量格式 ["KEY=VALUE", ...]
func mapToEnv(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, fmt.Sprintf("%s=%s", k, values[k]))
	}
	return env
}

// readExitCode 从错误中提取进程退出码，若无法提取则返回 -1
func readExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1
	}
	return exitErr.ExitCode()
}
