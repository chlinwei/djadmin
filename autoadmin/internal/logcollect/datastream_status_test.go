package logcollect

import (
	"encoding/json"
	"strings"
	"testing"
)

// 契约用例：存储水位页按**逻辑服务**标注"已停用 / 未开启采集"，前端读这两个字段。
// 配置事实来自逻辑服务行（不需要查 ES），所以它们必须在每条流的 JSON 里出现——
// 缺字段会让页面把"停用的服务"显示成和正常采集一样。
func TestDataStreamEntryExposesServiceCollectionState(t *testing.T) {
	payload, err := json.Marshal(dataStreamEntry{
		Name: "logs-kul-tib-test-nginx-wuhan-test", Service: "nginx",
		Recognized: true, ServiceEnabled: false, CollectedByService: true,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"service_enabled":false`, `"service_collection_enabled":true`} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload 缺少 %s：%s", want, payload)
		}
	}
}

// 基于维度码的流名匹配：编码可含连字符（tomcat-svc / wuhan-test），纯切分必有歧义。
func TestResolveStreamName(t *testing.T) {
	matcher := streamNameMatcher{
		prefix: "logs",
		services: []streamServiceKey{
			// Enabled/CollectEnabled 是"服务级采集开关"，随解析结果带到页面用于标注
			// "已停用 / 未开启采集"（见 dataStreamEntry 的说明）。
			{Match: "kul-tib-test-tomcat-svc-", Project: "kul", Environment: "test", BusinessSystem: "tib", Service: "tomcat-svc", Enabled: true, CollectEnabled: true},
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
		{"logs-kul-tib-test-tomcat-svc-wuhan-test", parsedStreamName{Stream: "logs-kul-tib-test-tomcat-svc-wuhan-test", Project: "kul", Environment: "test", BusinessSystem: "tib", Service: "tomcat-svc", Tier: "wuhan-test", Recognized: true, ServiceEnabled: true, ServiceCollectEnabled: true}},
		{".ds-logs-kul-tib-test-tomcat-svc-hot-000015", parsedStreamName{Stream: "logs-kul-tib-test-tomcat-svc-hot", Project: "kul", Environment: "test", BusinessSystem: "tib", Service: "tomcat-svc", Tier: "hot", Recognized: true, ServiceEnabled: true, ServiceCollectEnabled: true}},
		// 旧命名（业务系统段序调整前）：无服务段，剩余段必须是已知档位
		{"logs-kul-test-tib-wuhan-test", parsedStreamName{Stream: "logs-kul-test-tib-wuhan-test", Project: "kul", Environment: "test", BusinessSystem: "tib", Tier: "wuhan-test", Recognized: true}},
		{".ds-logs-kul-test-cdm-wuhan-test-000015", parsedStreamName{Stream: "logs-kul-test-cdm-wuhan-test", Project: "kul", Environment: "test", BusinessSystem: "cdm", Tier: "wuhan-test", Recognized: true}},
		// 未识别
		{"logs-test-tib", parsedStreamName{Stream: "logs-test-tib"}},
		{"logs-kul-tib-test-unknown-tier", parsedStreamName{Stream: "logs-kul-tib-test-unknown-tier"}},
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
