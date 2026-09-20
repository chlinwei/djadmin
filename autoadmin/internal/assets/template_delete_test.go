package assets

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

// 外键无级联：删模板必须先自底向上删子表（含日志定义的服务级覆盖行），再删父行。
// 顺序错了会在真库命中外键 1451——现场表现就是"提示删除成功、刷新后记录还在"。
func TestDeleteDeploymentTemplateRemovesChildrenThenParent(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	repository := NewRepository(database)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,create_time,update_time,remark,name,path_pattern")).WillReturnRows(
		sqlmock.NewRows([]string{
			"id", "create_time", "update_time", "remark", "name", "path_pattern",
			"extra_fields", "processing_rule_id", "filter_include_rule_id", "filter_exclude_rule_id",
		}).AddRow(int64(7), time.Now(), time.Now(), nil, "catalina.out", "/opt/logs/*.log",
			[]byte("[]"), nil, nil, nil),
	)
	// 日志定义被服务级覆盖行引用：删定义前先清覆盖行。
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM assets_application_service_log_setting")).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, table := range []string{
		"DELETE FROM assets_application_log_definition WHERE deployment_template_id",
		"DELETE FROM assets_application_port",
		"DELETE FROM assets_application_path",
		"DELETE FROM assets_application_config_file",
		"DELETE FROM assets_application_control_action",
		"DELETE FROM assets_docker_control_config",
		"DELETE FROM assets_docker_compose_control_config",
	} {
		mock.ExpectExec(regexp.QuoteMeta(table)).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM assets_application_deployment_template WHERE id")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err = repository.DeleteDeploymentTemplate(context.Background(), 7); err != nil {
		t.Fatalf("删除失败：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("调用顺序/次数不符（子表必须先删）：%v", err)
	}
}

// 模板仍被逻辑服务引用时父行删除命中外键 1451：整段事务回滚，translate 转成
// ErrDeleteProtected（前端据此提示"仍被引用"而不是假装成功）。
func TestDeleteDeploymentTemplateDeleteProtectedRollsBack(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service, err := NewService(NewRepository(database), "", "test-django-secret")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,create_time,update_time,remark,name,path_pattern")).WillReturnRows(
		sqlmock.NewRows([]string{
			"id", "create_time", "update_time", "remark", "name", "path_pattern",
			"extra_fields", "processing_rule_id", "filter_include_rule_id", "filter_exclude_rule_id",
		}).AddRow(int64(7), time.Now(), time.Now(), nil, "catalina.out", "/opt/logs/*.log",
			[]byte("[]"), nil, nil, nil),
	)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM assets_application_service_log_setting")).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, table := range []string{
		"DELETE FROM assets_application_log_definition WHERE deployment_template_id",
		"DELETE FROM assets_application_port",
		"DELETE FROM assets_application_path",
		"DELETE FROM assets_application_config_file",
		"DELETE FROM assets_application_control_action",
		"DELETE FROM assets_docker_control_config",
		"DELETE FROM assets_docker_compose_control_config",
	} {
		mock.ExpectExec(regexp.QuoteMeta(table)).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	// 父行删除被 FK 挡住（MySQL 1451：Cannot delete or update a parent row）。
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM assets_application_deployment_template WHERE id")).
		WillReturnError(&mysql.MySQLError{Number: 1451, Message: "Cannot delete or update a parent row"})
	mock.ExpectRollback()

	err = service.DeleteDeploymentTemplate(context.Background(), 7)
	if !errors.Is(err, ErrDeleteProtected) {
		t.Fatalf("期望 ErrDeleteProtected，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("被引用时必须在同一事务里回滚：%v", err)
	}
}
