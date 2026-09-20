package logcollect

import (
	"strings"
	"testing"
)

// 测试里的输出段标识（地址/账号/TLS）。真实值由 filebeatOutputIdentity 从默认集群算出。
const testOutputIdentity = "url=http://127.0.0.1:9200\nusername=admin\nverify_tls=false"

// 指纹必须覆盖输出段：主配置 filebeat.yml 由集群地址/账号/TLS 决定，漏掉就会出现
// 「只改集群 → 指纹没变 → 下发被跳过 → 主配置停留在旧值」；同时确认指纹对同一输入稳定
// （否则每次下发都判成漂移、无谓重启 Filebeat）。
func TestRenderFingerprintCoversOutputIdentity(t *testing.T) {
	entries := []hostLogRenderInput{
		{
			Prefix: "autoadmin", Application: "mgmt", Service: "mgmt", ServiceID: 1,
			Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std",
			Pipeline: "rule-a", LogName: "log_error",
			ResolvedPath: "/data/logs/log_error.log", Macros: map[string]string{},
		},
	}
	instances := []hostInstanceInput{{Service: "mgmt", ServiceID: 1, Instance: "nginx-106", HostIP: "192.168.201.106"}}

	base := renderHostLogConfig(entries, instances, testOutputIdentity)
	if len(base.Fragments) == 0 {
		t.Fatal("前置条件不成立：需要产出片段才能比较指纹")
	}
	if again := renderHostLogConfig(entries, instances, testOutputIdentity); again.Fingerprint != base.Fingerprint {
		t.Fatalf("同一输入的指纹必须稳定：%s != %s", again.Fingerprint, base.Fingerprint)
	}

	// 集群地址、账号、TLS 开关任一变化都必须改变指纹。
	for name, identity := range map[string]string{
		"地址变化":   "url=http://10.0.0.9:9200\nusername=admin\nverify_tls=false",
		"账号变化":   "url=http://127.0.0.1:9200\nusername=filebeat\nverify_tls=false",
		"TLS 变化": "url=http://127.0.0.1:9200\nusername=admin\nverify_tls=true",
		"无输出段":   "",
	} {
		if changed := renderHostLogConfig(entries, instances, identity); changed.Fingerprint == base.Fingerprint {
			t.Errorf("%s：指纹未变化（%s），会导致该变更被跳过", name, changed.Fingerprint)
		}
	}

	// 片段本身变化同样要改指纹（回归保护：加入 output 段不能把片段挤掉）。
	mutated := append([]hostLogRenderInput{}, entries...)
	mutated[0].ResolvedPath = "/data/logs/other.log"
	if changed := renderHostLogConfig(mutated, instances, testOutputIdentity); changed.Fingerprint == base.Fingerprint {
		t.Error("片段内容变化未改变指纹")
	}
}

// Filebeat 渲染：每个 服务×日志定义 一个 inputs.d yml，文件内每个实例一个 filestream input；
// 维度字段、索引命名、pipeline、多行语义。
func TestRenderHostLogConfig(t *testing.T) {
	entries := []hostLogRenderInput{
		{
			Prefix: "logs", Application: "tib", Service: "tomcat-svc", ServiceID: 7,
			Project: "kul", Environment: "test", BusinessSystem: "tib",
			Tier: "wuhan-test", Pipeline: "springboot-tomcat-exception",
			LogName: "catalina.out", ResolvedPath: "${APP_HOME}/logs/catalina.out",
			Multiline: true, StartPattern: `\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3}`, FlushTimeout: 3000,
		},
	}
	instances := []hostInstanceInput{
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1", HostIP: "192.168.201.211", Macros: map[string]string{"APP_HOME": "/data/tomcat1"}},
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat2"},
	}
	rendered := renderHostLogConfig(entries, instances, testOutputIdentity)
	if len(rendered.Fragments) != 2 {
		t.Fatalf("fragments = %d, want 2 (inputs.yml + data-keep)", len(rendered.Fragments))
	}
	var inputFragment logConfigFragment
	for _, fragment := range rendered.Fragments {
		if contains(fragment.Path, "inputs.d") {
			inputFragment = fragment
		}
	}
	if inputFragment.Path != "/etc/filebeat/inputs.d/tib__tomcat-svc__7__catalina.out.yml" {
		t.Fatalf("inputs path wrong: %s", inputFragment.Path)
	}
	for _, want := range []string{
		"- type: filestream",
		"  id: 'tib__tomcat-svc__7__catalina.out__tomcat1'",
		"  fields_under_root: true",
		"    service: 'tomcat-svc'",
		"    instance: 'tomcat1'",
		"    application: 'tib'",
		"    log_name: 'catalina.out'",
		"    business_system: 'tib'",
		"    project: 'kul'",
		"    environment: 'test'",
		"    host_ip: '192.168.201.211'",
		// log_path 不再静态注入（静态值会是路径模式），改由 input 级处理器把
		// Filebeat 的真实文件路径拷上来——日志详情要显示具体文件，不是 `*` 模式。
		"  processors:",
		"          - from: log.file.path",
		"            to: log_path",
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
			Prefix: "logs", Application: "tib", Service: "tomcat-svc", ServiceID: 7,
			Project: "kul", Environment: "test", BusinessSystem: "tib",
			Tier: "std", LogName: "catalina.out", ResolvedPath: "${APP_HOME}/logs/catalina.out",
		},
	}
	instances := []hostInstanceInput{
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1"},
	}
	rendered := renderHostLogConfig(entries, instances, testOutputIdentity)
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

