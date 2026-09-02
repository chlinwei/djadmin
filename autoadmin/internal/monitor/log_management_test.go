package monitor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 索引名段清洗回归：非法字符转连字符、前后连字符去除、空值兜底 unknown。
func TestSafeIndexSegment(t *testing.T) {
	cases := map[string]string{
		"logs":       "logs",
		"Log Std":    "log-std",
		"  --hot--":  "hot",
		"大小写ABC": "abc",
		"":           "unknown",
		"///":        "unknown",
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
	if got := buildISMPolicyName("logs", "std"); got != "logs-std-retention" {
		t.Fatalf("buildISMPolicyName = %q", got)
	}
}

// ISM policy 期望体回归：滚动阈值不小于 1gb、删除按 retention_days、按索引名后缀挂载。
func TestBuildISMPolicyBody(t *testing.T) {
	body := buildISMPolicyBody("logs", retentionTierRow{Code: "std", RetentionDays: 30, DailySizeGB: 5, RolloverMinIndexAge: "1d"})
	policy, ok := body["policy"].(gin.H)
	if !ok {
		t.Fatalf("policy missing: %v", body)
	}
	states := policy["states"].([]any)
	if len(states) != 2 {
		t.Fatalf("states = %v", states)
	}
	hot := states[0].(gin.H)
	if hot["name"] != "hot" || policy["default_state"] != "hot" {
		t.Fatalf("hot state = %v", hot)
	}
	rollover := hot["actions"].([]any)[0].(gin.H)["rollover"].(gin.H)
	if rollover["min_primary_shard_size"] != "5gb" || rollover["min_index_age"] != "1d" {
		t.Fatalf("rollover = %v", rollover)
	}
	transition := hot["transitions"].([]any)[0].(gin.H)
	if transition["state_name"] != "delete" || transition["conditions"].(gin.H)["min_index_age"] != "30d" {
		t.Fatalf("transition = %v", transition)
	}
	template := policy["ism_template"].([]any)[0].(gin.H)
	if got := asAnySlice(template["index_patterns"]); len(got) != 1 || got[0] != "logs-*-std" {
		t.Fatalf("ism_template = %v", template)
	}

	// 小写入量档位的滚动阈值必须被抬到 1gb。
	small := buildISMPolicyBody("logs", retentionTierRow{Code: "tiny", RetentionDays: 7, DailySizeGB: 0.2, RolloverMinIndexAge: "1d"})
	smallRollover := small["policy"].(gin.H)["states"].([]any)[0].(gin.H)["actions"].([]any)[0].(gin.H)["rollover"].(gin.H)
	if smallRollover["min_primary_shard_size"] != "1gb" {
		t.Fatalf("small rollover = %v", smallRollover)
	}
}

func TestBuildIndexTemplateBody(t *testing.T) {
	body := buildIndexTemplateBody("logs")
	if body["index_patterns"].([]string)[0] != "logs-*" {
		t.Fatalf("patterns = %v", body["index_patterns"])
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

// ISM 实际状态签名提取：只取 rollover/delete/patterns，忽略集群补充的元数据。
func TestISMPolicySignature(t *testing.T) {
	remote := map[string]any{
		"policy_id":         "logs-std-retention",
		"last_updated_time": 1725264000000,
		"policy": map[string]any{
			"description": "server-side description",
			"default_state": "hot",
			"states": []any{
				map[string]any{
					"name": "hot",
					"actions": []any{
						map[string]any{"rollover": map[string]any{"min_primary_shard_size": "5gb", "min_index_age": "1d"}},
					},
					"transitions": []any{
						map[string]any{"state_name": "delete", "conditions": map[string]any{"min_index_age": "30d"}},
					},
				},
				map[string]any{"name": "delete", "actions": []any{map[string]any{"delete": map[string]any{}}}},
			},
			"ism_template": []any{map[string]any{"index_patterns": []any{"logs-*-std"}, "priority": 100}},
		},
	}
	actual := ismPolicySignature(remote["policy"].(map[string]any))
	desired := ismPolicySignature(buildISMPolicyBody("logs", retentionTierRow{Code: "std", RetentionDays: 30, DailySizeGB: 5, RolloverMinIndexAge: "1d"})["policy"].(gin.H))
	if keys := differingSignatureKeys(actual, desired); len(keys) != 0 {
		t.Fatalf("expected no drift, got %v (actual=%v desired=%v)", keys, actual, desired)
	}
	// 保留天数变更 → delete_after 漂移
	drifted := ismPolicySignature(buildISMPolicyBody("logs", retentionTierRow{Code: "std", RetentionDays: 7, DailySizeGB: 5, RolloverMinIndexAge: "1d"})["policy"].(gin.H))
	if keys := differingSignatureKeys(actual, drifted); len(keys) == 0 || keys[0] != "delete_after" {
		t.Fatalf("expected delete_after drift, got %v", keys)
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
