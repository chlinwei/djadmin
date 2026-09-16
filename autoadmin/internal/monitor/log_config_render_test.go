package monitor

import "testing"

// 渲染是纯函数：服务×日志定义产出 inputs/outputs 片段，实例聚合同文件，
// Index 走服务级命名，宏按 服务级→实例级 顺序合并替换。
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
	if len(rendered.Fragments) != 4 {
		t.Fatalf("fragments = %d, want 4 (input+output+parsers+db-keep)", len(rendered.Fragments))
	}
	var inputFragment, outputFragment, parsersFragment logConfigFragment
	for _, fragment := range rendered.Fragments {
		if contains(fragment.Path, "inputs.d") {
			inputFragment = fragment
		}
		if contains(fragment.Path, "outputs.d") {
			outputFragment = fragment
		}
		if contains(fragment.Path, "parsers.d") {
			parsersFragment = fragment
		}
	}
	// Index 必须是服务级命名
	if !contains(outputFragment.Content, "Index               logs-kul-tib-test-tomcat-svc-wuhan-test") {
		t.Errorf("output index wrong:\n%s", outputFragment.Content)
	}
	// Pipeline 指令
	if !contains(outputFragment.Content, "Pipeline            springboot-tomcat-exception") {
		t.Errorf("output pipeline missing:\n%s", outputFragment.Content)
	}
	// Match 实例段通配
	if !contains(outputFragment.Content, "Match               tib.tomcat-svc.*.catalina.out") {
		t.Errorf("output match wrong:\n%s", outputFragment.Content)
	}
	// 每实例一个 INPUT，Tag 带实例名（未展开宏的 tomcat2 不应出现）
	if !contains(inputFragment.Content, "Tag               tib.tomcat-svc.tomcat1.catalina.out") {
		t.Errorf("input tags wrong:\n%s", inputFragment.Content)
	}
	// 宏替换：实例宏只作用于自身路径
	if !contains(inputFragment.Content, "Path              /data/tomcat1/logs/catalina.out") {
		t.Errorf("instance macro substitution failed:\n%s", inputFragment.Content)
	}
	// 未展开宏的实例必须被跳过，不能带病下发（${VAR} 未定义会让 tail 静默失效）
	if contains(inputFragment.Content, "${APP_HOME}") {
		t.Errorf("unresolved macro path must be skipped:\n%s", inputFragment.Content)
	}
	// tomcat1 正常、tomcat2 宏未定义被跳过：input 片段只剩 tomcat1 的 Tag
	if contains(inputFragment.Content, "tomcat2") {
		t.Errorf("instance with unresolved macro should not appear:\n%s", inputFragment.Content)
	}
	// 跳过实例必须产生告警
	if len(rendered.Warnings) == 0 || !contains(rendered.Warnings[0], "tomcat2") {
		t.Errorf("warnings should record skipped instances: %v", rendered.Warnings)
	}
	// Fluent Bit v4 只认下划线风格属性名，驼峰会导致启动失败
	if contains(inputFragment.Content, "RefreshInterval") || contains(inputFragment.Content, "RotateWait") || contains(inputFragment.Content, "Mem_Buf_Limit") {
		t.Errorf("camelCase properties are invalid in Fluent Bit v4:\n%s", inputFragment.Content)
	}
	if !contains(inputFragment.Content, "refresh_interval  5") ||
		!contains(inputFragment.Content, "rotate_wait       30") ||
		!contains(inputFragment.Content, "mem_buf_limit     10MB") {
		t.Errorf("snake_case properties missing:\n%s", inputFragment.Content)
	}
	if rendered.Fingerprint == "" {
		t.Errorf("fingerprint empty")
	}
	// 多行合并必须在 Fluent Bit 侧完成（pipeline 拿到的已是按行拆开的文档）：
	// MULTILINE_PARSER 落在独立 parsers 文件（v4 禁止出现在主配置/主配置 include 的
	// 片段中），INPUT 通过 Multiline.parser 引用
	if len(rendered.Fragments) != 4 {
		t.Fatalf("fragments = %d, want 4 (input+output+parsers+db-keep)", len(rendered.Fragments))
	}
	if parsersFragment.Path != "/etc/fluent-bit/parsers.d/djadmin-multiline.conf" {
		t.Fatalf("parsers file path wrong: %s", parsersFragment.Path)
	}
	if !contains(parsersFragment.Content, "[MULTILINE_PARSER]") {
		t.Errorf("multiline parser missing:\n%s", parsersFragment.Content)
	}
	if !contains(parsersFragment.Content, "Name          multiline_tib.tomcat-svc.catalina.out") {
		t.Errorf("multiline parser name wrong:\n%s", parsersFragment.Content)
	}
	if !contains(parsersFragment.Content, `Rule          "start_state" "/\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3}/" "continuation"`) {
		t.Errorf("multiline start rule wrong:\n%s", parsersFragment.Content)
	}
	if !contains(parsersFragment.Content, "Flush_Timeout 3000") {
		t.Errorf("multiline flush timeout wrong:\n%s", parsersFragment.Content)
	}
	if contains(inputFragment.Content, "[MULTILINE_PARSER]") {
		t.Errorf("multiline parser must not be in input fragment (main config scope):\n%s", inputFragment.Content)
	}
	if !contains(inputFragment.Content, "Multiline.parser  multiline_tib.tomcat-svc.catalina.out") {
		t.Errorf("input should reference multiline parser:\n%s", inputFragment.Content)
	}
	// 每实例必须带 record_modifier 注入维度字段，否则 OpenSearch 按 service/instance
	// term 过滤（服务树日志查询）会匹配不到任何文档
	if !contains(inputFragment.Content, "[FILTER]") || !contains(inputFragment.Content, "Name    record_modifier") {
		t.Errorf("record_modifier filter missing:\n%s", inputFragment.Content)
	}
	if !contains(inputFragment.Content, "Match   tib.tomcat-svc.tomcat1.catalina.out") {
		t.Errorf("filter match should be per-instance tag:\n%s", inputFragment.Content)
	}
	if !contains(inputFragment.Content, "Record  service tomcat-svc") ||
		!contains(inputFragment.Content, "Record  instance tomcat1") ||
		!contains(inputFragment.Content, "Record  application tib") ||
		!contains(inputFragment.Content, "Record  log_name catalina.out") ||
		!contains(inputFragment.Content, "Record  business_system tib") ||
		!contains(inputFragment.Content, "Record  environment test") ||
		!contains(inputFragment.Content, "Record  host_ip 192.168.201.211") {
		t.Errorf("dimension fields missing in filter:\n%s", inputFragment.Content)
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
