package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"
)

// ---- 巡检 OPA 策略检查项 ----
//
// check_plan 里 OPA 检查项字段：
//   - policy: Rego 策略文本，必须产出 data.baseline.assertions 全量断言清单
//     （{name, pass, expected, actual}），空集即通过
//   - input_commands: 可选，shell 采集列表 [{key, exec, parse(raw|lines|json)}]
//   - input_files: 可选，文件直读列表 [{key, path, parse(raw|lines|json)}]
//   - run_user: 采集命令执行用户，留空默认 root，走与 goss 一致的降权语义
//
// input.json 信封：保留字段 host(ip/name)/vars；每条采集结果为顶层字段，
// 附 source/exit_code/exists 元信息。任一采集失败 → 检查直接 error
// （输入不完整时策略结论不可信），不进入求值。

// 结果契约固定查询：策略必须产出 data.baseline.assertions 全量断言清单。
// OPA 以 Go 库（rego 包）形式进程内求值：无二进制释放/noexec/进程开销。
const opaAssertionsQuery = "data.baseline.assertions"

// opaReservedInputKeys input 信封保留字段，采集 key 不允许占用。
var opaReservedInputKeys = map[string]bool{"host": true, "vars": true}

// opaInputRefPattern 匹配策略里对 input.<key> 的引用（取首段 key）。
var opaInputRefPattern = regexp.MustCompile(`input\.([A-Za-z_][A-Za-z0-9_]*)`)

// opaMissingInputKeys 返回策略引用了但采集列表里没有的 input key（剥离注释后扫描）。
func opaMissingInputKeys(policy string, collected map[string]bool) []string {
	stripped := &strings.Builder{}
	for _, line := range strings.Split(policy, "\n") {
		if index := strings.Index(line, "#"); index >= 0 {
			line = line[:index]
		}
		stripped.WriteString(line)
		stripped.WriteString("\n")
	}
	seen := map[string]bool{}
	var missing []string
	for _, match := range opaInputRefPattern.FindAllStringSubmatch(stripped.String(), -1) {
		key := match[1]
		if collected[key] || opaReservedInputKeys[key] || seen[key] {
			continue
		}
		seen[key] = true
		missing = append(missing, key)
	}
	sort.Strings(missing)
	return missing
}

func (e *Executor) checkOpa(ctx context.Context, check map[string]any) applicationCheckResult {
	config, _ := check["config"].(map[string]any)
	if config == nil {
		return newPlanCheckResult(check, "error", nil, "opa 检查项缺少 config")
	}
	policy := valueString(config["policy"])
	if strings.TrimSpace(policy) == "" {
		return newPlanCheckResult(check, "error", nil, "opa 策略不能为空")
	}
	commands, _ := config["input_commands"].([]any)
	files, _ := config["input_files"].([]any)
	if len(commands) == 0 && len(files) == 0 {
		return newPlanCheckResult(check, "error", nil, "opa 检查项至少要有一条采集（input_commands/input_files）")
	}
	// 策略引用的 input.<key> 必须在采集列表里，否则引用 undefined、断言静默消失
	// （旧数据没经过后端保存校验时在这里兜底成明确的计划级 error）。
	collected := map[string]bool{}
	for _, raw := range append(append([]any{}, commands...), files...) {
		entry, _ := raw.(map[string]any)
		if key := valueString(entry["key"]); key != "" {
			collected[key] = true
		}
	}
	if missing := opaMissingInputKeys(policy, collected); len(missing) > 0 {
		return newPlanCheckResult(check, "error", nil,
			fmt.Sprintf("策略引用了未采集的 input 字段: %s", strings.Join(missing, ", ")))
	}

	// ---- 采集：命令走 run_user 降权，文件直接读；任一失败即整体 error。 ----
	input := map[string]any{
		"host": map[string]any{
			"ip":   valueString(check["host_ip"]),
			"name": valueString(check["host_name"]),
		},
		"vars": opaInputVars(check["vars"]),
	}
	runUser := valueString(check["run_user"])
	if runUser == "" {
		runUser = "root"
	}

	// 采集脚本落临时目录执行（目录/脚本对 run_user 可穿越可读）：
	// 用户命令不再嵌入 su -c 的两层 shell 引号——非 POSIX 登录 shell（如 csh）会把
	// '"'"' 转义弄乱，报"寻找匹配的引号"类语法错误。
	workDir, workErr := os.MkdirTemp("", "djagent-opa-run-")
	if workErr != nil {
		return newPlanCheckResult(check, "error", nil, fmt.Sprintf("创建采集临时目录失败: %v", workErr))
	}
	defer os.RemoveAll(workDir)
	if err := os.Chmod(workDir, 0o755); err != nil {
		return newPlanCheckResult(check, "error", nil, fmt.Sprintf("设置采集临时目录权限失败: %v", err))
	}

	collect, collectErr := opaCollect(ctx, workDir, input, commands, files, runUser)
	if collectErr != nil {
		return newPlanCheckResult(check, "error", nil, collectErr.Error())
	}

	// ---- 求值：rego 库进程内执行，不碰系统、不降权。 ----
	// 契约唯一：data.baseline.assertions 全量断言清单（pass/fail 都在，报告完整展示）；
	// 失败断言同时计入 violations。空集（含 undefined）= 通过。
	assertionEntries, assertionsDefined, evalErr := evalOpaQuery(ctx, opaAssertionsQuery, policy, input)
	if evalErr != nil {
		return newPlanCheckResult(check, "error", nil, evalErr.Error())
	}
	violations := make([]map[string]any, 0)
	details := make([]map[string]any, 0, len(assertionEntries))
	if assertionsDefined {
		for _, rawEntry := range assertionEntries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				return newPlanCheckResult(check, "error", nil,
					fmt.Sprintf("assertions 元素必须是对象（含 name/pass）: %v", rawEntry))
			}
			pass, ok := entry["pass"].(bool)
			if !ok {
				return newPlanCheckResult(check, "error", nil,
					fmt.Sprintf("断言 %v 缺少 pass 布尔值", entry["name"]))
			}
			name := valueString(entry["name"])
			if name == "" {
				name = valueString(entry["msg"])
			}
			msg := valueString(entry["msg"])
			details = append(details, map[string]any{
				"resource": "OPA: assertions", "property": "assertion", "successful": pass,
				"message": msg, "expected": entry["expected"], "actual": entry["actual"],
				"title": name,
			})
			if !pass {
				item, _ := entry["item"].(map[string]any)
				if item == nil {
					item = map[string]any{"expected": entry["expected"], "actual": entry["actual"]}
				}
				violations = append(violations, map[string]any{"msg": firstNonEmptyStrings(msg, name), "item": item})
			}
		}
	}

	failed := len(violations)
	messages := make([]string, 0, failed)
	for _, violation := range violations {
		if msg := valueString(violation["msg"]); msg != "" {
			messages = append(messages, msg)
		}
	}

	actual := map[string]any{
		"violation_count": failed,
		"violations":      violations,
		"inputs":          collect,
		"details":         details,
		"run_user":        runUser,
	}
	expected := map[string]any{
		"query":  opaAssertionsQuery,
		"policy": policy,
	}
	if failed > 0 {
		result := newPlanCheckResult(check, "fail", actual,
			fmt.Sprintf("OPA 策略违规 %d 项: %s", failed, strings.Join(messages, "; ")))
		result.Expected = expected
		return result
	}
	result := newPlanCheckResult(check, "pass", actual, "")
	result.Expected = expected
	return result
}

