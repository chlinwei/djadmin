package monitor

import "testing"

// 基于维度码的流名匹配：编码可含连字符（tomcat-svc / wuhan-test），纯切分必有歧义。
func TestResolveStreamName(t *testing.T) {
	matcher := streamNameMatcher{
		prefix: "logs",
		services: []streamServiceKey{
			{Match: "kul-tib-test-tomcat-svc-", Project: "kul", Environment: "test", BusinessSystem: "tib", Service: "tomcat-svc"},
		},
		legacy: []streamLegacyKey{
			{Match: "kul-test-tib-", Project: "kul", Environment: "test", BusinessSystem: "tib"},
			{Match: "kul-test-cdm-", Project: "kul", Environment: "test", BusinessSystem: "cdm"},
		},
		tiers: map[string]bool{"wuhan-test": true, "hot": true},
	}
	cases := []struct {
		index string
		want  parsedStreamName
	}{
		// 新命名：logs-<项目>-<业务系统>-<环境>-<服务>-<档位>（业务系统段在环境段前）
		{"logs-kul-tib-test-tomcat-svc-wuhan-test", parsedStreamName{"logs-kul-tib-test-tomcat-svc-wuhan-test", "kul", "test", "tib", "tomcat-svc", "wuhan-test", true}},
		{".ds-logs-kul-tib-test-tomcat-svc-hot-000015", parsedStreamName{"logs-kul-tib-test-tomcat-svc-hot", "kul", "test", "tib", "tomcat-svc", "hot", true}},
		// 旧命名（业务系统段序调整前）：无服务段，剩余段必须是已知档位
		{"logs-kul-test-tib-wuhan-test", parsedStreamName{"logs-kul-test-tib-wuhan-test", "kul", "test", "tib", "", "wuhan-test", true}},
		{".ds-logs-kul-test-cdm-wuhan-test-000015", parsedStreamName{"logs-kul-test-cdm-wuhan-test", "kul", "test", "cdm", "", "wuhan-test", true}},
		// 未识别
		{"logs-test-tib", parsedStreamName{"logs-test-tib", "", "", "", "", "", false}},
		{"logs-kul-tib-test-unknown-tier", parsedStreamName{"logs-kul-tib-test-unknown-tier", "", "", "", "", "", false}},
	}
	for _, c := range cases {
		got := matcher.resolveStreamName(stripBackingIndexSuffixes("logs", c.index))
		if got != c.want {
			t.Errorf("resolve(%q) = %+v, want %+v", c.index, got, c.want)
		}
	}
}

func TestLogDataStreamName(t *testing.T) {
	if name := LogDataStreamName("", "kul", "test", "tib", "tomcat-svc", "hot"); name != "autoadmin-kul-tib-test-tomcat-svc-hot" {
		t.Errorf("LogDataStreamName = %q", name)
	}
}
