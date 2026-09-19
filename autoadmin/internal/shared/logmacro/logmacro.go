// Package logmacro 是日志路径宏的唯一实现。
//
// 为什么单独抽成共享包（2026-09-19）：宏的解析顺序（**部署模板 macro_definitions 的 value →
// 服务级 macro_values → 部署实例 runtime_variables**，模板 app_home 作为 APP_HOME 默认值）
// 有两类消费方 —— **下发渲染**（logcollect，逐实例展开出主机上的真实路径）与**界面展示**
// （assets 的 log-config，给"解析后路径"那一列用）。两边各写一份合并顺序，迟早出现
// "界面显示的路径和主机上实际采的不一样"，而那种不一致比显示占位符更糟（用户会照着界面找文件）。
// 所以合并顺序只有这一份，两处都调它。
package logmacro

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseValues 把 JSON 对象（`{"APP_HOME":"/opt/x"}`）摊平成 name→value；非对象/非法 JSON 按空处理。
func ParseValues(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return map[string]string{}
	}
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return map[string]string{}
	}
	result := map[string]string{}
	for key, value := range decoded {
		result[key] = strings.TrimSpace(fmt.Sprint(value))
	}
	return result
}

// TemplateDefaults 把部署模板的 macro_definitions（`[{name,value,description}]`）摊平成
// name→value，作为宏解析的默认值；服务级 macro_values 覆盖同名项。
// 口径与服务弹窗一致（弹窗显示"服务覆盖 ?? 模板默认"），避免前端看着有值、后端展不开。
func TemplateDefaults(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" {
		return map[string]string{}
	}
	var definitions []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(trimmed), &definitions); err != nil {
		return map[string]string{}
	}
	result := map[string]string{}
	for _, definition := range definitions {
		name := strings.TrimSpace(definition.Name)
		if name == "" {
			continue
		}
		result[name] = strings.TrimSpace(definition.Value)
	}
	return result
}

// Merge 合并宏集合：**后面的覆盖前面的**（默认值在前、覆盖值在后）。
func Merge(sets ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, set := range sets {
		for key, value := range set {
			merged[key] = value
		}
	}
	return merged
}

// InstanceValues 实例级宏：runtime_variables 为基底，部署模板 app_home 作为 APP_HOME 的默认值
// （实例显式配了 APP_HOME 就听实例的）。
func InstanceValues(runtimeVariablesRaw, appHome string) map[string]string {
	macros := ParseValues(runtimeVariablesRaw)
	if appHome = strings.TrimSpace(appHome); appHome != "" {
		if _, exists := macros["APP_HOME"]; !exists {
			macros["APP_HOME"] = appHome
		}
	}
	return macros
}

// Resolve 按合并后的宏替换路径里的 `${KEY}`；**未定义的宏保持原样**（调用方据此发现"还没展开"）。
func Resolve(path string, sets ...map[string]string) string {
	merged := Merge(sets...)
	result := path
	for key, value := range merged {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	return result
}

// Pending 返回路径里还没展开的宏名（去重、保持出现顺序）。界面用它标注"这些要到实例上才展开"。
func Pending(path string) []string {
	seen := map[string]bool{}
	pending := []string{}
	rest := path
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			break
		}
		end := strings.Index(rest[start:], "}")
		if end < 0 {
			break
		}
		name := rest[start+2 : start+end]
		if name != "" && !seen[name] {
			seen[name] = true
			pending = append(pending, name)
		}
		rest = rest[start+end+1:]
	}
	return pending
}