// opaCollect 执行全部采集并把结果写入 input 顶层字段；返回采集元信息（诊断用）。
// 任一采集失败返回错误（整体 error）。
func opaCollect(ctx context.Context, workDir string, input map[string]any, commands, files []any, runUser string) (map[string]any, error) {
	collected := make(map[string]any, len(commands)+len(files))
	scriptSeq := 0

	// writeCommandScript 把用户命令原样写入脚本文件（不经任何 shell 引号转义），
	// 返回可执行脚本路径。
	writeCommandScript := func(key, execText string) (string, error) {
		scriptPath := filepath.Join(workDir, fmt.Sprintf("collect-%d.sh", scriptSeq))
		scriptSeq++
		script := "#!/bin/sh\n" + execText + "\n"
		if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
			return "", fmt.Errorf("采集 %q 写脚本失败: %v", key, err)
		}
		if err := os.Chmod(scriptPath, 0o755); err != nil {
			return "", fmt.Errorf("采集 %q 设置脚本权限失败: %v", key, err)
		}
		return scriptPath, nil
	}

	readCommand := func(raw any) error {
		item, valid := raw.(map[string]any)
		if !valid {
			return fmt.Errorf("input_commands 条目必须是对象")
		}
		key, execText, parse, err := opaInputKey(item)
		if err != nil {
			return err
		}
		scriptPath, scriptErr := writeCommandScript(key, execText)
		if scriptErr != nil {
			return scriptErr
		}
		var stdout, stderr strings.Builder
		// root 直接执行，非 root 走 su -l 登录 shell（与 goss 相同的环境语义）；
		// su -c 只携带纯脚本路径，不含任何用户命令字符，两层引号问题不复存在。
		command := opaRunUserCommand(ctx, "/bin/sh "+scriptPath, runUser)
		command.Stdout = &stdout
		command.Stderr = &stderr
		runErr := command.Run()
		exitCode := 0
		if runErr != nil {
			exitCode = -1
			var exitErr *exec.ExitError
			if asExitError(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
		}
		if exitCode != 0 {
			detail := strings.TrimSpace(stderr.String())
			if detail == "" {
				detail = strings.TrimSpace(stdout.String())
			}
			// 退出码 126 = 不可执行：十有八九是把文件路径填进了采集命令（exec），
			// shell 试图把文件当程序执行。提示改用文件采集。
			hint := ""
			if exitCode == 126 {
				hint = "（exec 会被当作命令执行：如需读取文件内容，请改用 input_files 文件采集）"
			}
			return fmt.Errorf("采集 %q 失败（退出码 %d）: %s%s", key, exitCode, detail, hint)
		}
		input[key] = opaParsedOutput("command", stdout.String(), exitCode, parse)
		collected[key] = map[string]any{"source": "command", "exit_code": exitCode}
		return nil
	}

	readFile := func(raw any) error {
		item, valid := raw.(map[string]any)
		if !valid {
			return fmt.Errorf("input_files 条目必须是对象")
		}
		key, path, parse, err := opaInputKey(item)
		if err != nil {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("采集 %q 读取 %s 失败: %v", key, path, readErr)
		}
		input[key] = opaParsedOutput("file", string(data), 0, parse)
		input[key].(map[string]any)["exists"] = true
		collected[key] = map[string]any{"source": "file", "exists": true}
		return nil
	}

	for _, raw := range commands {
		if err := readCommand(raw); err != nil {
			return collected, err
		}
	}
	for _, raw := range files {
		if err := readFile(raw); err != nil {
			return collected, err
		}
	}
	return collected, nil
}

