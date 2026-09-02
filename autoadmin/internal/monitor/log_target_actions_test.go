package monitor

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// Fluent Bit 包匹配回归用例：按 agent 采集的系统信息（os_type/os_id_like/os_version_id）
// 选择 deb/rpm 与平台目录（rhel7/rhel9/ubuntu），选错包会导致离线安装直接失败。
func TestHostPackageFormat(t *testing.T) {
	cases := map[logTargetRow]string{
		{OSType: "Ubuntu", OSIDLike: "debian"}:     "deb",
		{OSType: "Debian GNU/Linux", OSIDLike: ""}: "deb",
		{OSType: "CentOS Linux", OSIDLike: "rhel"}: "rpm",
		{OSType: "Red Hat Enterprise Linux"}:       "rpm",
		{OSType: "Kylin Linux Advanced Server"}:    "rpm",
		{OSType: "", OSIDLike: ""}:                 "rpm",
	}
	for row, want := range cases {
		if got := hostPackageFormat(row); got != want {
			t.Fatalf("hostPackageFormat(%v) = %s, want %s", row, got, want)
		}
	}
}

func TestPackageFamilyMatches(t *testing.T) {
	if !packageFamilyMatches("ubuntu", "ubuntu debian") {
		t.Fatal("ubuntu host should match ubuntu package")
	}
	if !packageFamilyMatches("rhel", "red hat enterprise linux") {
		t.Fatal("RHEL host should match rhel package")
	}
	if packageFamilyMatches("ubuntu", "red hat enterprise linux rhel fedora") {
		t.Fatal("RHEL host must not match ubuntu package")
	}
	if !packageFamilyMatches("any", "whatever") {
		t.Fatal("family=any matches everything")
	}
}

func TestOSMajor(t *testing.T) {
	if got := osMajor(logTargetRow{OSVersionID: "9.4"}); got != "9" {
		t.Fatalf("osMajor(9.4) = %s, want 9", got)
	}
	if got := osMajor(logTargetRow{OSVersionID: "22.04"}); got != "22" {
		t.Fatalf("osMajor(22.04) = %s, want 22", got)
	}
	if got := osMajor(logTargetRow{}); got != "" {
		t.Fatalf("osMajor(empty) = %s, want empty", got)
	}
}

func TestFirstOpenSearchEndpoint(t *testing.T) {
	host, port, err := firstOpenSearchEndpoint("https://10.0.0.1:9200,https://10.0.0.2:9200")
	if err != nil || host != "10.0.0.1" || port != "9200" {
		t.Fatalf("firstOpenSearchEndpoint = %s:%s, %v", host, port, err)
	}
	host, port, err = firstOpenSearchEndpoint("10.0.0.3")
	if err != nil || host != "10.0.0.3" || port != "9200" {
		t.Fatalf("default port = %s:%s, %v", host, port, err)
	}
	if _, _, err := firstOpenSearchEndpoint(""); err == nil {
		t.Fatal("empty hosts should fail")
	}
}

func newLogTargetTestServer(t *testing.T, database *sql.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.POST("/log-targets/:id/retry/", handler.RetryLogTarget)
	return engine
}

// 派发守卫回归：目标不存在必须 404；agent 离线必须 400，
// 不允许像旧 exporter 安装链路那样把失败静默成成功。
func TestRetryLogTargetGuards(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	selectQuery := regexp.QuoteMeta(`SELECT l.id,l.host_id,l.managed_enabled,l.install_status,
		COALESCE(h.instance_name,''),COALESCE(h.ip,''),COALESCE(h.agent_id,''),
		COALESCE(s.os_type,''),COALESCE(s.os_id_like,''),COALESCE(s.os_version_id,'')
		FROM monitor_log_collection_target l
		JOIN assets_host h ON h.id=l.host_id
		LEFT JOIN assets_hostsystem s ON s.host_id=l.host_id
		WHERE l.id=?`)

	gin.SetMode(gin.TestMode)

	// 404：目标不存在
	engine := newLogTargetTestServer(t, database)
	mock.ExpectQuery(selectQuery).WithArgs(int64(999)).WillReturnError(sql.ErrNoRows)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/log-targets/999/retry/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":404`) {
		t.Fatalf("missing target: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	// 400：agent 离线（gateway 为 nil 视同离线）
	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "host_id", "managed_enabled", "install_status", "instance_name", "ip", "agent_id", "os_type", "os_id_like", "os_version_id"}).
			AddRow(3, 221, true, "success", "localhost", "10.25.66.150", "localhost", "Ubuntu", "debian", "22.04"))
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/log-targets/3/retry/", nil))
	if !strings.Contains(recorder.Body.String(), "host agent is offline") {
		t.Fatalf("offline guard: body=%s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
