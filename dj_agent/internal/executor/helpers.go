package executor

import (
	"fmt"
	"strconv"
	"strings"
)

// anyToSlice 把接口值安全转成 []any（非切片返回 nil）。
func anyToSlice(raw any) []any {
	value, _ := raw.([]any)
	return value
}

// valueString 把任意值规范化为字符串：数字/布尔等也 stringify（原 TrimSpace(fmt.Sprint) 语义，
// 供 plan 字段读取各处使用）。
func valueString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// numberToInt 把 JSON 数字（int/float64/string）转成 int。
func numberToInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case float64:
		return int(typed), nil
	case string:
		return strconv.Atoi(typed)
	default:
		return 0, fmt.Errorf("invalid number")
	}
}

// stringSliceFromAny 把值转成 []string：接受 []string 或 []any 或 nil，逐项 Trim 并去空。
func stringSliceFromAny(value any) []string {
	if value == nil {
		return nil
	}
	if typed, ok := value.([]string); ok {
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			normalized := strings.TrimSpace(item)
			if normalized != "" {
				result = append(result, normalized)
			}
		}
		return result
	}
	rawList, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(rawList))
	for _, item := range rawList {
		normalized := valueString(item)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}
