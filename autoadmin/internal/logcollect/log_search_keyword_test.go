package logcollect

import (
	"testing"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 关键词框的两种模式（2026-09-19）：面板上那个框写着 Lucene，就必须真的支持 Lucene 语法。
//
//	message（默认）——只搜日志正文：冒号被转义，显式字段前缀不生效；
//	lucene        ——完整语法：`host_ip:"192.168.201.209"` 这类字段过滤原样下发。
//
// 现场就是这么来的：用户按框里的提示写 `host_ip: "192.168.201.209"`，被转义成在正文里找字面量
// "host_ip:"，于是 0 条——而平台上没有任何提示告诉他为什么。
func TestKeywordQueryModes(t *testing.T) {
	cases := []struct {
		name      string
		keyword   string
		mode      string
		wantQuery string
	}{
		{
			name:      "默认（不传 mode）= 只搜正文：冒号被转义",
			keyword:   `host_ip: "192.168.201.209"`,
			wantQuery: `host_ip\: "192.168.201.209"`,
		},
		{
			name:      "mode=message 同上",
			keyword:   "level:ERROR",
			mode:      "message",
			wantQuery: `level\:ERROR`,
		},
		{
			name:      "mode=lucene 原样下发：字段过滤可用",
			keyword:   `host_ip:"192.168.201.209" AND log_level:ERROR`,
			mode:      "lucene",
			wantQuery: `host_ip:"192.168.201.209" AND log_level:ERROR`,
		},
		{
			name:      "未知 mode 回落成只搜正文（更安全的一侧）",
			keyword:   "a:b",
			mode:      "lucene2",
			wantQuery: `a\:b`,
		},
		{
			name:      "关键词两端空白不影响",
			keyword:   "  timeout  ",
			mode:      "lucene",
			wantQuery: "timeout",
		},
	}
	for _, testCase := range cases {
		clause := keywordQuery(testCase.keyword, testCase.mode)
		if clause == nil {
			t.Fatalf("%s: 非空关键词必须给出子句", testCase.name)
		}
		queryString := (*clause)["query_string"].(gin.H)
		if got := queryString["query"].(string); got != testCase.wantQuery {
			t.Errorf("%s: query = %q，want %q", testCase.name, got, testCase.wantQuery)
		}
		// 裸词两种模式都落在日志正文上：否则 `timeout AND error` 的语义会随默认字段漂移。
		if queryString["default_field"] != "log_message" {
			t.Errorf("%s: default_field = %v，want log_message", testCase.name, queryString["default_field"])
		}
		// 开"能按字段查"的口子，不等于允许 `*foo` 前置通配（ES 侧是纯扫描）。
		if queryString["allow_leading_wildcard"] != false {
			t.Errorf("%s: 前置通配必须继续禁止", testCase.name)
		}
	}
	if clause := keywordQuery("   ", "lucene"); clause != nil {
		t.Errorf("空关键词不该产生子句，得到 %v", *clause)
	}
}

// 超长关键词截断（500 字符）：query_string 解析超长表达式既慢又容易报错。
func TestKeywordQueryTruncatesLongInput(t *testing.T) {
	long := make([]rune, 600)
	for index := range long {
		long[index] = 'a'
	}
	clause := keywordQuery(string(long), "lucene")
	queryString := (*clause)["query_string"].(gin.H)
	if got := len(queryString["query"].(string)); got != 500 {
		t.Fatalf("query 长度 = %d，want 500", got)
	}
}

// 跨服务检索（2026-09-28）：范围过滤必须是"每个服务一个四元组子句 + 任一命中"，
// 且服务编码跨业务重复时靠 project/business_system/environment 区分（见 000044）。
func TestMultiServiceScopeFilter(t *testing.T) {
	dimsList := []db.ListApplicationServiceStreamDimsByIDsRow{
		{ID: 1, ServiceCode: "nginx", ProjectCode: "yilake", BusinessSystemCode: "tib", EnvironmentCode: "poc"},
		{ID: 2, ServiceCode: "nginx", ProjectCode: "yilake", BusinessSystemCode: "cdm", EnvironmentCode: "prod"},
	}
	filter := multiServiceScopeFilter(dimsList)
	boolFilter, ok := filter["bool"].(gin.H)
	if !ok {
		t.Fatalf("顶层应是 bool")
	}
	should, _ := boolFilter["should"].([]any)
	if len(should) != 2 {
		t.Fatalf("每个服务一个子句，得到 %d", len(should))
	}
	if boolFilter["minimum_should_match"] != 1 {
		t.Fatalf("必须 minimum_should_match=1（任一命中即在范围内）")
	}
	first, _ := should[0].(gin.H)["bool"].(gin.H)["filter"].([]any)
	if len(first) != 4 {
		t.Fatalf("每个子句按四元组过滤（service/project/business_system/environment），得到 %d 项", len(first))
	}
}

func TestParseIDList(t *testing.T) {
	ids := parseIDList(" 1, 2,2,abc,3 ")
	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Fatalf("parseIDList 应去重/忽略非法段并保序，得到 %v", ids)
	}
	if len(parseIDList("")) != 0 || len(parseIDList(",,x")) != 0 {
		t.Fatalf("空/非法输入应返回空列表")
	}
}

// 上层节点的"概览优先"（2026-09-28）：facets 必须能按 逻辑服务/业务系统/环境/项目 分组，
// 且这些维度都要能作为过滤条件下钻（buildLogQuery 的白名单）。
func TestFacetAllowedFieldsCoverRangeDimensions(t *testing.T) {
	for _, field := range []string{"service", "project", "business_system", "environment"} {
		if !logFacetAllowedFields[field] {
			t.Fatalf("facet 白名单缺范围维度 %s（概览页将无法按它分组）", field)
		}
	}
}

// 日志路径维度映射到运行时字段。
func TestFacetAggField(t *testing.T) {
	if facetAggField("log_path") != logPathRuntimeField {
		t.Fatalf("log_path 应用运行时字段聚合")
	}
	if facetAggField("service") != "service" || facetAggField("") != "" {
		t.Fatalf("普通字段原样、空字段返回空串")
	}
}
