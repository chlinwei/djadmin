package binding

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func newQueryContext(query string) *gin.Context {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return context
}

func TestOptionalBoolParam(t *testing.T) {
	tests := []struct {
		query string
		want  sql.NullBool
	}{
		{"", sql.NullBool{}},
		{"enabled=", sql.NullBool{}},
		{"enabled=++", sql.NullBool{}},
		{"enabled=+true+", sql.NullBool{Bool: true, Valid: true}},
		{"enabled=false", sql.NullBool{Bool: false, Valid: true}},
		{"enabled=1", sql.NullBool{Bool: true, Valid: true}},
		{"enabled=0", sql.NullBool{Bool: false, Valid: true}},
		{"enabled=TRUE", sql.NullBool{Bool: true, Valid: true}},
		{"enabled=FALSE", sql.NullBool{Bool: false, Valid: true}},
		{"enabled=invalid", sql.NullBool{}},
		{"enabled=2", sql.NullBool{}},
		{"enabled=false&enabled=true", sql.NullBool{Bool: false, Valid: true}},
		{"enabled=&enabled=true", sql.NullBool{}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := OptionalBoolParam(newQueryContext(tt.query), "enabled"); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOptionalInt64Param(t *testing.T) {
	tests := []struct {
		query string
		want  sql.NullInt64
	}{
		{"", sql.NullInt64{}},
		{"application=", sql.NullInt64{}},
		{"application=++", sql.NullInt64{}},
		{"application=+42+", sql.NullInt64{Int64: 42, Valid: true}},
		{"application=0", sql.NullInt64{Int64: 0, Valid: true}},
		{"application=-7", sql.NullInt64{Int64: -7, Valid: true}},
		{"application=9223372036854775807", sql.NullInt64{Int64: 9223372036854775807, Valid: true}},
		{"application=-9223372036854775808", sql.NullInt64{Int64: -9223372036854775808, Valid: true}},
		{"application=9223372036854775808", sql.NullInt64{}},
		{"application=-9223372036854775809", sql.NullInt64{}},
		{"application=1.5", sql.NullInt64{}},
		{"application=invalid", sql.NullInt64{}},
		{"application=12&application=34", sql.NullInt64{Int64: 12, Valid: true}},
		{"application=&application=34", sql.NullInt64{}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := OptionalInt64Param(newQueryContext(tt.query), "application"); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOptionalStringParam(t *testing.T) {
	tests := []struct {
		query string
		want  sql.NullString
	}{
		{"", sql.NullString{}},
		{"status=", sql.NullString{}},
		{"status=++", sql.NullString{}},
		{"status=+running+", sql.NullString{String: "running", Valid: true}},
		{"status=" + url.QueryEscape(" 主机 状态 "), sql.NullString{String: "主机 状态", Valid: true}},
		{"status=first&status=second", sql.NullString{String: "first", Valid: true}},
		{"status=&status=second", sql.NullString{}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := OptionalStringParam(newQueryContext(tt.query), "status"); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSearchPatternParam(t *testing.T) {
	tests := []struct {
		query string
		want  sql.NullString
	}{
		{"", sql.NullString{}},
		{"search=", sql.NullString{}},
		{"search=++", sql.NullString{}},
		{"search=+host+", sql.NullString{String: "%host%", Valid: true}},
		{"search=" + url.QueryEscape(" 主机 日志 "), sql.NullString{String: "%主机 日志%", Valid: true}},
		{"search=" + url.QueryEscape("50%_done"), sql.NullString{String: "%50%_done%", Valid: true}},
		{"search=first&search=second", sql.NullString{String: "%first%", Valid: true}},
		{"search=&search=second", sql.NullString{}},
		{"keyword=host", sql.NullString{}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := SearchPatternParam(newQueryContext(tt.query)); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
