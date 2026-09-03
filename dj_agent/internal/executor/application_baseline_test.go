package executor

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

)

func TestCheckApplicationPlan_SkipsRunningOnlyCheckWhenStopped(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "must-not-read")
	checks := (&Executor{}).checkApplicationPlanForState(context.Background(), map[string]any{
		"check_plan": map[string]any{
			"schema_version": 1,
			"checks": []any{map[string]any{
				"key": "running-only", "type": "config_json", "name": "仅运行时检查",
				"executor": "schema_validate", "requires_running": true,
				"path": markerPath, "document_type": "json",
				"schema": map[string]any{"type": "json_schema", "content": "{}"},
			}},
		},
	}, false)

	if len(checks) != 1 || checks[0].Status != "skipped" || checks[0].Message != "应用未运行，已跳过该检查项" {
		t.Fatalf("running-only check must be skipped: %#v", checks)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("skipped check must not read its document: %v", err)
	}
}

func TestCheckSchema_ValidatesInlineContentWithoutTemplateExpansion(t *testing.T) {
	result := checkSchema(map[string]any{
		"key": "inline-json", "type": "config_json", "name": "JSON 调试",
		"document_type": "json",
		"content":       `{"path":"C:\\apps\\${APPLICATION_VERSION}"}`,
		"schema": map[string]any{
			"type": "json_schema", "version": "2020-12",
			"content": `{"type":"object","properties":{"path":{"const":"C:\\apps\\${APPLICATION_VERSION}"}},"required":["path"]}`,
		},
	})

	if result.Status != "pass" {
		t.Fatalf("inline content must remain unchanged: %#v", result)
	}
	actual, ok := result.Actual.(map[string]any)
	if !ok || actual["path"] != "inline" {
		t.Fatalf("inline source must be identified in result: %#v", result.Actual)
	}
}

func TestApplicationSystemdCommand_SystemScope(t *testing.T) {
	command, err := applicationSystemdActionCommand(context.Background(), map[string]any{
		"service_name":  "tomcat.service",
		"systemd_scope": "system",
	}, "status")
	if err != nil {
		t.Fatalf("system scope returned error: %v", err)
	}
	if len(command.Args) != 3 || command.Args[1] != "is-active" || command.Args[2] != "tomcat.service" {
		t.Fatalf("unexpected systemctl command: %#v", command.Args)
	}
}

func TestApplicationSystemdCommand_UserScopeSetsRuntimeEnvironment(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("failed to get current user: %v", err)
	}
	command, err := applicationSystemdActionCommand(context.Background(), map[string]any{
		"service_name":  "tomcat",
		"systemd_scope": "user",
		"run_user":      currentUser.Username,
	}, "status")
	if err != nil {
		t.Fatalf("user scope returned error: %v", err)
	}
	if len(command.Args) != 3 || command.Args[0] != "/bin/bash" || command.Args[1] != "-lc" || command.Args[2] != "systemctl --user is-active tomcat" {
		t.Fatalf("unexpected systemctl command: %#v", command.Args)
	}

	currentUID, parseErr := strconv.ParseUint(currentUser.Uid, 10, 32)
	if parseErr != nil {
		t.Fatalf("invalid current user UID: %v", parseErr)
	}
	if uint64(os.Geteuid()) != currentUID && command.SysProcAttr == nil {
		t.Fatal("cross-user systemctl command must set process credentials")
	}
}

func schematronCheck(path, key, name, test string) map[string]any {
	content := fmt.Sprintf(`<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xslt">
  <pattern><rule context="/"><assert test=%q>%s</assert></rule></pattern>
</schema>`, test, name)
	return map[string]any{
		"key": key, "type": "config_xml", "name": name,
		"executor": "schema_validate", "path": path, "document_type": "xml",
		"schema": map[string]any{"type": "schematron", "version": "iso", "content": content},
	}
}

