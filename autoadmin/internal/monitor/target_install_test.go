package monitor

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 平台归一回归：与 Django monitor/package_selector.normalize_host_platform 契约一致，
// 选错平台键会导致离线安装包匹配失败或跨发行版误装。
func TestNormalizeExporterPlatform(t *testing.T) {
	cases := []struct {
		row            targetInstallRow
		family         string
		major          string
		arch           string
	}{
		{targetInstallRow{OSID: "centos", OSVersionID: "9.4", Architecture: "x86_64"}, "rhel", "9", "amd64"},
		{targetInstallRow{OSID: "kylin", OSIDLike: "rhel fedora", OSVersionID: "V10", Architecture: "x86_64"}, "rhel", "V10", "amd64"},
		{targetInstallRow{OSID: "ubuntu", OSVersionID: "22.04", Architecture: "x86_64"}, "ubuntu", "22", "amd64"},
		{targetInstallRow{OSID: "deepin", OSIDLike: "debian", Architecture: "aarch64"}, "debian", "", "arm64"},
		{targetInstallRow{OSID: "windows", Architecture: "x86_64"}, "", "", "amd64"},
		{targetInstallRow{OSID: "ubuntu", Architecture: ""}, "ubuntu", "", ""},
	}
	for index, item := range cases {
		family, major, arch := normalizeExporterPlatform(item.row)
		if family != item.family || major != item.major || arch != item.arch {
			t.Fatalf("case %d: normalizeExporterPlatform(%v) = %s/%s/%s, want %s/%s/%s",
				index, item.row, family, major, arch, item.family, item.major, item.arch)
		}
	}
}

