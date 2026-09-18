package monitor

import (
	"database/sql"
	"testing"

	"autoadmin/internal/logcollect"
	db "autoadmin/internal/platform/database/generated"
)

// 配置态筛选的语义：只命中已纳管的采集目标；未纳管主机不进任何状态桶（否则
// "待下发"里会混进根本没纳管 Filebeat 的机器）；评估失败的主机归入 unknown。
func TestFilterRowsByConfigState(t *testing.T) {
	rows := []db.ListMonitorHostsRow{
		{ID: 1, LogTargetID: sql.NullInt64{Int64: 11, Valid: true}}, // synced
		{ID: 2, LogTargetID: sql.NullInt64{Int64: 12, Valid: true}}, // drift
		{ID: 3, LogTargetID: sql.NullInt64{Int64: 13, Valid: true}}, // never
		{ID: 4}, // 未纳管
		{ID: 5, LogTargetID: sql.NullInt64{Int64: 15, Valid: true}}, // 评估里缺失 → unknown
	}
	states := map[int64]logcollect.LogConfigState{
		1: {HostID: 1, Status: logcollect.LogConfigSynced},
		2: {HostID: 2, Status: logcollect.LogConfigDrift},
		3: {HostID: 3, Status: logcollect.LogConfigNever},
	}

	cases := []struct {
		want string
		ids  []int64
	}{
		{logcollect.LogConfigSynced, []int64{1}},
		{logcollect.LogConfigDrift, []int64{2}},
		{logcollect.LogConfigNever, []int64{3}},
		{logConfigStateUnknown, []int64{5}},
	}
	for _, testCase := range cases {
		filtered := filterRowsByConfigState(rows, states, testCase.want)
		if len(filtered) != len(testCase.ids) {
			t.Fatalf("%s: 命中 %d 台，want %d", testCase.want, len(filtered), len(testCase.ids))
		}
		for index, hostID := range testCase.ids {
			if filtered[index].ID != hostID {
				t.Errorf("%s: 第 %d 台 = %d, want %d", testCase.want, index, filtered[index].ID, hostID)
			}
		}
	}
}

// 内存分页：越界返回空页而不是 panic（筛选路径下 SQL 不再兜底分页）。
func TestPageRows(t *testing.T) {
	rows := []db.ListMonitorHostsRow{{ID: 1}, {ID: 2}, {ID: 3}}
	cases := []struct {
		page, size int
		ids        []int64
	}{
		{1, 2, []int64{1, 2}},
		{2, 2, []int64{3}},
		{3, 2, nil},
		{1, 10, []int64{1, 2, 3}},
	}
	for _, testCase := range cases {
		page := pageRows(rows, testCase.page, testCase.size)
		if len(page) != len(testCase.ids) {
			t.Fatalf("page=%d size=%d: %d 行，want %d", testCase.page, testCase.size, len(page), len(testCase.ids))
		}
		for index, hostID := range testCase.ids {
			if page[index].ID != hostID {
				t.Errorf("page=%d: 第 %d 行 = %d, want %d", testCase.page, index, page[index].ID, hostID)
			}
		}
	}
}

func TestValidConfigStateFilter(t *testing.T) {
	for _, valid := range []string{"synced", "drift", "never", "unknown"} {
		if !validConfigStateFilter(valid) {
			t.Errorf("%q 应合法", valid)
		}
	}
	for _, invalid := range []string{"", "SYNCED", "pending", "synced,drift"} {
		if validConfigStateFilter(invalid) {
			t.Errorf("%q 应非法", invalid)
		}
	}
}
