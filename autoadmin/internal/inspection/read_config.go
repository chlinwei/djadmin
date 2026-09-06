package inspection

import (
	"database/sql"
	"strings"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func optionalSearchPattern(context *gin.Context) sql.NullString {
	search := strings.TrimSpace(context.Query("search"))
	if search == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: "%" + search + "%", Valid: true}
}

type hostGroupNode struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	ParentID *int64          `json:"parent_id"`
	Children []hostGroupNode `json:"children"`
}

func hostGroupNodeFrom(row db.ListHostGroupTreeNodesRow) hostGroupNode {
	node := hostGroupNode{ID: row.ID, Name: row.Name, Children: []hostGroupNode{}}
	if row.ParentID.Valid {
		node.ParentID = &row.ParentID.Int64
	}
	return node
}

func buildGroupTree(groups []hostGroupNode, parentID int64) []hostGroupNode {
	result := make([]hostGroupNode, 0)
	for _, group := range groups {
		current := int64(0)
		if group.ParentID != nil {
			current = *group.ParentID
		}
		if current != parentID {
			continue
		}
		group.Children = buildGroupTree(groups, group.ID)
		result = append(result, group)
	}
	return result
}
