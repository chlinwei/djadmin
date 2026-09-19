package logcollect

import (
	"strings"
	"testing"
)

// 现场报错原文（ES 8.13.0 对 `{"rename": {..., "override_target": true}}` 的真实响应，2026-09-19）。
const realOverrideTargetError = `elasticsearch 400 Bad Request: {"error":{"root_cause":[{"type":"parse_exception","reason":"processor [rename] doesn't support one or more provided configuration parameters [override_target]","processor_type":"rename"}],"type":"parse_exception","reason":"processor [rename] doesn't support one or more provided configuration parameters [override_target]","processor_type":"rename"},"status":400}`

func TestPipelineCompatHintExplainsEngineDialect(t *testing.T) {
	cases := []struct {
		name      string
		esError   string
		wantHints []string
		wantEmpty bool
	}{
		{
			name:    "现场报错：指出这是 OpenSearch 的叫法，并给出 Elasticsearch 的等价参数",
			esError: realOverrideTargetError,
			wantHints: []string{
				"override_target 是 OpenSearch 的叫法",
				"override",
				"重新发布",
			},
		},
		{
			// 反向：集群是 OpenSearch 一侧时，报错里出现的是 ES 的拼法。
			name:      "反向（集群是 OpenSearch）：建议改回 override_target",
			esError:   `{"error":{"reason":"processor [rename] doesn't support one or more provided configuration parameters [override]"}}`,
			wantHints: []string{"override 是 Elasticsearch 的叫法", "override_target"},
		},
		{
			name:      "提示里不能把处理器名当参数名（否则会写成 rename 不属于处理器定义这种错话）",
			esError:   realOverrideTargetError,
			wantHints: []string{"参数 override_target 是"},
		},
		{
			name:      "其他处理器参数（表里没有的）：给通用说明，不瞎猜别名",
			esError:   `{"error":{"reason":"processor [grok] doesn't support one or more provided configuration parameters [foo_bar]"}}`,
			wantHints: []string{"foo_bar", "引擎里可能叫法不同"},
		},
		{
			name:      "与参数无关的 ES 报错：不追加任何提示（原样返回）",
			esError:   `elasticsearch 400 Bad Request: {"error":{"reason":"parse_exception: pipeline with id [x] does not exist"}}`,
			wantEmpty: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			hint := pipelineCompatHint(testCase.esError)
			if testCase.wantEmpty {
				if hint != "" {
					t.Fatalf("期望不加提示，实际：%s", hint)
				}
				return
			}
			for _, want := range testCase.wantHints {
				if !strings.Contains(hint, want) {
					t.Errorf("提示缺少 %q：%s", want, hint)
				}
			}
			// 反面：把处理器名当成参数名是这个提示最容易犯的错。
			if strings.Contains(hint, "参数 rename") {
				t.Errorf("把处理器名 rename 当成了参数名：%s", hint)
			}
		})
	}
}
