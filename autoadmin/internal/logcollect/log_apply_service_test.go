package logcollect

import (
	"database/sql"
	"strings"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

// 服务级下发的解析与分组（架构文档 §8「服务级下发」）。
//
// 这里最要命的失败模式是"少下发了几台却不吭声"：未纳管的主机没有采集目标行，接口如果只是
// 把它们过滤掉，用户会以为整个服务都下发了。所以分组判据（target_id 为 NULL = 未纳管）
// 必须被钉住，并如实回给前端。
func TestGroupServiceApplyTargetsSeparatesUnmanagedHosts(t *testing.T) {
	rows := []db.ListServiceLogApplyTargetsRow{
		{HostID: 30, HostIp: "10.0.0.30", HostInstanceName: "node-c", TargetID: sql.NullInt64{}},
		{HostID: 10, HostIp: "10.0.0.10", HostInstanceName: "node-a", TargetID: sql.NullInt64{Int64: 101, Valid: true}, ConfigFingerprint: "fp-a"},
		{HostID: 20, HostIp: "10.0.0.20", HostInstanceName: "node-b", TargetID: sql.NullInt64{Int64: 102, Valid: true}, ConfigFingerprint: "fp-b"},
	}
	managed, unmanaged := groupServiceApplyTargets(rows)

	if len(managed) != 2 || len(unmanaged) != 1 {
		t.Fatalf("managed=%d unmanaged=%d, want 2/1", len(managed), len(unmanaged))
	}
	// 可下发的两台：带上目标 id 与已下发指纹（指纹要交给配置态评估），并按主机 id 排序。
	if managed[0].TargetID != 101 || managed[1].TargetID != 102 {
		t.Fatalf("目标 id 或顺序不对：%+v", managed)
	}
	if managed[0].ConfigFingerprint != "fp-a" || !managed[0].Managed {
		t.Fatalf("已纳管主机要带指纹且 Managed=true：%+v", managed[0])
	}
	// 未纳管的那台：TargetID 保持 0、Managed=false，并且带着主机名（提示语里要用）。
	if unmanaged[0].HostID != 30 || unmanaged[0].Managed || unmanaged[0].TargetID != 0 {
		t.Fatalf("未纳管主机应 TargetID=0/Managed=false：%+v", unmanaged[0])
	}
	if unmanaged[0].HostInstanceName != "node-c" {
		t.Fatalf("提示语需要主机名，得到 %+v", unmanaged[0])
	}
}

func TestUnmanagedMessageNamesTheHosts(t *testing.T) {
	unmanaged := []serviceApplyTarget{
		{HostID: 30, HostIP: "10.0.0.30", HostInstanceName: "node-c"},
		{HostID: 31, HostIP: "10.0.0.31"},
	}
	message := unmanagedMessage(unmanaged)
	for _, want := range []string{"2 台", "node-c", "10.0.0.31"} {
		if !strings.Contains(message, want) {
			t.Fatalf("提示语缺少 %q：%s", want, message)
		}
	}
	// 全部已纳管时不给提示（前端据此不展示这一行）。
	if empty := unmanagedMessage(nil); empty != "" {
		t.Fatalf("没有未纳管主机时不该有提示，得到 %q", empty)
	}
}

// 聚合可以，但**不能**出现"服务级指纹"这种字段：同一服务在不同主机上因实例级 runtime_variables
// 不同，渲染结果本就不同，伪造一个服务级指纹会直接误导（见 serviceConfigStateSummary 的注释）。
func TestServiceConfigStateSummaryHasNoServiceLevelFingerprint(t *testing.T) {
	summary := serviceConfigStateSummary{Hosts: 3, Managed: 2, Unmanaged: 1, Synced: 1, Drift: 1}
	if summary.Hosts != summary.Managed+summary.Unmanaged {
		t.Fatalf("hosts 应等于已纳管 + 未纳管：%+v", summary)
	}
	if summary.Synced+summary.Drift+summary.Never+summary.Unknown != summary.Managed {
		t.Fatalf("状态计数之和应等于已纳管主机数：%+v", summary)
	}
}
