package inspection

import (
	"fmt"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListGroups(context *gin.Context) {
	page, size := pagination(context)
	where, arguments := "", make([]any, 0)
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		where = " WHERE name LIKE ? OR description LIKE ?"
		arguments = append(arguments, "%"+search+"%", "%"+search+"%")
	}
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM inspection_group`+where, arguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT id,name,scope,description,enabled,create_time,update_time FROM inspection_group`+where+` ORDER BY name,id LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		checkRows, checkErr := handler.db.QueryContext(context, `SELECT id,name,executor,execution_location,config,severity,enabled,`+"`order`"+` FROM inspection_check WHERE group_id=? ORDER BY `+"`order`"+`,id`, item["id"])
		if checkErr != nil {
			response.Error(context, checkErr)
			return
		}
		item["checks"], err = scanRows(checkRows)
		if err != nil {
			response.Error(context, err)
			return
		}
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) ListTasks(context *gin.Context) {
	page, size := pagination(context)
	where, arguments := "", make([]any, 0)
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		where = " WHERE t.name LIKE ? OR g.name LIKE ? OR s.name LIKE ?"
		pattern := "%" + search + "%"
		arguments = append(arguments, pattern, pattern, pattern)
	}
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM inspection_task t JOIN inspection_group g ON g.id=t.group_id LEFT JOIN assets_application_service s ON s.id=t.logical_service_id`+where, arguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT t.id,t.name,t.inspection_name,t.group_id AS `+"`group`"+`,g.name AS group_name,g.scope AS scope,t.logical_service_id AS logical_service,COALESCE(s.name,'') AS logical_service_name,t.selected_host_ids,t.concurrency,t.timeout_seconds,t.cron_expression,t.next_run_time,t.last_run_time,t.enabled,t.create_time,t.update_time FROM inspection_task t JOIN inspection_group g ON g.id=t.group_id LEFT JOIN assets_application_service s ON s.id=t.logical_service_id`+where+` ORDER BY t.id DESC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		if fmt.Sprint(item["scope"]) == "per_host" {
			ids, _ := item["selected_host_ids"].([]any)
			item["target_type"] = "host_group"
			if len(ids) == 0 {
				item["target_name"] = "未选择范围"
			} else {
				item["target_name"] = fmt.Sprintf("%d 台主机", len(ids))
			}
		} else {
			item["target_type"] = "logical_service"
			item["target_name"] = item["logical_service_name"]
		}
	}
	response.Paginated(context, items, count, int32(page), int32(size))
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
