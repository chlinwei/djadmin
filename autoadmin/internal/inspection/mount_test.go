package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ---- taskGroupItem / decodeTaskGroups：曾因 DTO 缺 service_id 字段导致接口
// 响应丢键，前端树形归属与"目标"展示全部失效。序列化断言钉住字段必须透出。

func TestDecodeTaskGroupsKeepsServiceMountFields(t *testing.T) {
	raw := []byte(`[{"id":34,"name":"artemis check","scope":"per_deployment","category":"application",
		"mount_type":"service","project_id":null,"environment_id":null,
		"business_system_id":null,"service_id":10,"instance_mode":"all"}]`)

	groups := decodeTaskGroups(raw)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	item := groups[0]
	if item.MountType != mountService || item.ServiceID == nil || *item.ServiceID != 10 || item.InstanceMode != "all" {
		t.Fatalf("decoded group = %+v, want service mount with service_id=10", item)
	}
	// 序列化后 service_id 必须仍在响应里（DTO 缺字段的回归锚点）。
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"service_id":10`) {
		t.Fatalf("serialized group lost service_id: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"mount_type":"service"`) {
		t.Fatalf("serialized group lost mount_type: %s", encoded)
	}
}

func TestDecodeTaskGroupsKeepsMissingMountType(t *testing.T) {
	// mount_type 缺失不再默认 legacy（legacy 已删除）；原值透出由校验拦截。
	groups := decodeTaskGroups([]byte(`[{"id":33,"name":"tomcat check","category":"general"}]`))
	if len(groups) != 1 || groups[0].MountType != "" {
		t.Fatalf("missing mount_type should stay empty, got %+v", groups)
	}
}

// ---- mergeTaskInput：bindings 字段曾经不存在于 taskInput，前端提交被静默丢弃。

func TestMergeTaskInputPrefersBindingsOverGroupsAndGroup(t *testing.T) {
	bindingGroup := int64(7)
	groupsPayload := json.RawMessage(`[8]`)
	state := taskState{}
	mergeTaskInput(&state, taskInput{
		Group:  ptrInt64(9),
		Groups: &groupsPayload,
		Bindings: &[]groupBindingInput{{
			Group: &bindingGroup, MountType: mountService, ServiceID: ptrInt64(10), InstanceMode: instanceAll,
		}},
	})
	if len(state.Bindings) != 1 || state.Bindings[0].MountType != mountService || *state.Bindings[0].ServiceID != 10 {
		t.Fatalf("bindings should win and keep mount payload: %+v", state.Bindings)
	}
	if len(state.GroupIDs) != 1 || state.GroupIDs[0] != 7 {
		t.Fatalf("GroupIDs = %v, want [7]", state.GroupIDs)
	}
}

func TestMergeTaskInputGroupsObjectArray(t *testing.T) {
	groupsPayload := json.RawMessage(`[{"group_id":8,"mount_type":"business","business_system_id":2,"instance_mode":"once"}]`)
	state := taskState{}
	mergeTaskInput(&state, taskInput{Groups: &groupsPayload})
	if len(state.Bindings) != 1 || state.Bindings[0].MountType != mountBusiness || *state.Bindings[0].BusinessSystemID != 2 {
		t.Fatalf("object-array groups should parse to business mount: %+v", state.Bindings)
	}
	if state.Bindings[0].InstanceMode != instanceOnce {
		t.Fatalf("instance_mode = %q, want once", state.Bindings[0].InstanceMode)
	}
}

func TestMergeTaskInputGroupsIDArrayFallsBackToLegacy(t *testing.T) {
	groupsPayload := json.RawMessage(`[8,9]`)
	state := taskState{}
	mergeTaskInput(&state, taskInput{Groups: &groupsPayload})
	// ID 数组没有挂载信息，挂载模型下直接丢弃（不再是 legacy 静态范围）。
	if len(state.Bindings) != 0 {
		t.Fatalf("bare id-array groups should be dropped, got %+v", state.Bindings)
	}
}

// ---- mountTargetName：挂载点任务没有 logical_service_id，目标名按挂载点生成；
// 曾导致执行记录"目标"列为空。

