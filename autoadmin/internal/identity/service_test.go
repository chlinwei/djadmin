package identity

import (
	"database/sql"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

func TestBuildMenuTree(t *testing.T) {
	zero := sql.NullInt32{Int32: 0, Valid: true}
	rows := []db.SysMenu{
		{ID: 3, Name: "second", ParentID: zero, OrderNum: sql.NullInt32{Int32: 2, Valid: true}},
		{ID: 2, Name: "child", ParentID: sql.NullInt32{Int32: 1, Valid: true}, OrderNum: sql.NullInt32{Int32: 1, Valid: true}},
		{ID: 1, Name: "first", ParentID: zero, OrderNum: sql.NullInt32{Int32: 1, Valid: true}},
		{ID: 4, Name: "orphan", ParentID: sql.NullInt32{Int32: 999, Valid: true}},
		{ID: 5, Name: "cycle-a", ParentID: sql.NullInt32{Int32: 6, Valid: true}},
		{ID: 6, Name: "cycle-b", ParentID: sql.NullInt32{Int32: 5, Valid: true}},
	}

	tree := buildMenuTree(rows)
	if len(tree) != 2 || tree[0].ID != 1 || tree[1].ID != 3 {
		t.Fatalf("unexpected root ordering: %+v", tree)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].ID != 2 {
		t.Fatalf("unexpected child tree: %+v", tree[0].Children)
	}
}
