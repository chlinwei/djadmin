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

// 目标名现在由执行时的挂载点生成（mountTargetName），列表 DTO 不再计算
// TargetType；Groups 必须完整透出（含 mount_type/service_id）。
func TestInspectionTaskResponseFromKeepsGroups(t *testing.T) {
	row := db.ListInspectionTasksTypedRow{
		ID: 1, Name: "task", Enabled: true,
		Groups: json.RawMessage(`[{"id":34,"name":"artemis check","category":"application","mount_type":"service","service_id":10,"instance_mode":"all"}]`),
	}
	result := inspectionTaskResponseFrom(row)

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	groups, ok := decoded["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("groups = %#v, want one entry", decoded["groups"])
	}
	group := groups[0].(map[string]any)
	if group["mount_type"] != "service" || group["service_id"] != float64(10) {
		t.Fatalf("group = %#v, want service mount with service_id=10", group)
	}
	if _, exists := decoded["scope"]; exists {
		t.Fatal("scope field should be removed from task response")
	}
	if _, exists := decoded["logical_service"]; exists {
		t.Fatal("logical_service field should be removed from task response")
	}
}
