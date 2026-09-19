package logcollect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 样例事件必须带 Filebeat 自己的字段（2026-09-19 现场回归，见 sample_event.go）。
//
// 现场：规则第 1 个处理器 `rename log → message`（Fluent Bit 时代的写法）在样例只有
// `{"message": ...}` 时是静默 no-op（ignore_missing），平台上全绿；而在主机上 `log` 是
// Filebeat 的文件元信息对象，它把 message 覆盖成对象 → ES 400 拒收每一条 → 一条数据都没有。
// 这里钉住"样例文档的形状与主机上一致"。
func TestSampleDocsCarryFilebeatEventFields(t *testing.T) {
	docs, err := logSampleDocs("2026-09-19 10:00:00 ERROR boom\n", false, "")
	if err != nil {
		t.Fatalf("logSampleDocs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d 条，want 1", len(docs))
	}
	doc, ok := docs[0].(map[string]any)
	if !ok {
		t.Fatalf("doc type = %T", docs[0])
	}
	if doc["message"] != "2026-09-19 10:00:00 ERROR boom" {
		t.Fatalf("message = %#v，原始行必须原样进 message", doc["message"])
	}
	// log 必须是**对象**（Filebeat 的 log.file.path / log.offset），不是字符串——正是这一点让
	// `rename log → message` 在主机上具有破坏性。
	log, ok := doc["log"].(map[string]any)
	if !ok {
		t.Fatalf("log = %#v，want 对象（Filebeat 文件元信息）", doc["log"])
	}
	file, _ := log["file"].(map[string]any)
	if _, ok := file["path"].(string); !ok {
		t.Fatalf("log.file.path = %#v，want 字符串", file["path"])
	}
	for _, field := range []string{"host", "agent", "ecs", "event", "input"} {
		if _, ok := doc[field].(map[string]any); !ok {
			t.Errorf("%s = %#v，want 对象（Filebeat 自带字段）", field, doc[field])
		}
	}
	// 平台注入的维度字段也要在：规则里写 {{log_name}} 的处理器在真实事件上取得到值。
	for _, field := range []string{"service", "instance", "application", "log_name", "log_path", "host_ip"} {
		if value, _ := doc[field].(string); value == "" {
			t.Errorf("%s 缺失，样例里也必须有（真实事件一定有）", field)
		}
	}
}

// 多行还原出来的记录同样补齐（不能只补单行那条路径）。
func TestSampleDocsCarryFilebeatEventFieldsForMultiline(t *testing.T) {
	docs, err := logSampleDocs("2026-09-19 10:00:00 ERROR boom\n\tat Foo.bar(Foo.java:1)\n", true, `^\d{4}-\d{2}-\d{2}`)
	if err != nil {
		t.Fatalf("logSampleDocs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d 条，want 1", len(docs))
	}
	doc := docs[0].(map[string]any)
	if !strings.Contains(doc["message"].(string), "\tat Foo.bar") {
		t.Fatalf("message = %#v，堆栈应并入同一条记录", doc["message"])
	}
	if _, ok := doc["log"].(map[string]any); !ok {
		t.Fatalf("多行记录同样要带 Filebeat 字段，得到 log = %#v", doc["log"])
	}
}

// 调用方自己给的字段优先（调试页的"样例文档 JSON"模式可以自己造 log/message）。
func TestWithFilebeatEventFieldsKeepsCallerValues(t *testing.T) {
	merged := withFilebeatEventFields(map[string]any{
		"message": "我的样例",
		"log":     "我自己给的 log",
		"extra":   "x",
	})
	if merged["message"] != "我的样例" || merged["log"] != "我自己给的 log" || merged["extra"] != "x" {
		t.Fatalf("调用方给的值被覆盖了：%#v", merged)
	}
	if _, ok := merged["host"].(map[string]any); !ok {
		t.Fatalf("没给的字段应被补齐，得到 host = %#v", merged["host"])
	}
}

