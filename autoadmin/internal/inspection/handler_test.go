package inspection

import (
	"testing"
)

func TestBuildGroupTreeAttachesRootsAndChildren(t *testing.T) {
	parent := int64(0)
	root := hostGroupNode{ID: 1, Name: "root", Children: []hostGroupNode{}}
	child := hostGroupNode{ID: 2, Name: "child", ParentID: &parent, Children: []hostGroupNode{}}
	parent = 1
	tree := buildGroupTree([]hostGroupNode{root, child}, 0)
	if len(tree) != 1 || tree[0].ID != 1 || len(tree[0].Children) != 1 || tree[0].Children[0].ID != 2 {
		t.Fatalf("buildGroupTree = %+v, want root 1 with child 2", tree)
	}
	if tree[0].Children[0].ParentID == nil || *tree[0].Children[0].ParentID != 1 {
		t.Fatalf("child parent_id = %v, want 1", tree[0].Children[0].ParentID)
	}
}
