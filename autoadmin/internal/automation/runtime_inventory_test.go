package automation

import (
	"database/sql"
	"testing"
)

func newNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func newNullInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: true}
}

// 回归用例：前端列表"启用状态"开关只 PATCH {enabled}，不携带 name。
// resolveInventoryUpdate 必须保留未提供字段的原值，否则会退回 "name is required" 报错。
func TestResolveInventoryUpdatePartialEnabledOnly(t *testing.T) {
	existing := inventoryExisting{
		Name: "prod-hosts", Remark: newNullString("生产环境"),
		SelectedHostIDs: newNullString(`[1,2,3]`),
		Enabled:         true, UpdateOnLaunch: false,
		UpdateCacheTimeout: newNullInt64(600),
	}
	enabled := false
	merged, err := resolveInventoryUpdate(existing, inventoryInput{Enabled: &enabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Name != "prod-hosts" {
		t.Fatalf("name = %q, want preserved", merged.Name)
	}
	if merged.Remark == nil || *merged.Remark != "生产环境" {
		t.Fatalf("remark = %#v, want preserved", merged.Remark)
	}
	if len(merged.SelectedHostIDs) != 3 || merged.SelectedHostIDs[0] != 1 {
		t.Fatalf("selected_host_ids = %#v, want [1 2 3]", merged.SelectedHostIDs)
	}
	if *merged.Enabled != false {
		t.Fatal("enabled should be updated to false")
	}
	if *merged.UpdateOnLaunch != false || *merged.UpdateCacheTimeout != 600 {
		t.Fatalf("update_on_launch=%v timeout=%d, want preserved", *merged.UpdateOnLaunch, *merged.UpdateCacheTimeout)
	}
}

func TestResolveInventoryUpdateFullPayloadOverrides(t *testing.T) {
	existing := inventoryExisting{
		Name: "old", SelectedHostIDs: newNullString(`[1]`),
		Enabled: true, UpdateCacheTimeout: newNullInt64(300),
	}
	name := "new"
	remark := "updated"
	hosts := []int64{9, 8}
	disabled := false
	launch := true
	timeout := 120
	merged, err := resolveInventoryUpdate(existing, inventoryInput{
		Name: name, Remark: &remark, SelectedHostIDs: hosts,
		Enabled: &disabled, UpdateOnLaunch: &launch, UpdateCacheTimeout: &timeout,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Name != "new" || *merged.Remark != "updated" {
		t.Fatalf("name/remark not overridden: %q %v", merged.Name, merged.Remark)
	}
	if len(merged.SelectedHostIDs) != 2 || merged.SelectedHostIDs[1] != 8 {
		t.Fatalf("hosts not overridden: %#v", merged.SelectedHostIDs)
	}
	if *merged.Enabled || !*merged.UpdateOnLaunch || *merged.UpdateCacheTimeout != 120 {
		t.Fatalf("flags not overridden: %v %v %d", *merged.Enabled, *merged.UpdateOnLaunch, *merged.UpdateCacheTimeout)
	}
}

// 库中 update_cache_timeout 为 NULL 的存量行：合并时回退默认值 300。
func TestResolveInventoryUpdateNullTimeoutFallsBackToDefault(t *testing.T) {
	existing := inventoryExisting{Name: "legacy", Enabled: true}
	merged, err := resolveInventoryUpdate(existing, inventoryInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.UpdateCacheTimeout == nil || *merged.UpdateCacheTimeout != 300 {
		t.Fatalf("timeout = %#v, want default 300", merged.UpdateCacheTimeout)
	}
	if !*merged.Enabled || merged.Name != "legacy" {
		t.Fatalf("existing values not preserved: %q %v", merged.Name, *merged.Enabled)
	}
}

func TestResolveInventoryUpdateRejectsExplicitEmptyName(t *testing.T) {
	// 语义固化：显式传空 name 与未传 name 等价，都按"保留原值"处理；
	// 只有当现值 name 也为空（脏数据）且未提供 name 时才报错，防止 UPDATE 写入空名。
	existing := inventoryExisting{Name: "prod-hosts", Enabled: true}
	merged, err := resolveInventoryUpdate(existing, inventoryInput{Name: "   "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Name != "prod-hosts" {
		t.Fatalf("name = %q, want preserved", merged.Name)
	}
	if _, err = resolveInventoryUpdate(inventoryExisting{}, inventoryInput{}); err == nil {
		t.Fatal("expected error when merged name is empty")
	}
}

func TestDecodeJSONInt64Array(t *testing.T) {
	cases := []struct {
		raw    string
		expect []int64
	}{
		{`[1,2,3]`, []int64{1, 2, 3}},
		{`[]`, []int64{}},
		{``, nil},
		{`not-json`, nil},
		{`{"a":1}`, nil},
	}
	for _, item := range cases {
		got := decodeJSONInt64Array(item.raw)
		if len(got) != len(item.expect) {
			t.Fatalf("decodeJSONInt64Array(%q) = %#v, want %#v", item.raw, got, item.expect)
		}
		for index := range got {
			if got[index] != item.expect[index] {
				t.Fatalf("decodeJSONInt64Array(%q) = %#v, want %#v", item.raw, got, item.expect)
			}
		}
	}
}

