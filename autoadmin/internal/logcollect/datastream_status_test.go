package logcollect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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

// 按服务收窄索引模式（日志中心页的"本服务水位"tab）：必须复用识别环节算好的维度段，
// 而不是拿四个码现拼——段序是"项目-业务系统-环境-服务"，与 logstream.Name 的形参顺序不一致，
// 拼错就永远查不到这个服务的任何索引（页面表现为"本服务没有任何流"，且不报错）。
func TestScopeIndexPattern(t *testing.T) {
	matcher := streamNameMatcher{
		prefix: "autoadmin",
		services: []streamServiceKey{
			{Match: "yilake-tib-poc-nginx-", Project: "yilake", Environment: "poc", BusinessSystem: "tib", Service: "nginx"},
			{Match: "yilake-tib-poc-mgmt-", Project: "yilake", Environment: "poc", BusinessSystem: "tib", Service: "mgmt"},
		},
		tiers: map[string]bool{"wuhan-test": true},
	}
	cases := []struct {
		name        string
		serviceCode string
		wantPattern string
		wantFound   bool
	}{
		{name: "不给服务编码＝全量视图（现有存储水位页行为不变）", serviceCode: "", wantPattern: "autoadmin-*", wantFound: true},
		{name: "给服务编码＝收窄到该服务的维度段", serviceCode: "nginx", wantPattern: "autoadmin-yilake-tib-poc-nginx-*", wantFound: true},
		{name: "另一个服务", serviceCode: "mgmt", wantPattern: "autoadmin-yilake-tib-poc-mgmt-*", wantFound: true},
		{name: "编码不存在＝找不到，由调用方回空视图（不能回落全量）", serviceCode: "not-exist", wantFound: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pattern, found := scopeIndexPattern("autoadmin", c.serviceCode, matcher)
			if found != c.wantFound {
				t.Fatalf("found = %v, want %v", found, c.wantFound)
			}
			if !c.wantFound {
				if pattern != "" {
					t.Fatalf("找不到时不该给出模式，得到 %q", pattern)
				}
				return
			}
			if pattern != c.wantPattern {
				t.Fatalf("pattern = %q, want %q", pattern, c.wantPattern)
			}
		})
	}
}

// 按服务收窄的 ES 查询必须把该服务**所有档位**的流都取回来，包括改档位后留下的历史流。
//
// 现场（2026-09-19）：nginx 把档位从 wuhan-test 改成 hot 之后，旧流
// `autoadmin-yilake-tib-poc-nginx-wuhan-test` 还留着 1.0 GB / 475 万条。收窄模式是
// `<前缀>-<项目>-<业务系统>-<环境>-<服务>-*`（末尾是服务段后的连字符），所以档位段整体落在
// 通配符里——若有人把模式写成"拼上当前档位"，历史流就会从这个页面消失（数据还在，只是看不见）。
func TestScopedPatternKeepsHistoricalTiers(t *testing.T) {
	matcher := streamNameMatcher{
		prefix: "autoadmin",
		services: []streamServiceKey{
			{Match: "yilake-tib-poc-nginx-", Project: "yilake", Environment: "poc", BusinessSystem: "tib", Service: "nginx"},
		},
		tiers: map[string]bool{"hot": true, "wuhan-test": true},
	}
	pattern, found := scopeIndexPattern("autoadmin", "nginx", matcher)
	if !found {
		t.Fatal("已知服务编码应当能找到收窄模式")
	}
	// 当前档位流与历史档位流都要匹配这个模式。
	for _, stream := range []string{
		"autoadmin-yilake-tib-poc-nginx-hot",
		"autoadmin-yilake-tib-poc-nginx-wuhan-test",
	} {
		if !wildcardMatches(pattern, stream) {
			t.Fatalf("模式 %q 应当匹配 %q（改档位留下的历史流不能被排除）", pattern, stream)
		}
	}
	// 别的服务不能被匹配进来。
	if wildcardMatches(pattern, "autoadmin-yilake-tib-poc-mgmt-wuhan-test") {
		t.Fatalf("模式 %q 不应匹配其他服务的流", pattern)
	}
}