func TestMonitorTargetPending(t *testing.T) {
	query := regexp.QuoteMeta(`SELECT id,status,create_time FROM monitor_target_install_history WHERE target_id=? ORDER BY id DESC LIMIT 1`)
	expireUpdate := regexp.QuoteMeta(`UPDATE monitor_target_install_history SET status='failed',error_message_snapshot='任务执行超时（进程中断遗留），已自动过期',update_time=? WHERE id=? AND status IN ('pending','running')`)

	gin.SetMode(gin.TestMode)
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())

	// 无历史：不阻塞
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	mock.ExpectQuery(query).WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
	pending, historyID, err := monitorTargetPending(ginContext, database, 7)
	if err != nil || pending || historyID != 0 {
		t.Fatalf("no history: pending=%v historyID=%d err=%v", pending, historyID, err)
	}
	database.Close()

	// 最近一次已结束：不阻塞
	database, mock, err = sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	mock.ExpectQuery(query).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "create_time"}).AddRow(11, "failed", time.Now().Add(-time.Hour)))
	if pending, _, err = monitorTargetPending(ginContext, database, 7); err != nil || pending {
		t.Fatalf("finished history: pending=%v err=%v", pending, err)
	}
	database.Close()

	// 新鲜 pending：阻塞
	database, mock, err = sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	mock.ExpectQuery(query).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "create_time"}).AddRow(11, "pending", time.Now().Add(-2*time.Minute)))
	if pending, _, err = monitorTargetPending(ginContext, database, 7); err != nil || !pending {
		t.Fatalf("fresh pending: pending=%v err=%v", pending, err)
	}
	database.Close()

	// 超过 31 分钟的 pending：进程中断遗留，自动过期后放行
	database, mock, err = sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	mock.ExpectQuery(query).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "create_time"}).AddRow(11, "pending", time.Now().Add(-45*time.Minute)))
	mock.ExpectExec(expireUpdate).WithArgs(sqlmock.AnyArg(), int64(11)).WillReturnResult(sqlmock.NewResult(0, 1))
	if pending, _, err = monitorTargetPending(ginContext, database, 7); err != nil || pending {
		t.Fatalf("stale pending: pending=%v err=%v", pending, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
	database.Close()
}

// 派发前置守卫回归：无包/架构缺失/平台不匹配/无 playbook 必须把原因写入
// target.install_message（Django 行为），而不是静默成功或直接报错。
func TestPrepareExporterDispatchGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	row := targetInstallRow{ID: 5, HostID: 221, ManagedEnabled: true, ExporterType: "node_exporter",
		HostName: "localhost", HostIP: "10.25.66.150", AgentID: "localhost",
		OSID: "centos", OSVersionID: "9.4", Architecture: "x86_64"}
	failedUpdate := regexp.QuoteMeta(`UPDATE monitor_target SET install_status=?,install_message=?,update_time=? WHERE id=?`)

	runGuard := func(t *testing.T, setup func(mock sqlmock.Sqlmock)) (string, string) {
		t.Helper()
		database, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("create sql mock: %v", err)
		}
		defer database.Close()
		handler := &Handler{db: database}
		setup(mock)
		_, _, _, err = handler.prepareExporterDispatch(ginContext, row, "install")
		var status, message string
		if err == nil {
			t.Fatal("expected guard failure, got nil")
		}
		business, ok := err.(guardFailure)
		if !ok {
			t.Fatalf("expected guardFailure, got %v", err)
		}
		status, message = business.status, business.message
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
		return status, message
	}

	// 仓库无启用包
	_, message := runGuard(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,file,sha256,
		service_file_content,service_run_as_user,service_run_as_group,work_directory,install_playbook_template_id
		FROM monitor_software_package
		WHERE package_type='exporter' AND name=? AND enabled=TRUE AND file<>''
		ORDER BY create_time DESC`)).
			WithArgs("node_exporter").
			WillReturnRows(sqlmock.NewRows([]string{"version", "arch", "platform_family", "platform_major", "package_format", "file", "sha256", "service_file_content", "service_run_as_user", "service_run_as_group", "work_directory", "install_playbook_template_id"}))
		mock.ExpectExec(failedUpdate).WithArgs("failed", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	})
	if !strings.Contains(message, "本地软件仓库缺少 node_exporter") {
		t.Fatalf("no package message: %s", message)
	}


	// 平台不匹配（仅 ubuntu deb 包，主机是 rhel9）
	_, message = runGuard(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,file,sha256,
		service_file_content,service_run_as_user,service_run_as_group,work_directory,install_playbook_template_id
		FROM monitor_software_package
		WHERE package_type='exporter' AND name=? AND enabled=TRUE AND file<>''
		ORDER BY create_time DESC`)).
			WithArgs("node_exporter").
			WillReturnRows(sqlmock.NewRows([]string{"version", "arch", "platform_family", "platform_major", "package_format", "file", "sha256", "service_file_content", "service_run_as_user", "service_run_as_group", "work_directory", "install_playbook_template_id"}).
				AddRow("1.7.0", "amd64", "ubuntu", "22", "deb", "monitor_packages/node_exporter/x.deb", "abc", "", "", "", "/tmp", sql.NullInt64{Int64: 3, Valid: true}))
		mock.ExpectExec(failedUpdate).WithArgs("failed", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	})
	if !strings.Contains(message, "rhel-9/amd64") {
		t.Fatalf("platform mismatch message: %s", message)
	}

	// 命中包但未配置安装 playbook
	_, message = runGuard(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,file,sha256,
		service_file_content,service_run_as_user,service_run_as_group,work_directory,install_playbook_template_id
		FROM monitor_software_package
		WHERE package_type='exporter' AND name=? AND enabled=TRUE AND file<>''
		ORDER BY create_time DESC`)).
			WithArgs("node_exporter").
			WillReturnRows(sqlmock.NewRows([]string{"version", "arch", "platform_family", "platform_major", "package_format", "file", "sha256", "service_file_content", "service_run_as_user", "service_run_as_group", "work_directory", "install_playbook_template_id"}).
				AddRow("1.7.0", "amd64", "rhel", "9", "rpm", "monitor_packages/node_exporter/x.rpm", "abc", "", "", "", "/tmp", sql.NullInt64{}))
		mock.ExpectExec(failedUpdate).WithArgs("failed", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	})
	if !strings.Contains(message, "未配置安装 Playbook") {
		t.Fatalf("playbook missing message: %s", message)
	}
}

// 命中 rpm 包 + 本地文件存在 + 校验和清单齐备时，extra_vars 必须与 Django 派发字段一致。
func TestPrepareExporterDispatchInstallExtraVars(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())

	packageRoot := t.TempDir()
	relative := "monitor_packages/node_exporter/linux-amd64/node_exporter-1.7.0.linux-amd64.tar.gz"
	if err := os.MkdirAll(filepath.Join(packageRoot, filepath.Dir(relative)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, filepath.FromSlash(relative)), []byte("pkg"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database, packageRoot: packageRoot}
	row := targetInstallRow{ID: 5, HostID: 221, ManagedEnabled: true, ExporterType: "node_exporter",
		OSID: "rocky", OSVersionID: "9.4", Architecture: "x86_64"}
	selectQuery := regexp.QuoteMeta(`SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,file,sha256,
		service_file_content,service_run_as_user,service_run_as_group,work_directory,install_playbook_template_id
		FROM monitor_software_package
		WHERE package_type='exporter' AND name=? AND enabled=TRUE AND file<>''
		ORDER BY create_time DESC`)
	mock.ExpectQuery(selectQuery).WithArgs("node_exporter").
		WillReturnRows(sqlmock.NewRows([]string{"version", "arch", "platform_family", "platform_major", "package_format", "file", "sha256", "service_file_content", "service_run_as_user", "service_run_as_group", "work_directory", "install_playbook_template_id"}).
			AddRow("1.7.0", "amd64", "rhel", "9", "rpm", relative, "sha-rhel9", "[Unit]", "", "dj-agent", "/var/lib/exporter", sql.NullInt64{Int64: 3, Valid: true}))
	checksumQuery := regexp.QuoteMeta(`SELECT os,arch,sha256 FROM monitor_software_package
		WHERE package_type='exporter' AND name=? AND version=? AND enabled=TRUE AND sha256<>''`)
	mock.ExpectQuery(checksumQuery).WithArgs("node_exporter", "1.7.0").
		WillReturnRows(sqlmock.NewRows([]string{"os", "arch", "sha256"}).
			AddRow("linux", "amd64", "sha-rhel9").AddRow("linux", "arm64", "sha-arm64"))

	_, playbookID, extra, err := handler.prepareExporterDispatch(ginContext, row, "install")
	if err != nil {
		t.Fatalf("prepareExporterDispatch: %v", err)
	}
	if playbookID != 3 {
		t.Fatalf("playbook id = %d, want 3", playbookID)
	}
	if extra["exporter_name"] != "node_exporter" || extra["exporter_version"] != "1.7.0" {
		t.Fatalf("exporter fields: %v", extra)
	}
	if extra["service_name"] != "node_exporter.service" {
		t.Fatalf("service_name = %v", extra["service_name"])
	}
	if extra["service_run_as_user"] != "dj-agent" || extra["service_run_as_group"] != "dj-agent" {
		t.Fatalf("run_as defaults: %v/%v", extra["service_run_as_user"], extra["service_run_as_group"])
	}
	if extra["package_local_path"] != filepath.Join(packageRoot, filepath.FromSlash(relative)) {
		t.Fatalf("package_local_path = %v", extra["package_local_path"])
	}
	if extra["package_file_name"] != "node_exporter-1.7.0.linux-amd64.tar.gz" {
		t.Fatalf("package_file_name = %v", extra["package_file_name"])
	}
	checksums, ok := extra["checksums"].(gin.H)
	if !ok || checksums["linux-amd64"] != "sha-rhel9" || checksums["linux-arm64"] != "sha-arm64" {
		t.Fatalf("checksums = %v", extra["checksums"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 卸载派发只需要 exporter_name/service_name 两个变量，且不要求本地包文件存在。
func TestPrepareExporterDispatchUninstall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	row := targetInstallRow{ID: 5, HostID: 221, ManagedEnabled: false, ExporterType: "node_exporter"}
	selectQuery := regexp.QuoteMeta(`SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,COALESCE(file,''),COALESCE(sha256,''),
			service_file_content,service_run_as_user,service_run_as_group,work_directory,uninstall_playbook_template_id
			FROM monitor_software_package
			WHERE package_type='exporter' AND name=? AND enabled=TRUE
			ORDER BY create_time DESC LIMIT 1`)
	mock.ExpectQuery(selectQuery).WithArgs("node_exporter").
		WillReturnRows(sqlmock.NewRows([]string{"version", "arch", "platform_family", "platform_major", "package_format", "file", "sha256", "service_file_content", "service_run_as_user", "service_run_as_group", "work_directory", "uninstall_playbook_template_id"}).
			AddRow("1.7.0", "amd64", "rhel", "9", "rpm", "monitor_packages/node_exporter/missing.rpm", "abc", "", "", "", "", sql.NullInt64{Int64: 9, Valid: true}))

	_, playbookID, extra, err := handler.prepareExporterDispatch(ginContext, row, "uninstall")
	if err != nil {
		t.Fatalf("prepareExporterDispatch: %v", err)
	}
	if playbookID != 9 {
		t.Fatalf("playbook id = %d, want 9", playbookID)
	}
	if len(extra) != 2 || extra["exporter_name"] != "node_exporter" || extra["service_name"] != "node_exporter.service" {
		t.Fatalf("uninstall extra_vars = %v", extra)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 官方下载地址构造：按官方 release 命名规则拼接（Django build_node_exporter_official_url 契约）。
func TestBuildNodeExporterOfficialURL(t *testing.T) {
	got := buildNodeExporterOfficialURL("1.8.2", "linux", "amd64")
	want := "https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-amd64.tar.gz"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	if nodeExporterVersionPattern.MatchString("v1.8.2") {
		t.Fatal("v-prefixed version must be rejected (caller strips prefix first)")
	}
	if !nodeExporterVersionPattern.MatchString("1.8.2") {
		t.Fatal("plain semver should match")
	}
}
