package inspection

import (
	"database/sql"
	"strings"

	"github.com/gin-gonic/gin"
)

func optionalSearchPattern(context *gin.Context) sql.NullString {
	search := strings.TrimSpace(context.Query("search"))
	if search == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: "%" + search + "%", Valid: true}
}
