package inspection

import (
	"fmt"
	"strings"

	"autoadmin/internal/api/response"
	"database/sql"

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
	groupRows, err := handler.db.QueryContext(context, `SELECT id,name,parent_id FROM assets_hostgroup ORDER BY name,id`)
	if err != nil {
		response.Error(context, err)
		return
	}
	groups, err := scanRows(groupRows)
	if err != nil {
		response.Error(context, err)
		return
	}
	hostRows, err := handler.db.QueryContext(context, `SELECT id,instance_name,ip,group_id,agent_id FROM assets_host WHERE is_deleted_in_cloud=FALSE ORDER BY instance_name,id`)
	if err != nil {
		response.Error(context, err)
		return
	}
	hosts, err := scanRows(hostRows)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"groups": buildGroupTree(groups, nil), "hosts": hosts})
}

func buildGroupTree(groups []gin.H, parent any) []gin.H {
	result := make([]gin.H, 0)
	for _, group := range groups {
		if fmt.Sprint(group["parent_id"]) != fmt.Sprint(parent) {
			continue
		}
		copy := gin.H{"id": group["id"], "name": group["name"], "parent_id": group["parent_id"]}
		copy["children"] = buildGroupTree(groups, group["id"])
		result = append(result, copy)
	}
	return result
}
