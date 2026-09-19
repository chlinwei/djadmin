package assets

import (
	"context"
	"errors"
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
	mock.ExpectExec("UPDATE assets_application_service").WillReturnResult(sqlmock.NewResult(0, 1))

	// 期望：BeginTx 之后被调用，且**没有 Commit**（defer Rollback 生效）。
	_, err = service.repository.SaveApplicationService(context.Background(), 15, ApplicationServiceInput{
		Name: "tomcat", Code: "tomcat", Application: 1, BusinessSystem: 1,
		ApplicationVersion: 1, DeploymentTemplate: 1,
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
	mock.ExpectExec("UPDATE assets_application_service").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// 服务行不存在时会走 ErrNotFound（sqlmock 没给 SELECT），只要不 panic、不因 NULL checker 报错即可。
	_, _ = service.repository.SaveApplicationService(context.Background(), 15, ApplicationServiceInput{
		Name: "tomcat", Code: "tomcat", Application: 1, BusinessSystem: 1,
		ApplicationVersion: 1, DeploymentTemplate: 1,
	}, service.checkLogConfigConsistency)
}
