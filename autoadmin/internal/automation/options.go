package automation

import (
	"database/sql"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

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
	// 搜索：空串传 NULL，查询里是 `... LIKE narg(pattern) OR narg(pattern) IS NULL` 的"不过滤"分支。
	search := strings.TrimSpace(context.Query("search"))
	pattern := sql.NullString{}
	if search != "" {
		pattern = sql.NullString{String: "%" + search + "%", Valid: true}
	}
	queries := db.New(handler.db)
	count, err := queries.CountAutomationHostOptions(context, db.CountAutomationHostOptionsParams{Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListAutomationHostOptions(context, db.ListAutomationHostOptionsParams{
		Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"id": row.ID, "instance_name": row.InstanceName, "hostname": nullableString(row.Hostname), "ip": nullableString(row.Ip), "group_id": nullableInt(row.GroupID)})
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) GroupTree(context *gin.Context) {
	rows, err := db.New(handler.db).ListAutomationHostGroupTree(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	nodes := make(map[int64]gin.H)
	ordered := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		node := gin.H{"id": row.ID, "name": row.Name, "parent_id": nullableInt(row.ParentID), "children": []gin.H{}}
		nodes[row.ID] = node
		ordered = append(ordered, node)
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

// nullableInt32 与 nullableInt 同理，用于 int（host_id_snapshot 一类）列。
func nullableInt32(value sql.NullInt32) any {
	if value.Valid {
		return value.Int32
	}
	return nil
}
