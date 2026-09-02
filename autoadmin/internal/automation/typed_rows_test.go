package automation

import (
	"encoding/json"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

// 回归用例：inventoryRowToMap 必须保留旧版 scanAutomationRows 的全部 key 名（下游 ~20 处
// 用 map key 直接取值，包括 PrecheckInventoryLimit 等执行链路代码），并且 enabled/
// update_on_launch 现在必须是真正的 bool，不能再依赖 boolValue() 兜底猜测类型。
func TestInventoryRowToMapPreservesKeysAndBooleanTypes(t *testing.T) {
	row := db.AutomationInventory{
		ID: 1, Name: "prod", Enabled: true, UpdateOnLaunch: false,
		SelectedHostIds: json.RawMessage(`[1,2,3]`),
	}
	item := inventoryRowToMap(row)

	if enabled, ok := item["enabled"].(bool); !ok || !enabled {
		t.Fatalf("enabled = %#v (%T), want bool true", item["enabled"], item["enabled"])
	}
	if updateOnLaunch, ok := item["update_on_launch"].(bool); !ok || updateOnLaunch {
		t.Fatalf("update_on_launch = %#v (%T), want bool false", item["update_on_launch"], item["update_on_launch"])
	}
	// boolValue() 仍是 PrecheckInventoryLimit 等调用点用的读取方式，确认它对真 bool 也兼容。
	if !boolValue(item["enabled"]) {
		t.Fatalf("boolValue(item[enabled]) = false, want true")
	}
	hostIDs := intSlice(item["selected_host_ids"])
	if len(hostIDs) != 3 || hostIDs[0] != 1 || hostIDs[2] != 3 {
		t.Fatalf("selected_host_ids decoded = %#v, want [1 2 3]", hostIDs)
	}
}

func TestTaskRowToMapFromGetIncludesPlaybookAndInventoryAliases(t *testing.T) {
	inventoryID := int64(9)
	row := db.GetTaskTypedRow{
		ID: 5, Name: "deploy", Enabled: true, EnvVars: json.RawMessage(`{}`),
		TemplateName: "site.yml", TemplateContent: "---", InventoryName: "prod-hosts",
	}
	row.InventoryID.Int64, row.InventoryID.Valid = inventoryID, true
	item := taskRowToMapFromGet(row)

	if enabled, ok := item["enabled"].(bool); !ok || !enabled {
		t.Fatalf("enabled = %#v, want bool true", item["enabled"])
	}
	if item["playbook_template"] != item["playbook_template_id"] {
		t.Fatalf("playbook_template alias mismatch: %#v vs %#v", item["playbook_template"], item["playbook_template_id"])
	}
	if item["inventory"] != inventoryID {
		t.Fatalf("inventory alias = %#v, want %d", item["inventory"], inventoryID)
	}
}

func TestJobRowToMapTypedIncludesExecutionTimeout(t *testing.T) {
	row := db.GetJobTypedRow{
		ID: 7, JobID: "uuid-1", Status: "success", ExecutionTimeoutSeconds: 600,
		InventorySnapshot: json.RawMessage(`{}`), ExtraVars: json.RawMessage(`{}`), ResultSummary: json.RawMessage(`{}`),
	}
	item := jobRowToMapTyped(row)

	if item["execution_timeout_seconds"] != int64(600) {
		t.Fatalf("execution_timeout_seconds = %#v, want int64(600)", item["execution_timeout_seconds"])
	}
	if item["job_id"] != row.JobID {
		t.Fatalf("job_id = %#v, want %q", item["job_id"], row.JobID)
	}
}
