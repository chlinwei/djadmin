package monitor

import "testing"

// Filebeat 渲染：每个 服务×日志定义 一个 inputs.d yml，文件内每个实例一个 filestream input；
// 维度字段、索引命名、pipeline、多行语义。
func TestRenderHostLogConfig(t *testing.T) {
	entries := []hostLogRenderInput{
		{
			Prefix: "logs", Application: "tib", Service: "tomcat-svc",
			Project: "kul", Environment: "test", BusinessSystem: "tib",
			Tier: "wuhan-test", Pipeline: "springboot-tomcat-exception",
			LogName: "catalina.out", ResolvedPath: "${APP_HOME}/logs/catalina.out",
			Multiline: true, StartPattern: `\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3}`, FlushTimeout: 3000,
		},
	}
	instances := []hostInstanceInput{
		{Service: "tomcat-svc", Instance: "tomcat1", HostIP: "192.168.201.211", Macros: map[string]string{"APP_HOME": "/data/tomcat1"}},
		{Service: "tomcat-svc", Instance: "tomcat2"},
	}
	rendered := renderHostLogConfig(entries, instances)
	if len(rendered.Fragments) != 2 {
		t.Fatalf("fragments = %d, want 2 (inputs.yml + data-keep)", len(rendered.Fragments))
	}
	var inputFragment logConfigFragment
	for _, fragment := range rendered.Fragments {
		if contains(fragment.Path, "inputs.d") {
			inputFragment = fragment
		}
	}
	if inputFragment.Path != "/etc/filebeat/inputs.d/tib__tomcat-svc__catalina.out.yml" {
		t.Fatalf("inputs path wrong: %s", inputFragment.Path)
	}
	for _, want := range []string{
		"- type: filestream",
		"  id: 'tib__tomcat-svc__catalina.out__tomcat1'",
		"  fields_under_root: true",
		"    service: 'tomcat-svc'",
		"    instance: 'tomcat1'",
		"    application: 'tib'",
		"    log_name: 'catalina.out'",
		"    business_system: 'tib'",
		"    environment: 'test'",
		"    host_ip: '192.168.201.211'",
		"  index: 'logs-kul-tib-test-tomcat-svc-wuhan-test'",
		"  pipeline: 'logs-tib-springboot-tomcat-exception'",
		"  paths:",
		"    - '/data/tomcat1/logs/catalina.out'",
		"        negate: true",
		"        match: after",
		"        timeout: '3000ms'",
	} {
		if !contains(inputFragment.Content, want) {
			t.Errorf("missing %q in:\n%s", want, inputFragment.Content)
		}
	}
	// 未展开宏的实例跳过并告警，且不能带病下发
	if contains(inputFragment.Content, "${APP_HOME}") || contains(inputFragment.Content, "tomcat2") {
		t.Errorf("unresolved instance must be skipped:\n%s", inputFragment.Content)
	}
	if len(rendered.Warnings) == 0 || !contains(rendered.Warnings[0], "tomcat2") {
		t.Errorf("warnings should record skipped instances: %v", rendered.Warnings)
	}
	if rendered.Fingerprint == "" {
		t.Errorf("fingerprint empty")
	}
}

// APP_HOME 宏来源：部署模板 app_home 是权威默认值；实例运行时变量显式配置的值优先；
// 模板 app_home 为空时不注入；非法 JSON 与空配置按空宏处理。
func TestInstanceMacros(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		appHome  string
		expected map[string]string
	}{
		{"模板注入默认值", "{}", "/data/tomcat", map[string]string{"APP_HOME": "/data/tomcat"}},
		{"实例变量优先", `{"APP_HOME":"/opt/tomcat"}`, "/data/tomcat", map[string]string{"APP_HOME": "/opt/tomcat"}},
		{"模板为空不注入", "{}", "", map[string]string{}},
		{"非法JSON按空宏", "not-json", "/data/tomcat", map[string]string{"APP_HOME": "/data/tomcat"}},
		{"app_home两侧空白被裁剪", "{}", "  /data/tomcat  ", map[string]string{"APP_HOME": "/data/tomcat"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := instanceMacros(testCase.raw, testCase.appHome)
			if len(got) != len(testCase.expected) {
				t.Fatalf("macros = %v, want %v", got, testCase.expected)
			}
			for key, value := range testCase.expected {
				if got[key] != value {
					t.Errorf("macros[%s] = %q, want %q", key, got[key], value)
				}
			}
		})
	}
}