func schemaCheckPlan(serverXMLPath, usersXMLPath, contextXMLPath, password string, requiredRoles []any) map[string]any {
	roleAssertions := make([]string, 0, len(requiredRoles))
	for _, role := range requiredRoles {
		roleAssertions = append(roleAssertions, fmt.Sprintf("contains(concat(',', translate(@roles, ' ', ''), ','), ',%s,')", role))
	}
	managerTest := fmt.Sprintf("boolean(//user[@username='admin' and @password='%s' and %s])", password, strings.Join(roleAssertions, " and "))
	return map[string]any{
		"schema_version":        1,
		"required_capabilities": []any{"schema_validate:v1"},
		"checks": []any{
			schematronCheck(serverXMLPath, "max_post_size", "Connector maxPostSize", "boolean(//Connector[contains(translate(@protocol, 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz'), 'http') and (number(@maxPostSize) >= 524288000 or @maxPostSize = '-1')])"),
			schematronCheck(usersXMLPath, "manager_user", "Manager 用户", managerTest),
			schematronCheck(contextXMLPath, "forbidden_valve", "默认 RemoteAddrValve 不存在", "not(//Valve[@className='org.apache.catalina.valves.RemoteAddrValve' and @allow='^.*$'])"),
		},
	}
}

func TestCheckSchema_JSONSchemaPassAndFail(t *testing.T) {
	tempDirectory := t.TempDir()
	documentPath := filepath.Join(tempDirectory, "app.json")
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"maxPostSize":{"type":"integer","minimum":524288000}},"required":["maxPostSize"]}`
	check := map[string]any{
		"key": "json", "type": "config_json", "name": "JSON Schema", "executor": "schema_validate",
		"path": documentPath, "document_type": "json",
		"schema": map[string]any{"type": "json_schema", "version": "2020-12", "content": schema},
	}
	if err := os.WriteFile(documentPath, []byte(`{"maxPostSize":524288000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := checkSchema(check); result.Status != "pass" {
		t.Fatalf("valid JSON must pass: %#v", result)
	}
	if err := os.WriteFile(documentPath, []byte(`{"maxPostSize":1024}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := checkSchema(check); result.Status != "fail" {
		t.Fatalf("invalid JSON must fail: %#v", result)
	}
}

func TestCheckSchema_RegexpPresentAbsentAndInvalid(t *testing.T) {
	tempDirectory := t.TempDir()
	documentPath := filepath.Join(tempDirectory, "application.properties")
	if err := os.WriteFile(documentPath, []byte("debug = false\npassword = secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	check := map[string]any{
		"key": "text", "type": "config_text", "name": "Regexp", "executor": "schema_validate",
		"path": documentPath, "document_type": "text",
		"schema": map[string]any{"type": "regexp", "version": "re2"},
	}

	testCases := []struct {
		name    string
		content string
		status  string
	}{
		{name: "required text exists", content: `{"pattern":"(?m)^debug\\s*=\\s*false$","expect":"present"}`, status: "pass"},
		{name: "required text missing", content: `{"pattern":"(?m)^debug\\s*=\\s*true$","expect":"present"}`, status: "fail"},
		{name: "forbidden text exists", content: `{"pattern":"(?m)^password\\s*=","expect":"absent"}`, status: "fail"},
		{name: "forbidden text missing", content: `{"pattern":"(?m)^allowAll\\s*=","expect":"absent"}`, status: "pass"},
		{name: "invalid RE2", content: `{"pattern":"(?=debug)","expect":"present"}`, status: "error"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			check["schema"].(map[string]any)["content"] = testCase.content
			if result := checkSchema(check); result.Status != testCase.status {
				t.Fatalf("expected %s, got %#v", testCase.status, result)
			}
		})
	}
}

func TestCheckSchema_StructuredConfigFormats(t *testing.T) {
	testCases := []struct {
		name         string
		documentType string
		valid        string
		invalid      string
		schema       string
	}{
		{
			name: "INI", documentType: "ini",
			valid: "[server]\nport = 8080\ndebug = false\n", invalid: "[server]\nport = 70000\ndebug = true\n",
			schema: `{"type":"object","properties":{"server":{"type":"object","properties":{"port":{"type":"integer","minimum":1,"maximum":65535},"debug":{"const":false}},"required":["port","debug"]}},"required":["server"]}`,
		},
		{
			name: "TOML", documentType: "toml",
			valid: "[server]\nport = 8080\ndebug = false\n", invalid: "[server]\nport = 70000\ndebug = true\n",
			schema: `{"type":"object","properties":{"server":{"type":"object","properties":{"port":{"type":"integer","minimum":1,"maximum":65535},"debug":{"const":false}},"required":["port","debug"]}},"required":["server"]}`,
		},
		{
			name: "Properties", documentType: "properties",
			valid: "server.port=8080\nserver.debug=false\n", invalid: "server.port=70000\nserver.debug=true\n",
			schema: `{"type":"object","properties":{"server.port":{"type":"integer","minimum":1,"maximum":65535},"server.debug":{"const":false}},"required":["server.port","server.debug"]}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			documentPath := filepath.Join(t.TempDir(), "application.conf")
			check := map[string]any{
				"key": "structured", "type": "config_" + testCase.documentType, "name": testCase.name,
				"executor": "schema_validate", "path": documentPath, "document_type": testCase.documentType,
				"schema": map[string]any{"type": "json_schema", "version": "2020-12", "content": testCase.schema},
			}
			if err := os.WriteFile(documentPath, []byte(testCase.valid), 0o600); err != nil {
				t.Fatal(err)
			}
			if result := checkSchema(check); result.Status != "pass" {
				t.Fatalf("valid %s must pass: %#v", testCase.name, result)
			}
			if err := os.WriteFile(documentPath, []byte(testCase.invalid), 0o600); err != nil {
				t.Fatal(err)
			}
			if result := checkSchema(check); result.Status != "fail" {
				t.Fatalf("invalid %s must fail: %#v", testCase.name, result)
			}
		})
	}
}