// wildcardMatches 只实现 `*` 通配（够表达这里的索引模式），避免为测试引入正则语义差异。
func wildcardMatches(pattern, value string) bool {
	parts := strings.SplitN(pattern, "*", 2)
	if len(parts) == 1 {
		return pattern == value
	}
	return strings.HasPrefix(value, parts[0]) && strings.HasSuffix(value, parts[1])
}

// 端到端：给两档位的流喂进 fetchDataStreamEntries，两条都要在结果里且各自带对档位。
func TestFetchDataStreamEntriesIncludesBothTiers(t *testing.T) {
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if strings.Contains(request.URL.Path, "_cat/indices") {
			// 收窄后的模式必须原样带在请求路径上（收窄发生在 ES 查询里，不是查完再过滤）。
			if !strings.Contains(request.URL.Path, "autoadmin-yilake-tib-poc-nginx-*") {
				t.Errorf("索引查询路径没有收窄到本服务：%s", request.URL.Path)
			}
			_, _ = writer.Write([]byte(`[
				{"index":".ds-autoadmin-yilake-tib-poc-nginx-hot-2026.09.19-000001","health":"green","status":"open","docs.count":"10","store.size":"55867","creation.date.string":"2026-09-19T02:00:00.000Z"},
				{"index":".ds-autoadmin-yilake-tib-poc-nginx-wuhan-test-2026.09.18-000001","health":"green","status":"open","docs.count":"4758176","store.size":"1054670278","creation.date.string":"2026-09-18T02:00:00.000Z"}
			]`))
			return
		}
		_, _ = writer.Write([]byte(`{"indices":{}}`))
	}))
	defer elasticsearchServer.Close()

	cluster := elasticsearchCluster{ID: 1, Hosts: elasticsearchServer.URL, IndexPrefix: "autoadmin", Timeout: 5}
	handler := &Handler{}
	matcher := streamNameMatcher{
		prefix: "autoadmin",
		services: []streamServiceKey{
			{Match: "yilake-tib-poc-nginx-", Project: "yilake", Environment: "poc", BusinessSystem: "tib", Service: "nginx", Enabled: true, CollectEnabled: true},
		},
		tiers: map[string]bool{"hot": true, "wuhan-test": true},
	}

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	entries, _, err := handler.fetchDataStreamEntries(context, cluster, matcher, "autoadmin-yilake-tib-poc-nginx-*")
	if err != nil {
		t.Fatalf("fetch entries: %v", err)
	}
	tiers := map[string]float64{}
	for _, entry := range entries {
		if !entry.Recognized || entry.Service != "nginx" {
			t.Fatalf("两条流都应识别为本服务：%+v", entry)
		}
		tiers[entry.Tier] = entry.Docs
	}
	if tiers["hot"] != 10 || tiers["wuhan-test"] != 4758176 {
		t.Fatalf("两档位的流都要在结果里且文档数正确，得到 %+v", tiers)
	}
}

