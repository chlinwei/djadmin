package assets

import (
	"errors"
	"testing"

	"autoadmin/internal/shared/apperror"
)

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

func TestPlanTemplateLogWritesEmptySubmissionRemovesAll(t *testing.T) {
	plan, err := planTemplateLogWrites([]int64{11, 12}, nil, false)
	if err != nil {
		t.Fatalf("写计划报错：%v", err)
	}
	if len(plan.Removed) != 2 || len(plan.Updates) != 0 || len(plan.Inserts) != 0 {
		t.Fatalf("空提交应只产生删除：%+v", plan)
	}
}