func TestParseConfigScalar(t *testing.T) {
	testCases := []struct {
		input    string
		expected any
	}{
		{input: "true", expected: true},
		{input: "FALSE", expected: false},
		{input: "1", expected: int64(1)},
		{input: "0", expected: int64(0)},
		{input: "1.5", expected: 1.5},
		{input: "  value  ", expected: "  value  "},
	}
	for _, testCase := range testCases {
		if actual := parseConfigScalar(testCase.input); actual != testCase.expected {
			t.Errorf("parseConfigScalar(%q) = %#v, want %#v", testCase.input, actual, testCase.expected)
		}
	}
}

func TestCheckApplicationPlan_RejectsUnsupportedProtocol(t *testing.T) {
	versionChecks := (&Executor{}).checkApplicationPlanForState(context.Background(), map[string]any{
		"check_plan": map[string]any{"schema_version": 2},
	}, true)
	if len(versionChecks) != 1 || versionChecks[0].Status != "error" {
		t.Fatalf("unsupported plan version must fail: %#v", versionChecks)
	}

	capabilityChecks := (&Executor{}).checkApplicationPlanForState(context.Background(), map[string]any{
		"check_plan": map[string]any{
			"schema_version":        1,
			"required_capabilities": []any{"shell_script:v1"},
		},
	}, true)
	if len(capabilityChecks) != 1 || capabilityChecks[0].Status != "error" || capabilityChecks[0].Actual != "shell_script:v1" {
		t.Fatalf("unsupported capability must fail explicitly: %#v", capabilityChecks)
	}

	executorChecks := (&Executor{}).checkApplicationPlanForState(context.Background(), map[string]any{
		"check_plan": map[string]any{
			"schema_version": 1,
			"checks": []any{map[string]any{
				"key": "unsupported", "executor": "shell_script",
			}},
		},
	}, true)
	if len(executorChecks) != 1 || executorChecks[0].Status != "error" {
		t.Fatalf("unknown executor must fail explicitly: %#v", executorChecks)
	}
}

