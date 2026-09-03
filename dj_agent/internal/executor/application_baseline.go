package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
	"github.com/magiconair/properties"
	"github.com/pelletier/go-toml/v2"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

var safeResourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*$`)

const maxInlineSchemaDocumentBytes = 1024 * 1024

type applicationCheckResult struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (e *Executor) checkApplicationBaseline(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	// 巡检中心模式：只执行 check_plan 下发的检查项（schema_validate / goss），
	// 早期的应用控制状态、端口、路径、日志内置检查已随 baseline 方案移除。
	checks := make([]applicationCheckResult, 0)

	if ctx.Err() != nil {
		return canceledApplicationBaselineResult(job, started, ctx.Err())
	}
	checks = append(checks, e.checkApplicationPlanForState(ctx, job.Params, true)...)
	if ctx.Err() != nil {
		return canceledApplicationBaselineResult(job, started, ctx.Err())
	}

	passed := true
	for _, check := range checks {
		if check.Status != "pass" && check.Status != "skipped" {
			passed = false
			break
		}
	}

	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action,
		Status: protocol.StatusSuccess, ExitCode: 0,
		StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
		Data: map[string]any{"passed": passed, "checks": checks},
	}
}

func canceledApplicationBaselineResult(job protocol.Job, started time.Time, err error) protocol.JobResult {
	return canceledJobResult(job, started, err, -1)
}

var applicationCheckCapabilities = map[string]struct{}{
	"goss:v1":                    {},
	"schema_validate:v1":        {},
	"schema_validate:inline:v1": {},
}

func (e *Executor) checkApplicationPlanForState(ctx context.Context, params map[string]any, applicationRunning bool) []applicationCheckResult {
	plan, ok := params["check_plan"].(map[string]any)
	if !ok {
		return nil
	}
	version, versionErr := numberToInt(plan["schema_version"])
	if versionErr != nil || version != 1 {
		return []applicationCheckResult{{Key: "check_plan", Type: "plan", Name: "应用检查计划", Status: "error", Message: "不支持的检查计划版本"}}
	}
	for _, capability := range stringSliceFromAny(plan["required_capabilities"]) {
		if _, supported := applicationCheckCapabilities[capability]; !supported {
			return []applicationCheckResult{{Key: "check_plan", Type: "plan", Name: "应用检查计划", Status: "error", Actual: capability, Message: "dj-agent 不支持检查计划要求的能力"}}
		}
	}

	rawChecks, _ := plan["checks"].([]any)
	results := make([]applicationCheckResult, 0, len(rawChecks))
	for _, rawCheck := range rawChecks {
		check, valid := rawCheck.(map[string]any)
		if !valid {
			results = append(results, applicationCheckResult{Key: "invalid", Type: "plan", Name: "无效检查项", Status: "error", Message: "检查项必须是对象"})
			continue
		}
		if requiresRunning, _ := check["requires_running"].(bool); requiresRunning && !applicationRunning {
			results = append(results, newPlanCheckResult(check, "skipped", nil, "应用未运行，已跳过该检查项"))
			continue
		}
		switch valueString(check["executor"]) {
		case "goss":
			results = append(results, e.checkGoss(ctx, check))
		case "schema_validate":
			results = append(results, checkSchema(check))
		default:
			results = append(results, newPlanCheckResult(check, "error", nil, "不支持的检查执行器"))
		}
	}
	return results
}

func newPlanCheckResult(check map[string]any, status string, actual any, message string) applicationCheckResult {
	return applicationCheckResult{
		Key: valueString(check["key"]), Type: valueString(check["type"]), Name: valueString(check["name"]),
		Status: status, Expected: check["expected"], Actual: actual, Message: message,
	}
}

func readSchemaDocument(check map[string]any) ([]byte, string, error) {
	if rawContent, exists := check["content"]; exists {
		content, valid := rawContent.(string)
		if !valid {
			return nil, "inline", fmt.Errorf("待校验文档内容必须为字符串")
		}
		if len(content) > maxInlineSchemaDocumentBytes {
			return nil, "inline", fmt.Errorf("待校验文档内容不能超过 1 MiB")
		}
		return []byte(content), "inline", nil
	}
	path := valueString(check["path"])
	if !filepath.IsAbs(path) {
		return nil, path, fmt.Errorf("待校验文档路径必须为绝对路径")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("读取待校验文档失败: %w", err)
	}
	return content, path, nil
}

func checkSchema(check map[string]any) applicationCheckResult {
	document, path, readErr := readSchemaDocument(check)
	if readErr != nil {
		return newPlanCheckResult(check, "error", nil, readErr.Error())
	}
	schema, valid := check["schema"].(map[string]any)
	if !valid || valueString(schema["content"]) == "" {
		return newPlanCheckResult(check, "error", nil, "Schema 定义格式无效")
	}

	var actual any
	var validationErr error
	switch valueString(schema["type"]) {
	case "json_schema":
		actual, validationErr = validateJSONSchema(document, valueString(check["document_type"]), schema)
	case "schematron":
		actual, validationErr = validateSchematron(document, schema)
	case "regexp":
		actual, validationErr = validateRegexp(document, valueString(check["document_type"]), schema)
	default:
		return newPlanCheckResult(check, "error", nil, "不支持的 Schema 类型")
	}
	check["expected"] = map[string]any{"schema_type": schema["type"], "schema_version": schema["version"]}
	if validationErr != nil {
		return newPlanCheckResult(check, "error", actual, validationErr.Error())
	}
	violations := schemaViolations(actual)
	if len(violations) > 0 {
		return newPlanCheckResult(check, "fail", map[string]any{"path": path, "violations": violations}, fmt.Sprintf("Schema 校验失败，共 %d 项", len(violations)))
	}
	return newPlanCheckResult(check, "pass", map[string]any{"path": path, "violations": []any{}}, "")
}

func validateJSONSchema(document []byte, documentType string, schemaDefinition map[string]any) (any, error) {
	if valueString(schemaDefinition["version"]) != "2020-12" {
		return nil, fmt.Errorf("JSON Schema 仅支持版本 2020-12")
	}
	var schemaDocument any
	decoder := json.NewDecoder(strings.NewReader(valueString(schemaDefinition["content"])))
	decoder.UseNumber()
	if err := decoder.Decode(&schemaDocument); err != nil {
		return nil, fmt.Errorf("解析 JSON Schema 失败: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("schema.json", schemaDocument); err != nil {
		return nil, fmt.Errorf("加载 JSON Schema 失败: %w", err)
	}
	compiledSchema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("编译 JSON Schema 失败: %w", err)
	}

	instance, err := parseSchemaInstance(document, documentType)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 文档失败: %w", strings.ToUpper(documentType), err)
	}
	validationErr := compiledSchema.Validate(instance)
	if validationErr == nil {
		return []map[string]any{}, nil
	}
	structuredError, ok := validationErr.(*jsonschema.ValidationError)
	if !ok {
		return nil, fmt.Errorf("执行 JSON Schema 校验失败: %w", validationErr)
	}
	violations := make([]map[string]any, 0)
	appendJSONSchemaViolations(structuredError, &violations)
	return violations, nil
}

func parseSchemaInstance(document []byte, documentType string) (any, error) {
	var instance any
	switch documentType {
	case "json":
		decoder := json.NewDecoder(bytes.NewReader(document))
		decoder.UseNumber()
		err := decoder.Decode(&instance)
		return instance, err
	case "yaml":
		err := yaml.Unmarshal(document, &instance)
		return instance, err
	case "toml":
		err := toml.Unmarshal(document, &instance)
		return instance, err
	case "ini":
		return parseINI(document)
	case "properties":
		return parseProperties(document)
	default:
		return nil, fmt.Errorf("JSON Schema 不支持文档类型 %s", documentType)
	}
}

func parseINI(document []byte) (map[string]any, error) {
	configuration, err := ini.LoadSources(ini.LoadOptions{AllowBooleanKeys: false}, document)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any)
	for _, section := range configuration.Sections() {
		values := make(map[string]any)
		for _, key := range section.Keys() {
			values[key.Name()] = parseConfigScalar(key.String())
		}
		if len(values) == 0 {
			continue
		}
		sectionName := section.Name()
		if sectionName == ini.DEFAULT_SECTION {
			sectionName = "_global"
		}
		result[sectionName] = values
	}
	return result, nil
}

func parseProperties(document []byte) (map[string]any, error) {
	configuration, err := properties.Load(document, properties.UTF8)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any, configuration.Len())
	for key, value := range configuration.Map() {
		result[key] = parseConfigScalar(value)
	}
	return result, nil
}

func parseConfigScalar(value string) any {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "true":
		return true
	case "false":
		return false
	}
	if integer, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return integer
	}
	if number, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return number
	}
	return value
}

func validateRegexp(document []byte, documentType string, schemaDefinition map[string]any) (any, error) {
	if documentType != "text" {
		return nil, fmt.Errorf("Regexp 仅支持普通文本文档")
	}
	if valueString(schemaDefinition["version"]) != "re2" {
		return nil, fmt.Errorf("Regexp 仅支持 RE2")
	}
	var rule struct {
		Pattern string `json:"pattern"`
		Expect  string `json:"expect"`
	}
	if err := json.Unmarshal([]byte(valueString(schemaDefinition["content"])), &rule); err != nil {
		return nil, fmt.Errorf("解析 Regexp 规则失败: %w", err)
	}
	if rule.Pattern == "" || (rule.Expect != "present" && rule.Expect != "absent") {
		return nil, fmt.Errorf("Regexp 规则必须包含 pattern，expect 仅支持 present 或 absent")
	}
	compiledPattern, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("Regexp pattern 不是有效的 RE2 表达式: %w", err)
	}
	matched := compiledPattern.Match(document)
	valid := matched == (rule.Expect == "present")
	if valid {
		return []map[string]any{}, nil
	}
	message := "未匹配到必须存在的文本"
	if rule.Expect == "absent" {
		message = "匹配到禁止存在的文本"
	}
	return []map[string]any{{"path": "/", "keyword": "regexp", "message": message}}, nil
}

func appendJSONSchemaViolations(validationErr *jsonschema.ValidationError, violations *[]map[string]any) {
	if len(validationErr.Causes) == 0 {
		*violations = append(*violations, map[string]any{
			"path":    "/" + strings.Join(validationErr.InstanceLocation, "/"),
			"keyword": fmt.Sprint(validationErr.ErrorKind),
			"message": validationErr.Error(),
		})
		return
	}
	for _, cause := range validationErr.Causes {
		appendJSONSchemaViolations(cause, violations)
	}
}

func validateSchematron(document []byte, schemaDefinition map[string]any) (any, error) {
	if valueString(schemaDefinition["version"]) != "iso" {
		return nil, fmt.Errorf("Schematron 仅支持版本 iso")
	}
	schemaDocument, err := xmlquery.Parse(strings.NewReader(valueString(schemaDefinition["content"])))
	if err != nil {
		return nil, fmt.Errorf("解析 Schematron 失败: %w", err)
	}
	unsupported, err := xmlquery.Query(schemaDocument, "//*[local-name()='report' or local-name()='include' or local-name()='extends' or local-name()='let']")
	if err != nil {
		return nil, fmt.Errorf("分析 Schematron 失败: %w", err)
	}
	if unsupported != nil {
		return nil, fmt.Errorf("当前 Schematron 执行器不支持 report/include/extends/let")
	}
	xmlDocument, err := xmlquery.Parse(bytes.NewReader(document))
	if err != nil {
		return nil, fmt.Errorf("解析 XML 文档失败: %w", err)
	}
	namespaces := make(map[string]string)
	for _, namespace := range xmlquery.Find(schemaDocument, "//*[local-name()='ns']") {
		if prefix, uri := namespace.SelectAttr("prefix"), namespace.SelectAttr("uri"); prefix != "" && uri != "" {
			namespaces[prefix] = uri
		}
	}
	violations := make([]map[string]any, 0)
	for _, rule := range xmlquery.Find(schemaDocument, "//*[local-name()='rule']") {
		contextExpression := strings.TrimSpace(rule.SelectAttr("context"))
		compiledContext, compileErr := xpath.CompileWithNS(contextExpression, namespaces)
		if compileErr != nil {
			return nil, fmt.Errorf("Schematron rule context 无效: %w", compileErr)
		}
		contexts := compiledContext.Select(xmlquery.CreateXPathNavigator(xmlDocument))
		assertions := xmlquery.Find(rule, "./*[local-name()='assert']")
		if len(assertions) == 0 {
			return nil, fmt.Errorf("Schematron rule 必须包含 assert")
		}
		for contexts.MoveNext() {
			for _, assertion := range assertions {
				test := strings.TrimSpace(assertion.SelectAttr("test"))
				compiledTest, testErr := xpath.CompileWithNS(test, namespaces)
				if testErr != nil {
					return nil, fmt.Errorf("Schematron assert test 无效: %w", testErr)
				}
				if !xpathResultTruthy(compiledTest.Evaluate(contexts.Current().Copy())) {
					violations = append(violations, map[string]any{
						"path": contextExpression, "keyword": "assert", "message": strings.TrimSpace(assertion.InnerText()),
					})
				}
			}
		}
	}
	return violations, nil
}

func xpathResultTruthy(result any) bool {
	switch value := result.(type) {
	case bool:
		return value
	case float64:
		return value != 0
	case string:
		return value != ""
	case *xpath.NodeIterator:
		return value.MoveNext()
	default:
		return false
	}
}

func schemaViolations(actual any) []map[string]any {
	violations, _ := actual.([]map[string]any)
	return violations
}

func applicationSystemdActionCommand(ctx context.Context, params map[string]any, action string) (*exec.Cmd, error) {
	serviceName := valueString(params["service_name"])
	if !safeResourceNamePattern.MatchString(serviceName) {
		return nil, fmt.Errorf("systemd 服务名格式无效")
	}
	systemdAction := map[string]string{"start": "start", "stop": "stop", "status": "is-active"}[action]
	if systemdAction == "" {
		return nil, fmt.Errorf("不支持的应用控制动作")
	}

	scope := valueString(params["systemd_scope"])
	switch scope {
	case "system":
		return exec.CommandContext(ctx, "systemctl", systemdAction, serviceName), nil
	case "user":
		runUser := valueString(params["run_user"])
		if runUser == "" {
			return nil, fmt.Errorf("用户级 Systemd 必须配置运行用户")
		}
		return applicationRunUserCommand(ctx, fmt.Sprintf("systemctl --user %s %s", systemdAction, serviceName), runUser)
	default:
		return nil, fmt.Errorf("Systemd 作用域无效")
	}
}

func systemdUserEnvironment(environment []string, targetUser *user.User, uid uint64) []string {
	overrides := map[string]string{
		"HOME":            targetUser.HomeDir,
		"USER":            targetUser.Username,
		"LOGNAME":         targetUser.Username,
		"XDG_RUNTIME_DIR": fmt.Sprintf("/run/user/%d", uid),
	}
	result := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, overridden := overrides[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "XDG_RUNTIME_DIR"} {
		result = append(result, key+"="+overrides[key])
	}
	return result
}

func numberToInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case float64:
		return int(typed), nil
	case string:
		return strconv.Atoi(typed)
	default:
		return 0, fmt.Errorf("invalid number")
	}
}

func valueString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringSliceFromAny(value any) []string {
	if value == nil {
		return nil
	}
	if typed, ok := value.([]string); ok {
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			normalized := strings.TrimSpace(item)
			if normalized != "" {
				result = append(result, normalized)
			}
		}
		return result
	}
	rawList, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(rawList))
	for _, item := range rawList {
		normalized := valueString(item)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}
