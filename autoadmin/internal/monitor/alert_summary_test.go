package monitor

import (
	"encoding/json"
	"testing"
)

// 「问题」这一栏的文案口径：当前告警与历史告警必须同一处实现（2026-09-20 现场：
// 当前告警有「问题」列、历史告警没有，同一个告警两处长得不一样）。
func TestAlertSummaryTextFallsBackToDescription(t *testing.T) {
	cases := []struct {
		name        string
		annotations map[string]any
		want        string
	}{
		{"有 summary", map[string]any{"summary": "错误率超过 5%"}, "错误率超过 5%"},
		{"只有 description 时退回它", map[string]any{"description": "5 分钟内错误率持续超标"}, "5 分钟内错误率持续超标"},
		{"都没有则为空", map[string]any{}, ""},
		{"summary 非字符串时按描述取", map[string]any{"summary": 42, "description": "兜底"}, "42"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := alertSummaryText(testCase.annotations); got != testCase.want {
				t.Fatalf("alertSummaryText = %q, want %q", got, testCase.want)
			}
		})
	}
}

// 历史行的 annotations 是落库的 JSON：解析失败/为空时「问题」列必须是空（界面显示"-"），
// 不能把坏 JSON 当文案吐出来，也不能 panic。
func TestParseAnnotationsDegradesToEmpty(t *testing.T) {
	if got := parseAnnotations(json.RawMessage(`{"summary":"x"}`)); got["summary"] != "x" {
		t.Fatalf("正常 JSON 应解析出来，得到 %#v", got)
	}
	for _, raw := range []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("{"), json.RawMessage("null")} {
		if got := parseAnnotations(raw); len(got) != 0 {
			t.Fatalf("坏数据 %q 应降级成空 map，得到 %#v", string(raw), got)
		}
	}
}