func TestCheckTomcatBaseline_MaxPostSizePassesAndManagerUserPasses(t *testing.T) {
	tempDirectory := t.TempDir()
	serverXMLPath := filepath.Join(tempDirectory, "server.xml")
	usersXMLPath := filepath.Join(tempDirectory, "tomcat-users.xml")
	contextXMLPath := filepath.Join(tempDirectory, "context.xml")
	serverXML := `<?xml version="1.0" encoding="UTF-8"?>
<Server>
  <Service>
    <Connector port="8080" protocol="HTTP/1.1" maxPostSize="629145600"/>
  </Service>
</Server>`
	usersXML := `<?xml version="1.0" encoding="UTF-8"?>
<tomcat-users>
  <role rolename="manager"/>
  <role rolename="manager-gui"/>
  <role rolename="admin"/>
  <role rolename="admin-gui"/>
  <role rolename="manager-script"/>
  <role rolename="manager-jmx"/>
  <role rolename="manager-status"/>
  <user username="admin" password="tsystems.com" roles="manager,manager-gui,admin,admin-gui,manager-script,manager-status,manager-jmx"/>
</tomcat-users>`
	contextXML := `<?xml version="1.0" encoding="UTF-8"?>
<Context antiResourceLocking="false" privileged="true">
</Context>`
	if err := os.WriteFile(serverXMLPath, []byte(serverXML), 0o600); err != nil {
		t.Fatalf("failed to write server.xml: %v", err)
	}
	if err := os.WriteFile(usersXMLPath, []byte(usersXML), 0o600); err != nil {
		t.Fatalf("failed to write tomcat-users.xml: %v", err)
	}
	if err := os.WriteFile(contextXMLPath, []byte(contextXML), 0o600); err != nil {
		t.Fatalf("failed to write context.xml: %v", err)
	}

	checks := (&Executor{}).checkApplicationPlanForState(context.Background(), map[string]any{
		"check_plan": schemaCheckPlan(serverXMLPath, usersXMLPath, contextXMLPath, "tsystems.com", []any{"manager", "manager-gui", "admin", "admin-gui", "manager-script", "manager-jmx", "manager-status"}),
	}, true)
	if len(checks) != 3 {
		t.Fatalf("expected 3 tomcat checks, got: %#v", checks)
	}
	if checks[0].Status != "pass" {
		t.Fatalf("maxPostSize check should pass: %#v", checks[0])
	}
	if checks[1].Status != "pass" {
		t.Fatalf("manager user check should pass: %#v", checks[1])
	}
	if checks[2].Status != "pass" {
		t.Fatalf("manager context check should pass: %#v", checks[2])
	}
}

func TestCheckTomcatBaseline_MaxPostSizeAndManagerUserFail(t *testing.T) {
	tempDirectory := t.TempDir()
	serverXMLPath := filepath.Join(tempDirectory, "server.xml")
	usersXMLPath := filepath.Join(tempDirectory, "tomcat-users.xml")
	contextXMLPath := filepath.Join(tempDirectory, "context.xml")
	serverXML := `<?xml version="1.0" encoding="UTF-8"?>
<Server>
  <Service>
    <Connector port="8080" protocol="HTTP/1.1" maxPostSize="4194304"/>
  </Service>
</Server>`
	usersXML := `<?xml version="1.0" encoding="UTF-8"?>
<tomcat-users>
  <role rolename="manager"/>
  <user username="admin" password="wrong" roles="manager"/>
</tomcat-users>`
	contextXML := `<?xml version="1.0" encoding="UTF-8"?>
<Context antiResourceLocking="false" privileged="true">
  <Valve className="org.apache.catalina.valves.RemoteAddrValve" allow="^.*$"/>
</Context>`
	if err := os.WriteFile(serverXMLPath, []byte(serverXML), 0o600); err != nil {
		t.Fatalf("failed to write server.xml: %v", err)
	}
	if err := os.WriteFile(usersXMLPath, []byte(usersXML), 0o600); err != nil {
		t.Fatalf("failed to write tomcat-users.xml: %v", err)
	}
	if err := os.WriteFile(contextXMLPath, []byte(contextXML), 0o600); err != nil {
		t.Fatalf("failed to write context.xml: %v", err)
	}

	checks := (&Executor{}).checkApplicationPlanForState(context.Background(), map[string]any{
		"check_plan": schemaCheckPlan(serverXMLPath, usersXMLPath, contextXMLPath, "tsystems.com", []any{"manager", "manager-gui"}),
	}, true)
	if len(checks) != 3 {
		t.Fatalf("expected 3 tomcat checks, got: %#v", checks)
	}
	if checks[0].Status != "fail" {
		t.Fatalf("maxPostSize check should fail: %#v", checks[0])
	}
	if checks[1].Status != "fail" {
		t.Fatalf("manager user check should fail: %#v", checks[1])
	}
	managerActual, ok := checks[1].Actual.(map[string]any)
	if !ok || len(managerActual["violations"].([]map[string]any)) != 1 {
		t.Fatalf("manager user failure must include the Schematron violation: %#v", checks[1])
	}
	if checks[2].Status != "fail" {
		t.Fatalf("manager context check should fail: %#v", checks[2])
	}
}
