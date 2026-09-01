package assets

import (
	"database/sql"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

func TestBuildHostGroupTreeAccumulatesDescendantHosts(t *testing.T) {
	rows := []db.ListAllHostGroupsRow{
		{ID: 1, Name: "root", HostCount: 2},
		{ID: 2, Name: "child", ParentID: sql.NullInt64{Int64: 1, Valid: true}, HostCount: 3},
		{ID: 3, Name: "leaf", ParentID: sql.NullInt64{Int64: 2, Valid: true}, HostCount: 5},
	}

	tree := buildHostGroupTree(rows)
	if len(tree) != 1 || tree[0].HostCount != 10 {
		t.Fatalf("expected one root with 10 hosts, got %#v", tree)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].HostCount != 8 {
		t.Fatalf("expected child subtree with 8 hosts, got %#v", tree[0].Children)
	}
	if len(tree[0].Children[0].Children) != 1 || tree[0].Children[0].Children[0].HostCount != 5 {
		t.Fatalf("expected leaf with 5 hosts, got %#v", tree[0].Children[0].Children)
	}
}

func TestBuildHostGroupTreeDoesNotExposeCyclesAsRoots(t *testing.T) {
	rows := []db.ListAllHostGroupsRow{
		{ID: 1, Name: "one", ParentID: sql.NullInt64{Int64: 2, Valid: true}},
		{ID: 2, Name: "two", ParentID: sql.NullInt64{Int64: 1, Valid: true}},
	}

	if tree := buildHostGroupTree(rows); len(tree) != 0 {
		t.Fatalf("expected corrupt cycle to be isolated, got %#v", tree)
	}
}
