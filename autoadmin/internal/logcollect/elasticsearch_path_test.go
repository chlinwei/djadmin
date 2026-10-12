package logcollect

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 统计维度白名单必须包含「日志路径」：它和日志详情显示的是同一个字段（Filebeat log.file.path），
// 通配路径下按它分组才能区分具体文件。新增维度要同步前端 statsFieldOptions 与 FACET_FILTER_KEY。
func TestLogFacetAllowedFieldsIncludesLogPath(t *testing.T) {
	for _, field := range []string{"error_fingerprint", "log_level", "instance", "host_ip", "log_name", "log_path"} {
		if !logFacetAllowedFields[field] {
			t.Fatalf("统计维度白名单缺少 %q", field)
		}
	}
}

// 日志详情里的"日志路径"必须是该事件**实际来自的文件**，而不是采集配置里的路径模式
// （2026-09-20 现场：通配路径 `/home/esb/data/logs/*/log_error.log` 被原样显示）。
// 权威值来自 Filebeat 的 `log.file.path`。
func TestLogPathKeywordAndFinalPipeline(t *testing.T) {
	if standardLogFields["log_path"].(gin.H)["type"] != "keyword" {
		t.Fatal("真实路径必须有 keyword 索引")
	}
	processors := buildMappingGuardPipelineBody(mappingGuardModeTag)["processors"].([]any)
	script := processors[len(processors)-1].(gin.H)["script"].(gin.H)
	source, _ := script["source"].(string)
	if script["tag"] != "canonical_log_path" || !strings.Contains(source, "ctx.log.file.path") ||
		!strings.Contains(source, "ctx.log_path = path") || !strings.Contains(source, "ctx.remove('log_path')") {
		t.Fatalf("写入必须用权威路径校准 log_path：%s", source)
	}
}

func TestNestedFilePath(t *testing.T) {
	cases := []struct {
		name   string
		source map[string]any
		want   string
	}{
		{
			name: "有 log.file.path：取真实文件",
			source: map[string]any{
				"log_path": "/home/esb/data/logs/*/log_error.log",
				"log":      map[string]any{"file": map[string]any{"path": "/home/esb/data/logs/app1/log_error.log"}},
			},
			want: "/home/esb/data/logs/app1/log_error.log",
		},
		{
			name:   "没有 log 对象：空",
			source: map[string]any{"log_path": "/home/esb/data/logs/*/log_error.log"},
			want:   "",
		},
		{
			name:   "log.file 不是对象：空",
			source: map[string]any{"log": map[string]any{"file": "oops"}},
			want:   "",
		},
		{
			name:   "path 不是字符串：空",
			source: map[string]any{"log": map[string]any{"file": map[string]any{"path": 42}}},
			want:   "",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := nestedFilePath(testCase.source); got != testCase.want {
				t.Fatalf("nestedFilePath = %q, want %q", got, testCase.want)
			}
		})
	}
}
