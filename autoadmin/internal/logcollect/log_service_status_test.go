package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"

	"autoadmin/internal/assets"

	"github.com/DATA-DOG/go-sqlmock"
)

// 一批服务的配置态评估（日志中心层级视图的"待下发"列）。
//
// 这里要钉住两件事：
//  1. **共享主机上各服务互不牵连**（改 A 不能让 B 变待下发）——逐 (服务 × 主机) 判定，不是整机判定；
//  2. **每台主机只渲染一次**：查询次数不随主机数增长（渲染是最贵的一步，层级视图一次问几十个服务）。
//
// 判据本身（serviceConfigStatus）另有单测，这里测的是归组与取数。

func hostExpected(hostFingerprint string, serviceFingerprints map[string]string) hostExpectedFingerprints {
	return hostExpectedFingerprints{host: hostFingerprint, service: serviceFingerprints}
}

// 同一台主机承载两个服务：只改 B 的子指纹时，A 仍算已同步。
func TestCountServicePendingHostsIsolatesServicesOnSharedHost(t *testing.T) {
	targets := map[int64]serviceLogTargets{
		11: {managed: map[int64]serviceAppliedFingerprints{10: {service: "sub-a", host: "host-fp"}}},
		12: {managed: map[int64]serviceAppliedFingerprints{10: {service: "sub-b-old", host: "host-fp"}}},
	}
	expected := map[int64]hostExpectedFingerprints{
		10: hostExpected("host-fp", map[string]string{"11": "sub-a", "12": "sub-b-new"}),
	}

	counts := countServicePendingHosts(targets, expected)

	if counts[11].Synced != 1 || counts[11].Drift != 0 {
		t.Fatalf("A 的子指纹没变，应仍是已同步：%+v", counts[11])
	}
	if counts[12].Drift != 1 {
		t.Fatalf("B 的配置变了，应记待下发：%+v", counts[12])
	}
	if counts[11].Managed != 1 || counts[11].Unmanaged != 0 || counts[11].Hosts != 1 {
		t.Fatalf("主机计数不对：%+v", counts[11])
	}
}

// 未纳管的主机单列（"下发不到"≠"待下发"），且不参与配置态判定。
func TestCountServicePendingHostsSeparatesUnmanagedHosts(t *testing.T) {
	targets := map[int64]serviceLogTargets{
		11: {
			unmanaged: 2,
			managed:   map[int64]serviceAppliedFingerprints{10: {service: "sub-a", host: "host-fp"}},
		},
	}
	expected := map[int64]hostExpectedFingerprints{
		10: hostExpected("host-fp", map[string]string{"11": "sub-a"}),
	}

	counts := countServicePendingHosts(targets, expected)

	if counts[11].Hosts != 3 || counts[11].Managed != 1 || counts[11].Unmanaged != 2 {
		t.Fatalf("承载 3 台（1 台可下发 + 2 台未纳管）：%+v", counts[11])
	}
	if counts[11].Synced != 1 || counts[11].Drift != 0 || counts[11].Never != 0 {
		t.Fatalf("未纳管的机器不该进任何配置态分桶：%+v", counts[11])
	}
}

// 从未下发过：本服务有期望配置（子指纹非空）但主机上没记录，且整机配置与期望也不一致。
func TestCountServicePendingHostsCountsNeverDelivered(t *testing.T) {
	targets := map[int64]serviceLogTargets{
		11: {
			managed: map[int64]serviceAppliedFingerprints{
				10: {service: "", host: "stale-host-fp"},
				20: {service: "", host: "host-fp"},
			},
		},
	}
	expected := map[int64]hostExpectedFingerprints{
		// 主机 10：期望与已下发的整机指纹不一致 → 从未下发。
		10: hostExpected("host-fp", map[string]string{"11": "sub-a"}),
		// 主机 20：整机指纹一致（存量目标没有服务级记录）→ 按兜底口径算已同步，不误报。
		20: hostExpected("host-fp", map[string]string{"11": "sub-a"}),
	}

	counts := countServicePendingHosts(targets, expected)

	if counts[11].Never != 1 || counts[11].Synced != 1 || counts[11].Drift != 0 {
		t.Fatalf("应 1 台从未下发、1 台按整机指纹兜底为已同步：%+v", counts[11])
	}
}

// 服务在这台主机上没有片段（本服务停采）时，期望与已下发都空 → 已同步，不算待下发。
func TestCountServicePendingHostsTreatsEmptyBothSidesAsSynced(t *testing.T) {
	targets := map[int64]serviceLogTargets{
		11: {managed: map[int64]serviceAppliedFingerprints{10: {service: "", host: "host-fp"}}},
	}
	expected := map[int64]hostExpectedFingerprints{10: hostExpected("host-fp", map[string]string{})}

	counts := countServicePendingHosts(targets, expected)

	if counts[11].Synced != 1 || counts[11].Drift != 0 || counts[11].Never != 0 {
		t.Fatalf("两边都空 = 已一致：%+v", counts[11])
	}
}

