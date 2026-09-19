package rbac

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

func menuRow(id int32, name string, parent sql.NullInt32, order sql.NullInt32) db.SysMenu {
	return db.SysMenu{ID: id, Name: name, ParentID: parent, OrderNum: order}
}

func nullInt32(value int32) sql.NullInt32 { return sql.NullInt32{Int32: value, Valid: true} }

// 回归用例：MenuTree 是权限菜单的核心组装逻辑，必须保证层级、排序与空 children 归一。
func TestMenuTreeBuildsHierarchyAndSorts(t *testing.T) {
	rows := []db.SysMenu{
		menuRow(2, "资产管理", sql.NullInt32{}, nullInt32(2)),
		menuRow(52, "Credential管理", nullInt32(2), nullInt32(1)),
		menuRow(56, "应用管理", nullInt32(2), nullInt32(2)),
		menuRow(1, "系统管理", sql.NullInt32{}, nullInt32(1)),
		menuRow(10, "菜单B", nullInt32(1), nullInt32(2)),
		menuRow(9, "菜单A", nullInt32(1), nullInt32(1)),
	}
	tree := menuTree(rows)
	if len(tree) != 2 {
		t.Fatalf("root count = %d, want 2", len(tree))
	}
	// 根节点按 order_num 排序：系统管理(1) 在前。
	if tree[0].Name != "系统管理" || tree[1].Name != "资产管理" {
		t.Fatalf("root order wrong: [%s, %s]", tree[0].Name, tree[1].Name)
	}
	if len(tree[0].Children) != 2 || tree[0].Children[0].Name != "菜单A" {
		t.Fatalf("children order wrong: %#v", tree[0].Children)
	}
	// 叶子节点 children 必须是空切片而不是 nil（前端依赖可迭代）。
	if tree[1].Children == nil || len(tree[1].Children) != 2 {
		t.Fatalf("asset children wrong: %#v", tree[1].Children)
	}
	if tree[1].Children[1].Children == nil || len(tree[1].Children[1].Children) != 0 {
		t.Fatalf("leaf children should be empty slice, got %#v", tree[1].Children[1].Children)
	}
}

// 孤儿节点（parent 指向不存在的 ID）不能挂到根上，也不能凭空出现。
func TestMenuTreeDropsOrphans(t *testing.T) {
	rows := []db.SysMenu{
		menuRow(1, "根", sql.NullInt32{}, sql.NullInt32{}),
		menuRow(2, "孤儿", nullInt32(999), sql.NullInt32{}),
	}
	tree := menuTree(rows)
	if len(tree) != 1 || tree[0].Name != "根" {
		t.Fatalf("orphan leaked to root: %#v", tree)
	}
}

// parent 环（A↔B）不得导致死循环或把环内节点输出到根。
func TestMenuTreeHandlesCycles(t *testing.T) {
	rows := []db.SysMenu{
		menuRow(1, "根", sql.NullInt32{}, sql.NullInt32{}),
		menuRow(2, "A", nullInt32(3), sql.NullInt32{}),
		menuRow(3, "B", nullInt32(2), sql.NullInt32{}),
	}
	tree := menuTree(rows)
	if len(tree) != 1 || tree[0].Name != "根" {
		t.Fatalf("cycle leaked to root: %#v", tree)
	}
}

// 自引用环（parent=自身）同样不得输出。
func TestMenuTreeHandlesSelfCycle(t *testing.T) {
	rows := []db.SysMenu{
		menuRow(1, "根", sql.NullInt32{}, sql.NullInt32{}),
		menuRow(2, "自引用", nullInt32(2), sql.NullInt32{}),
	}
	tree := menuTree(rows)
	if len(tree) != 1 || tree[0].Name != "根" {
		t.Fatalf("self cycle leaked: %#v", tree)
	}
}

func TestMapMenuNullableFields(t *testing.T) {
	row := menuRow(5, "用户管理", nullInt32(1), nullInt32(3))
	row.Path = sql.NullString{String: "/sys/user", Valid: true}
	row.Component = sql.NullString{}
	item := mapMenu(row)
	if item.ParentID == nil || *item.ParentID != 1 {
		t.Fatalf("parent_id wrong: %#v", item.ParentID)
	}
	if item.OrderNum == nil || *item.OrderNum != 3 {
		t.Fatalf("order_num wrong: %#v", item.OrderNum)
	}
	if item.Path == nil || *item.Path != "/sys/user" {
		t.Fatalf("path wrong: %#v", item.Path)
	}
	if item.Component != nil {
		t.Fatalf("invalid component should map to nil, got %#v", item.Component)
	}
}

// validateParent：parent 为空/0 合法；parent=自身必须拒绝（不触库，可纯测）。
func TestValidateParent(t *testing.T) {
	service := &Service{}
	if err := service.validateParent(context.Background(), 5, nil); err != nil {
		t.Fatalf("nil parent should be valid, got %v", err)
	}
	zero := int32(0)
	if err := service.validateParent(context.Background(), 5, &zero); err != nil {
		t.Fatalf("zero parent should be valid, got %v", err)
	}
	self := int32(5)
	if err := service.validateParent(context.Background(), 5, &self); err != ErrMenuSelfParent {
		t.Fatalf("self parent = %v, want ErrMenuSelfParent", err)
	}
}

// 回归（2026-09-19 现场）：字段类型不匹配时必须**说出是哪个字段**。
//
// 现场：编辑菜单时只要动了「显示顺序」（那时绑的是文本框，一改就是字符串），保存必失败；
// 而错误信息一边是笼统的"请求参数错误"、另一边（服务端日志）只有一个 `<nil>`，
// 谁都没法定位。绑定错误的原文里带着字段名，直接透给调用方。
func TestMenuBindErrorNamesTheField(t *testing.T) {
	var request menuRequest
	err := json.Unmarshal([]byte(`{"name":"日志中心","order_num":"6"}`), &request)
	if err == nil {
		t.Fatal("字符串写进 int32 字段必须报错，否则这条用例失去意义")
	}
	message := menuBindError(err).Error()
	if !strings.Contains(message, "order_num") {
		t.Fatalf("错误信息里要指出字段名，得到 %q", message)
	}
	if !strings.Contains(message, "请求参数错误") {
		t.Fatalf("保留原有的中文前缀，得到 %q", message)
	}
}
