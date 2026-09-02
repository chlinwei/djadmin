package inspection

import (
	"encoding/json"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

// 回归用例：inspection_group/inspection_check 的 enabled 是 TINYINT(1) 列，序列化到 JSON
// 响应里必须是布尔值；config 必须是解码后的对象，不能是转义字符串。
func TestInspectionCheckResponseFromDecodesConfigAndBoolean(t *testing.T) {
	row := db.ListInspectionChecksByGroupRow{
		ID: 1, Name: "disk-space", Enabled: true, Config: json.RawMessage(`{"threshold":90}`),
	}
	response := inspectionCheckResponseFrom(row)

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if decoded["enabled"] != true {
		t.Fatalf("enabled = %#v, want JSON boolean true", decoded["enabled"])
	}
	config, ok := decoded["config"].(map[string]any)
	if !ok {
		t.Fatalf("config type = %T, want a JSON object", decoded["config"])
	}
	if config["threshold"] != float64(90) {
		t.Fatalf("config.threshold = %#v, want 90", config["threshold"])
	}
}

func TestInspectionTaskResponseFromComputesTargetTypeAndName(t *testing.T) {
	perHost := db.ListInspectionTasksTypedRow{
		ID: 1, Scope: "per_host", Enabled: true, SelectedHostIds: json.RawMessage(`[1,2]`),
	}
	result := inspectionTaskResponseFrom(perHost)
	if result.TargetType != "host_group" || result.TargetName != "2 台主机" {
		t.Fatalf("per_host target = %q/%q, want host_group/2 台主机", result.TargetType, result.TargetName)
	}

	logical := db.ListInspectionTasksTypedRow{
		ID: 2, Scope: "service_once", Enabled: false, SelectedHostIds: json.RawMessage(`[]`),
		LogicalServiceName: "checkout-service",
	}
	result = inspectionTaskResponseFrom(logical)
	if result.TargetType != "logical_service" || result.TargetName != "checkout-service" {
		t.Fatalf("logical target = %q/%q, want logical_service/checkout-service", result.TargetType, result.TargetName)
	}
	if result.Enabled {
		t.Fatalf("enabled = true, want false")
	}
}
