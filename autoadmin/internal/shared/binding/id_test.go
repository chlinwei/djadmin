package binding

import (
	"encoding/json"
	"testing"
)

func TestIDAcceptsNumberAndString(t *testing.T) {
	var ids []ID
	if err := json.Unmarshal([]byte(`[1,"2"]`), &ids); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	values := Int32s(ids)
	if len(values) != 2 || values[0] != 1 || values[1] != 2 {
		t.Fatalf("unexpected IDs: %v", values)
	}
}

func TestIDRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{`[0]`, `[-1]`, `["x"]`} {
		var ids []ID
		if err := json.Unmarshal([]byte(input), &ids); err == nil {
			t.Fatalf("expected %s to fail", input)
		}
	}
}
