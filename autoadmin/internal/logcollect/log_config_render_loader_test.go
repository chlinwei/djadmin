package logcollect

import (
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 批量装载渲染输入：**两条查询覆盖全部入参主机**，查询次数不随主机数增长。
//
// 这是 500–1000 台规模下的硬约束（计划文档 §2.4）：按主机循环查库会把列表/体检退化成
// N×2 次查询。sqlmock 只登记两条期望，代码若退回逐主机查询就会因为出现未登记的查询而失败，
// 因此本用例同时是 N+1 的回归保护。
//
// 不匹配具体占位符/参数（MySQL 是 `IN (?,?)`、PG 是 `= ANY($1)`），以便两个 tag 下都成立。
func TestLoadHostLogRenderInputsBatchesByHost(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery("FROM assets_application_service_deployment").WillReturnRows(
		sqlmock.NewRows([]string{"host_id", "service_code", "instance_name", "runtime_variables", "app_home", "host_ip"}).
			AddRow(int64(1), "svc-a", "inst-1", []byte(`{"APP_HOME":"/opt/instance"}`), "/data/tpl", "10.0.0.1").
			AddRow(int64(1), "svc-a", "inst-2", []byte(`{}`), "/data/tpl", "10.0.0.1").
			AddRow(int64(2), "svc-b", "inst-3", []byte(`{}`), "", "10.0.0.2"),
	)
	mock.ExpectQuery("FROM assets_application_service s").WillReturnRows(
		sqlmock.NewRows([]string{
			"host_id", "project_code", "environment_code", "business_system_code", "service_code",
			"application_code", "tier_code", "pipeline_name", "log_name", "path_pattern",
			"macro_values", "macro_definitions", "multiline_enabled", "start_pattern", "flush_timeout",
		}).
			AddRow(int64(1), "kul", "test", "tib", "svc-a", "app-a", "hot", "rule-a", "catalina.out",
				"${APP_HOME}/logs/catalina.out", []byte(`{"LOG_DIR":"/svc/logs"}`),
				[]byte(`[{"name":"TEMPLATE_ONLY","value":"/tpl"}]`), true, `\d{4}`, uint32(3000)).
			AddRow(int64(2), "kul", "prod", "tib", "svc-b", "app-b", "std", "rule-b", "app.log",
				"/data/app.log", []byte(`{}`), []byte(`[]`), false, "", uint32(2000)),
	)

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	handler := &Handler{db: database}

	// 请求 3 台主机，但只有 1、2 有数据：host 3 不应出现在结果里。
	sets, err := handler.loadHostLogRenderInputs(context, []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("批量装载失败：%v", err)
	}
	// ExpectationsWereMet 为真 + err 为 nil ⇒ 恰好执行了上面登记的两条查询。
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("查询次数/语句不符（疑似退化成逐主机查询）：%v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("分组结果 = %d 台，want 2（无采集配置的主机不应出现）", len(sets))
	}

	first := sets[1]
	if len(first.Instances) != 2 || len(first.Entries) != 1 {
		t.Fatalf("host 1: instances=%d entries=%d, want 2/1", len(first.Instances), len(first.Entries))
	}
	// 实例宏：运行时变量优先，模板 app_home 作为 APP_HOME 兜底。
	if got := first.Instances[0].Macros["APP_HOME"]; got != "/opt/instance" {
		t.Errorf("实例运行时变量应覆盖模板 app_home，得到 %q", got)
	}
	if got := first.Instances[1].Macros["APP_HOME"]; got != "/data/tpl" {
		t.Errorf("实例未配置时应回落到模板 app_home，得到 %q", got)
	}
	if first.Instances[0].HostIP != "10.0.0.1" || first.Instances[0].Service != "svc-a" {
		t.Errorf("实例维度映射错误：%+v", first.Instances[0])
	}

	entry := first.Entries[0]
	if entry.Project != "kul" || entry.Environment != "test" || entry.BusinessSystem != "tib" ||
		entry.Service != "svc-a" || entry.Application != "app-a" {
		t.Errorf("维度码映射错误（历史上曾把项目/服务写反）：%+v", entry)
	}
	if entry.Tier != "hot" || entry.Pipeline != "rule-a" || entry.LogName != "catalina.out" {
		t.Errorf("档位/规则/日志名映射错误：%+v", entry)
	}
	if entry.FlushTimeout != 3000 || !entry.Multiline || entry.StartPattern != `\d{4}` {
		t.Errorf("多行/超时映射错误：%+v", entry)
	}
	// 宏优先级：模板默认值在前、服务级 macro_values 覆盖同名项。
	if entry.Macros["TEMPLATE_ONLY"] != "/tpl" {
		t.Errorf("模板宏默认值应保留，得到 %+v", entry.Macros)
	}
	if entry.Macros["LOG_DIR"] != "/svc/logs" {
		t.Errorf("服务级宏应生效，得到 %+v", entry.Macros)
	}

	if second := sets[2]; len(second.Instances) != 1 || second.Instances[0].Service != "svc-b" {
		t.Errorf("host 2 分组错误：%+v", second)
	}
}

// 空入参不得产生任何查询（切片为空时 sqlc 会生成 `IN (NULL)`，调用方应先短路）。
func TestLoadHostLogRenderInputsEmptyShortCircuits(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	handler := &Handler{db: database}

	sets, err := handler.loadHostLogRenderInputs(context, nil)
	if err != nil {
		t.Fatalf("空入参应直接返回：%v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("空入参应返回空 map，得到 %d 项", len(sets))
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("空入参不应执行查询：%v", err)
	}
}
