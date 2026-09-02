package inspection

import (
	"encoding/json"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

func TestHostScopeNodeFromPreservesNulls(t *testing.T) {
	node := hostScopeNodeFrom(db.ListHostScopeTreeHostsRow{ID: 7})
	encoded, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"instance_name", "ip", "group_id", "agent_id"} {
		if value, ok := decoded[key]; !ok || value != nil {
			t.Fatalf("hostScopeNodeFrom null %s = %v, want null", key, value)
		}
	}
}

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
