package logcollect

import "testing"

// 保留档位的"值 + 单位"（迁移 000045 / 架构文档 §4.5）：小时是新增能力，天是存量语义。
// 这里钉两件最容易错的事：
//  1. 容量反推（§4.6）必须按单位折算——小时档位不折算会把预估占用虚高 24 倍；
//  2. 单位白名单——库里只允许出现 `d`/`h` 两种单位。

func TestEstimatedTierTotalGBConvertsHours(t *testing.T) {
	cases := []struct {
		name        string
		dailySizeGB float64
		value       int64
		unit        string
		want        float64
	}{
		{"每天 10GB 保留 30 天", 10, 30, "d", 300},
		{"每天 10GB 保留 12 小时 = 半天", 10, 12, "h", 5},
		{"每天 10GB 保留 1 小时", 10, 1, "h", 0.42},
		{"单位缺失按天（存量数据）", 10, 7, "", 70},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := estimatedTierTotalGB(testCase.dailySizeGB, testCase.value, testCase.unit); got != testCase.want {
				t.Fatalf("estimatedTierTotalGB = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestRetentionUnitOrDefault(t *testing.T) {
	if got := retentionUnitOrDefault("h"); got != "h" {
		t.Fatalf("h 应被接受，得到 %q", got)
	}
	if got := retentionUnitOrDefault("H"); got != "h" {
		t.Fatalf("大小写应归一，得到 %q", got)
	}
	// 只认 d/h：分钟不在白名单里（ILM 轮询粒度 10 分钟，分钟级保留没有实际意义），
	// 非法值一律按天兜底，绝不让库里出现第三种单位。
	for _, raw := range []any{"d", "m", "days", "", nil, 3} {
		if got := retentionUnitOrDefault(raw); got != "d" && got != "h" {
			t.Fatalf("非法单位 %#v 归一成了 %q", raw, got)
		}
	}
	if got := retentionUnitOrDefault("m"); got != "d" {
		t.Fatalf("分钟应按天兜底（不是被当成小时），得到 %q", got)
	}
}

func TestRetentionPeriodText(t *testing.T) {
	if got := retentionPeriodText(retentionTierRow{RetentionValue: 30, RetentionUnit: "d"}); got != "保留 30 天" {
		t.Fatalf("天档位文案 = %q", got)
	}
	if got := retentionPeriodText(retentionTierRow{RetentionValue: 6, RetentionUnit: "h"}); got != "保留 6 小时" {
		t.Fatalf("小时档位文案 = %q", got)
	}
}

// 写入校验：值 + 单位一起看（范围按折算后的小时数判）。
func TestValidateRetentionTierPeriod(t *testing.T) {
	spec := retentionSpec
	cases := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{"天数在范围内", map[string]any{"retention_value": float64(30)}, false},
		{"小时在范围内", map[string]any{"retention_value": float64(12), "retention_unit": "h"}, false},
		{"值小于 1", map[string]any{"retention_value": float64(0)}, true},
		{"超过 3650 天", map[string]any{"retention_value": float64(3651)}, true},
		{"小时数折算后未超上限", map[string]any{"retention_value": float64(4000), "retention_unit": "h"}, false},
		{"单位不合法", map[string]any{"retention_value": float64(1), "retention_unit": "m"}, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := validateResource(spec, testCase.input, 0)
			if testCase.wantErr && got == "" {
				t.Fatal("应当报错却通过了")
			}
			if !testCase.wantErr && got != "" {
				t.Fatalf("不该报错，得到 %q", got)
			}
		})
	}
}
