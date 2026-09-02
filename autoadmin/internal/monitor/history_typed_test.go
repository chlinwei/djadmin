package monitor

import (
	"encoding/json"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

// 回归用例：resolved_by_reconciliation 是 monitor_alert_history 的 TINYINT(1) 列，
// 序列化到 JSON 响应里必须是布尔值，不能是数字 0/1。
func TestAlertHistoryResponseSerializesResolvedByReconciliationAsBoolean(t *testing.T) {
	row := db.ListAlertHistoriesRow{
		ID: 1, Alertname: "HighCPU", ResolvedByReconciliation: true,
		Labels: json.RawMessage(`{}`), Annotations: json.RawMessage(`{}`), RuleSnapshot: json.RawMessage(`{"expr":"up==0"}`),
	}

	encoded, err := json.Marshal(alertHistoryResponseFrom(row))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if decoded["resolved_by_reconciliation"] != true {
		t.Fatalf("resolved_by_reconciliation = %#v, want JSON boolean true", decoded["resolved_by_reconciliation"])
	}
	if _, ok := decoded["rule_details"].(map[string]any); !ok {
		t.Fatalf("rule_details type = %T, want a JSON object", decoded["rule_details"])
	}
}

func TestNotificationStatusFromMatchesExpectedPriority(t *testing.T) {
	cases := []struct {
		name                            string
		count, failed, active, delivery int64
		want                            string
	}{
		{"no notifications", 0, 0, 0, 0, "none"},
		{"has failure", 3, 1, 0, 2, "failed"},
		{"active in progress", 3, 0, 1, 1, "in_progress"},
		{"delivered successfully", 3, 0, 0, 3, "success"},
		{"delivered nothing counts as failed", 1, 0, 0, 0, "failed"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := notificationStatusFrom(testCase.count, testCase.failed, testCase.active, testCase.delivery)
			if got != testCase.want {
				t.Fatalf("notificationStatusFrom(%d,%d,%d,%d) = %q, want %q", testCase.count, testCase.failed, testCase.active, testCase.delivery, got, testCase.want)
			}
		})
	}
}
