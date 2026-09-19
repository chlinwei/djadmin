package assets

import (
	"testing"
	"time"
)

// 格式认证指纹：只有"会改格式"的输入变化才让指纹变（= 认证失效）。
// 对应架构文档 §4.8 的四类变化：换模板/改名/改路径、改规则、换挂规则、应用版本升级，
// 外加服务级宏（换宏 = 换了个文件在采）。
func TestLogFormatFingerprint(t *testing.T) {
	baseTime := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	base := logFormatFingerprintInput{
		LogDefinition: 11, LogName: "catalina", PathPattern: "${APP_HOME}/logs/catalina.out",
		RuleID: 7, RuleUpdatedAt: baseTime, MacroValues: `{"APP_HOME":"/opt/app"}`, ApplicationVersion: 3,
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
			input.RuleUpdatedAt = baseTime.Add(time.Second)
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

	// 微秒精度：亚微秒的差异不该被格式化吞掉再放大成"假失效"（MySQL/PG 都是微秒列）。
	tiny := base
	tiny.RuleUpdatedAt = baseTime.Add(500 * time.Nanosecond)
	if logFormatFingerprintOf(tiny) != baseline {
		t.Fatalf("亚微秒差异不应改变指纹（列精度是微秒）")
	}
	// 时区不同的同一时刻必须得到同一指纹（驱动回读的时区差异不该造成假失效）。
	shifted := base
	shifted.RuleUpdatedAt = baseTime.In(time.FixedZone("UTC+8", 8*3600))
	if logFormatFingerprintOf(shifted) != baseline {
		t.Fatalf("同一时刻的不同时区表示应得到同一指纹")
	}
}
