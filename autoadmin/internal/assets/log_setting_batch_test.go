package assets

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// 批量按行保存覆盖值（日志中心页勾选多行后一起改档位/过滤/采集开关）要守住的语义：
//  1. 每一条 item 仍是**该行覆盖值的全集**（缺列 = 清成不覆盖），所以必须逐行写、逐行带全列；
//  2. 整批在**一个事务**里完成：要么都落库要么都不落，不留改了一半的配置；
//  3. 不属于本服务模板的日志记 ok=false（批量删除的口径），剩下的照写，不整体失败。

// 不属于本服务模板的那一条要被挑出来，且**不产生任何写库**；其余条目照常写入。
func TestBatchSaveServiceLogOverridesSkipsForeignLogDefinition(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	// 校验读只做一次（不是每个 id 读一遍）：模板里只有 24。
	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	// 有效的那一条：档位 3、include 规则 7（三态里 >0 的一支）。
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(24), sqlmock.AnyArg(), int64(15),
			int64(7), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ruleID := int64(7)
	results, err := service.BatchSaveServiceLogOverrides(context.Background(), 15, []ServiceLogOverrideInput{
		{LogDefinition: 24, RetentionTier: int64Ptr(3), CollectionFilterRule: &ruleID},
		{LogDefinition: 999},
	})
	if err != nil {
		t.Fatalf("batch save: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("逐条结果数 = %d，want 2（与提交顺序一一对应）", len(results))
	}
	if !results[0].OK || results[0].LogDefinition != 24 {
		t.Fatalf("第一条应成功且带回日志定义 id，得到 %+v", results[0])
	}
	if results[1].OK || results[1].Message == "" {
		t.Fatalf("不属于本模板的日志应 ok=false 并说明原因，得到 %+v", results[1])
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 全部条目都不适用时：不报错、不写库，逐条给原因（前端按 ok=false 提示）。
func TestBatchSaveServiceLogOverridesAllForeignWritesNothing(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))

	results, err := service.BatchSaveServiceLogOverrides(context.Background(), 15, []ServiceLogOverrideInput{{LogDefinition: 999}})
	if err != nil {
		t.Fatalf("batch save: %v", err)
	}
	if len(results) != 1 || results[0].OK {
		t.Fatalf("应返回一条 ok=false 的结果，得到 %+v", results)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生写入: %v", err)
	}
}

// 事务中途失败 → 整批返回 error（不能返回"成功了几条"，那会让界面以为改了一半是正常的）。
func TestBatchSaveServiceLogOverridesFailsWholeBatchWhenWriteFails(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WillReturnError(errors.New("deadlock"))
	mock.ExpectRollback()

	if _, err = service.BatchSaveServiceLogOverrides(context.Background(), 15, []ServiceLogOverrideInput{{LogDefinition: 24}}); err == nil {
		t.Fatal("写入失败时批量保存必须报错")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

// 参数非法在读库之前就拒绝：空 items、少了服务 id、超过上限。
func TestBatchSaveServiceLogOverridesRejectsInvalidArguments(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service := newTestService(t, NewRepository(database))

	if _, err = service.BatchSaveServiceLogOverrides(context.Background(), 15, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("空 items 应报 ErrInvalid，得到 %v", err)
	}
	if _, err = service.BatchSaveServiceLogOverrides(context.Background(), 0, []ServiceLogOverrideInput{{LogDefinition: 24}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("缺少服务 id 应报 ErrInvalid，得到 %v", err)
	}
	oversized := make([]ServiceLogOverrideInput, MaxBatchLogOverrides+1)
	if _, err = service.BatchSaveServiceLogOverrides(context.Background(), 15, oversized); !errors.Is(err, ErrInvalid) {
		t.Fatalf("超过上限应报 ErrInvalid，得到 %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生任何查询: %v", err)
	}
}

// handler 层：请求体里的每一列都要真的走到 SQL，响应体是批量删除那套口径（count + 逐条 ok/message）。
//
// 单条接口 2026-09-19 踩过"绑定结构漏列 → 那一列被静默清成 NULL"的坑；批量接口直接绑定
// ServiceLogOverrideInput（与单条同结构），这条用例把"批量也带全四列"钉在请求体上。
func TestBatchSaveApplicationServiceLogSettingsPassesEveryOverrideColumn(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service, err := NewService(NewRepository(database), "", "test-django-secret")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler := NewHandler(service, nil, "")

	mock.ExpectQuery(regexp.QuoteMeta(listServiceTemplateLogsQuery)).
		WithArgs(int64(15)).
		WillReturnRows(overrideFixtureRows(sql.NullString{}, nil, sql.NullInt64{}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO assets_application_service_log_setting")).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(24), sqlmock.AnyArg(), int64(15),
			// include 选规则 7、exclude 显式关闭（0）——0 与 null 必须区分开。
			int64(7), int64(0),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body, _ := json.Marshal(map[string]any{
		"items": []map[string]any{{
			"log_definition_id": 24, "collection_enabled": false, "retention_tier": 3,
			"collection_filter_rule": 7, "collection_exclude_filter_rule": 0,
		}},
	})
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "15"}}
	context.Request = httptest.NewRequest("POST", "/assets/application-services/15/log-config/settings/batch/", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	handler.BatchSaveApplicationServiceLogSettings(context)

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("请求体里的覆盖列没有全部走到 SQL（漏列会静默清空那一列）: %v", err)
	}
	var payload struct {
		Data struct {
			Count   int                             `json:"count"`
			Results []ServiceLogOverrideBatchResult `json:"results"`
		} `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应体: %v（body=%s）", err, recorder.Body.String())
	}
	if payload.Data.Count != 1 || len(payload.Data.Results) != 1 || !payload.Data.Results[0].OK {
		t.Fatalf("响应应为 {count:1, results:[{id:24, ok:true}]}，得到 %s", recorder.Body.String())
	}
	if payload.Data.Results[0].LogDefinition != 24 {
		t.Fatalf("逐条结果要带回日志定义 id，得到 %+v", payload.Data.Results[0])
	}
}

// 空 items 是请求错误（400），不到服务层。
func TestBatchSaveApplicationServiceLogSettingsRejectsEmptyItems(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	service, err := NewService(NewRepository(database), "", "test-django-secret")
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	handler := NewHandler(service, nil, "")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "15"}}
	context.Request = httptest.NewRequest("POST", "/assets/application-services/15/log-config/settings/batch/",
		bytes.NewReader([]byte(`{"items":[]}`)))
	context.Request.Header.Set("Content-Type", "application/json")

	handler.BatchSaveApplicationServiceLogSettings(context)

	// 错误走统一信封（code != 200、data 为 null）：这里是服务层的 ErrInvalid 被翻译的结果。
	var payload struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应体: %v（body=%s）", err, recorder.Body.String())
	}
	if payload.Code == 200 {
		t.Fatalf("空 items 应是错误响应，得到 %s", recorder.Body.String())
	}
	if string(payload.Data) != "null" {
		t.Fatalf("错误响应 data 应为 null，得到 %s", payload.Data)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("不该发生任何查询: %v", err)
	}
}
