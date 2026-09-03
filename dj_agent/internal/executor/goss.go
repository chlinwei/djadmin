package executor

import (
	"context"
	"crypto/sha256"
	"errors"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

// 内嵌 goss 官方发布二进制（resources/goss，版本见 resources/goss/VERSION）。
// Makefile 构建前会按 GOARCH 把对应架构的二进制复制进来，因此这里两个架构名都声明，
// 未打包的架构构建时该 embed 模式匹配不到文件会直接失败——Makefile 保证总会有一份。
//
//go:embed embedded/goss/goss-linux-*
var embeddedGoss embed.FS

// gossBinaryVersion 与 resources/goss/VERSION 保持一致；释放目录带版本号，
// 升级 Agent 时新旧目录天然隔离，不会出现半新半旧的二进制。
const gossBinaryVersion = "v0.4.10"

func gossEmbeddedName() (string, error) {
	entries, err := embeddedGoss.ReadDir("embedded/goss")
	if err != nil {
		return "", fmt.Errorf("读取内嵌 goss 失败: %w", err)
	}
	if len(entries) != 1 {
		return "", fmt.Errorf("内嵌 goss 数量异常: %d", len(entries))
	}
	return "embedded/goss/" + entries[0].Name(), nil
}

// gossReleaseBaseDirs 返回候选释放目录（按优先级）：
//  1. Agent 可执行文件所在目录（如 /usr/local/bin）—— Agent 能跑就说明这里可写可执行；
//  2. os.TempDir —— 兜底。
// 不使用 /var/lib：目标机对该目录常有 noexec 挂载或受限权限，历史问题较多。
func gossReleaseBaseDirs() []string {
	if executablePath, err := os.Executable(); err == nil {
		return []string{filepath.Dir(executablePath), os.TempDir()}
	}
	return []string{os.TempDir()}
}

// ensureGossBinary 把内嵌 goss 释放到候选目录并返回可执行路径。
// 释放流程：内容哈希比对（一致则补执行位后复用）→ 临时文件写入 → chmod 0755 → 原子 rename。
// 目录不可写或分区 noexec 导致无法就位时自动尝试下一个候选目录。
func ensureGossBinary() ([]string, error) {
	name, err := gossEmbeddedName()
	if err != nil {
		return nil, err
	}
	data, err := embeddedGoss.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("读取内嵌 goss 失败: %w", err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])

	var lastErr error
	candidatePaths := make([]string, 0, 3)
	for _, baseDir := range gossReleaseBaseDirs() {
		releaseDir := filepath.Join(baseDir, "goss", gossBinaryVersion)
		if mountNoexec(releaseDir) {
			lastErr = fmt.Errorf("%s 所在文件系统挂载了 noexec", releaseDir)
			continue
		}
		// 目录与文件都必须允许 run_user 穿越并执行（goss 按巡检 run_user 降权运行）：
		// MkdirAll 不会改已存在目录的权限，从叶子目录向上逐层放开到 baseDir。
		if err = os.MkdirAll(releaseDir, 0o755); err != nil {
			lastErr = fmt.Errorf("创建 %s 失败: %w", releaseDir, err)
			continue
		}
		for dir := releaseDir; strings.HasPrefix(dir, baseDir); dir = filepath.Dir(dir) {
			if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() && info.Mode().Perm()&0o055 != 0o055 {
				_ = os.Chmod(dir, info.Mode().Perm()|0o055)
			}
			if dir == baseDir || dir == string(filepath.Separator) {
				break
			}
		}
		target := filepath.Join(releaseDir, "goss")
		if existing, readErr := os.ReadFile(target); readErr == nil {
			if existingSum := sha256.Sum256(existing); hex.EncodeToString(existingSum[:]) == digest {
				// 哈希一致但可能缺执行位（升级通道落盘的文件常是 0644），补上再复用。
				if chmodErr := os.Chmod(target, 0o755); chmodErr == nil {
					candidatePaths = append(candidatePaths, target)
					continue
				}
			}
		}
		path, releaseErr := releaseGossBinary(releaseDir, data)
		if releaseErr == nil {
			candidatePaths = append(candidatePaths, path)
			continue
		}
		lastErr = releaseErr
	}
	if len(candidatePaths) > 0 {
		return candidatePaths, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("goss 二进制释放失败: %w", lastErr)
	}
	return nil, fmt.Errorf("goss 二进制释放失败: 无可用目录")
}

