package assets

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

type dbType = db.DBTX

type stubConsistencyChecker struct {
	err error
	// 记下收到的事务句柄是否为 nil、serviceID 是否传对（校验必须在**同一个事务**里跑）。
	calledWithPool bool
	serviceID      int64
}

func (stub *stubConsistencyChecker) CheckServiceLogConfigConsistency(ctx context.Context, pool dbType, serviceID int64) error {
	stub.calledWithPool = pool != nil
	stub.serviceID = serviceID
	return stub.err
}

func (stub *stubConsistencyChecker) CheckTemplateLogConfigConsistency(ctx context.Context, pool dbType, templateID int64) error {
	return stub.err
}

func (stub *stubConsistencyChecker) CheckDeploymentLogConfigConsistency(ctx context.Context, pool dbType, deploymentID int64) error {
	return stub.err
}

// 配置不自洽时：**保存整体失败**（事务回滚），错误原样带给用户。
// 这条链是"保存逻辑服务 → 写库 → 提交前渲染校验 → 失败即回滚"，所以校验必须拿到事务句柄。
func TestSaveApplicationServiceRejectsInconsistentLogConfig(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	service, err := NewService(NewRepository(database), "", "test-django-secret")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	message := "日志采集配置不自洽，已拒绝保存（请先修好这些问题）：服务 tomcat 实例 tomcat1 与 服务 tomcat 实例 tomcat2 在同一主机上展开成同一路径 /home/esb/tomcat/logs/catalina.out"
	checker := &stubConsistencyChecker{err: errors.New(message)}
	service.SetLogConfigConsistencyChecker(checker)

	mock.ExpectBegin()
	expectServiceSaveFence(mock, 15)
	mock.ExpectExec("UPDATE assets_application_service").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT deployment_id FROM assets_application_service_deployment").
		WithArgs(int64(15)).WillReturnRows(sqlmock.NewRows([]string{"deployment_id"}))

	// 期望：BeginTx 之后被调用，且**没有 Commit**（defer Rollback 生效）。
	_, err = service.repository.SaveApplicationService(context.Background(), 15, ApplicationServiceInput{
		Name: "tomcat", Code: "tomcat", Application: 1, BusinessSystem: 1,
		ApplicationVersion: 1, DeploymentTemplate: 1,
		LogRetentionTier: new(int64(3)),
	}, service.checkLogConfigConsistency)

	if err == nil || err.Error() != message {
		t.Fatalf("期望原样返回自洽性校验的错误，得到 %v", err)
	}
	if !checker.calledWithPool {
		t.Fatal("校验必须拿到事务句柄（而不是另开连接读旧状态）")
	}
	if checker.serviceID != 15 {
		t.Fatalf("校验要针对被保存的那个服务，得到 %d", checker.serviceID)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("期望没有提交（保存失败即回滚）: %v", err)
	}
}

// 没注入校验器时（单测/无日志域部署）保存照旧，不能因为缺校验器就报错。
func TestSaveApplicationServiceWithoutCheckerSkipsValidation(t *testing.T) {
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
	expectServiceSaveFence(mock, 15)
	mock.ExpectExec("UPDATE assets_application_service").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// 服务行不存在时会走 ErrNotFound（sqlmock 没给 SELECT），只要不 panic、不因 NULL checker 报错即可。
	_, _ = service.repository.SaveApplicationService(context.Background(), 15, ApplicationServiceInput{
		Name: "tomcat", Code: "tomcat", Application: 1, BusinessSystem: 1,
		ApplicationVersion: 1, DeploymentTemplate: 1,
		LogRetentionTier: new(int64(3)),
	}, service.checkLogConfigConsistency)
}

func TestSaveDeploymentTemplateRollsBackOnLogConfigConflict(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	repository := NewRepository(database)
	message := errors.New("template creates duplicate log path")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE assets_application_deployment_template").WillReturnResult(sqlmock.NewResult(0, 1))

	_, err = repository.SaveDeploymentTemplate(context.Background(), 22, DeploymentTemplateInput{
		Name: "template", ControlType: "systemd",
	}, func(ctx context.Context, pool dbType, templateID int64) error {
		if pool == nil || templateID != 22 {
			t.Fatalf("template checker got pool=%v templateID=%d", pool != nil, templateID)
		}
		return message
	})
	if !errors.Is(err, message) {
		t.Fatalf("template conflict must reject the save, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("template conflict must roll back without commit: %v", err)
	}
}

func TestSaveDeploymentRollsBackOnLogConfigConflict(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	repository := NewRepository(database)
	message := errors.New("deployment creates duplicate log path")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE assets_application_deployment").WillReturnResult(sqlmock.NewResult(0, 1))

	_, err = repository.SaveApplicationDeployment(context.Background(), 19, ApplicationDeploymentInput{
		InstanceName: "instance", Host: 42,
	}, func(ctx context.Context, pool dbType, deploymentID int64) error {
		if pool == nil || deploymentID != 19 {
			t.Fatalf("deployment checker got pool=%v deploymentID=%d", pool != nil, deploymentID)
		}
		return message
	})
	if !errors.Is(err, message) {
		t.Fatalf("deployment conflict must reject the save, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("deployment conflict must roll back without commit: %v", err)
	}
}

func TestSaveServiceLogOverrideRollsBackOnLogConfigConflict(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))
	message := errors.New("override creates duplicate log path")
	service.SetLogConfigConsistencyChecker(&stubConsistencyChecker{err: message})

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO assets_application_service_log_setting").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	disabled := false
	_, err = service.SaveServiceLogOverride(context.Background(), 15, ServiceLogOverrideInput{
		LogDefinition: 24, CollectionEnabled: &disabled,
	})
	if !errors.Is(err, message) {
		t.Fatalf("override conflict must reject the save, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("override conflict must roll back without commit: %v", err)
	}
}
