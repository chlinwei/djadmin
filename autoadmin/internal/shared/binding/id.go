package binding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

type ID int32

func (id *ID) UnmarshalJSON(data []byte) error {
	raw := bytes.TrimSpace(data)
	if len(raw) == 0 {
		return fmt.Errorf("empty ID")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return fmt.Errorf("decode ID string: %w", err)
		}
		raw = []byte(text)
	}
	value, err := strconv.ParseInt(string(raw), 10, 32)
	if err != nil || value < 1 {
		return fmt.Errorf("ID must be a positive integer")
	}
	*id = ID(value)
	return nil
}

func Int32s(ids []ID) []int32 {
	result := make([]int32, len(ids))
	for index, id := range ids {
		result[index] = int32(id)
	}
	return result
}
