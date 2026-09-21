package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// Shell 类 playbook 模板的校验与执行包装。
//
// 设计要点（2026-09-21 定稿）：
//   - 语法/静态检查只用 shellcheck（覆盖 bash -n 的语法能力 + 引号/变量等静态分析），
//     不依赖宿主机 bash；二进制的就绪方式两种：平台上传（shellcheckDir/shellcheck，离线内网）
//     或服务器直接安装（PATH）。解析顺序：上传优先 → PATH → 未就绪。
//   - 执行时不做任何校验侧包装以外的身份处理：包装壳只有 hosts/gather_facts/shell 任务，
//     身份沿用任务级机制（默认 ssh 用户 root；run_as_user 配置后由 --become --become-user 切换）。
//   - content 始终存原始脚本原文，包装只在执行渲染快照时发生（幂等，可重放）。

const (
	playbookFormatPlaybook = "playbook"
	playbookFormatShell    = "shell"

	shellcheckFileName = "shellcheck"
)

// shellcheckFinding 对应 shellcheck -f json1 的单条告警（level: error/warning/info/style）。
// 字段名以 ShellCheck 的 JSON 约定为准（endLine 是驼峰，不是 end_line）。
type shellcheckFinding struct {
	Line    int    `json:"line"`
	EndLine int    `json:"endLine"`
	Column  int    `json:"column"`
	Level   string `json:"level"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// shellcheckStatus 是组件状态接口的响应：ready=false 时前端引导上传或服务器安装。
type shellcheckStatus struct {
	Ready   bool   `json:"ready"`
	Source  string `json:"source"` // uploaded=平台上传 / system=服务器 PATH 安装
	Version string `json:"version"`
	Path    string `json:"path"`
}

// shellcheckBinaryPath 解析可用的 shellcheck 二进制：上传的优先，其次 PATH。找不到返回空串。
func (handler *Handler) shellcheckBinaryPath() string {
	if handler.shellcheckDir != "" {
		uploaded := filepath.Join(handler.shellcheckDir, shellcheckFileName)
		if info, err := os.Stat(uploaded); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return uploaded
		}
	}
	if path, err := exec.LookPath(shellcheckFileName); err == nil {
		return path
	}
	return ""
}

// shellcheckVersion 跑一次 --version 取首行（形如 "ShellCheck - shell script analysis tool" /
// "version: 0.9.0"），失败返回空串（探测失败不阻塞状态展示）。
func shellcheckVersion(ctx context.Context, binary string) string {
	output, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		if version, found := strings.CutPrefix(strings.TrimSpace(line), "version:"); found {
			return strings.TrimSpace(version)
		}
	}
	return strings.TrimSpace(strings.Split(string(output), "\n")[0])
}

// ShellcheckStatus 组件就绪状态（GET /sys/automation/shellcheck/binary/）。
func (handler *Handler) ShellcheckStatus(context *gin.Context) {
	binary := handler.shellcheckBinaryPath()
	if binary == "" {
		response.Success(context, gin.H{"ready": false})
		return
	}
	source := "system"
	if handler.shellcheckDir != "" && filepath.Join(handler.shellcheckDir, shellcheckFileName) == binary {
		source = "uploaded"
	}
	response.Success(context, shellcheckStatus{
		Ready: true, Source: source, Version: shellcheckVersion(context, binary), Path: binary,
	})
}

// UploadShellcheckBinary 平台上传 shellcheck 二进制（POST，multipart 字段 file，覆盖式）。
// 离线内网环境的就绪通道：存 <shellcheckDir>/shellcheck 并 chmod 0755。
func (handler *Handler) UploadShellcheckBinary(context *gin.Context) {
	fileHeader, err := context.FormFile("file")
	if err != nil {
		context.JSON(http.StatusOK, gin.H{"code": 400, "msg": "请选择要上传的 shellcheck 二进制文件（multipart 字段 file）", "data": nil})
		return
	}
	if handler.shellcheckDir == "" {
		context.JSON(http.StatusOK, gin.H{"code": 400, "msg": "服务器未配置 shellcheck 存储目录", "data": nil})
		return
	}
	if err := os.MkdirAll(handler.shellcheckDir, 0755); err != nil {
		context.JSON(http.StatusOK, gin.H{"code": 400, "msg": "创建存储目录失败: " + err.Error(), "data": nil})
		return
	}
	target := filepath.Join(handler.shellcheckDir, shellcheckFileName)
	if err := context.SaveUploadedFile(fileHeader, target); err != nil {
		context.JSON(http.StatusOK, gin.H{"code": 400, "msg": "保存上传文件失败: " + err.Error(), "data": nil})
		return
	}
	if err := os.Chmod(target, 0755); err != nil {
		context.JSON(http.StatusOK, gin.H{"code": 400, "msg": "设置可执行权限失败: " + err.Error(), "data": nil})
		return
	}
	handler.ShellcheckStatus(context)
}

// DeleteShellcheckBinary 删除平台上传的二进制（回退到只认服务器 PATH 安装）。
func (handler *Handler) DeleteShellcheckBinary(context *gin.Context) {
	if handler.shellcheckDir != "" {
		_ = os.Remove(filepath.Join(handler.shellcheckDir, shellcheckFileName))
	}
	handler.ShellcheckStatus(context)
}

// validateShellScript 用 shellcheck 校验脚本：返回 (findings, error)。
//   - 组件未就绪 → error（文案带两种就绪方式），前端引导上传或安装；
//   - shellcheck 正常跑完 → error 为 nil，findings 是全部告警（含 error 级），
//     "能否保存"由调用方按级别判定（error 级拒绝，warning/info/style 放行）。
func (handler *Handler) validateShellScript(ctx context.Context, content string) ([]shellcheckFinding, error) {
	binary := handler.shellcheckBinaryPath()
	if binary == "" {
		return nil, errors.New("服务器未就绪 ShellCheck：请在「自动化运维 → 模板」页面上传 shellcheck 二进制，或在服务器执行 yum/apt install shellcheck")
	}
	directory, err := os.MkdirTemp("", "autoadmin-shellcheck-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	scriptPath := filepath.Join(directory, "script.sh")
	if err := os.WriteFile(scriptPath, []byte(content), 0600); err != nil {
		return nil, err
	}
	output, err := exec.CommandContext(ctx, binary, "--shell=bash", "-f", "json1", scriptPath).Output()
	if err != nil {
		// shellcheck 对脚本报错也以退出码区分，但 JSON 照样输出在 stdout；只有真正跑挂了才走这里。
		if exitErr, ok := err.(*exec.ExitError); ok && len(output) > 0 {
			_ = exitErr
		} else {
			return nil, fmt.Errorf("执行 shellcheck 失败: %w", err)
		}
	}
	return parseShellcheckOutput(output)
}

// parseShellcheckOutput 解析 shellcheck 的 JSON 输出。
//
// 格式差异：`-f json1`（0.8+）返回对象 `{"comments":[{...}]}`，而更早/其它格式返回数组；
// 空结果时可能直接是 `[]` / `{}` / 空串。这里按首字符自适应，避免把"没有告警"解析成错误。
func parseShellcheckOutput(output []byte) ([]shellcheckFinding, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var list []shellcheckFinding
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, fmt.Errorf("解析 shellcheck 输出失败: %w", err)
		}
		return list, nil
	}
	var wrapper struct {
		Comments []shellcheckFinding `json:"comments"`
	}
	if err := json.Unmarshal(trimmed, &wrapper); err != nil {
		return nil, fmt.Errorf("解析 shellcheck 输出失败: %w", err)
	}
	return wrapper.Comments, nil
}

// fatalShellcheckFindings 找出 error 级告警（语法错误等）：它们存在时拒绝保存。
func fatalShellcheckFindings(findings []shellcheckFinding) []shellcheckFinding {
	fatal := []shellcheckFinding{}
	for _, finding := range findings {
		if finding.Level == "error" {
			fatal = append(fatal, finding)
		}
	}
	return fatal
}

// wrapShellPlaybook 把裸 shell 脚本包装成单任务 playbook（执行渲染快照时调用）。
// 身份/变量不在这里处理：默认走 inventory 的 ssh 用户（root），run_as 由 --become 切换，
// 参数由 --extra-vars 注入——包装壳只负责"把脚本原文作为唯一的 shell 任务跑起来"。
func wrapShellPlaybook(templateName, script string) string {
	escapedName := strings.ReplaceAll(strings.TrimSpace(templateName), "'", "")
	var builder strings.Builder
	builder.WriteString("- hosts: all\n  gather_facts: false\n  tasks:\n    - name: ")
	builder.WriteString(fmt.Sprintf("%q\n", escapedName))
	builder.WriteString("      ansible.builtin.shell: |")
	for _, line := range strings.Split(strings.TrimRight(script, "\n"), "\n") {
		builder.WriteString("\n        ")
		builder.WriteString(line)
	}
	builder.WriteString("\n      args:\n        executable: /bin/bash\n")
	return builder.String()
}