// 取数路径：**每个服务一次承载主机查询**，但渲染输入只按主机并集取一次（两条查询），
// 且只渲染一次 —— 这条断言就是"每台主机只渲染一次"的守卫：多渲染一次不会多出查询，
// 但把主机的渲染输入也按服务取的话，下面期望的两条查询会变成 2×服务数。
func TestServiceLogPendingHostsBatchesRenderInputsAcrossServices(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}

	// 两个服务都承载在同一台主机 10 上。
	applyTargetColumns := []string{
		"host_id", "host_ip", "host_instance_name", "target_id", "config_fingerprint", "runtime_status",
		"service_fingerprints", "service_id", "service_code",
	}
	// 片段取得足够独特：sqlmock 是按顺序匹配的，几条查询里都出现 "FROM assets_application_service_deployment sd"。
	applyTargetsQuery := "monitor_log_collection_target l ON l.host_id = d.host_id"
	renderInstancesQuery := "COALESCE(d.runtime_variables, '{}') AS runtime_variables"
	renderEntriesQuery := "rule_definition.id = ld.processing_rule_id"
	mock.ExpectQuery(regexp.QuoteMeta(applyTargetsQuery)).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows(applyTargetColumns).AddRow(
			int64(10), "10.0.0.10", "node-a", sql.NullInt64{Int64: 101, Valid: true}, "host-fp", "running",
			json.RawMessage(`{}`), int64(11), "svc-a",
		))
	mock.ExpectQuery(regexp.QuoteMeta(applyTargetsQuery)).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows(applyTargetColumns).AddRow(
			int64(10), "10.0.0.10", "node-a", sql.NullInt64{Int64: 101, Valid: true}, "host-fp", "running",
			json.RawMessage(`{}`), int64(12), "svc-b",
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM monitor_elasticsearch_cluster")).
		WillReturnRows(sqlmock.NewRows([]string{"hosts", "username", "password", "COALESCE(index_prefix, 'logs')", "verify_tls"}).
			AddRow("http://es:9200", "", "", "autoadmin", false))
	// 主机并集只取一次渲染输入（两条查询覆盖全部主机）。
	mock.ExpectQuery(regexp.QuoteMeta(renderInstancesQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"host_id", "service_id", "service_code", "instance_name", "runtime_variables", "app_home", "host_ip"}))
	mock.ExpectQuery(regexp.QuoteMeta(renderEntriesQuery)).
		WillReturnRows(sqlmock.NewRows([]string{
			"host_id", "project_code", "environment_code", "business_system_code", "service_code", "service_id",
			"application_code", "tier_code", "pipeline_name", "log_name", "path_pattern",
			"retention_tier_id", "collection_enabled", "macro_values", "macro_definitions",
			"multiline_enabled", "start_pattern", "flush_timeout",
			"filter_include_rule_id", "filter_exclude_rule_id", "collection_filter_rule_id", "collection_exclude_filter_rule_id",
		}))

	counts, err := handler.ServiceLogPendingHosts(context.Background(), []int64{11, 12})
	if err != nil {
		t.Fatalf("service log pending hosts: %v", err)
	}
	// 主机上没有已下发的服务级记录、期望子指纹也为空（没有渲染输入）→ 两边都空 = 已一致，
	// 不该因为"整机指纹对不上"就慌报待下发（判据是逐 (服务 × 主机) 的，见 serviceConfigStatus）。
	if counts[11].Synced != 1 || counts[12].Synced != 1 {
		t.Fatalf("空渲染输入下两个服务都应算已同步：%+v", counts)
	}
	if counts[11].Managed != 1 || counts[11].Hosts != 1 || counts[12].Managed != 1 || counts[12].Hosts != 1 {
		t.Fatalf("每个服务各自记一台承载主机：%+v", counts)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("取数路径与预期不符（主机并集应只取一次渲染输入）: %v", err)
	}
}

// 算不出来（没有启用的默认集群）必须报错：界面据此显示"-"，不能把"没算出来"显示成"都已同步"。
func TestServiceLogPendingHostsFailsWhenNoCluster(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}

	mock.ExpectQuery(regexp.QuoteMeta("monitor_log_collection_target l ON l.host_id = d.host_id")).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{
			"host_id", "host_ip", "host_instance_name", "target_id", "config_fingerprint", "runtime_status",
			"service_fingerprints", "service_id", "service_code",
		}).AddRow(
			int64(10), "10.0.0.10", "node-a", sql.NullInt64{Int64: 101, Valid: true}, "host-fp", "running",
			json.RawMessage(`{}`), int64(11), "svc-a",
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM monitor_elasticsearch_cluster")).WillReturnError(sql.ErrNoRows)

	if _, err = handler.ServiceLogPendingHosts(context.Background(), []int64{11}); err == nil {
		t.Fatal("没有可用集群时必须报错，而不是返回空结果")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 空入参：不查库、返回空 map（不报错）。
func TestServiceLogPendingHostsWithNoServices(t *testing.T) {
	handler := &Handler{}
	counts, err := handler.ServiceLogPendingHosts(context.Background(), nil)
	if err != nil {
		t.Fatalf("空入参不该报错：%v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("空入参应返回空 map，得到 %+v", counts)
	}
}

// 计数类型必须能被 assets 侧的汇总消费（接口方向的编译期守卫：实现了 assets 定义的接口）。
var _ assets.ServiceLogPendingEvaluator = (*Handler)(nil)
