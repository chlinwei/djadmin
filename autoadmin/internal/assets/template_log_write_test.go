package assets

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplyTemplateLogWritesPersistsFilePatterns(t *testing.T) {
	const pattern = `-(星期[一二三四五六日天]|Mon|Tue|Wed|Thu|Fri|Sat|Sun)\.log$`
	for _, creating := range []bool{false, true} {
		t.Run(map[bool]string{true: "create-or-copy", false: "edit"}[creating], func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			now := time.Now().UTC()
			raw, err := json.Marshal([]string{pattern})
			if err != nil {
				t.Fatal(err)
			}
			input := TemplateLogInput{ID: 7, Name: "app.log", PathPattern: "/logs/*.log", ExcludeFilePatterns: []string{pattern}}
			if creating {
				mock.ExpectExec("INSERT INTO assets_application_log_definition").
					WithArgs(now, now, nil, "app.log", "/logs/*.log", int64(5), []byte(`{}`), []byte(`[]`), nil, nil, nil, raw).
					WillReturnResult(sqlmock.NewResult(8, 1))
			} else {
				mock.ExpectQuery("FROM assets_application_log_definition WHERE").
					WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{
					"id", "create_time", "update_time", "remark", "name", "path_pattern", "extra_fields",
					"exclude_directories", "processing_rule_id", "filter_include_rule_id", "filter_exclude_rule_id", "exclude_file_patterns",
				}).AddRow(7, now, now, nil, "app.log", "/logs/*.log", []byte(`{}`), []byte(`[]`), nil, nil, nil, []byte(`[]`)))
				mock.ExpectExec("UPDATE assets_application_log_definition").
					WithArgs(now, nil, "app.log", "/logs/*.log", []byte(`{}`), []byte(`[]`), nil, nil, nil, raw, int64(7), int64(5)).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if err := applyTemplateLogWrites(context.Background(), db.New(database), 5, creating, []TemplateLogInput{input}, now); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// 模板保存的日志定义写计划：按 id 增量（改/删/增），而不是整表删重建。
// 覆盖"改名保住 id""别的模板的 id 不许用""复制模板时 id 一律当新增""空名拦截"这几条边界。
func TestPlanTemplateLogWrites(t *testing.T) {
	submitted := []TemplateLogInput{
		{ID: 11, Name: "catalina", PathPattern: "/opt/app/logs/catalina.out", ExtraFields: []byte(`{}`)},
		{Name: "gc", PathPattern: "/opt/app/logs/gc.log", ExtraFields: []byte(`{}`)},
	}
	plan, err := planTemplateLogWrites([]int64{11, 12}, submitted, false)
	if err != nil {
		t.Fatalf("写计划报错：%v", err)
	}
	if len(plan.Updates) != 1 || plan.Updates[0].ID != 11 {
		t.Fatalf("更新组不符：%+v", plan.Updates)
	}
	if len(plan.Inserts) != 1 || plan.Inserts[0].Name != "gc" {
		t.Fatalf("新增组不符：%+v", plan.Inserts)
	}
	if len(plan.Removed) != 1 || plan.Removed[0] != 12 {
		t.Fatalf("删除组不符（应只删未提交的 12）：%+v", plan.Removed)
	}
}

func TestPlanTemplateLogWritesRejectsForeignID(t *testing.T) {
	_, err := planTemplateLogWrites([]int64{11}, []TemplateLogInput{{ID: 99, Name: "x"}}, false)
	if !errors.Is(err, ErrInvalidRelation) {
		t.Fatalf("别的模板的日志定义 id 应报 ErrInvalidRelation，实际 %v", err)
	}
}

func TestPlanTemplateLogWritesTreatsAllAsInsertWhenCreating(t *testing.T) {
	// 复制模板：前端把源模板的行（带 id）原样提交，这里必须全部当新增，且不产生删除。
	plan, err := planTemplateLogWrites(nil, []TemplateLogInput{{ID: 11, Name: "catalina"}}, true)
	if err != nil {
		t.Fatalf("写计划报错：%v", err)
	}
	if len(plan.Inserts) != 1 || plan.Inserts[0].ID != 0 || len(plan.Updates) != 0 || len(plan.Removed) != 0 {
		t.Fatalf("复制模板应全部按新增：%+v", plan)
	}
}

func TestPlanTemplateLogWritesRejectsEmptyName(t *testing.T) {
	// 空名会生成 `<app>__<svc>__.yml` 这种监听不到文件的片段，直接拦掉。
	_, err := planTemplateLogWrites([]int64{11}, []TemplateLogInput{{ID: 11, Name: "   "}}, false)
	if err == nil {
		t.Fatal("空日志名应被拒绝")
	}
	if _, ok := apperror.As(err); !ok {
		t.Fatalf("应是带 code 的业务错误（400），实际 %v", err)
	}
}

func TestPlanTemplateLogWritesExcludeDirectories(t *testing.T) {
	input := TemplateLogInput{Name: "error", ExcludeDirectories: []string{"${LOG_DIR}/backup", "${LOG_DIR}/test-*"}}
	plan, err := planTemplateLogWrites(nil, []TemplateLogInput{input}, true)
	if err != nil || len(plan.Inserts) != 1 || len(plan.Inserts[0].ExcludeDirectories) != 2 {
		t.Fatalf("valid exclusions not retained: %+v %v", plan, err)
	}
	input.ExcludeDirectories = []string{"/logs/**/backup"}
	if _, err := planTemplateLogWrites(nil, []TemplateLogInput{input}, true); err == nil {
		t.Fatal("recursive glob must be rejected")
	} else if _, ok := apperror.As(err); !ok {
		t.Fatalf("invalid exclusion must be a business error: %v", err)
	}
}

func TestPlanTemplateLogWritesExcludeFilePatterns(t *testing.T) {
	const pattern = `-(星期[一二三四五六日天]|Mon|Tue|Wed|Thu|Fri|Sat|Sun)\.log$`
	for _, creating := range []bool{false, true} {
		input := TemplateLogInput{ID: 11, Name: "app.log", ExcludeFilePatterns: []string{pattern}}
		plan, err := planTemplateLogWrites([]int64{11}, []TemplateLogInput{input}, creating)
		if err != nil {
			t.Fatal(err)
		}
		items := plan.Updates
		if creating {
			items = plan.Inserts
		}
		if len(items) != 1 || len(items[0].ExcludeFilePatterns) != 1 || items[0].ExcludeFilePatterns[0] != pattern {
			t.Fatalf("file expression lost on create/edit/copy: %+v", plan)
		}
		input.ExcludeFilePatterns = []string{"(?=invalid)"}
		if _, err := planTemplateLogWrites([]int64{11}, []TemplateLogInput{input}, creating); err == nil {
			t.Fatal("invalid regex accepted")
		} else if _, ok := apperror.As(err); !ok {
			t.Fatalf("regex errors must be explicit business validation errors: %v", err)
		}
	}
}

func TestPlanTemplateLogWritesEmptySubmissionRemovesAll(t *testing.T) {
	plan, err := planTemplateLogWrites([]int64{11, 12}, nil, false)
	if err != nil {
		t.Fatalf("写计划报错：%v", err)
	}
	if len(plan.Removed) != 2 || len(plan.Updates) != 0 || len(plan.Inserts) != 0 {
		t.Fatalf("空提交应只产生删除：%+v", plan)
	}
}
