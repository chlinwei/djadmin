package logcollect

import (
	"strings"
	"testing"
)

// 处理规则"保存时"的两类拦截都在这一个文件里钉住（现场来源见各用例注释）：
// 正则是 RE2 判定（Filebeat 也是 RE2），必备字段是静态粗筛（真正的强制在 mapping-guard）。

// 保存路径的接线用例：处理规则发布时就按 RE2 挡住非法正则。
// 为什么必须钉住：这条链路上"能存进去"才是真正的坑——存进去之后主机上的 Filebeat
// 用同一个引擎编译同一串正则，起不来/多行不生效，而界面与调试页都显示正常。
func TestProcessingRuleSaveRejectsNonRE2Pattern(t *testing.T) {
	pipelineBody := map[string]any{"processors": []any{
		map[string]any{"set": map[string]any{"field": "log_level", "value": "INFO"}},
		map[string]any{"set": map[string]any{"field": "log_message", "value": "{{message}}"}},
		map[string]any{"fingerprint": map[string]any{"fields": []any{"message"}, "target_field": "error_fingerprint"}},
	}}
	base := func() map[string]any {
		return map[string]any{
			"name": "springboot-tomcat-exception", "description": "tomcat 异常堆栈",
			"input_format": "log", "cluster": float64(1), "multiline_enabled": true,
			"flush_timeout": float64(5000), "pipeline_body": pipelineBody,
		}
	}

	t.Run("现场那条 Java/JS 写法的首行正则被拦下并给出等价写法", func(t *testing.T) {
		input := base()
		input["start_pattern"] = `^(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\u4e00-\u9fa5]{3,4}-\d{4})`
		problem := validateResource(processingSpec, input, 0)
		if !strings.Contains(problem, "首行正则不合法") || !strings.Contains(problem, `\u4e00 → \x{4e00}`) {
			t.Fatalf("期望拦住并给出改写建议，实际：%q", problem)
		}
	})

	t.Run("续行正则同样按 RE2 校验（平台不消费，但存着就是坑）", func(t *testing.T) {
		input := base()
		input["start_pattern"] = `^\d{4}-\d{2}-\d{2}`
		// 否定前瞻是"续行"最自然的写法，而 RE2 不支持断言。
		input["continuation_pattern"] = `^(?!(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\u4e00-\u9fa5]{3,4}-\d{4}))`
		problem := validateResource(processingSpec, input, 0)
		if !strings.Contains(problem, "续行正则不合法") {
			t.Fatalf("期望拦住非法续行正则，实际：%q", problem)
		}
	})

	t.Run("改写为 RE2 写法后通过（与现场修复后的规则一致：续行清空）", func(t *testing.T) {
		input := base()
		input["start_pattern"] = `^(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\x{4e00}-\x{9fa5}]{3,4}-\d{4})`
		// 续行正则留空：Filebeat 用 negate/match=after 表达，本来就不需要它。
		input["continuation_pattern"] = ""
		if problem := validateResource(processingSpec, input, 0); problem != "" {
			t.Fatalf("期望通过，实际报错：%s", problem)
		}
	})
}

// 保存时的"必备字段"粗筛要认 painless：ES 的 JSON 日志键里带点（log.level），而 ingest 的字段
// 参数把点当路径分隔符、ES 8.13 的 json 处理器又没有 expand_dots，所以只能靠 script 取字段。
// 粗筛不认 script 会把能跑的规则拦在发布门外（现场：Elasticsearch 服务端日志规则）。
func TestPipelineOutputsRecognizeScriptAssignments(t *testing.T) {
	script := func(source string) map[string]any {
		return map[string]any{"processors": []any{
			map[string]any{"script": map[string]any{"lang": "painless", "source": source}},
		}}
	}
	cases := []struct {
		name   string
		source string
		field  string
		want   bool
	}{
		{"点号赋值", `if (ctx.es['log.level'] != null) { ctx.log_level = ctx.es['log.level'].toString(); }`, "log_level", true},
		{"方括号赋值（字段名带引号）", `ctx['log_message'] = ctx.message;`, "log_message", true},
		{"无空格赋值", `ctx.error_fingerprint=ctx.error_template;`, "error_fingerprint", true},
		// 只读不能算产出：判断分支里出现字段名太常见了，认了就等于这条粗筛失效。
		{"只在条件里读", `if (ctx.log_level != null) { ctx.parsed = true; }`, "log_level", false},
		{"别的字段赋值", `ctx.app_fields = ['x': 1];`, "log_message", false},
		// `==` 不是赋值（不能把相等比较当成产出）。
		{"相等比较", `if (ctx.log_message == null) { ctx.log_message2 = 'x'; }`, "log_message", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := pipelineWritesField(script(testCase.source), testCase.field)
			if got != testCase.want {
				t.Fatalf("pipelineWritesField(%s) = %v, want %v；源码：%s", testCase.field, got, testCase.want, testCase.source)
			}
		})
	}
}

// JSON 日志（键里带点的 ECS 格式）的规则必须能通过**保存校验**：这类日志只能靠 script 取字段
// （ingest 字段参数把点当路径、ES 8.13 的 json 处理器没有 expand_dots），所以这条用例把
// "script 产出必备字段 → validateResource 放行"整条路径钉住。现场来源：Elasticsearch 服务端日志规则。
func TestJSONLogRuleWithScriptPassesSaveValidation(t *testing.T) {
	esLogPipeline := map[string]any{
		"description": "Elasticsearch 服务端 JSON 日志（ECS）：单行 JSON，从 message 解析出必备字段",
		"processors": []any{
			map[string]any{"json": map[string]any{"field": "message", "target_field": "es", "ignore_failure": true}},
			map[string]any{"script": map[string]any{"lang": "painless", "ignore_failure": true, "source": `
if (ctx.es == null) { if (ctx.log_message == null) { ctx.log_message = ctx.message; } return; }
if (ctx.log_message == null && ctx.es['message'] != null) { ctx.log_message = ctx.es['message'].toString(); }
if (ctx.es['log.level'] != null) { ctx.log_level = ctx.es['log.level'].toString().toUpperCase(); }
`}},
			map[string]any{"fingerprint": map[string]any{"fields": []any{"log_message"}, "target_field": "error_fingerprint", "ignore_missing": true}},
		},
	}
	input := map[string]any{
		"name": "elasticsearch-server-log", "description": "Elasticsearch 服务端 JSON 日志",
		"cluster": float64(1), "input_format": "json", "multiline_enabled": false,
		"flush_timeout": float64(2000), "pipeline_body": esLogPipeline,
	}
	if problem := validateResource(processingSpec, input, 0); problem != "" {
		t.Fatalf("JSON 日志规则被保存校验拦下（说明 script 产出必备字段没被认）：%s", problem)
	}
	// 反面：把 script 拿掉后 log_level/log_message 就无产出，必须被拦（否则粗筛形同虚设）。
	broken := map[string]any{
		"name": "elasticsearch-server-log", "description": "Elasticsearch 服务端 JSON 日志",
		"cluster": float64(1), "input_format": "json", "multiline_enabled": false,
		"flush_timeout": float64(2000),
		"pipeline_body": map[string]any{"processors": []any{
			map[string]any{"json": map[string]any{"field": "message", "target_field": "es"}},
		}},
	}
	if problem := validateResource(processingSpec, broken, 0); !strings.Contains(problem, "log_level") {
		t.Fatalf("没有产出必备字段的 pipeline 应被拦下并点名缺哪个：%q", problem)
	}
}
