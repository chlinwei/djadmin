package logcollect

import (
	"encoding/json"
	"strings"
	"testing"

	"autoadmin/internal/assets"

	"github.com/gin-gonic/gin"
)

func TestServiceExtraPipelineName(t *testing.T) {
	if got := serviceExtraPipelineName("logs", "", "tomcat-access", 42); got != "logs-general-tomcat-access@s42" {
		t.Fatalf("serviceExtraPipelineName = %q", got)
	}
	if got := serviceExtraPipelineName("autoadmin", "MyApp", "Rule_A", 7); got != "autoadmin-myapp-rule_a@s7" {
		t.Fatalf("段清洗应与 processingPipelineName 一致，得到 %q", got)
	}
}

func TestBuildServiceExtraPipelineBodyChainsBaseFirst(t *testing.T) {
	extra := json.RawMessage(`[{"drop_fields":{"fields":["host.name"]}}]`)
	body, err := buildServiceExtraPipelineBody("logs-general-tomcat-access", extra)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	processors, _ := body["processors"].([]any)
	if len(processors) != 2 {
		t.Fatalf("processors 数 = %d, want 2（pipeline 串联 + 1 个额外处理器）", len(processors))
	}
	first, ok := processors[0].(gin.H)
	if !ok {
		t.Fatalf("第一个处理器应是 gin.H，得到 %T", processors[0])
	}
	pipeline, _ := first["pipeline"].(gin.H)
	if pipeline["name"] != "logs-general-tomcat-access" {
		t.Fatalf("第一个处理器必须是按名调用基础 pipeline，得到 %v", first)
	}
	// description 提醒别直接改集群上的这份。
	if !strings.Contains(body["description"].(string), "平台生成") {
		t.Fatalf("description 应说明这是平台生成的包装: %v", body["description"])
	}
}

func TestBuildServiceExtraPipelineBodyRejectsNestedPipeline(t *testing.T) {
	if _, err := buildServiceExtraPipelineBody("base", json.RawMessage(`[{"pipeline":{"name":"x"}}]`)); err == nil {
		t.Fatalf("嵌套 pipeline 处理器应被拒绝")
	}
}

func TestRenderPointsServiceToWrapperPipeline(t *testing.T) {
	extra := json.RawMessage(`[{"drop_fields":{"fields":["host.name"]}}]`)
	base := renderHostLogConfig([]hostLogRenderInput{{
		Prefix: "autoadmin", Application: "app-a", Service: "svc-a", ServiceID: 10,
		Instance: "inst-1", Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std",
		Pipeline: "rule-a", LogName: "app.log", ResolvedPath: "/data/app.log",
	}}, []hostInstanceInput{{Service: "svc-a", ServiceID: 10, Instance: "inst-1", HostIP: "10.0.0.1"}}, "")
	wrapper := renderHostLogConfig([]hostLogRenderInput{{
		Prefix: "autoadmin", Application: "app-a", Service: "svc-a", ServiceID: 10,
		Instance: "inst-1", Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std",
		Pipeline: "rule-a", LogName: "app.log", ResolvedPath: "/data/app.log",
		ExtraProcessors: extra,
	}}, []hostInstanceInput{{Service: "svc-a", ServiceID: 10, Instance: "inst-1", HostIP: "10.0.0.1"}}, "")

	if !strings.Contains(base.Fragments[0].Content, "pipeline: 'autoadmin-app-a-rule-a'") {
		t.Fatalf("未启用后处理时应指基础 pipeline:\n%s", base.Fragments[0].Content)
	}
	if !strings.Contains(wrapper.Fragments[0].Content, "pipeline: 'autoadmin-app-a-rule-a@s10'") {
		t.Fatalf("启用后处理时应指包装 pipeline:\n%s", wrapper.Fragments[0].Content)
	}
	// 指向变化必须反映到指纹（否则"配了后处理"不会触发重新下发）。
	if base.Fingerprint == wrapper.Fingerprint {
		t.Fatalf("包装指向变化应改变片段指纹")
	}
}

func TestExtraProcessorsEnabledGuardsGarbage(t *testing.T) {
	if assets.ExtraProcessorsEnabled(nil) || assets.ExtraProcessorsEnabled(json.RawMessage("not-json")) {
		t.Fatalf("nil/坏 JSON 都应视为未启用")
	}
}
