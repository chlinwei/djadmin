package assets

import (
	"encoding/json"
	"testing"
)

func testLogRuleSignature() string {
	signature, err := logFormatRuleSignature(logFormatRuleInput{
		InputFormat: "plain", FlushTimeout: 2000, PipelineBody: json.RawMessage(`{"processors":[]}`),
	})
	if err != nil {
		panic(err)
	}
	return signature
}

// 格式认证指纹：只有"会改格式"的输入变化才让指纹变（= 认证失效）。
// 对应架构文档 §4.8 的四类变化：换模板/改名/改路径、改规则、换挂规则、应用版本升级，
// 外加服务级宏（换宏 = 换了个文件在采）。
func TestLogFormatFingerprint(t *testing.T) {
	base := logFormatFingerprintInput{
		LogDefinition: 11, LogName: "catalina", PathPattern: "${APP_HOME}/logs/catalina.out",
		RuleID: 7, RuleParsingSignature: testLogRuleSignature(), MacroValues: `{"APP_HOME":"/opt/app"}`, ApplicationVersion: 3,
	}
	baseline := logFormatFingerprintOf(base)
	if baseline == "" || len(baseline) != 32 {
		t.Fatalf("指纹形态不符：%q", baseline)
	}
	// 同输入必须稳定（比对是纯字符串相等，抖动会让认证无意义地失效）。
	if again := logFormatFingerprintOf(base); again != baseline {
		t.Fatalf("同输入指纹应稳定：%s vs %s", baseline, again)
	}

	cases := []struct {
		name   string
		mutate func(input *logFormatFingerprintInput)
	}{
		{"换模板/换日志定义", func(input *logFormatFingerprintInput) { input.LogDefinition = 12 }},
		{"改名", func(input *logFormatFingerprintInput) { input.LogName = "catalina-2" }},
		{"改路径", func(input *logFormatFingerprintInput) { input.PathPattern = "${APP_HOME}/logs/*.log" }},
		{"换挂另一条规则", func(input *logFormatFingerprintInput) { input.RuleID = 8 }},
		{"改规则（pipeline_body/多行/正则）", func(input *logFormatFingerprintInput) {
			input.RuleParsingSignature += "-changed"
		}},
		{"改服务级宏（换了个文件在采）", func(input *logFormatFingerprintInput) {
			input.MacroValues = `{"APP_HOME":"/opt/app2"}`
		}},
		{"应用/中间件版本升级", func(input *logFormatFingerprintInput) { input.ApplicationVersion = 4 }},
	}
	for _, testCase := range cases {
		mutated := base
		testCase.mutate(&mutated)
		if got := logFormatFingerprintOf(mutated); got == baseline {
			t.Errorf("%s 应让指纹变化（认证失效），实际没变：%s", testCase.name, got)
		}
	}

}

func TestLogFormatRuleSignature(t *testing.T) {
	base := logFormatRuleInput{
		InputFormat: "plain", MultilineEnabled: true, StartPattern: "^INFO", ContinuationPattern: "^ ",
		FlushTimeout: 2000, PipelineBody: json.RawMessage(`{"processors":[{"set":{"field":"log_level","value":"INFO"}}],"on_failure":[]}`),
	}
	baseline, err := logFormatRuleSignature(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*logFormatRuleInput)
	}{
		{"input format", func(input *logFormatRuleInput) { input.InputFormat = "json" }},
		{"multiline", func(input *logFormatRuleInput) { input.MultilineEnabled = false }},
		{"start regex", func(input *logFormatRuleInput) { input.StartPattern = "^ERROR" }},
		{"continuation regex", func(input *logFormatRuleInput) { input.ContinuationPattern = "^\\t" }},
		{"flush timeout", func(input *logFormatRuleInput) { input.FlushTimeout++ }},
		{"processor", func(input *logFormatRuleInput) {
			input.PipelineBody = json.RawMessage(`{"processors":[{"set":{"field":"log_level","value":"ERROR"}}],"on_failure":[]}`)
		}},
		{"failure handling", func(input *logFormatRuleInput) {
			input.PipelineBody = json.RawMessage(`{"processors":[{"set":{"field":"log_level","value":"INFO"}}],"on_failure":[{"drop":{}}]}`)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			changed := base
			testCase.mutate(&changed)
			signature, err := logFormatRuleSignature(changed)
			if err != nil {
				t.Fatal(err)
			}
			if signature == baseline {
				t.Fatal("解析配置变更必须使指纹变化")
			}
		})
	}
	t.Run("metadata and JSON formatting", func(t *testing.T) {
		changed := base
		changed.PipelineBody = json.RawMessage(`{
			"description": "只修改描述", "_meta": {"owner":"ops"}, "version": 2,
			"on_failure": [],
			"processors": [{"set":{"value":"INFO","field":"log_level"}}]
		}`)
		signature, err := logFormatRuleSignature(changed)
		if err != nil {
			t.Fatal(err)
		}
		if signature != baseline {
			t.Fatal("元数据、排版及对象键顺序不应使指纹变化")
		}
	})
	t.Run("processor order", func(t *testing.T) {
		first := base
		second := base
		first.PipelineBody = json.RawMessage(`{"processors":[{"set":{"field":"x","value":1}},{"remove":{"field":"x"}}]}`)
		second.PipelineBody = json.RawMessage(`{"processors":[{"remove":{"field":"x"}},{"set":{"field":"x","value":1}}]}`)
		left, err := logFormatRuleSignature(first)
		if err != nil {
			t.Fatal(err)
		}
		right, err := logFormatRuleSignature(second)
		if err != nil {
			t.Fatal(err)
		}
		if left == right {
			t.Fatal("处理器顺序必须参与指纹")
		}
	})
	t.Run("large numbers retain precision", func(t *testing.T) {
		first := base
		second := base
		first.PipelineBody = json.RawMessage(`{"processors":[{"set":{"field":"x","value":9007199254740992}}]}`)
		second.PipelineBody = json.RawMessage(`{"processors":[{"set":{"field":"x","value":9007199254740993}}]}`)
		left, err := logFormatRuleSignature(first)
		if err != nil {
			t.Fatal(err)
		}
		right, err := logFormatRuleSignature(second)
		if err != nil {
			t.Fatal(err)
		}
		if left == right {
			t.Fatal("数值精度损失不得使不同解析配置的指纹相同")
		}
	})
	for _, body := range []string{"", "null", "[]", "{", `{} {}`} {
		t.Run("invalid pipeline "+body, func(t *testing.T) {
			invalid := base
			invalid.PipelineBody = json.RawMessage(body)
			if _, err := logFormatRuleSignature(invalid); err == nil {
				t.Fatal("无效解析配置必须返回错误")
			}
		})
	}
}
