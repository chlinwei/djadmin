package logcollect

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func chainHost(id int64, name string, managed, agentOnline bool, runtimeStatus, configState string) serviceApplyTarget {
	return serviceApplyTarget{
		HostID: id, HostIP: "10.0.0.1", HostInstanceName: name,
		Managed: managed, AgentOnline: agentOnline, RuntimeStatus: runtimeStatus, ConfigState: configState,
	}
}

// 采集链路诊断（日志中心 → 日志配置）：这是"查不到日志时看清断在哪一层"的唯一入口，
// 所以每层的判据都要能被单独钉住——尤其"没查过状态"不能被说成"已停止"，
// 否则会把用户引去重启一个本来在跑的服务。
func TestSummarizeServiceChainHosts(t *testing.T) {
	managed := []serviceApplyTarget{
		chainHost(1, "node-a", true, true, "running", LogConfigSynced),
		chainHost(2, "node-b", true, true, "stopped", LogConfigNever),
		chainHost(3, "node-c", true, false, "error", LogConfigDrift),
		// runtime_status 为空 = 从没查过，必须与"已停止"分开计数。
		chainHost(4, "node-d", true, true, "", LogConfigUnknown),
	}
	summary := summarizeServiceChainHosts(managed)
	if summary.Total != 4 || summary.AgentOnline != 3 {
		t.Fatalf("总数/在线数不对：%+v", summary)
	}
	if summary.FilebeatRunning != 1 || summary.FilebeatStopped != 1 || summary.FilebeatError != 1 || summary.FilebeatUnknown != 1 {
		t.Fatalf("Filebeat 四种态要分开计：%+v", summary)
	}
	if summary.ConfigSynced != 1 || summary.ConfigPending != 2 {
		t.Fatalf("配置态：已同步 1、待下发（drift+never）2：%+v", summary)
	}
}

func TestHostLayerAgentTreatsUnmanagedAsItsOwnProblem(t *testing.T) {
	managed := []serviceApplyTarget{chainHost(1, "node-a", true, true, "running", LogConfigSynced)}
	unmanaged := []serviceApplyTarget{chainHost(2, "node-b", false, false, "", "")}

	layer := hostLayer("agent", "Agent 在线", managed, unmanaged)
	// 只有"未纳管"这一种问题时降级为 warn：它不是配置漂移，报 drift 会让人去点下发。
	if layer["status"] != logHealthWarn {
		t.Fatalf("只有未纳管时不该报 drift：%v", layer["status"])
	}
	items := layerItems(t, layer)
	if len(items) != 2 {
		t.Fatalf("未纳管主机也要出现在层里：%+v", items)
	}
	if detail := itemDetail(items[1]); !strings.Contains(detail, "未纳管") {
		t.Fatalf("未纳管的理由要说清：%+v", items[1])
	}
}

func TestHostLayerRuntimeSeparatesUnknownFromStopped(t *testing.T) {
	layer := hostLayer("runtime", "采集进程", []serviceApplyTarget{chainHost(1, "node-a", true, true, "", LogConfigSynced)}, nil)
	items := layerItems(t, layer)
	if items[0]["status"] != logHealthWarn {
		t.Fatalf("空状态是「没查过」，要给 warn 让人去刷新：%+v", items[0])
	}
	detail := itemDetail(items[0])
	if !strings.Contains(detail, "需刷新") || strings.Contains(detail, "已停止") {
		t.Fatalf("不能把「没查过」说成「已停止」：%s", detail)
	}

	stopped := hostLayer("runtime", "采集进程", []serviceApplyTarget{chainHost(2, "node-b", true, true, "stopped", LogConfigSynced)}, nil)
	if itemDetail(layerItems(t, stopped)[0]) == detail {
		t.Fatalf("「已停止」与「没查过」的说明不能是同一句")
	}
}

func TestHostLayerConfigCountsNeverAsPending(t *testing.T) {
	managed := []serviceApplyTarget{
		chainHost(1, "node-a", true, true, "running", LogConfigSynced),
		chainHost(2, "node-b", true, true, "running", LogConfigNever),
		chainHost(3, "node-c", true, true, "running", LogConfigDrift),
	}
	layer := hostLayer("host_configs", "主机配置", managed, nil)
	if layer["status"] != logHealthDrift {
		t.Fatalf("有漂移/从未下发时层状态应为 drift：%v", layer["status"])
	}
	if summary, _ := layer["summary"].(string); !strings.Contains(summary, "1/3") {
		t.Fatalf("摘要要给出「几台里几台好」：%s", summary)
	}
	if items := layerItems(t, layer); len(items) != 3 {
		t.Fatalf("每台主机一项：%+v", items)
	}
}

func TestChainHostLabelFallsBackWhenFieldsAreMissing(t *testing.T) {
	if label := chainHostLabel(serviceApplyTarget{HostID: 7}); !strings.Contains(label, "host-7") {
		t.Fatalf("缺名字和 IP 时要回落到 host id（宁难看也不能空白）：%s", label)
	}
	if label := chainHostLabel(chainHost(1, "node-a", true, true, "", "")); label != "node-a（10.0.0.1）" {
		t.Fatalf("正常情况要给「名字（IP）」：%s", label)
	}
}

// 两种"没有规则"的处置方向不同：模板下没日志定义 = 还没配（warn）；有定义但都没挂规则 = drift
// （按约定这类日志不采集，要人去挂规则）。报反了用户就会去改错地方。
func TestPipelineLayerFromRuleItemsSeparatesNotConfiguredFromNotAttached(t *testing.T) {
	noDefinition := pipelineLayerFromRuleItems(nil, 0)
	if noDefinition["status"] != logHealthWarn {
		t.Fatalf("模板下没有日志定义应为 warn：%v", noDefinition["status"])
	}
	notAttached := pipelineLayerFromRuleItems(nil, 3)
	if notAttached["status"] != logHealthDrift {
		t.Fatalf("有日志定义但都没挂规则应为 drift：%v", notAttached["status"])
	}
	withRules := pipelineLayerFromRuleItems([]gin.H{logHealthItem("logs-x-rule", logHealthOK, "3 个处理器")}, 3)
	if withRules["status"] != logHealthOK {
		t.Fatalf("规则都发布且一致应为 ok：%v", withRules["status"])
	}
}

func layerItems(t *testing.T, layer gin.H) []gin.H {
	t.Helper()
	items, ok := layer["items"].([]gin.H)
	if !ok {
		t.Fatalf("items 不是 []gin.H：%T", layer["items"])
	}
	return items
}

func itemDetail(item gin.H) string {
	detail, _ := item["detail"].(string)
	return detail
}