// schema 违规判定要豁免 Filebeat 自有字段：它们本来就不在索引模板 mapping 里，
// dynamic=false 丢掉是设计如此，不是规则写错字段。不豁免的话每条规则的试算都会多出 6 条噪音。
func TestNonStandardDocumentFieldsExemptsFilebeatFields(t *testing.T) {
	source := map[string]any{"message": "x", "app_fields": map[string]any{"pid": "1"}}
	for field := range filebeatOwnedFieldNames() {
		source[field] = map[string]any{"sample": true}
	}
	source["custom_field"] = "规则自己造的字段"
	result := map[string]any{"docs": []any{map[string]any{"doc": map[string]any{"_source": source}}}}
	got := nonStandardDocumentFields(result, map[string]bool{"message": true, "app_fields": true})
	if len(got) != 1 || got[0] != "custom_field" {
		t.Fatalf("nonStandardDocumentFields = %v，want [custom_field]（Filebeat 字段豁免、规则自造字段仍要点名）", got)
	}
}

// clobberedMessageHint 直指现场原因：message 被搬成对象时要说清"原始行丢了"，别只报"缺字段"。
func TestClobberedMessageHint(t *testing.T) {
	clobbered := map[string]any{"docs": []any{map[string]any{"doc": map[string]any{"_source": map[string]any{
		"message": map[string]any{"file": map[string]any{"path": "/var/log/x.log"}, "offset": 1},
	}}}}}
	if hint := clobberedMessageHint(clobbered, "raw line"); !strings.Contains(hint, "rename log") {
		t.Fatalf("message 被覆盖成对象时应给出 rename 提示，得到 %q", hint)
	}
	deleted := map[string]any{"docs": []any{map[string]any{"doc": map[string]any{"_source": map[string]any{}}}}}
	if hint := clobberedMessageHint(deleted, "raw line"); !strings.Contains(hint, "删掉") {
		t.Fatalf("message 被删掉时应直说，得到 %q", hint)
	}
	intact := map[string]any{"docs": []any{map[string]any{"doc": map[string]any{"_source": map[string]any{"message": "raw line"}}}}}
	if hint := clobberedMessageHint(intact, "raw line"); hint != "" {
		t.Fatalf("message 正常时不该有提示，得到 %q", hint)
	}
	// 调用方根本没给 message（比如"样例文档 JSON"模式只造了别的字段）时不算"被覆盖"。
	if hint := clobberedMessageHint(clobbered, ""); hint != "" {
		t.Fatalf("没有原始行可对比时不该冤枉处理器，得到 %q", hint)
	}
}

// 现场回归（kul 的 tomcat，2026-09-19）：**规则试跑必须判不通过**。
//
// mock 返回的是集群当时的真实响应形状：`message` 变成 Filebeat 的 log 对象、
// 必备字段全缺（grok 取不到原始行）。此前没有这条判据，采集链路全绿、ES 里一条都没有。
func TestRulePipelineRunVerdictFlagsClobberedMessage(t *testing.T) {
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			// 索引模板取不到 → 判定回退内置标准字段，不影响本用例。
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		_, _ = writer.Write([]byte(`{"docs":[{"doc":{"_source":{
			"@timestamp":"2026-09-19T12:24:51Z",
			"message":{"file":{"path":"/home/esb/tomcat/apache-tomcat-9.0.35/logs/catalina.out"},"offset":1000},
			"app_fields":{"parse_error":"field [message] of type [java.util.HashMap] cannot be cast to [java.lang.String]"}
		}}}]}`))
	}))
	defer elasticsearchServer.Close()

	gin.SetMode(gin.TestMode)
	handler := &Handler{}
	cluster := elasticsearchCluster{Hosts: elasticsearchServer.URL, IndexPrefix: "autoadmin", Timeout: 5}
	rule := rulePipelineInput{
		Name: "springboot-tomcat-exception",
		SampleLog: strings.Join([]string{
			"2026-09-19 10:00:00 ERROR 3229551 --- [main] com.example.Boom : 起不来",
			"\tat com.example.Boom.go(Boom.java:42)",
		}, "\n"),
		MultilineEnabled: true,
		StartPattern:     `^\d{4}-\d{2}-\d{2}`,
	}
	verdict := handler.rulePipelineRunVerdict(ginContextForTest(), cluster, "autoadmin-tomcat-springboot-tomcat-exception", rule)
	if !verdict.failed {
		t.Fatalf("必须判不通过，得到 %+v", verdict)
	}
	for _, want := range []string{"log_level", "log_message", "拒收", "rename log"} {
		if !strings.Contains(verdict.detail, want) {
			t.Errorf("判定说明里应包含 %q：%s", want, verdict.detail)
		}
	}
}

