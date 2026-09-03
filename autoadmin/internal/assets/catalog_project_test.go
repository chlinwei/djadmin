package assets

import (
	"database/sql"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// 回归用例：前端"关联业务系统"列依赖 business_system_names（Go 版迁移时曾遗漏聚合，
// 列恒为空）。聚合在 SQL 侧用 '||' 连接（名称可能含逗号），Go 侧由 splitBusinessSystemNames 拆分。
func TestSplitBusinessSystemNames(t *testing.T) {
	cases := []struct {
		name   string
		raw    sql.NullString
		expect []string
	}{
		{"multiple", sql.NullString{String: "TIB||cdm", Valid: true}, []string{"TIB", "cdm"}},
		{"single", sql.NullString{String: "foms", Valid: true}, []string{"foms"}},
		{"name containing comma", sql.NullString{String: "ESB,核心||foms", Valid: true}, []string{"ESB,核心", "foms"}},
		{"empty segments skipped", sql.NullString{String: "a||||b", Valid: true}, []string{"a", "b"}},
		{"empty string", sql.NullString{String: "", Valid: true}, []string{}},
		{"null (LEFT JOIN miss)", sql.NullString{Valid: false}, []string{}},
	}
	for _, item := range cases {
		got := splitBusinessSystemNames(item.raw)
		if len(got) != len(item.expect) {
			t.Fatalf("%s: got %#v, want %#v", item.name, got, item.expect)
		}
		for index := range got {
			if got[index] != item.expect[index] {
				t.Fatalf("%s: got %#v, want %#v", item.name, got, item.expect)
			}
		}
	}
}

func TestProjectWithSystemsMapsAllFields(t *testing.T) {
	now := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	row := db.ListProjectsRow{
		ID: 7, CreateTime: now, UpdateTime: now, Remark: sql.NullString{String: "备注", Valid: true},
		Name: "kul", Code: "KUL", Owner: "ops", Enabled: true,
		BusinessSystemNames: sql.NullString{String: "TIB||cdm", Valid: true},
		BusinessSystemIds:   sql.NullString{String: "1||7", Valid: true},
	}
	item := projectWithSystems(row)
	if item.ID != 7 || item.Name != "kul" || item.Code != "KUL" || item.Owner != "ops" || !item.Enabled {
		t.Fatalf("base fields wrong: %#v", item)
	}
	if item.Remark == nil || *item.Remark != "备注" {
		t.Fatalf("remark wrong: %#v", item.Remark)
	}
	if len(item.BusinessSystemNames) != 2 || item.BusinessSystemNames[0] != "TIB" {
		t.Fatalf("business_system_names wrong: %#v", item.BusinessSystemNames)
	}
	if len(item.BusinessSystems) != 2 || item.BusinessSystems[0] != 1 || item.BusinessSystems[1] != 7 {
		t.Fatalf("business_systems wrong: %#v", item.BusinessSystems)
	}
}

// 回归用例：服务树"资源占比"的"按项目"维度依赖 project.business_systems（ID 数组），
// Go 版曾只返回 names 导致该维度饼图恒为空。
func TestSplitBusinessSystemIDs(t *testing.T) {
	cases := []struct {
		name   string
		raw    sql.NullString
		expect []int64
	}{
		{"multiple", sql.NullString{String: "1||7", Valid: true}, []int64{1, 7}},
		{"single", sql.NullString{String: "6", Valid: true}, []int64{6}},
		{"empty string", sql.NullString{String: "", Valid: true}, []int64{}},
		{"null", sql.NullString{Valid: false}, []int64{}},
		{"non-numeric skipped", sql.NullString{String: "3||abc||0||9", Valid: true}, []int64{3, 9}},
	}
	for _, item := range cases {
		got := splitBusinessSystemIDs(item.raw)
		if len(got) != len(item.expect) {
			t.Fatalf("%s: got %#v, want %#v", item.name, got, item.expect)
		}
		for index := range got {
			if got[index] != item.expect[index] {
				t.Fatalf("%s: got %#v, want %#v", item.name, got, item.expect)
			}
		}
	}
}

// 回归用例：GetProject 的 COALESCE 子查询聚合列被 sqlc 推导为 interface{}，需兼容 string/[]byte。
func TestBusinessSystemIDsFromAny(t *testing.T) {
	if got := businessSystemIDsFromAny("2||4"); len(got) != 2 || got[1] != 4 {
		t.Fatalf("string value wrong: %#v", got)
	}
	if got := businessSystemIDsFromAny([]byte("2||4")); len(got) != 2 || got[1] != 4 {
		t.Fatalf("bytes value wrong: %#v", got)
	}
	if got := businessSystemIDsFromAny(nil); len(got) != 0 {
		t.Fatalf("nil value wrong: %#v", got)
	}
}
