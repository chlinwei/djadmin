package logcollect

import "testing"

// 服务视图状态判定：本服务自己一致就 synced，不被同主机其他服务的改动带偏；
// 存量目标（服务级记录为空但整机一致）兜底成 synced，不误报"从未下发"。
func TestServiceConfigStatus(t *testing.T) {
	cases := []struct {
		name                            string
		expectedService, appliedService string
		expectedHost, appliedHost       string
		want                            string
	}{
		{"本服务子指纹一致 → synced", "s1", "s1", "h1", "h1", LogConfigSynced},
		{"本服务子指纹不一致 → drift", "s2", "s1", "h2", "h1", LogConfigDrift},
		{"本服务无片段（期望空）→ synced", "", "", "h1", "h1", LogConfigSynced},
		{"本服务无片段但有残留记录 → drift", "", "s1", "h1", "h1", LogConfigDrift},
		{"存量：无服务级记录但整机一致 → synced（兜底）", "s1", "", "h1", "h1", LogConfigSynced},
		{"无服务级记录且整机也不一致 → never", "s1", "", "h2", "h1", LogConfigNever},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := serviceConfigStatus(testCase.expectedService, testCase.appliedService, testCase.expectedHost, testCase.appliedHost); got != testCase.want {
				t.Fatalf("serviceConfigStatus = %q, want %q", got, testCase.want)
			}
		})
	}
}
