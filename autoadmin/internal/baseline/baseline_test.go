package baseline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// ---- SaveBaseline 输入校验：坏输入必须在触碰数据库之前被拦截。
// 这些用例用 sqlmock 但不设置任何 Expectation——一旦 SQL 泄漏执行会直接失败。

func ginTestContext(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	request.Header.Set("Content-Type", "application/json")
	context.Request = request
	context.Params = gin.Params{{Key: "id", Value: "7"}}
	return context, recorder
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return decoded
}

func TestSaveBaselineRejectsEmptyName(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	handler := &Handler{db: database}
	name := "  "
	context, recorder := ginTestContext("POST", "/sys/security/baseline/", map[string]any{"name": &name})
	handler.SaveBaseline(context)

	// 业务错误走 HTTP 200 + body code=400（与 response.BusinessError 契约一致）。
	if code := decodeResponse(t, recorder)["code"]; code != float64(400) {
		t.Fatalf("code = %v, want 400", code)
	}
	if msg := decodeResponse(t, recorder)["msg"]; msg != "基线名称不能为空" {
		t.Fatalf("msg = %v, want 基线名称不能为空", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no SQL expected for invalid input: %v", err)
	}
}

func TestSaveBaselineRejectsDuplicateItemNames(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	handler := &Handler{db: database}
	context, recorder := ginTestContext("POST", "/sys/security/baseline/", map[string]any{
		"name": "等保2.0",
		"items": []map[string]any{
			{"name": "密码长度", "category_id": 3, "config": opaItemConfig()},
			{"name": " 密码长度 ", "category_id": 3, "config": opaItemConfig()},
		},
	})
	handler.SaveBaseline(context)

	// 业务错误走 HTTP 200 + body code=400（与 response.BusinessError 契约一致）。
	if code := decodeResponse(t, recorder)["code"]; code != float64(400) {
		t.Fatalf("code = %v, want 400", code)
	}
	// TrimSpace 后判重——" 密码长度 " 与 "密码长度" 必须视为重复。
	if msg := decodeResponse(t, recorder)["msg"]; !strings.Contains(fmt.Sprint(msg), "名称不能重复") {
		t.Fatalf("msg = %v, want duplicate-name error", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no SQL expected for invalid input: %v", err)
	}
}

// OPA 条目 config 非法（策略缺 assertions contains）必须在碰库前被拦截。
func TestSaveBaselineRejectsInvalidOpaConfig(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	handler := &Handler{db: database}
	context, recorder := ginTestContext("POST", "/sys/security/baseline/", map[string]any{
		"name": "等保2.0",
		"items": []map[string]any{
			{"name": "ssh协议版本", "category_id": 3, "config": map[string]any{"input_files": []any{}, "policy": "package baseline\n\nviolations contains"}},
		},
	})
	handler.SaveBaseline(context)

	// 业务错误走 HTTP 200 + body code=400（与 response.BusinessError 契约一致）。
	if code := decodeResponse(t, recorder)["code"]; code != float64(400) {
		t.Fatalf("code = %v, want 400", code)
	}
	if !strings.Contains(fmt.Sprint(decodeResponse(t, recorder)["msg"]), "assertions contains") {
		t.Fatalf("want invalid-OPA-policy error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no SQL expected for invalid input: %v", err)
	}
}

// ---- SaveBaseline 落库形状：items 整体覆盖（DELETE 后重建），条目参数顺序钉死。

func TestSaveBaselineInsertShape(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// 类目归属校验（策略提交的 category_id 必须属于该基线）。
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM baseline_category WHERE baseline_id=? AND id IN (?)")).
		WithArgs(int64(7), int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE baseline SET name=?,version=?,description=?,enabled=?,update_time=NOW(6) WHERE id=?")).
		WithArgs("等保2.0主机基线", "v1", "desc", true, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// items 提交即整体重建：先删后插。
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM baseline_item WHERE baseline_id=?")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO baseline_item(create_time,update_time,baseline_id,category_id,sort,name,description,config,severity)")).
		WithArgs(int64(7), int64(3), 0, "密码长度", "desc", opaItemConfigBytes(), "high").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	handler := &Handler{db: database}
	name := "等保2.0主机基线"
	enabled := true
	context, recorder := ginTestContext("PATCH", "/sys/security/baseline/7/", map[string]any{
		"name": &name, "description": "desc", "enabled": &enabled,
		"items": []map[string]any{
			// severity 非法值必须落为 high；这里传了 category_id，钉住参数顺序（曾因占位符错位写串列）。
			{"name": "密码长度", "category_id": 3, "description": "desc",
				"config": map[string]any{
					"input_commands": []any{map[string]any{"key": "k", "exec": "echo x", "parse": "raw"}},
					"policy":         "package baseline\n\nassertions contains a if {\n\ta := {\"name\": \"x\", \"pass\": true}\n}",
				}, "severity": "invalid"},
		},
	})
	// 编辑（path 带 id）→ 走 UPDATE 分支。
	handler.SaveBaseline(context)

	if recorder.Code != 200 {
		t.Fatalf("status = %d body %s, want 200", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 新建基线（path 无 id）时提交策略直接拒绝：类目还没建立，挂不上去。
func TestSaveBaselineRejectsItemsOnCreate(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	handler := &Handler{db: database}
	context, recorder := ginTestContext("POST", "/sys/security/baseline/", map[string]any{
		"name": "等保2.0",
		"items": []map[string]any{
			{"name": "密码长度", "category_id": 3, "config": opaItemConfig()},
		},
	})
	// 新建（path 无 id）→ 走 INSERT 分支。
	context.Params = nil
	handler.SaveBaseline(context)

	if code := decodeResponse(t, recorder)["code"]; code != float64(400) {
		t.Fatalf("code = %v, want 400", code)
	}
	if msg := decodeResponse(t, recorder)["msg"]; !strings.Contains(fmt.Sprint(msg), "请先保存基线") {
		t.Fatalf("msg = %v, want 请先保存基线", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no SQL expected for invalid input: %v", err)
	}
}

// ---- StartScan 输入校验：挂载类型与项目/环境必填在碰库前拦截。

func TestStartScanRejectsUnsupportedMountType(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	handler := &Handler{db: database}
	context, recorder := ginTestContext("POST", "/sys/security/baseline/7/scan/", map[string]any{
		"mount_type": "service", "project_id": 1,
	})
	context.Params = gin.Params{{Key: "id", Value: "7"}}
	handler.StartScan(context)

	// 业务错误走 HTTP 200 + body code=400（与 response.BusinessError 契约一致）。
	if code := decodeResponse(t, recorder)["code"]; code != float64(400) {
		t.Fatalf("code = %v, want 400", code)
	}
	// OS 基线只扫主机：service 挂载必须被拒绝。
	if msg := decodeResponse(t, recorder)["msg"]; !strings.Contains(fmt.Sprint(msg), "只支持按项目") {
		t.Fatalf("msg = %v, want mount-type error", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no SQL expected for invalid input: %v", err)
	}
}

func TestStartScanRejectsEnvironmentWithoutEnvironmentID(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	handler := &Handler{db: database}
	context, recorder := ginTestContext("POST", "/sys/security/baseline/7/scan/", map[string]any{
		"mount_type": "environment", "project_id": 1,
	})
	context.Params = gin.Params{{Key: "id", Value: "7"}}
	handler.StartScan(context)

	// 业务错误走 HTTP 200 + body code=400（与 response.BusinessError 契约一致）。
	if code := decodeResponse(t, recorder)["code"]; code != float64(400) {
		t.Fatalf("code = %v, want 400", code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no SQL expected for invalid input: %v", err)
	}
}

// ---- flushResults：批量 INSERT 形状与分批边界。

func TestFlushResultsInsertShape(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	results := make([]gin.H, 0, 3)
	for i := 0; i < 3; i++ {
		results = append(results, map[string]any{
			"item_id": i + 1, "item_name": fmt.Sprintf("item-%d", i+1), "chapter": "身份鉴别",
			"severity": "high", "status": "pass", "message": "ok", "actual": "x",
		})
	}

	// 11 列/行 × 3 行 = 33 占位符；expected_value 落 JSON（nil → null）。
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO baseline_scan_result(scan_id,host_id,item_id,item_name,chapter,severity,status,expected_value,actual_value,message,remediation) VALUES (?,?,?,?,?,?,?,?,?,?,?),(?,?,?,?,?,?,?,?,?,?,?),(?,?,?,?,?,?,?,?,?,?,?)")).
		WithArgs(int64(5), int64(9), 1, "item-1", "身份鉴别", "high", "pass", []byte("null"), []byte(`"x"`), "ok", nil,
			int64(5), int64(9), 2, "item-2", "身份鉴别", "high", "pass", []byte("null"), []byte(`"x"`), "ok", nil,
			int64(5), int64(9), 3, "item-3", "身份鉴别", "high", "pass", []byte("null"), []byte(`"x"`), "ok", nil).
		WillReturnResult(sqlmock.NewResult(0, 3))

	flushResults(context.Background(), database, 5, 9, results)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestFlushResultsBatchesOverHundred(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	results := make([]gin.H, 0, 150)
	for i := 0; i < 150; i++ {
		results = append(results, map[string]any{"item_id": i + 1, "status": "fail"})
	}

	// 100 行/批：150 条 → 100 + 50 两批（两条 INSERT、列数由形状用例钉住）。
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO baseline_scan_result")).
		WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO baseline_scan_result")).
		WillReturnResult(sqlmock.NewResult(0, 50))

	flushResults(context.Background(), database, 5, 9, results)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// ---- 主机上下文变量：OS 基线只允许 HOST_IP/HOST_NAME，应用变量必须保持字面量。

func TestExpandHostVarsOnlyExpandsHostContext(t *testing.T) {
	host := scanHost{HostID: 1, HostName: "node-1", HostIP: "10.0.0.5"}
	context := hostContext(host)

	got := expandHostVars("sshd on ${HOST_IP}, host ${HOST_NAME}", context)
	if got != "sshd on 10.0.0.5, host node-1" {
		t.Fatalf("expanded = %q", got)
	}

	// 回归锚点：应用变量（${APP_XXX}）不是主机上下文的一部分，
	// 误展开会导致 OS 基线项静默变成错误值——必须保持字面量。
	appVar := "path ${APP_INSTALL_DIR} on ${HOST_IP}"
	if got := expandHostVars(appVar, context); got != "path ${APP_INSTALL_DIR} on 10.0.0.5" {
		t.Fatalf("app var leaked: %q", got)
	}
}

func TestHostContextOnlyContainsHostVars(t *testing.T) {
	context := hostContext(scanHost{HostName: "node-1", HostIP: "10.0.0.5"})
	if len(context) != 2 {
		t.Fatalf("context keys = %d, want 2", len(context))
	}
	for _, key := range []string{"HOST_IP", "HOST_NAME"} {
		if _, ok := context[key]; !ok {
			t.Fatalf("missing host var %q", key)
		}
	}
}

func TestNullInt64FromPtr(t *testing.T) {
	if value := nullInt64FromPtr(nil); value.Valid {
		t.Fatalf("nil should be invalid")
	}
	zero := int64(0)
	if value := nullInt64FromPtr(&zero); value.Valid {
		t.Fatalf("zero should be invalid")
	}
	five := int64(5)
	if value := nullInt64FromPtr(&five); !value.Valid || value.Int64 != 5 {
		t.Fatalf("5 should be valid, got %+v", value)
	}
}

// ---- finishScan：终态聚合语义（任一 failed → failed；全 skipped → skipped）。

func TestFinishScanAggregatesSummary(t *testing.T) {
	cases := []struct {
		name                 string
		success, failed, skip int
		want                 string
	}{
		{"any failed wins", 3, 1, 2, "failed"},
		{"all success", 4, 0, 0, "success"},
		{"all skipped", 0, 0, 4, "skipped"},
		{"no targets at all", 0, 0, 0, "failed"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("create sql mock: %v", err)
			}
			defer database.Close()

			mock.ExpectQuery(regexp.QuoteMeta("SELECT SUM(status='success'),SUM(status='failed'),SUM(status='skipped') FROM security_scan_target WHERE scan_id=?")).
				WithArgs(int64(5)).
				WillReturnRows(sqlmock.NewRows([]string{"s", "f", "k"}).AddRow(testCase.success, testCase.failed, testCase.skip))
			mock.ExpectExec(regexp.QuoteMeta("UPDATE security_scan SET status=?,summary=?,end_time=NOW(6),update_time=NOW(6) WHERE id=?")).
				WithArgs(testCase.want, sqlmock.AnyArg(), int64(5)).
				WillReturnResult(sqlmock.NewResult(0, 1))

			(&Handler{db: database}).finishScan(context.Background(), 5, 4)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("database expectations: %v", err)
			}
		})
	}
}

var _ = http.StatusOK // keep net/http import tied to httptest helpers

// opaItemConfig 构造一条合法的 OPA 条目 config。
func opaItemConfig() map[string]any {
	return map[string]any{
		"input_commands": []any{map[string]any{"key": "k", "exec": "echo x", "parse": "raw"}},
		"policy": `package baseline

assertions contains a if {
	a := {"name": "x", "pass": true}
}`,
	}
}

// opaItemConfigBytes opaItemConfig 的 JSON 序列化（落库的 config 列形状）。
func opaItemConfigBytes() []byte {
	raw, _ := json.Marshal(opaItemConfig())
	return raw
}

// ---- 策略单条 CRUD（弹窗确认即落库）：校验顺序 + 落库形状 ----

func itemTestContext(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	context, recorder := ginTestContext(method, path, body)
	return context, recorder
}

func TestAddBaselineItemInsertShape(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// 校验链：类目归属 → 类目内最大 sort → 插入。
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM baseline_category WHERE id=? AND baseline_id=?")).
		WithArgs(int64(3), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT CAST(COALESCE(MAX(sort), -1) AS SIGNED) AS max_sort FROM baseline_item WHERE category_id = ?")).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"max_sort"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO baseline_item(create_time,update_time,baseline_id,category_id,sort,name,description,config,severity)")).
		WithArgs(int64(7), int64(3), int64(1), "密码长度", "desc", opaItemConfigBytes(), "high").
		WillReturnResult(sqlmock.NewResult(88, 1))

	handler := &Handler{db: database}
	context, recorder := itemTestContext("POST", "/sys/security/baseline/7/items/", map[string]any{
		"name": "密码长度", "category_id": 3, "description": "desc",
		"config": opaItemConfig(), "severity": "invalid", // 非法值必须归一为 high
	})
	handler.AddBaselineItem(context)

	if code := decodeResponse(t, recorder)["code"]; code != float64(200) {
		t.Fatalf("code = %v body %s, want success", code, recorder.Body.String())
	}
	if id := decodeResponse(t, recorder)["data"].(map[string]any)["id"]; id != float64(88) {
		t.Fatalf("id = %v, want 88", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestAddBaselineItemRejectsForeignCategory(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	// 类目不属于该基线 → 400，且不得触碰 INSERT。
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM baseline_category WHERE id=? AND baseline_id=?")).
		WithArgs(int64(3), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	handler := &Handler{db: database}
	context, recorder := itemTestContext("POST", "/sys/security/baseline/7/items/", map[string]any{
		"name": "密码长度", "category_id": 3, "config": opaItemConfig(),
	})
	handler.AddBaselineItem(context)

	if code := decodeResponse(t, recorder)["code"]; code != float64(400) {
		t.Fatalf("code = %v, want 400", code)
	}
	if msg := decodeResponse(t, recorder)["msg"]; !strings.Contains(fmt.Sprint(msg), "类目") {
		t.Fatalf("msg = %v, want category error", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestDeleteBaselineItem(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM baseline_item WHERE id=? AND baseline_id=?")).
		WithArgs(int64(88), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	handler := &Handler{db: database}
	context, recorder := itemTestContext("DELETE", "/sys/security/baseline/7/items/88/", nil)
	context.Params = gin.Params{{Key: "id", Value: "7"}, {Key: "itemId", Value: "88"}}
	handler.DeleteBaselineItem(context)

	if code := decodeResponse(t, recorder)["code"]; code != float64(200) {
		t.Fatalf("code = %v body %s, want success", code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
