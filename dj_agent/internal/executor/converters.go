package executor

import (
	"fmt"
	"strings"
)

// toString 将任意值转换为字符串，nil 返回空字符串
func toString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// toStringMap 将任意值转换为 map[string]string
func toStringMap(value any) map[string]string {
	result := map[string]string{}
	if value == nil {
		return result
	}
	if typed, ok := value.(map[string]string); ok {
		for key, item := range typed {
			result[key] = item
		}
		return result
	}
	if typed, ok := value.(map[string]any); ok {
		for key, item := range typed {
			result[key] = toString(item)
		}
	}
	return result
}

// toAnyMap 将任意值转换为 map[string]any
func toAnyMap(value any) map[string]any {
	result := map[string]any{}
	if value == nil {
		return result
	}
	if typed, ok := value.(map[string]any); ok {
		for key, item := range typed {
			result[key] = item
		}
		return result
	}
	if typed, ok := value.(map[string]string); ok {
		for key, item := range typed {
			result[key] = item
		}
	}
	return result
}

// toBool 将任意值转换为布尔值
// 支持 bool、string ("1"/"true"/"yes"/"on")、int/float64(非零)
func toBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		text := strings.TrimSpace(strings.ToLower(typed))
		return text == "1" || text == "true" || text == "yes" || text == "on"
	case int:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}
