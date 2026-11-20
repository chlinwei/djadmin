package automation

import (
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

const (
	hostOptionsPageSizeKey        = "sys.automation.host_options.max_page_size"
	defaultHostOptionsMaxPageSize = 1000
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
	queries := db.New(handler.db)
	maxSize := defaultHostOptionsMaxPageSize
	setting, err := queries.GetConfigByKey(context, hostOptionsPageSizeKey)
	if err == nil {
		if value, parseErr := strconv.ParseInt(strings.TrimSpace(setting.Value), 10, 32); parseErr == nil && value > 0 {
			maxSize = int(value)
		} else {
			slog.Warn("invalid host options page size limit; using default", "key", hostOptionsPageSizeKey, "default", maxSize)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		response.Error(context, err)
		return
	}
	size = min(size, maxSize)
	// sqlc takes a signed 32-bit offset; reject pages that would wrap.
	if page > math.MaxInt32 || page-1 > math.MaxInt32/size {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	// 搜索：空串传 NULL，查询里是 `... LIKE narg(pattern) OR narg(pattern) IS NULL` 的"不过滤"分支。
	search := strings.TrimSpace(context.Query("search"))
	pattern := sql.NullString{}
	if search != "" {
		pattern = sql.NullString{String: "%" + search + "%", Valid: true}
	}
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
		items = append(items, gin.H{"id": row.ID, "instance_name": nullableString(row.InstanceName), "hostname": nullableString(row.Hostname), "ip": nullableString(row.Ip), "group_id": nullableInt(row.GroupID)})
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) GroupTree(context *gin.Context) {
	rows, err := db.New(handler.db).ListAutomationHostGroupTree(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	groups := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, gin.H{"id": row.ID, "name": row.Name, "children": []gin.H{}})
	}
	response.Success(context, groups)
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