// 全部实例的路径宏都无法展开时，不应产出任何片段。
func TestRenderHostLogConfigAllSkipped(t *testing.T) {
	entries := []hostLogRenderInput{
		{
			Prefix: "logs", Application: "tib", Service: "tomcat-svc",
			Project: "kul", Environment: "test", BusinessSystem: "tib",
			Tier: "std", LogName: "catalina.out", ResolvedPath: "${APP_HOME}/logs/catalina.out",
		},
	}
	instances := []hostInstanceInput{
		{Service: "tomcat-svc", Instance: "tomcat1"},
	}
	rendered := renderHostLogConfig(entries, instances)
	if len(rendered.Fragments) != 0 {
		t.Fatalf("fragments = %d, want 0 when all instances unresolved", len(rendered.Fragments))
	}
	if len(rendered.Warnings) == 0 {
		t.Errorf("warnings should be non-empty")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// 宏解析口径：部署模板 macro_definitions 的 value 作默认值，服务级 macro_values 覆盖同名项。
func TestTemplateMacroDefaultsAndMerge(t *testing.T) {
	defaults := templateMacroDefaults(`[{"name":"LOG_DIR","value":"/home/esb/data/logs/mgmt","description":""},{"name":"ORACLE_SID","value":"orcl"}]`)
	if defaults["LOG_DIR"] != "/home/esb/data/logs/mgmt" || defaults["ORACLE_SID"] != "orcl" {
		t.Fatalf("templateMacroDefaults = %#v", defaults)
	}
	merged := mergeMacroValues(defaults, parseMacroJSON(`{"LOG_DIR":"/custom/logs"}`))
	if merged["LOG_DIR"] != "/custom/logs" {
		t.Fatalf("service macro_values should override template default, got %#v", merged)
	}
	if merged["ORACLE_SID"] != "orcl" {
		t.Fatalf("non-overridden template default should survive, got %#v", merged)
	}
	if got := templateMacroDefaults(`bad json`); len(got) != 0 {
		t.Fatalf("invalid macro_definitions should be empty, got %#v", got)
	}
}

// 日志定义名称里带 ${...} 直接作文件名会生成监听不到文件的坏片段，必须跳过并告警。
func TestRenderSkipsMacroInLogDefinitionName(t *testing.T) {
	entries := []hostLogRenderInput{
		{
			Prefix: "autoadmin", Application: "mgmt", Service: "mgmt",
			Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std",
			LogName: "${LOG_DIR}", ResolvedPath: "${LOG_DIR}/log_error.log",
			Macros: map[string]string{"LOG_DIR": "/home/esb/data/logs/mgmt"},
		},
	}
	instances := []hostInstanceInput{{Service: "mgmt", Instance: "yilake-nginx-106", HostIP: "192.168.201.106"}}
	rendered := renderHostLogConfig(entries, instances)
	if len(rendered.Fragments) != 0 {
		t.Fatalf("fragments = %d, want 0 when log name contains macro", len(rendered.Fragments))
	}
	if len(rendered.Warnings) == 0 {
		t.Fatal("warnings should mention the macro in log name")
	}
}

// 未关联处理规则 = 没有 pipeline，按约定不采集：跳过该日志定义并告警。
func TestRenderSkipsWhenNoProcessingRule(t *testing.T) {
	entries := []hostLogRenderInput{
		{
			Prefix: "autoadmin", Application: "mgmt", Service: "mgmt",
			Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std",
			LogName: "log_error", ResolvedPath: "/data/logs/log_error.log",
			Macros: map[string]string{},
		},
	}
	instances := []hostInstanceInput{{Service: "mgmt", Instance: "nginx-106", HostIP: "192.168.201.106"}}
	rendered := renderHostLogConfig(entries, instances)
	if len(rendered.Fragments) != 0 {
		t.Fatalf("fragments = %d, want 0 when no processing rule", len(rendered.Fragments))
	}
	found := false
	for _, warning := range rendered.Warnings {
		if contains(warning, "未关联处理规则") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings should mention missing processing rule, got %v", rendered.Warnings)
	}
}
