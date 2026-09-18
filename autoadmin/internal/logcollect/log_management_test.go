package logcollect

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 索引名段清洗回归：非法字符转连字符、前后连字符去除、空值兜底 unknown。
func TestSafeIndexSegment(t *testing.T) {
	cases := map[string]string{
		"logs":      "logs",
		"Log Std":   "log-std",
		"  --hot--": "hot",
		"大小写ABC":    "abc",
		"":          "unknown",
		"///":       "unknown",
	}
	for input, want := range cases {
		if got := safeIndexSegment(input); got != want {
			t.Fatalf("safeIndexSegment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildIndexTemplateNameAndPolicyName(t *testing.T) {
	if got := buildIndexTemplateName("Logs"); got != "logs-template" {
		t.Fatalf("buildIndexTemplateName = %q", got)
	}
	if got := buildILMPolicyName("logs", "std"); got != "logs-std-retention" {
		t.Fatalf("buildILMPolicyName = %q", got)
	}
	if got := buildTierIndexTemplateName("logs", "std"); got != "logs-std-template" {
		t.Fatalf("buildTierIndexTemplateName = %q", got)
	}
}

// ILM policy 期望体回归：滚动阈值不小于 1gb、删除按 retention_days。
func TestBuildILMPolicyBody(t *testing.T) {
	body := buildILMPolicyBody("logs", retentionTierRow{Code: "std", RetentionDays: 30, DailySizeGB: 5, RolloverMinIndexAge: "1d"})
	policy, ok := body["policy"].(gin.H)
	if !ok {
		t.Fatalf("policy missing: %v", body)
	}
	phases := policy["phases"].(gin.H)
	rollover := phases["hot"].(gin.H)["actions"].(gin.H)["rollover"].(gin.H)
	if rollover["max_primary_shard_size"] != "5gb" || rollover["max_age"] != "1d" {
		t.Fatalf("rollover = %v", rollover)
	}
	if got := phases["delete"].(gin.H)["min_age"]; got != "30d" {
		t.Fatalf("delete min_age = %v", got)
	}

	// 小写入量档位的滚动阈值必须被抬到 1gb。
	small := buildILMPolicyBody("logs", retentionTierRow{Code: "tiny", RetentionDays: 7, DailySizeGB: 0.2, RolloverMinIndexAge: "1d"})
	smallRollover := small["policy"].(gin.H)["phases"].(gin.H)["hot"].(gin.H)["actions"].(gin.H)["rollover"].(gin.H)
	if smallRollover["max_primary_shard_size"] != "1gb" {
		t.Fatalf("small rollover = %v", smallRollover)
	}

	// 档位模板必须是自包含的（data_stream + mappings + lifecycle），并匹配 `logs-*-<tier>`。
	tier := buildTierIndexTemplateBody("logs", retentionTierRow{Code: "std", RetentionDays: 30, DailySizeGB: 5})
	if _, ok := tier["data_stream"]; !ok {
		t.Fatalf("tier template must declare data_stream: %v", tier)
	}
	if got := tier["index_patterns"].([]string)[0]; got != "logs-*-std" {
		t.Fatalf("tier template pattern = %v", got)
	}
	settings := tier["template"].(gin.H)["settings"].(gin.H)
	if settings["index.lifecycle.name"] != "logs-std-retention" {
		t.Fatalf("tier template lifecycle = %v", settings)
	}
	if _, ok := tier["template"].(gin.H)["mappings"]; !ok {
		t.Fatalf("tier template must carry mappings: %v", tier)
	}
}

func TestBuildIndexTemplateBody(t *testing.T) {
	body := buildIndexTemplateBody("logs")
	if body["index_patterns"].([]string)[0] != "logs-*" {
		t.Fatalf("patterns = %v", body["index_patterns"])
	}
	// 基础模板不设 priority（默认 0）；与已有同 priority 模板的冲突由 bootstrap 前置检查报错。
	if _, ok := body["priority"]; ok {
		t.Fatalf("base template should not set priority: %v", body["priority"])
	}
	mappings := body["template"].(gin.H)["mappings"].(gin.H)
	if mappings["dynamic"] != false {
		t.Fatalf("dynamic = %v, want false（标准字段之外不再自动建 mapping）", mappings["dynamic"])
	}
	properties := mappings["properties"].(gin.H)
	for _, field := range []string{"@timestamp", "message", "service", "host_ip", "error_fingerprint", "app_fields"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("standard field %s missing", field)
		}
	}
}

// pipeline 签名只取 processors/on_failure 且 key 排序，与 Django json.dumps(sort_keys=True) 一致。
func TestPipelineSignature(t *testing.T) {
	full := gin.H{"processors": []any{gin.H{"grok": gin.H{"field": "message"}}}, "on_failure": []any{}, "description": "ignored"}
	trimmed := gin.H{"description": "different", "on_failure": []any{}, "processors": []any{gin.H{"grok": gin.H{"field": "message"}}}}
	if pipelineSignature(full) != pipelineSignature(trimmed) {
		t.Fatalf("signature must ignore non-behavior fields")
	}
	changed := gin.H{"processors": []any{}, "on_failure": []any{}}
	if pipelineSignature(full) == pipelineSignature(changed) {
		t.Fatalf("processor change must alter signature")
	}
	raw := pipelineSignature(json.RawMessage(`{"processors":[],"on_failure":[]}`))
	if !strings.HasPrefix(raw, "{") {
		t.Fatalf("raw signature = %q", raw)
	}
}

// ILM 实际状态签名提取：只取 rollover/delete，忽略 ES 补充的 min_age=0ms / delete_searchable_snapshot。
func TestILMPolicySignature(t *testing.T) {
	remote := map[string]any{
		"version":       1,
		"modified_date": "2026-09-17T05:09:15.460Z",
		"policy": map[string]any{
			"phases": map[string]any{
				"hot": map[string]any{
					"min_age": "0ms",
					"actions": map[string]any{
						"rollover": map[string]any{"max_primary_shard_size": "5gb", "max_age": "1d"},
					},
				},
				"delete": map[string]any{
					"min_age": "30d",
					"actions": map[string]any{"delete": map[string]any{"delete_searchable_snapshot": true}},
				},
			},
		},
	}
	actual := ilmPolicySignature(remote["policy"].(map[string]any))
	desired := ilmPolicySignature(buildILMPolicyBody("logs", retentionTierRow{Code: "std", RetentionDays: 30, DailySizeGB: 5, RolloverMinIndexAge: "1d"})["policy"].(gin.H))
	if keys := differingSignatureKeys(actual, desired); len(keys) != 0 {
		t.Fatalf("expected no drift, got %v (actual=%v desired=%v)", keys, actual, desired)
	}
	// 保留天数变更 → delete_after 漂移
	drifted := ilmPolicySignature(buildILMPolicyBody("logs", retentionTierRow{Code: "std", RetentionDays: 7, DailySizeGB: 5, RolloverMinIndexAge: "1d"})["policy"].(gin.H))
	if keys := differingSignatureKeys(actual, drifted); len(keys) == 0 || keys[0] != "delete_after" {
		t.Fatalf("expected delete_after drift, got %v", keys)
	}
}

// 冲突预检：pattern 去通配符后互为前缀视为重叠（logs* vs logs-*、logs vs logs-* 都算）。
func TestIndexPatternsOverlap(t *testing.T) {
	cases := []struct {
		left, right string
		want        bool
	}{
		{"logs*", "logs-*", true},
		{"logs", "logs-*", true},
		{"logs-*", "logs-*", true},
		{"logs-*", "nginx-*", false},
		{"logs-*-std", "logs-*", true},
		{"test", "logs-*", false},
	}
	for _, testCase := range cases {
		if got := indexPatternsOverlap(testCase.left, testCase.right); got != testCase.want {
			t.Errorf("indexPatternsOverlap(%q, %q) = %v, want %v", testCase.left, testCase.right, got, testCase.want)
		}
	}
	if got := splitCatPatterns("[logs*, logs-*]"); len(got) != 2 || got[0] != "logs*" || got[1] != "logs-*" {
		t.Fatalf("splitCatPatterns = %v", got)
	}
}

// pipeline id 命名：<前缀>-<应用 code|general>-<规则名>，各段归一化为小写安全段。
func TestProcessingPipelineName(t *testing.T) {
	cases := []struct{ prefix, application, rule, want string }{
		{"autoadmin", "tomcat-app", "access-err", "autoadmin-tomcat-app-access-err"},
		{"autoadmin", "", "access-err", "autoadmin-general-access-err"},
		{"", "", "x", "autoadmin-general-x"},
		{"Logs", "My App", "Rule.Name", "logs-my-app-rule-name"},
	}
	for _, testCase := range cases {
		if got := processingPipelineName(testCase.prefix, testCase.application, testCase.rule); got != testCase.want {
			t.Errorf("processingPipelineName(%q,%q,%q) = %q, want %q", testCase.prefix, testCase.application, testCase.rule, got, testCase.want)
		}
	}
}

func TestWorstLogHealthStatus(t *testing.T) {
	if got := worstLogHealthStatus([]string{logHealthOK, logHealthWarn}); got != logHealthWarn {
		t.Fatalf("worst = %q", got)
	}
	if got := worstLogHealthStatus([]string{logHealthDrift, logHealthError, logHealthOK}); got != logHealthError {
		t.Fatalf("worst = %q", got)
	}
	if got := worstLogHealthStatus(nil); got != logHealthOK {
		t.Fatalf("empty worst = %q", got)
	}
}
