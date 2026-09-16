package assets

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 回归测试：取主机标识的 SQL 曾从 4 列（多一个 agent_id）改成 3 列，而 rows.Scan
// 未同步删参，导致 POST /api/agent/install 直接 500：
//
//	sql: expected 3 destination arguments in Scan, not 4
//
// 这类错误编译期不报、只在真实跑到时炸，所以这里锁定「SQL 列数 == Scan 目标数」：
// mock 少给/多给一列都会让本用例失败。
func TestLoadAgentTargetHostsColumnArity(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// 方言无关片段：MySQL 侧是 IN (?,?) 展开成两个参数，PG 侧是数组参数（见 agentListTargetArgs）。
	query := regexp.QuoteMeta(`SELECT id, COALESCE(instance_name, '') AS instance_name, COALESCE(ip, '') AS ip`)
	mock.ExpectQuery(query).WithArgs(agentListTargetArgs(2)...).
		WillReturnRows(sqlmock.NewRows([]string{"id", "instance_name", "ip"}).
			AddRow(221, "localhost", "10.25.66.150").
			AddRow(222, "mysql134", "10.25.66.134"))

	hosts, err := loadAgentTargetHosts(context.Background(), database, []int64{221, 222})
	if err != nil {
		t.Fatalf("load hosts: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts = %d, want 2", len(hosts))
	}
	// host.HostName 已删除：instance_name 同时是展示名与网关会话 key，只有这一个来源。
	if hosts[0].ID != 221 || hosts[0].InstanceName != "localhost" || hosts[0].HostIP != "10.25.66.150" {
		t.Fatalf("unexpected first host: %+v", hosts[0])
	}
	if hosts[1].InstanceName != "mysql134" {
		t.Fatalf("unexpected second host: %+v", hosts[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 非正整数 host_id 必须在发 SQL 前就被拒绝，避免把 0 带进 IN 列表。
func TestLoadAgentTargetHostsRejectsNonPositiveID(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	if _, err := loadAgentTargetHosts(context.Background(), database, []int64{221, 0}); err != ErrInvalid {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发出任何 SQL: %v", err)
	}
}
