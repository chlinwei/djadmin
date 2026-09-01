package inspection

import "testing"

func TestNormalizeValueEnabledAsBoolean(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "bool true", value: true, want: true},
		{name: "integer true", value: int64(1), want: true},
		{name: "integer false", value: int64(0), want: false},
		{name: "bytes true", value: []byte("1"), want: true},
		{name: "bytes false", value: []byte("0"), want: false},
		{name: "string true", value: "true", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := normalizeValue("enabled", test.value).(bool)
			if !ok || got != test.want {
				t.Fatalf("normalizeValue(enabled, %v) = %v, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestNormalizeValueLeavesOtherNumericColumnsUntouched(t *testing.T) {
	got := normalizeValue("status", int64(1))
	if got != int64(1) {
		t.Fatalf("normalizeValue(status, 1) = %v, want int64(1)", got)
	}
}