// mountNoexec 判断目录所在文件系统是否挂载了 noexec（读 /proc/mounts，取最长匹配的挂载点）。
// 非 Linux 或读取失败返回 false，交给后续的 chmod/exec 错误兜底。
func mountNoexec(dir string) bool {
	raw, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	bestMountPoint, noexec := "", false
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mountPoint := strings.ReplaceAll(fields[1], "\\040", " ")
		if !strings.HasPrefix(absolute, mountPoint) {
			continue
		}
		if len(mountPoint) < len(bestMountPoint) {
			continue
		}
		bestMountPoint = mountPoint
		noexec = false
		for _, option := range strings.Split(fields[3], ",") {
			if option == "noexec" {
				noexec = true
				break
			}
		}
	}
	return noexec
}

func releaseGossBinary(releaseDir string, data []byte) (string, error) {	target := filepath.Join(releaseDir, "goss")
	tempFile, err := os.CreateTemp(releaseDir, ".goss-*")
	if err != nil {
		return "", fmt.Errorf("创建 goss 临时文件失败: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err = tempFile.Write(data); err != nil {
		tempFile.Close()
		return "", fmt.Errorf("写入 goss 临时文件失败: %w", err)
	}
	if err = tempFile.Close(); err != nil {
		return "", fmt.Errorf("关闭 goss 临时文件失败: %w", err)
	}
	if err = os.Chmod(tempPath, 0o755); err != nil {
		return "", fmt.Errorf("设置 goss 可执行权限失败: %w", err)
	}
	if err = os.Rename(tempPath, target); err != nil {
		return "", fmt.Errorf("落盘 goss 二进制失败: %w", err)
	}
	return target, nil
}

// ---- 巡检 goss 检查项 ----
//
// check_plan 里 executor=goss 的检查项字段：
//   - spec: goss YAML 文本（变量已在后端展开，YAML 内也可用 goss 模板语法）
//   - vars: 可选，传给 goss --vars-inline 的变量（JSON 对象）
//   - run_user: 执行用户，留空默认 root，走与巡检 shell 一致的降权语义
//   - environment: 注入的环境变量（KEY=VALUE 列表或对象），goss 进程及其 command 资源均可见

func (e *Executor) checkGoss(ctx context.Context, check map[string]any) applicationCheckResult {
	spec := valueString(check["spec"])
	if strings.TrimSpace(spec) == "" {
		return newPlanCheckResult(check, "error", nil, "goss spec 不能为空")
	}

	specDir, err := os.MkdirTemp("", "djagent-goss-")
	if err != nil {
		return newPlanCheckResult(check, "error", nil, fmt.Sprintf("创建 goss 工作目录失败: %v", err))
	}
	defer os.RemoveAll(specDir)
	// MkdirTemp 固定 0700，goss 以 run_user 运行时需要穿越+读取，放开到 0755/0644。
	if err = os.Chmod(specDir, 0o755); err != nil {
		return newPlanCheckResult(check, "error", nil, fmt.Sprintf("设置 goss 工作目录权限失败: %v", err))
	}
	specPath := filepath.Join(specDir, "goss.yaml")
	if err = os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		return newPlanCheckResult(check, "error", nil, fmt.Sprintf("写入 goss spec 失败: %v", err))
	}

	runUser := valueString(check["run_user"])
	if runUser == "" {
		runUser = "root"
	}
	// goss 必须以目标用户身份运行：进程/文件/用户组类资源按该用户的视角采集。
	// 一个候选路径执行被拒（权限位/noexec/SELinux）时自动换下一个目录重试，
	// 全部失败时把每个候选的诊断信息（权限位、noexec、SELinux）带进错误消息。
	candidatePaths, err := ensureGossBinary()
	if err != nil {
		return newPlanCheckResult(check, "error", nil, err.Error())
	}
	var stdout, stderr strings.Builder
	var runErr error
	executed := false
	for _, gossPath := range candidatePaths {
		command, userErr := gossRunUserCommand(ctx, gossPath, specPath, check["vars"], runUser)
		if userErr != nil {
			return newPlanCheckResult(check, "error", nil, userErr.Error())
		}
		command.Dir = specDir
		command.Env = mergeGossEnvironment(command.Env, check["environment"])
		stdout.Reset()
		stderr.Reset()
		command.Stdout = &stdout
		command.Stderr = &stderr
		runErr = command.Run()
		if ctx.Err() != nil {
			return newPlanCheckResult(check, "error", nil, fmt.Sprintf("goss 执行中断: %v", ctx.Err()))
		}
		if runErr != nil && errors.Is(runErr, os.ErrPermission) {
			continue
		}
		executed = true
		break
	}

	// goss 退出码：0=全部通过，1=有失败，3=超时，其他=运行错误；JSON 起 parse 判真实原因。
	var gossOutput gossJSONOutput
	parseErr := json.Unmarshal([]byte(stdout.String()), &gossOutput)
	if parseErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if !executed {
			return newPlanCheckResult(check, "error", nil, fmt.Sprintf("goss 执行被拒绝: %v; %s", runErr, gossDiagnostics(candidatePaths)))
		}
		if runErr != nil {
			return newPlanCheckResult(check, "error", nil, fmt.Sprintf("goss 运行失败（退出码异常且无 JSON 输出）: %v; %s", runErr, detail))
		}
		return newPlanCheckResult(check, "error", nil, fmt.Sprintf("goss 输出无法解析: %v; %s", parseErr, detail))
	}
	if len(gossOutput.Results) == 0 {
		return newPlanCheckResult(check, "error", nil, "goss spec 没有产生任何检查结果")
	}

	failed := 0
	messages := make([]string, 0)
	for _, item := range gossOutput.Results {
		if item.Successful {
			continue
		}
		failed++
		if item.SummaryLine != "" {
			messages = append(messages, item.SummaryLine)
		}
	}
	actual := map[string]any{
		"test_count":  gossOutput.Summary.TestCount,
		"failed_count": gossOutput.Summary.FailedCount,
		"skipped_count": gossOutput.Summary.SkippedCount,
		"failures":    messages,
		"run_user":    runUser,
	}
	// 期望值直接取自 goss 逐条测试的 matcher-result.expected，
	// 即 YAML 套件里声明的断言（exists/mode/contents/listening 等）。
	expectations := make([]map[string]any, 0, len(gossOutput.Results))
	for _, item := range gossOutput.Results {
		expectations = append(expectations, map[string]any{
			"resource": strings.TrimSpace(item.ResourceType + ": " + item.ResourceID),
			"property": item.Property,
			"expected": item.MatcherResult.Expected,
		})
	}
	expected := map[string]any{"tests": expectations, "failed_count": 0, "skipped_count": 0}
	if failed > 0 {
		result := newPlanCheckResult(check, "fail", actual, fmt.Sprintf("goss 校验失败 %d/%d 项: %s", failed, gossOutput.Summary.TestCount, strings.Join(messages, "; ")))
		result.Expected = expected
		return result
	}
	result := newPlanCheckResult(check, "pass", actual, "")
	result.Expected = expected
	return result
}

