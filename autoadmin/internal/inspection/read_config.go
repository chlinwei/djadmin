package inspection

import (
	"database/sql"
	"strings"

	"autoadmin/internal/api/response"
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

func (handler *Handler) HostScopeTree(context *gin.Context) {
	queries := db.New(handler.db)
	groupRows, err := queries.ListHostGroupTreeNodes(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	hostRows, err := queries.ListHostScopeTreeHosts(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	hosts := make([]hostScopeNode, 0, len(hostRows))
	for _, row := range hostRows {
		hosts = append(hosts, hostScopeNodeFrom(row))
	}
	groups := make([]hostGroupNode, 0, len(groupRows))
	for _, row := range groupRows {
		groups = append(groups, hostGroupNodeFrom(row))
	}
	response.Success(context, gin.H{"groups": buildGroupTree(groups, 0), "hosts": hosts})
}

type hostScopeNode struct {
	ID           int64   `json:"id"`
	InstanceName *string `json:"instance_name"`
	IP           *string `json:"ip"`
	GroupID      *int64  `json:"group_id"`
	AgentID      *string `json:"agent_id"`
}

func hostScopeNodeFrom(row db.ListHostScopeTreeHostsRow) hostScopeNode {
	node := hostScopeNode{ID: row.ID}
	if row.InstanceName.Valid {
		node.InstanceName = &row.InstanceName.String
	}
	if row.Ip.Valid {
		node.IP = &row.Ip.String
	}
	if row.GroupID.Valid {
		node.GroupID = &row.GroupID.Int64
	}
	if row.AgentID.Valid {
		node.AgentID = &row.AgentID.String
	}
	return node
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