// 历史流判定下沉到后端（全局视图也要能标，且只有后端知道"服务当前生效档位"）。
// 三条语义都要钉住：档位不在生效集合里才是历史流；无日志定义的服务全算历史流；
// 判不出来（查询失败 / 旧命名流没有服务)宁可不标。
func TestIsHistoricalStream(t *testing.T) {
	matcher := streamNameMatcher{
		prefix: "autoadmin",
		activeTiers: map[string]map[string]bool{
			"nginx": {"hot": true},                     // 只有 hot 生效 → wuhan-test 是历史流
			"redis": {"hot": true, "wuhan-test": true}, // 两个档位都生效 → 都不是历史流
			"mgmt":  {},                                // 有服务行但没有任何日志定义 → 全算历史流
		},
	}
	cases := []struct {
		name        string
		service     string
		tier        string
		wantHistory bool
	}{
		{name: "当前档位不是历史流", service: "nginx", tier: "hot", wantHistory: false},
		{name: "改档位留下的旧档位是历史流", service: "nginx", tier: "wuhan-test", wantHistory: true},
		{name: "多档位都在生效时都不是历史流", service: "redis", tier: "wuhan-test", wantHistory: false},
		{name: "没有日志定义的服务：存量流全算历史流", service: "mgmt", tier: "hot", wantHistory: true},
		{name: "不认识的服务不判", service: "unknown-svc", tier: "hot", wantHistory: true},
		{name: "旧命名流没有服务段，不判", service: "", tier: "hot", wantHistory: false},
		{name: "没有档位段，不判", service: "nginx", tier: "", wantHistory: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := matcher.isHistoricalStream(testCase.service, testCase.tier); got != testCase.wantHistory {
				t.Fatalf("isHistoricalStream(%q, %q) = %v, want %v", testCase.service, testCase.tier, got, testCase.wantHistory)
			}
		})
	}
}

// 契约：historical 要出现在流的 JSON 里（页面靠它区分"当前档位"与"历史档位"）。
func TestDataStreamEntryExposesHistoricalFlag(t *testing.T) {
	payload, err := json.Marshal(dataStreamEntry{Name: "logs-kul-tib-test-nginx-wuhan-test", Service: "nginx", Tier: "wuhan-test", Recognized: true, Historical: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(payload), `"historical":true`) {
		t.Fatalf("payload 缺少 historical：%s", payload)
	}
}

// readQueriesFile 读模块内的 SQL 文件（本包没有 assets 包那套助手，就地写一个）。
func readQueriesFile(t *testing.T, relativePath string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	raw, err := os.ReadFile(filepath.Join(dir, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(raw)
}

// namedSQLStatement 截出 `-- name: X` 到下一个 `-- name:` 之间的 SQL（sqlc 的语句边界）。
func namedSQLStatement(content, name string) string {
	marker := "-- name: " + name
	start := strings.Index(content, marker)
	if start < 0 {
		return ""
	}
	rest := content[start+len(marker):]
	if next := strings.Index(rest, "-- name: "); next >= 0 {
		rest = rest[:next]
	}
	return rest
}

// 契约：dims 必须给全四层（projects / business_systems / environments / services）。
// services 曾在 2026-09-18 因"前端从未消费"被删，现在日志中心的容量统计用它把每层成员列全
// （含没有任何日志的服务），并用它反推"某业务系统下的环境"（环境是服务上的属性，
// assets_business_environment 不挂在业务系统下，没有它就只能退回"流过才有环境"）。
// 这条测试钉的是"别再把这份 payload 当死数据删掉"。
func TestStorageOverviewDimsCarryAllFourLevels(t *testing.T) {
	statement := namedSQLStatement(readQueriesFile(t, "db/queries/mysql/monitor.sql"), "ListEnabledServiceStreamDims")
	if statement == "" {
		t.Fatal("找不到 ListEnabledServiceStreamDims（dims.services 的数据源）")
	}
	// 环境码要 COALESCE 成空串：它是可空的（服务可以没有环境），返回 NULL 会让前端分组拿到 null。
	for _, column := range []string{"s.code", "bs.code AS business_system", "COALESCE(e.code, '') AS environment"} {
		if !strings.Contains(statement, column) {
			t.Fatalf("服务维度查询缺少 %q：\n%s", column, statement)
		}
	}
	// 只列启用服务：与项目/业务系统/环境的维度表口径一致（停用维度不该出现在"该配没配"的清单里）。
	if !strings.Contains(statement, "s.enabled = TRUE") {
		t.Fatalf("服务维度查询应只列启用服务：\n%s", statement)
	}
}
