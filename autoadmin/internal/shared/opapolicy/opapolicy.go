// Package opapolicy 提供 OPA 巡检检查项配置的共享校验。
// 结果契约唯一：策略必须产出 data.baseline.assertions（{name, pass, expected, actual}
// 全量断言清单），空集即通过；失败断言即违规。
package opapolicy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ReservedInputKeys input 信封保留字段，采集 key 不允许占用。
var ReservedInputKeys = map[string]bool{"host": true, "vars": true}

// inputRefPattern 匹配策略里对 input.<key> 的引用（取首段 key）。
var inputRefPattern = regexp.MustCompile(`input\.([A-Za-z_][A-Za-z0-9_]*)`)

// Validate 校验 OPA 检查项 config（反序列化后的对象）。
// 返回 "" 表示合法，否则为面向用户的错误描述。
func Validate(config map[string]any) string {
	policy, _ := config["policy"].(string)
	if strings.TrimSpace(policy) == "" {
		return "OPA 策略不能为空"
	}
	if !strings.Contains(policy, "package baseline") {
		return "OPA 策略必须以 package baseline 开头（结果契约固定为 data.baseline.assertions）"
	}
	// OPA v1 语法，唯一契约：全量断言清单（pass/fail 都展示，空集即通过）。
	if !strings.Contains(policy, "assertions contains") {
		return `OPA 策略必须以 "assertions contains <元素> if { ... }" 产出全量断言清单`
	}
	total := 0
	collected := map[string]bool{}
	for _, field := range []string{"input_commands", "input_files"} {
		items, _ := config[field].([]any)
		total += len(items)
		for _, raw := range items {
			entry, ok := raw.(map[string]any)
			if !ok {
				return fmt.Sprintf("%s 条目必须是对象", field)
			}
			key, _ := entry["key"].(string)
			if strings.TrimSpace(key) == "" {
				return fmt.Sprintf("%s 条目缺少 key", field)
			}
			if ReservedInputKeys[key] {
				return fmt.Sprintf("采集 key %q 与 input 保留字段冲突", key)
			}
			collected[key] = true
			source, hasCommand := entry["exec"].(string)
			path, hasPath := entry["path"].(string)
			if (!hasCommand && !hasPath) || (hasCommand && strings.TrimSpace(source) == "") || (hasPath && strings.TrimSpace(path) == "") {
				return fmt.Sprintf("采集 %q 缺少 exec 或 path", key)
			}
			if parse, ok := entry["parse"].(string); ok {
				switch parse {
				case "raw", "lines", "json":
				default:
					return fmt.Sprintf("采集 %q 的 parse 仅支持 raw/lines/json", key)
				}
			}
		}
	}
	if total == 0 {
		return "OPA 检查项至少要有一条采集（input_commands/input_files）"
	}
	// 策略引用的 input.<key> 必须在采集列表里：否则引用在求值时为 undefined，
	// 断言会静默消失（表现为"这条不见了"），必须在保存时提前拦住。
	if missing := missingInputKeys(policy, collected); len(missing) > 0 {
		return fmt.Sprintf("策略引用了未采集的 input 字段: %s（请核对采集 key 拼写，或在采集列表补上）", strings.Join(missing, ", "))
	}
	return ""
}

// missingInputRefs 返回策略引用了但采集列表里没有的 input key（剥离注释后扫描，
// 保留字段 host/vars 放行）。返回 nil 表示没有缺失。
func missingInputKeys(policy string, collected map[string]bool) []string {
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
	for _, match := range inputRefPattern.FindAllStringSubmatch(stripped.String(), -1) {
		key := match[1]
		if collected[key] || ReservedInputKeys[key] || seen[key] {
			continue
		}
		seen[key] = true
		missing = append(missing, key)
	}
	sort.Strings(missing)
	return missing
}