// opaInputKey 校验并提取采集条目的 key/exec(path)/parse。
func opaInputKey(item map[string]any) (key, source, parse string, err error) {
	key = valueString(item["key"])
	source = valueString(item["exec"])
	if source == "" {
		source = valueString(item["path"])
	}
	parse = valueString(item["parse"])
	if parse == "" {
		parse = "raw"
	}
	if key == "" || source == "" {
		return "", "", "", fmt.Errorf("采集条目缺少 key 或 exec/path")
	}
	if opaReservedInputKeys[key] {
		return "", "", "", fmt.Errorf("采集 key %q 与 input 保留字段冲突", key)
	}
	switch parse {
	case "raw", "lines", "json":
	default:
		return "", "", "", fmt.Errorf("采集 %q 的 parse 不支持 %q（raw/lines/json）", key, parse)
	}
	return key, source, parse, nil
}

// opaParsedOutput 按声明格式把采集输出转为 input 字段。
func opaParsedOutput(source, output string, exitCode int, parse string) map[string]any {
	result := map[string]any{"source": source, "exit_code": exitCode}
	switch parse {
	case "lines":
		split := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
		lines := make([]string, 0, len(split))
		for _, line := range split {
			if line != "" {
				lines = append(lines, line)
			}
		}
		result["lines"] = lines
	case "json":
		var decoded any
		if err := json.Unmarshal([]byte(output), &decoded); err != nil {
			// JSON 解析失败保留原文，策略侧通过 raw 自行处理（或报采集错误）。
			result["raw"] = output
			result["json_error"] = err.Error()
		} else {
			result["data"] = decoded
		}
	default:
		result["raw"] = output
	}
	return result
}

// opaRunUserCommand 执行采集脚本命令：root 直接执行，非 root 走 su -l 登录 shell。
// commandText 是 "/bin/sh /path/collect-N.sh" 形式的纯脚本调用（无用户命令字符）。
func opaRunUserCommand(ctx context.Context, commandText, runUser string) *exec.Cmd {
	target, err := user.Lookup(runUser)
	if err == nil {
		if uid, parseErr := strconv.ParseUint(target.Uid, 10, 32); parseErr == nil && uint64(os.Geteuid()) == uid {
			return exec.CommandContext(ctx, "/bin/sh", "-c", commandText)
		}
	}
	if os.Geteuid() != 0 {
		return exec.CommandContext(ctx, "/bin/sh", "-c", commandText)
	}
	return exec.CommandContext(ctx, "su", "-l", runUser, "-c", commandText)
}

// opaInputVars 提取检查项的 vars（JSON 对象），策略通过 input.vars 引用。
func opaInputVars(raw any) map[string]any {
	vars, _ := raw.(map[string]any)
	if vars == nil {
		return map[string]any{}
	}
	return vars
}

// evalOpaQuery 进程内求值一个查询，返回结果元素列表。
// undefined（规则未触发、结果集为空）返回 defined=false。
func evalOpaQuery(ctx context.Context, query, policy string, input map[string]any) ([]any, bool, error) {
	resultSet, err := rego.New(
		rego.Query(query),
		rego.Module("baseline.rego", policy),
		rego.Input(input),
	).Eval(ctx)
	if err != nil {
		// 求值失败（含策略编译错误）必须 error：绝不能被当成 undefined（空集=通过）。
		return nil, false, fmt.Errorf("opa 求值失败: %v", err)
	}
	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		// undefined：规则未触发，结果集为空。
		return nil, false, nil
	}
	value, ok := resultSet[0].Expressions[0].Value.([]any)
	if !ok {
		// 求值结果为 null / 非数组：视为空集（无内容）。
		return nil, false, nil
	}
	return value, true, nil
}

func firstNonEmptyStrings(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}



func asExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*target = exitErr
	}
	return ok
}
