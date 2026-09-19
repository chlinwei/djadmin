package logcollect

import (
	"strings"
	"testing"
)

// mode/amount 的解析现在是两条清理路径（按服务 / 按未识别流）共用的：
// 分叉的话会出现"一条支持 days、另一条忘了校验上限"这种静默差异。
func TestParseCleanupMode(t *testing.T) {
	cases := []struct {
		mode       string
		amount     int
		wantMode   string
		wantCutoff string
		wantAmount int
		wantError  string
	}{
		{mode: "ALL", wantMode: "all", wantCutoff: ""},
		{mode: "hours", amount: 6, wantMode: "hours", wantCutoff: "now-6h", wantAmount: 6},
		{mode: "days", amount: 30, wantMode: "days", wantCutoff: "now-30d", wantAmount: 30},
		{mode: "hours", amount: 0, wantError: "between 1 and"},
		{mode: "days", amount: logCleanupMaxDays + 1, wantError: "between 1 and"},
		{mode: "", wantError: "mode must be all|hours|days"},
	}
	for _, testCase := range cases {
		mode, cutoff, amount, err := parseCleanupMode(testCase.mode, testCase.amount)
		if testCase.wantError != "" {
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("parseCleanupMode(%q, %d) error = %v，want 含 %q", testCase.mode, testCase.amount, err, testCase.wantError)
			}
			continue
		}
		if err != nil || mode != testCase.wantMode || cutoff != testCase.wantCutoff || amount != testCase.wantAmount {
			t.Fatalf("parseCleanupMode(%q, %d) = (%q, %q, %d, %v)", testCase.mode, testCase.amount, mode, cutoff, amount, err)
		}
	}
}

// "按流名清理"只收**一条具体的数据流名**：通配符、后备/系统索引、前缀之外的名字一律拒绝
// （每一条都在挡一种误删——这条路径的口子必须比服务维度那条更小，因为它没有服务可以收窄）。
func TestValidateCleanupStreamName(t *testing.T) {
	const prefix = "autoadmin"
	rejected := map[string]string{
		"":                                   "空名字",
		"   ":                                "全空格",
		"autoadmin-*":                        "通配符",
		"autoadmin-a,b":                      "逗号（多索引）",
		"autoadmin-?-x":                      "问号",
		".ds-autoadmin-kul-tib-test-svc-std": "后备索引",
		".kibana":                            "系统索引",
		"other-prefix-kul-tib-test-svc-std":  "前缀之外",
		"autoadmin":                          "只有前缀、没有实体名",
	}
	for name, why := range rejected {
		if err := validateCleanupStreamName(prefix, name); err == nil {
			t.Errorf("应拒绝 %q（%s）", name, why)
		}
	}
	if err := validateCleanupStreamName(prefix, "autoadmin-kul-tib-test-svc-std"); err != nil {
		t.Fatalf("合法流名被拒：%v", err)
	}
}