// gossRunUserCommand 以指定用户构造 goss validate 进程；降权语义与 applicationRunUserCommand 一致。
func gossRunUserCommand(ctx context.Context, gossPath, specPath string, rawVars any, runUser string) (*exec.Cmd, error) {
	arguments := []string{"-g", specPath, "validate", "--format", "json", "--no-color"}
	varsText := ""
	if rawVars != nil {
		switch typed := rawVars.(type) {
		case string:
			varsText = strings.TrimSpace(typed)
		default:
			if encoded, err := json.Marshal(typed); err == nil {
				varsText = string(encoded)
			}
		}
	}
	if varsText != "" {
		arguments = append(arguments, "--vars-inline", varsText)
	}
	command := exec.CommandContext(ctx, gossPath, arguments...)

	targetUser, err := user.Lookup(runUser)
	if err != nil {
		return nil, fmt.Errorf("无法查找 goss 运行用户 %q: %w", runUser, err)
	}
	targetUID, parseErr := strconv.ParseUint(targetUser.Uid, 10, 32)
	if parseErr != nil {
		return nil, fmt.Errorf("goss 运行用户 %q 的 UID 无效", runUser)
	}
	if uint64(os.Geteuid()) == targetUID {
		return command, nil
	}
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("dj-agent 必须以 root 或运行用户 %q 运行", runUser)
	}
	targetGID, gidErr := strconv.ParseUint(targetUser.Gid, 10, 32)
	if gidErr != nil {
		return nil, fmt.Errorf("goss 运行用户 %q 的 GID 无效", runUser)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid:    uint32(targetUID),
		Gid:    uint32(targetGID),
		Groups: supplementaryGroupIDs(targetUser),
	}}
	return command, nil
}