// 同一台主机上多个服务：渲染是**主机完整视图**，改/下发服务 B 不会丢服务 A 的片段
// （2026-09-20 现场疑问：A、B 共用主机，改 B 会不会影响 A？）。
// 这也是 agent 侧 `apply_filebeat_config` 的前提：片段目录全量托管、清单外的 *.yml 会被删——
// 如果渲染只带"当前服务"，B 下发时就会把 A 的片段删掉。
func TestRenderHostLogConfigKeepsOtherServicesOnSameHost(t *testing.T) {
	entries := []hostLogRenderInput{
		{Prefix: "autoadmin", Application: "app", Service: "svc-a", ServiceID: 1, Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std", Pipeline: "rule-a", LogName: "a.log", ResolvedPath: "${HOME}/a.log"},
		{Prefix: "autoadmin", Application: "app", Service: "svc-b", ServiceID: 2, Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std", Pipeline: "rule-b", LogName: "b.log", ResolvedPath: "${HOME}/b.log"},
	}
	instances := []hostInstanceInput{
		{Service: "svc-a", ServiceID: 1, Instance: "i1", HostIP: "10.0.0.1", Macros: map[string]string{"HOME": "/srv/a"}},
		{Service: "svc-b", ServiceID: 2, Instance: "i1", HostIP: "10.0.0.1", Macros: map[string]string{"HOME": "/srv/b"}},
	}
	rendered := renderHostLogConfig(entries, instances, testOutputIdentity)
	if len(rendered.Errors) != 0 {
		t.Fatalf("两个服务路径不同，不应有硬问题：%v", rendered.Errors)
	}
	if rendered.ServiceNum != 2 {
		t.Fatalf("service_num = %d, want 2（同一主机上的两个服务都要渲染）", rendered.ServiceNum)
	}
	input := ""
	for _, fragment := range rendered.Fragments {
		if contains(fragment.Path, "inputs.d") {
			input += fragment.Content
		}
	}
	for _, want := range []string{
		"id: 'app__svc-a__1__a.log__i1'",
		"id: 'app__svc-b__2__b.log__i1'",
		"'/srv/a/a.log'",
		"'/srv/b/b.log'",
	} {
		if !contains(input, want) {
			t.Errorf("主机完整渲染缺少 %q：\n%s", want, input)
		}
	}
}

// 服务级子指纹：改服务 B 只让 B 的子指纹变，A 的不变（这样服务视图里 A 不会被 B 的改动带成待下发）。
func TestRenderHostLogConfigServiceFingerprintsIsolateServices(t *testing.T) {
	instances := []hostInstanceInput{
		{Service: "svc-a", ServiceID: 1, Instance: "i1", HostIP: "10.0.0.1", Macros: map[string]string{"HOME": "/srv/a"}},
		{Service: "svc-b", ServiceID: 2, Instance: "i1", HostIP: "10.0.0.1", Macros: map[string]string{"HOME": "/srv/b"}},
	}
	entries := []hostLogRenderInput{
		{Prefix: "autoadmin", Application: "app", Service: "svc-a", ServiceID: 1, Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std", Pipeline: "rule-a", LogName: "a.log", ResolvedPath: "${HOME}/a.log"},
		{Prefix: "autoadmin", Application: "app", Service: "svc-b", ServiceID: 2, Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std", Pipeline: "rule-b", LogName: "b.log", ResolvedPath: "${HOME}/b.log"},
	}
	base := renderHostLogConfig(entries, instances, testOutputIdentity)
	if len(base.ServiceFingerprints) != 2 {
		t.Fatalf("应有两个服务的子指纹，得到 %#v", base.ServiceFingerprints)
	}

	// 只改 B 的 pipeline（内容变）。
	changed := append([]hostLogRenderInput(nil), entries...)
	changed[1].Pipeline = "rule-b2"
	after := renderHostLogConfig(changed, instances, testOutputIdentity)

	if after.ServiceFingerprints["1"] != base.ServiceFingerprints["1"] {
		t.Fatalf("改 B 不应改变 A 的子指纹：before=%s after=%s", base.ServiceFingerprints["1"], after.ServiceFingerprints["1"])
	}
	if after.ServiceFingerprints["2"] == base.ServiceFingerprints["2"] {
		t.Fatalf("改 B 必须改变 B 的子指纹")
	}
	if after.Fingerprint == base.Fingerprint {
		t.Fatalf("主机指纹必须变（整机仍需重新下发）")
	}
}

// .keep 是主机级信号，不能进服务子指纹，否则新增服务 B 会让 A 的子指纹也变。
func TestRenderHostLogConfigServiceFingerprintIgnoresKeep(t *testing.T) {
	instancesOnlyA := []hostInstanceInput{{Service: "svc-a", ServiceID: 1, Instance: "i1", HostIP: "10.0.0.1", Macros: map[string]string{"HOME": "/srv/a"}}}
	entryA := hostLogRenderInput{Prefix: "autoadmin", Application: "app", Service: "svc-a", ServiceID: 1, Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std", Pipeline: "rule-a", LogName: "a.log", ResolvedPath: "${HOME}/a.log"}
	onlyA := renderHostLogConfig([]hostLogRenderInput{entryA}, instancesOnlyA, testOutputIdentity)

	instancesBoth := append([]hostInstanceInput(nil), instancesOnlyA...)
	instancesBoth = append(instancesBoth, hostInstanceInput{Service: "svc-b", ServiceID: 2, Instance: "i1", HostIP: "10.0.0.1", Macros: map[string]string{"HOME": "/srv/b"}})
	entryB := hostLogRenderInput{Prefix: "autoadmin", Application: "app", Service: "svc-b", ServiceID: 2, Project: "p", Environment: "e", BusinessSystem: "b", Tier: "std", Pipeline: "rule-b", LogName: "b.log", ResolvedPath: "${HOME}/b.log"}
	both := renderHostLogConfig([]hostLogRenderInput{entryA, entryB}, instancesBoth, testOutputIdentity)

	if both.ServiceFingerprints["1"] != onlyA.ServiceFingerprints["1"] {
		t.Fatalf("新增 B 不应改变 A 的子指纹（.keep 只算主机级）：onlyA=%s both=%s", onlyA.ServiceFingerprints["1"], both.ServiceFingerprints["1"])
	}
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
	rendered := renderHostLogConfig(entries, instances, testOutputIdentity)
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
	rendered := renderHostLogConfig(entries, instances, testOutputIdentity)
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

// 首行正则必须是 Filebeat 能编译的（Go RE2）：编译不过的片段下发下去，主机上的 Filebeat
// 会因这一行起不来，**整台主机的日志一起停**——比"少采一个文件"严重，所以按本文件既有的
// "跳过并告警"处理，而不是把坏片段照发。
//
// 来源（2026-09-19）：规则 springboot-tomcat-exception 的首行正则写成 Java/JS 的
// `[A-Za-z\u4e00-\u9fa5]`，RE2 编译不过；当时的下发路径不做检查，认证时才报错。
func TestRenderHostLogConfigSkipsUncompilableMultilinePattern(t *testing.T) {
	instances := []hostInstanceInput{
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1", HostIP: "192.168.201.211"},
		{Service: "nginx", ServiceID: 8, Instance: "nginx1", HostIP: "192.168.201.212"},
	}
	entries := []hostLogRenderInput{
		{
			Prefix: "logs", Application: "tib", Service: "tomcat-svc", ServiceID: 7,
			Project: "kul", Environment: "test", BusinessSystem: "tib", Tier: "std",
			Pipeline: "springboot-tomcat-exception", LogName: "catalina.out",
			ResolvedPath: "/data/tomcat1/logs/catalina.out",
			Multiline:    true, StartPattern: `^(\d{4}-\d{2}-\d{2}|\d{2}-[A-Za-z\u4e00-\u9fa5]{3,4}-\d{4})`,
		},
		{
			// 同一个 host 上的另一条日志：坏规则只能影响它自己，不能连坐。
			Prefix: "logs", Application: "tib", Service: "nginx", ServiceID: 8,
			Project: "kul", Environment: "test", BusinessSystem: "tib", Tier: "std",
			Pipeline: "nginx-access", LogName: "access.log",
			ResolvedPath: "/var/log/nginx/access.log",
		},
	}

	rendered := renderHostLogConfig(entries, instances, testOutputIdentity)
	var paths []string
	for _, fragment := range rendered.Fragments {
		paths = append(paths, fragment.Path)
	}
	for _, path := range paths {
		if contains(path, "tomcat-svc") {
			t.Fatalf("首行正则编译不过的片段不应下发：%v", paths)
		}
	}
	if !contains(strings.Join(paths, "\n"), "tib__nginx__8__access.log.yml") {
		t.Fatalf("正常日志仍应下发：%v", paths)
	}
	warnings := strings.Join(rendered.Warnings, "\n")
	for _, want := range []string{"编译不过", "catalina.out", `\u4e00 → \x{4e00}`} {
		if !contains(warnings, want) {
			t.Errorf("warnings 缺少 %q：%s", want, warnings)
		}
	}
}
