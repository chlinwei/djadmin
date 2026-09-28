package assets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateServiceExtraProcessorsNormalizesEmpty(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("  []  "), json.RawMessage("[]")} {
		normalized, err := ValidateServiceExtraProcessors(raw)
		if err != nil {
			t.Fatalf("input %s: unexpected error %v", raw, err)
		}
		if string(normalized) != "[]" {
			t.Fatalf("input %s: normalized = %s, want []", raw, normalized)
		}
		if ExtraProcessorsEnabled(normalized) {
			t.Fatalf("[] 不应视为已启用")
		}
	}
}

func TestValidateServiceExtraProcessorsAcceptsProcessors(t *testing.T) {
	raw := json.RawMessage(`[{"drop_fields":{"fields":["host.name"]}},{"convert":{"field":"status","type":"integer"}}]`)
	normalized, err := ValidateServiceExtraProcessors(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ExtraProcessorsEnabled(normalized) {
		t.Fatalf("非空数组应视为已启用")
	}
	var items []map[string]any
	if err = json.Unmarshal(normalized, &items); err != nil || len(items) != 2 {
		t.Fatalf("归一化后应仍是 2 个处理器: %v %s", err, normalized)
	}
}

func TestValidateServiceExtraProcessorsRejectsBadShapes(t *testing.T) {
	cases := []struct {
		name    string
		raw     json.RawMessage
		contain string
	}{
		{"不是数组", json.RawMessage(`{"drop_fields":{}}`), "JSON 数组"},
		{"元素不是对象", json.RawMessage(`[1]`), "处理器对象"},
		{"多个处理器键", json.RawMessage(`[{"drop_fields":{},"rename":{}}]`), "恰好一个"},
		{"空对象", json.RawMessage(`[{}]`), "恰好一个"},
		{"嵌套 pipeline", json.RawMessage(`[{"pipeline":{"name":"x"}}]`), "pipeline"},
		{"超数量", json.RawMessage(strings.Repeat(`[{"drop_fields":{}}]`, 0) + `[` + strings.TrimSuffix(strings.Repeat(`{"drop_fields":{}},`, MaxServiceExtraProcessors+1), ",") + `]`), "最多"},
	}
	for _, testCase := range cases {
		if _, err := ValidateServiceExtraProcessors(testCase.raw); err == nil {
			t.Fatalf("%s: 应当拒绝", testCase.name)
		} else if !strings.Contains(err.Error(), testCase.contain) {
			t.Fatalf("%s: 报错应包含 %q，得到 %q", testCase.name, testCase.contain, err.Error())
		}
	}
}

func TestValidateServiceExtraProcessorsRejectsOversized(t *testing.T) {
	raw := json.RawMessage(`[{"drop_fields":{"fields":"` + strings.Repeat("x", MaxServiceExtraProcessorsBytes) + `"}}]`)
	if _, err := ValidateServiceExtraProcessors(raw); err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("超大 payload 应被拒绝，得到 %v", err)
	}
}
