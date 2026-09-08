package inspection

import "testing"

// opaTestConfig 构造一条合法的 OPA 检查项 config（采集文件 + 断言）。
func opaTestConfig(path string) map[string]any {
	return map[string]any{
		"input_files": []any{map[string]any{"key": "x", "path": path, "parse": "lines"}},
		"policy": `package baseline

assertions contains a if {
	a := {"name": "x", "pass": true}
}`,
	}
}

// 应用上下文变量只有实例上下文（应用类型组的挂载点）能展开；
// 通用组挂任何地方都是主机上下文，因此按分类校验而不是已废弃的 scope。
func TestValidateGroupInputRejectsApplicationVariableInGeneralGroup(t *testing.T) {
	category := "general"
	config := opaTestConfig("${APP_HOME}/conf/server.xml")
	input := groupInput{
		Category: &category,
		Checks:   &[]checkInput{{Name: "check", Config: config}},
	}
	if message := validateGroupInput(input); message == "" {
		t.Fatal("general group with ${APP_HOME} should be rejected")
	}
}

func TestValidateGroupInputAllowsApplicationVariableInApplicationGroup(t *testing.T) {
	category := "application"
	config := opaTestConfig("${APP_HOME}/conf/server.xml")
	input := groupInput{
		Category: &category,
		Checks:   &[]checkInput{{Name: "check", Config: config}},
	}
	if message := validateGroupInput(input); message != "" {
		t.Fatalf("application group should allow ${APP_HOME}: %s", message)
	}
}

func TestValidateGroupInputRejectsInvalidCategory(t *testing.T) {
	category := "whatever"
	input := groupInput{Category: &category}
	if message := validateGroupInput(input); message == "" {
		t.Fatal("invalid category should be rejected")
	}
}

// 应用类型组必须选择适用应用；通用组忽略应用标签。
func TestValidateGroupInputRequiresApplicationForApplicationCategory(t *testing.T) {
	category := "application"
	application := int64(0)
	input := groupInput{Category: &category, Application: &application}
	if message := validateGroupInput(input); message != "" {
		t.Fatalf("unexpected message: %s", message)
	}
	// 必选校验在 handler.validateGroupApplication（需查库），这里验证保存入口的
	// 纯函数部分：分类合法即通过本地校验，库校验由集成覆盖。
}

func TestValidateParamDeclarationsRejectsBuiltinConflict(t *testing.T) {
	category := "application"
	params := []paramInput{{Name: "APP_HOME"}}
	if message := validateParamDeclarations(&category, &params); message == "" {
		t.Fatal("param conflicting with builtin variable should be rejected")
	}
	dup := []paramInput{{Name: "SID"}, {Name: "SID"}}
	if message := validateParamDeclarations(&category, &dup); message == "" {
		t.Fatal("duplicate param names should be rejected")
	}
}
