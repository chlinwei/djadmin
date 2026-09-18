package assets

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// 回归用例：保存部署实例后必须**按 id** 回读刚保存的那一行。
//
// 2026-09-18 现场：用户在 `yilake nginx` 上编辑实例 105（部署实例 id=20，但全库最大 id 是 21），
// 保存后接口返回 404「资产不存在」，而库里那一行其实已经被改掉了。根因是
// SaveApplicationDeployment 的回读复用了列表查询的"第 1 页、每页 1 条"，
// 而列表是 `ORDER BY d.id DESC LIMIT ?` —— Size: 1 恒取全库 id 最大的一台，
// 于是"编辑任何不是最新的一台实例"都会被判成不存在。
//
// 这个用例钉住"回读走的是按 id 的查询"：一旦有人把它改回列表查询，mock 会因为
// 收到未预期的 SELECT ... LIMIT ? 而失败。
func TestSaveApplicationDeploymentReadsBackByID(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	repository := NewRepository(database)
	service := newTestService(t, repository)

	now := time.Now().UTC()
	// 更新语句（第 6 个参数是 host_id）——断言的目标只是"确实写了一次"。
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE assets_application_deployment")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// 回读：必须按 id 查（断言到 WHERE 为止：占位符形态两侧不同，`?` / `$1`）。
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_deployment d\nJOIN assets_host h ON h.id=d.host_id\nWHERE d.id =")).
		WithArgs(int64(20)).
		WillReturnRows(applicationDeploymentRow(20).AddRow(
			int64(20), now, now, nil, "yilake-nginx-105", true, int64(462), "192.168.201.105",
			"unknown", "", nil, "unknown", []byte(`{}`), int64(8),
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_service_deployment")).
		WithArgs(serviceDeploymentLinkArgs(20)...).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "service_id"}).
			AddRow(int64(20), int64(16)).
			AddRow(int64(20), int64(15)))

	item, err := service.SaveApplicationDeployment(context.Background(), 20, ApplicationDeploymentInput{
		InstanceName: "yilake-nginx-105", Host: 462, Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("保存不是最新的那一台实例不应报错：%v", err)
	}
	if item.ID != 20 || item.InstanceName != "yilake-nginx-105" {
		t.Fatalf("回读的应是刚保存的那一行，得到 %+v", item)
	}
	if len(item.ApplicationServiceIDs) != 2 {
		t.Fatalf("回读应带上服务关联 id，得到 %v", item.ApplicationServiceIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 目标实例不存在时仍要如实报"资产不存在"（不能因为改成按 id 读就把 404 变成 500）。
func TestSaveApplicationDeploymentKeepsNotFoundSemantics(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE assets_application_deployment")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("FROM assets_application_deployment d\nJOIN assets_host h ON h.id=d.host_id\nWHERE d.id =")).WithArgs(int64(9999)).
		WillReturnError(sql.ErrNoRows)

	if _, err := service.SaveApplicationDeployment(context.Background(), 9999, ApplicationDeploymentInput{
		InstanceName: "gone", Host: 462,
	}); err != ErrNotFound {
		t.Fatalf("不存在的实例应返回 ErrNotFound，得到 %v", err)
	}
}

// applicationDeploymentRow 是 GetApplicationDeploymentDetail 的列集（与列表查询一致）。
func applicationDeploymentRow(id int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "create_time", "update_time", "remark", "instance_name", "enabled", "host_id", "host_ip",
		"runtime_status", "runtime_status_output", "last_status_check_time", "ha_role",
		"runtime_variables", "application_id",
	})
}

// newTestService 只用于仓库/服务层的 SQL 断言，不需要真实的加密密钥。
func newTestService(t *testing.T, repository *Repository) *Service {
	t.Helper()
	service, err := NewService(repository, "", "django-secret")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}
