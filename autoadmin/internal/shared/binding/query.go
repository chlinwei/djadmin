package binding

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// 可选列表筛选中，空值及非法布尔/整数均表示不启用筛选；必填参数和资源 ID 不使用这组函数。
func OptionalBoolParam(context *gin.Context, name string) sql.NullBool {
	value := strings.TrimSpace(context.Query(name))
	parsed, err := strconv.ParseBool(value)
	if value == "" || err != nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: parsed, Valid: true}
}

func OptionalInt64Param(context *gin.Context, name string) sql.NullInt64 {
	value := strings.TrimSpace(context.Query(name))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if value == "" || err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: parsed, Valid: true}
}

func OptionalStringParam(context *gin.Context, name string) sql.NullString {
	value := strings.TrimSpace(context.Query(name))
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func SearchPatternParam(context *gin.Context) sql.NullString {
	search := strings.TrimSpace(context.Query("search"))
	if search == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: "%" + search + "%", Valid: true}
}