func TestMountTargetNameByBinding(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	ctx := context.Background()

	t.Run("service", func(t *testing.T) {
		serviceID := int64(10)
		bindings := []mountBinding{{MountType: mountService, ServiceID: nullInt64Of(serviceID)}}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT name FROM assets_application_service WHERE id=?`)).
			WithArgs(serviceID).
			WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("artemis"))
		// 业务链查询：目标名需带 项目/业务/环境 前缀。
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT s.id AS service_id, b.id AS business_system_id`)).
			WithArgs(serviceID).
			WillReturnRows(sqlmock.NewRows([]string{"service_id", "business_system_id", "business_system_name", "business_system_owner", "project_id", "project_name", "project_owner", "environment_id", "environment_name"}).
				AddRow(10, 7, "cdm", "", 1, "kul", "", 1, "test"))
		if got := handler.mountTargetName(ctx, bindings); got != "项目 kul · 业务 cdm @ test · 逻辑服务 artemis" {
			t.Fatalf("service mount name = %q", got)
		}
	})

	t.Run("business with environment", func(t *testing.T) {
		businessID, environmentID := int64(7), int64(1)
		bindings := []mountBinding{{MountType: mountBusiness, BusinessSystemID: nullInt64Of(businessID), EnvironmentID: nullInt64Of(environmentID)}}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT b.name, COALESCE(p.name,'') FROM assets_business_system b LEFT JOIN assets_project p ON p.id=b.project_id WHERE b.id=?`)).
			WithArgs(businessID).
			WillReturnRows(sqlmock.NewRows([]string{"name", "project"}).AddRow("cdm", "kul"))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT name FROM assets_business_environment WHERE id=?`)).
			WithArgs(environmentID).
			WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("test"))
		if got := handler.mountTargetName(ctx, bindings); got != "项目 kul · 业务 cdm @ test" {
			t.Fatalf("business mount name = %q", got)
		}
	})

	t.Run("project", func(t *testing.T) {
		projectID := int64(1)
		bindings := []mountBinding{{MountType: mountProject, ProjectID: nullInt64Of(projectID)}}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT name FROM assets_project WHERE id=?`)).
			WithArgs(projectID).
			WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("kul"))
		if got := handler.mountTargetName(ctx, bindings); got != "项目 kul" {
			t.Fatalf("project mount name = %q", got)
		}
	})
}

func nullInt64Of(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}

func ptrInt64(v int64) *int64 { return &v }

// 检查参数（实参）在展开中优先于内置变量；未赋值的变量保持字面量。
func TestResolveVariablesParamsTakePrecedence(t *testing.T) {
	target := runTarget{
		InstanceName: "artemis-1", HostIP: "10.0.0.5",
		AppHome: "/app/oracle", ServiceName: "oracle-svc",
	}
	params := map[string]string{
		"ORACLE_SID": "orcl1",
		"RUN_USER":   "monitor", // 参数覆盖内置同名变量
	}
	value := resolveVariables("pmon_${ORACLE_SID} user_${RUN_USER} home_${APP_HOME} unknown_${NOPE}", target, params)
	if !strings.Contains(value, "pmon_orcl1") {
		t.Fatalf("param ORACLE_SID not expanded: %s", value)
	}
	if !strings.Contains(value, "user_monitor") {
		t.Fatalf("param RUN_USER should override builtin: %s", value)
	}
	if !strings.Contains(value, "home_/app/oracle") {
		t.Fatalf("builtin variable broken: %s", value)
	}
	if strings.Contains(value, "${NOPE}") == false {
		t.Fatalf("unknown variable should stay literal: %s", value)
	}
}

// resolveCheckParams：引用按实例从内置变量/服务宏展开；缺参进 missing。
func TestResolveCheckParamsLiteralsRefsAndMissing(t *testing.T) {
	serviceID := int64(9)
	bindings := []mountBinding{{
		MountType:   mountService,
		ServiceID:   nullInt64Of(serviceID),
		Params:      json.RawMessage(`[{"name":"ORACLE_SID","required":true},{"name":"RUN_MODE","required":true},{"name":"THRESHOLD","default":"85"},{"name":"OPT","required":false}]`),
		ParamValues: json.RawMessage(`{"ORACLE_SID":{"ref":"ORACLE_SID"},"RUN_MODE":{"ref":"INSTANCE_NAME"},"THRESHOLD":{"value":"60"}}`),
	}}
	target := runTarget{
		InstanceName: "artemis-1", AppHome: "/app", RunUser: "oracle",
		Macros: map[string]string{"ORACLE_SID": "orcl1"},
	}
	params, missing := resolveCheckParams(bindings, target)
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	if params["ORACLE_SID"] != "orcl1" {
		t.Fatalf("macro ref = %q, want orcl1", params["ORACLE_SID"])
	}
	if params["RUN_MODE"] != "artemis-1" {
		t.Fatalf("standard ref = %q, want artemis-1", params["RUN_MODE"])
	}
	if params["THRESHOLD"] != "60" {
		t.Fatalf("literal = %q, want 60", params["THRESHOLD"])
	}
	if _, hasOpt := params["OPT"]; hasOpt {
		t.Fatal("optional param without assignment should be absent")
	}

	// 宏不存在 → missing
	target.Macros = map[string]string{}
	_, missing = resolveCheckParams(bindings, target)
	if len(missing) != 1 || !strings.Contains(missing[0], "ORACLE_SID") {
		t.Fatalf("missing = %v, want ORACLE_SID", missing)
	}
}

// 资产侧宏 Key 可能存成 ${ORACLE_SID}（模板渲染形式），读取时归一为裸名。
func TestMacrosFromRawNormalizesKey(t *testing.T) {
	macros := macrosFromRaw(json.RawMessage(`{"${ORACLE_SID}": "ACDM", "${test}": "test1", "BARE": "v"}`))
	if macros["ORACLE_SID"] != "ACDM" || macros["test"] != "test1" || macros["BARE"] != "v" {
		t.Fatalf("macros = %v", macros)
	}
}