// 跑得通就该通过：message 正常、三个必备字段齐 → failed=false（别把好规则判成坏规则）。
func TestRulePipelineRunVerdictPassesHealthyRule(t *testing.T) {
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		_, _ = writer.Write([]byte(`{"docs":[{"doc":{"_source":{"message":"2026-09-19 10:00:00 ERROR boom",
			"log_level":"ERROR","log_message":"boom","error_fingerprint":"abc"}}}]}`))
	}))
	defer elasticsearchServer.Close()

	handler := &Handler{}
	cluster := elasticsearchCluster{Hosts: elasticsearchServer.URL, IndexPrefix: "autoadmin", Timeout: 5}
	verdict := handler.rulePipelineRunVerdict(ginContextForTest(), cluster, "autoadmin-tomcat-springboot-tomcat-exception", rulePipelineInput{
		Name:      "springboot-tomcat-exception",
		SampleLog: "2026-09-19 10:00:00 ERROR boom",
	})
	if verdict.failed || verdict.skipped {
		t.Fatalf("健康规则应通过，得到 %+v", verdict)
	}
}

// 没配样例日志时**不判**（skipped），不能返回"通过"——"判不了"与"没问题"必须能区分。
func TestRulePipelineRunVerdictSkipsWithoutSample(t *testing.T) {
	handler := &Handler{}
	verdict := handler.rulePipelineRunVerdict(ginContextForTest(), elasticsearchCluster{}, "p", rulePipelineInput{Name: "r"})
	if verdict.failed || !verdict.skipped {
		t.Fatalf("没配样例日志应 skipped，得到 %+v", verdict)
	}
	if !strings.Contains(verdict.detail, "未做试跑") {
		t.Fatalf("要说清为什么没判：%s", verdict.detail)
	}
}

// 调试页样例文档补 Filebeat 字段后，上游请求体也必须是补齐后的（_source 里带上 log.file.path）。
func TestSimulateMergesFilebeatFieldsIntoUpstreamDocs(t *testing.T) {
	var capturedBody map[string]any
	elasticsearchServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		_, _ = writer.Write([]byte(`{"docs":[{"doc":{"_source":{"message":"hello"}}}]}`))
	}))
	defer elasticsearchServer.Close()

	handler := &Handler{}
	cluster := elasticsearchCluster{Hosts: elasticsearchServer.URL, IndexPrefix: "autoadmin", Timeout: 5}
	if _, err := handler.simulatePipeline(ginContextForTest(), cluster, "", map[string]any{"processors": []any{}},
		[]any{withFilebeatEventFields(map[string]any{"message": "hello"})}); err != nil {
		t.Fatalf("simulatePipeline: %v", err)
	}
	docs, _ := capturedBody["docs"].([]any)
	if len(docs) != 1 {
		t.Fatalf("upstream docs = %#v", capturedBody["docs"])
	}
	wrap, ok := docs[0].(map[string]any)
	if !ok {
		t.Fatalf("upstream docs[0] = %#v，want 对象", docs[0])
	}
	// 请求体是 [{"_source": {...}}]（ES _simulate 的要求），响应才是 doc.doc._source 两层。
	source, ok := wrap["_source"].(map[string]any)
	if !ok {
		t.Fatalf("upstream docs[0]._source = %#v，want 对象", wrap["_source"])
	}
	if source["message"] != "hello" {
		t.Fatalf("_source.message = %#v", source["message"])
	}
	log, ok := source["log"].(map[string]any)
	if !ok {
		t.Fatalf("_source.log = %#v，want Filebeat 的文件元信息对象", source["log"])
	}
	file, ok := log["file"].(map[string]any)
	if !ok {
		t.Fatalf("_source.log.file = %#v，want 对象", log["file"])
	}
	if _, ok := file["path"].(string); !ok {
		t.Fatalf("_source.log.file.path 缺失：%#v", file)
	}
}

// ginContextForTest 给不需要请求上下文的调用一个最小 context（simulatePipeline 只要求非 nil）。
func ginContextForTest() *gin.Context {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	return context
}
