package logcollect

import (
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 配置态评估：三种态各自的判定，以及"查询次数与主机数无关"的批量约束。
//
// 期望指纹不在这里重复实现一遍映射，而是先用 AppliedFingerprint="" 跑一次拿到 never + 期望值，
// 再把该值回灌成"已下发指纹"验证 synced；这两次跑通就说明比对口径自洽。
func TestEvaluateLogConfigStates(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	handler := &Handler{db: database}

	expectCluster := func() {
		mock.ExpectQuery("FROM monitor_elasticsearch_cluster").WillReturnRows(
			sqlmock.NewRows([]string{"hosts", "username", "password", "index_prefix", "verify_tls"}).
				AddRow("127.0.0.1:9200", "admin", "encrypted", "autoadmin", false),
		)
	}
	expectRenderInput := func() {
		mock.ExpectQuery("FROM assets_application_service_deployment").WillReturnRows(
			sqlmock.NewRows([]string{"host_id", "service_code", "instance_name", "runtime_variables", "app_home", "host_ip"}).
				AddRow(int64(1), "svc-a", "inst-1", []byte(`{}`), "/data/tpl", "10.0.0.1").
				AddRow(int64(2), "svc-b", "inst-2", []byte(`{}`), "/data/tpl", "10.0.0.2"),
		)
		mock.ExpectQuery("FROM assets_application_service s").WillReturnRows(
			sqlmock.NewRows([]string{
				"host_id", "project_code", "environment_code", "business_system_code", "service_code",
				"application_code", "tier_code", "pipeline_name", "log_name", "path_pattern",
				"macro_values", "macro_definitions", "multiline_enabled", "start_pattern", "flush_timeout",
			}).
				AddRow(int64(1), "kul", "test", "tib", "svc-a", "app-a", "hot", "rule-a", "catalina.out",
					"/data/tpl/logs/catalina.out", []byte(`{}`), []byte(`[]`), false, "", uint32(2000)).
				AddRow(int64(2), "kul", "prod", "tib", "svc-b", "app-b", "std", "rule-b", "app.log",
					"/data/tpl/logs/app.log", []byte(`{}`), []byte(`[]`), false, "", uint32(2000)),
		)
	}

	refs := []LogConfigTargetRef{
		{HostID: 1},                         // 从未下发
		{HostID: 2, AppliedFingerprint: ""}, // 与 1 同批，用来确认分组不串
	}

	// 第一次：两台都是 never，并拿到 host 1 的期望指纹。
	expectCluster()
	expectRenderInput()
	states, err := handler.EvaluateLogConfigStates(context, refs)
	if err != nil {
		t.Fatalf("评估失败：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("查询次数/语句不符（疑似退化成逐主机查询）：%v", err)
	}
	for _, hostID := range []int64{1, 2} {
		state := states[hostID]
		if state.Status != LogConfigNever {
			t.Errorf("host %d 首次评估应为 never，得到 %q", hostID, state.Status)
		}
		if state.ExpectedFingerprint == "" {
			t.Errorf("host %d 期望指纹不应为空", hostID)
		}
		if state.ServiceNum != 1 {
			t.Errorf("host %d service_num = %d, want 1", hostID, state.ServiceNum)
		}
	}
	expected := states[1].ExpectedFingerprint

	// 第二次：指纹一致 → synced；指纹不同 → drift。
	expectCluster()
	expectRenderInput()
	states, err = handler.EvaluateLogConfigStates(context, []LogConfigTargetRef{
		{HostID: 1, AppliedFingerprint: expected},
		{HostID: 2, AppliedFingerprint: "stale-fingerprint"},
	})
	if err != nil {
		t.Fatalf("评估失败：%v", err)
	}
	if got := states[1].Status; got != LogConfigSynced {
		t.Errorf("指纹一致应为 synced，得到 %q", got)
	}
	if got := states[2].Status; got != LogConfigDrift {
		t.Errorf("指纹不一致应为 drift，得到 %q", got)
	}
	if states[1].AppliedFingerprint != expected || states[1].ExpectedFingerprint != expected {
		t.Errorf("synced 态应回显两侧指纹：%+v", states[1])
	}
	// 指纹只差空白也应视为未下发（与下发路径 strings.TrimSpace 的口径一致）。
	expectCluster()
	expectRenderInput()
	states, err = handler.EvaluateLogConfigStates(context, []LogConfigTargetRef{{HostID: 1, AppliedFingerprint: "  "}})
	if err != nil {
		t.Fatalf("评估失败：%v", err)
	}
	if got := states[1].Status; got != LogConfigNever {
		t.Errorf("空白指纹应视为 never，得到 %q", got)
	}
}

// 空入参不查库（调用方在没有已纳管主机时不应产生任何查询）。
func TestEvaluateLogConfigStatesEmptyShortCircuits(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	handler := &Handler{db: database}

	states, err := handler.EvaluateLogConfigStates(context, nil)
	if err != nil {
		t.Fatalf("空入参应直接返回：%v", err)
	}
	if len(states) != 0 {
		t.Fatalf("空入参应返回空 map，得到 %d 项", len(states))
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("空入参不应执行查询：%v", err)
	}
}
