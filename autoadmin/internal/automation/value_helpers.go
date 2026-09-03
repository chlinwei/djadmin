package automation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func marshalJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// decodeJSONInt64Array 解析 JSON 数字数组列（如 selected_host_ids），非法内容按空数组处理。
func decodeJSONInt64Array(raw string) []int64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var items []int64
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

func jsonObject(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	var object map[string]any
	if raw, ok := value.([]byte); ok {
		_ = json.Unmarshal(raw, &object)
	}
	if object == nil {
		return map[string]any{}
	}
	return object
}

func stringValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}

func nullableID(value *int64) any {
	if value == nil || *value < 1 {
		return nil
	}
	return *value
}

func jsonID(value any) (int64, bool) {
	switch value := value.(type) {
	case int64:
		return value, value > 0
	case int:
		return int64(value), value > 0
	case float64:
		return int64(value), value > 0
	case string:
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return id, err == nil && id > 0
	default:
		return 0, false
	}
}

func int64Slice(value any) []int64 {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]int64, 0, len(items))
	for _, item := range items {
		switch item := item.(type) {
		case int64:
			result = append(result, item)
		case float64:
			result = append(result, int64(item))
		case string:
			var number int64
			if _, err := fmt.Sscan(strings.TrimSpace(item), &number); err == nil {
				result = append(result, number)
			}
		}
	}
	return result
}