// mergeGossEnvironment 把检查项配置的环境变量并入 goss 进程环境。
// 白名单沿用命令注入的限制：应用上下文 + 巡检注入的 HOST_* 变量。
var allowedEnvironmentKeys = []string{"APP_HOME", "RUN_USER", "WORK_DIRECTORY", "APPLICATION_NAME", "APPLICATION_CODE", "APPLICATION_VERSION", "INSTANCE_NAME"}

var gossAllowedEnvironmentKeys = append([]string{"HOST_IP", "HOST_NAME"}, allowedEnvironmentKeys...)

func mergeGossEnvironment(baseEnvironment []string, rawEnvironment any) []string {
	configured, _ := rawEnvironment.(map[string]any)
	if len(configured) == 0 {
		return baseEnvironment
	}
	overrides := make(map[string]string)
	for _, key := range gossAllowedEnvironmentKeys {
		if value, exists := configured[key]; exists {
			overrides[key] = valueString(value)
		}
	}
	result := make([]string, 0, len(baseEnvironment)+len(overrides))
	for _, entry := range baseEnvironment {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, overridden := overrides[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

type gossJSONOutput struct {
	Results []gossJSONResult `json:"results"`
	Summary gossJSONSummary  `json:"summary"`
}

type gossJSONResult struct {
	Successful   bool   `json:"successful"`
	Skipped      bool   `json:"skipped"`
	ResourceID   string `json:"resource-id"`
	ResourceType string `json:"resource-type"`
	Property     string `json:"property"`
	SummaryLine  string `json:"summary-line"`
	MatcherResult struct {
		Expected any `json:"expected"`
	} `json:"matcher-result"`
}

type gossJSONSummary struct {
	TestCount    int `json:"test-count"`
	FailedCount  int `json:"failed-count"`
	SkippedCount int `json:"skipped-count"`
}

// runGossInspection 供 checkApplicationBaseline 之外的独立入口使用（能力探测）。
func (e *Executor) runGossInspection(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	checks := make([]applicationCheckResult, 0)
	if plan, ok := job.Params["check_plan"].(map[string]any); ok {
		rawChecks, _ := plan["checks"].([]any)
		for _, rawCheck := range rawChecks {
			check, valid := rawCheck.(map[string]any)
			if !valid {
				continue
			}
			if valueString(check["executor"]) == "goss" {
				checks = append(checks, e.checkGoss(ctx, check))
			}
		}
	}
	passed := true
	for _, check := range checks {
		if check.Status != "pass" && check.Status != "skipped" {
			passed = false
			break
		}
	}
	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action,
		Status: protocol.StatusSuccess, ExitCode: 0,
		StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
		Data: map[string]any{"passed": passed, "checks": checks},
	}
}

// gossDiagnostics 汇总各候选 goss 路径的执行诊断（文件/目录权限位、noexec、SELinux）。
func gossDiagnostics(paths []string) string {
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		fileMode, dirMode := "?", "?"
		if info, err := os.Stat(path); err == nil {
			fileMode = fmt.Sprintf("%04o", info.Mode().Perm())
		}
		if info, err := os.Stat(filepath.Dir(path)); err == nil {
			dirMode = fmt.Sprintf("%04o", info.Mode().Perm())
		}
		parts = append(parts, fmt.Sprintf("%s(file=%s,dir=%s,noexec=%v,selinux=%s)", path, fileMode, dirMode, mountNoexec(path), selinuxStatus()))
	}
	return "候选: " + strings.Join(parts, ", ")
}

func selinuxStatus() string {
	if raw, err := os.ReadFile("/sys/fs/selinux/enforce"); err == nil {
		if strings.TrimSpace(string(raw)) == "1" {
			return "enforcing"
		}
		return "permissive"
	}
	return "absent"
}
