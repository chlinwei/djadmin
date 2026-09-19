package logcollect

import (
	"strings"
	"testing"
)

// 契约用例：处理规则的正则按 **Go RE2** 判定（Filebeat 也是 RE2），且错误说明要能照着改。
//
// 现场来源（2026-09-19）：规则 `springboot-tomcat-exception` 的首行正则写成
// `^(...|\d{2}-[A-Za-z\u4e00-\u9fa5]{3,4}-\d{4})`——Java/JS 的码点写法，RE2 编译不过。
// 保存时没人拦（当时只在认证时才编译），认证时报 "invalid escape sequence: `\u`"。
func TestValidateRulePatternUsesRE2AndExplainsHowToFix(t *testing.T) {
	cases := []struct {
		name        string
		field       string
		pattern     string
		wantProblem bool
		// wantHints 是错误说明里必须出现的片段（"照着改"的依据）。
		wantHints []string
	}{
		{
			name:        "现场规则：Java/JS 的 \\uXXXX 码点写法被拒，并给出 \\x{XXXX} 的等价写法",
			field:       "首行正则",
			pattern:     `^(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\u4e00-\u9fa5]{3,4}-\d{4})`,
			wantProblem: true,
			wantHints:   []string{"首行正则", `\u4e00 → \x{4e00}`},
		},
		{
			name:    "改写后（RE2 的 \\x{XXXX}）通过",
			field:   "首行正则",
			pattern: `^(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\x{4e00}-\x{9fa5}]{3,4}-\d{4})`,
		},
		{
			name:    "空模式不报错（multiline 关闭时字段就是空串）",
			field:   "续行正则",
			pattern: "   ",
		},
		{
			name:        "后向断言：报错文案要点出 RE2 不支持，而不是照抄 invalid named capture",
			field:       "首行正则",
			pattern:     `(?<=\d)ERROR`,
			wantProblem: true,
			wantHints:   []string{"不支持断言"},
		},
		{
			name:  "现场续行正则用的否定前瞻：RE2 同样不支持，且提示要给出多行的正解（留空）",
			field: "续行正则",
			// 现场规则 springboot-tomcat-exception 的原值（\u4e00 之外还有这层断言）。
			pattern:     `^(?!(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\x{4e00}-\x{9fa5}]{3,4}-\d{4}))`,
			wantProblem: true,
			wantHints:   []string{"不支持断言", "续行正则留空即可"},
		},
		{
			name:        "反向引用被拒",
			field:       "首行正则",
			pattern:     `(\w+)\s+\1`,
			wantProblem: true,
			wantHints:   []string{"反向引用与八进制转义都不支持"},
		},
		{
			name:        "原子组与占有量词被拒",
			field:       "首行正则",
			pattern:     `a*+`,
			wantProblem: true,
			wantHints:   []string{"占有量词"},
		},
		{
			name:    "字面反斜杠 + u 不算码点写法（\\u 是转义后的字面量）",
			field:   "首行正则",
			pattern: `\\u4e00`,
		},
		{
			name:        "字符类里的 \\1 同样被拒（RE2 连八进制转义都不支持）",
			field:       "首行正则",
			pattern:     `^[\1]x`,
			wantProblem: true,
			wantHints:   []string{"不支持"},
		},
		{
			name:    "RE2 支持的能力不受影响：命名组 / 内联标志 / 字符类 / POSIX 类",
			field:   "首行正则",
			pattern: `(?i)^(?P<date>\d{4}-\d{2}-\d{2})[[:space:]]+ERROR[\-]`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problem := validateRulePattern(testCase.field, testCase.pattern)
			if testCase.wantProblem && problem == "" {
				t.Fatalf("期望被判为非法，实际通过：%q", testCase.pattern)
			}
			if !testCase.wantProblem && problem != "" {
				t.Fatalf("期望通过，实际报错：%s", problem)
			}
			for _, hint := range testCase.wantHints {
				if !strings.Contains(problem, hint) {
					t.Fatalf("错误说明缺少 %q：%s", hint, problem)
				}
			}
		})
	}
}
