package automation

import (
	"database/sql"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) HostOptions(context *gin.Context) {
	page, _ := strconv.Atoi(context.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(context.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 30 {
		size = 30
	}
	search := strings.TrimSpace(context.Query("search"))
	pattern := "%" + search + "%"
	where := ` WHERE h.ip IS NOT NULL AND (?='' OR h.instance_name LIKE ? OR s.hostname LIKE ? OR h.ip LIKE ?)`

	var count int64
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM assets_host h LEFT JOIN assets_hostsystem s ON s.host_id=h.id`+where, search, pattern, pattern, pattern).Scan(&count); err != nil {
		response.Error(context, err)
		return
	}
	rows, err := handler.db.QueryContext(context, `SELECT h.id,h.instance_name,s.hostname,h.ip,h.group_id FROM assets_host h LEFT JOIN assets_hostsystem s ON s.host_id=h.id`+where+` ORDER BY h.id LIMIT ? OFFSET ?`, search, pattern, pattern, pattern, size, (page-1)*size)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()

	items := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var instanceName, hostname, ip sql.NullString
		var groupID sql.NullInt64
		if err = rows.Scan(&id, &instanceName, &hostname, &ip, &groupID); err != nil {
			response.Error(context, err)
			return
		}
		items = append(items, gin.H{"id": id, "instance_name": nullableString(instanceName), "hostname": nullableString(hostname), "ip": nullableString(ip), "group_id": nullableInt(groupID)})
	}
	if err = rows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) GroupTree(context *gin.Context) {
	rows, err := handler.db.QueryContext(context, `SELECT id,name,parent_id FROM assets_hostgroup ORDER BY id`)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()

	nodes := make(map[int64]gin.H)
	ordered := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var name string
		var parentID sql.NullInt64
		if err = rows.Scan(&id, &name, &parentID); err != nil {
			response.Error(context, err)
			return
		}
		node := gin.H{"id": id, "name": name, "parent_id": nullableInt(parentID), "children": []gin.H{}}
		nodes[id] = node
		ordered = append(ordered, node)
	}
	if err = rows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	roots := make([]gin.H, 0)
	for _, node := range ordered {
		parentID, ok := node["parent_id"].(int64)
		parent, found := nodes[parentID]
		if !ok || !found {
			roots = append(roots, node)
			continue
		}
		parent["children"] = append(parent["children"].([]gin.H), node)
	}
	response.Success(context, roots)
}

func nullableString(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}

func nullableInt(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}
