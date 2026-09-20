package logcollect

import (
	"strings"
	"testing"
)

// 采集过滤的三态与降级（2026-09-19）：过滤是**可选优化**，所以任何解析不出来的情况都要
// 退成"该方向不过滤 + 告警"，绝不能因此让这条日志不采、或让 Filebeat 起不来。
// 反向的错误（白名单落进 exclude 槽却按槽位硬解释）会反转语义、只采噪声，属于丢数据级别。

func filterRule(id int64, name, ruleType, pattern string, enabled bool) logFilterRuleRef {
	return logFilterRuleRef{ID: id, Name: name, RuleType: ruleType, Pattern: pattern, Enabled: enabled}
}

func pointer(value int64) *int64 { return &value }

func TestResolveLogFilterPrefersServiceOverride(t *testing.T) {
	rules := map[int64]logFilterRuleRef{
		1: filterRule(1, "template-include", logFilterRuleInclude, "TEMPLATE", true),
		2: filterRule(2, "service-include", logFilterRuleInclude, "SERVICE", true),
	}
	// 服务级 >0：覆盖模板默认。
	resolved := resolveLogFilter(pointer(1), nil, pointer(2), nil, rules)
	if resolved.Include == nil || resolved.Include.Pattern != "SERVICE" {
		t.Fatalf("服务级应覆盖模板默认：%+v", resolved.Include)
	}
	// 服务级 NULL：继承模板。
	resolved = resolveLogFilter(pointer(1), nil, nil, nil, rules)
	if resolved.Include == nil || resolved.Include.Pattern != "TEMPLATE" {
		t.Fatalf("服务级没配时应继承模板：%+v", resolved.Include)
	}
	// 服务级 0：显式关闭（模板配了也不过过滤）。
	resolved = resolveLogFilter(pointer(1), nil, pointer(0), nil, rules)
	if resolved.Include != nil {
		t.Fatalf("服务级 0 应显式关闭该方向：%+v", resolved.Include)
	}
	// 两层都没有：不过滤。
	if resolved := resolveLogFilter(nil, nil, nil, nil, rules); resolved.Include != nil || resolved.Exclude != nil {
		t.Fatal("两层都没配时应完全不过滤")
	}
}

func TestResolveLogFilterKeepsDirectionsIndependent(t *testing.T) {
	rules := map[int64]logFilterRuleRef{
		1: filterRule(1, "keep-errors", logFilterRuleInclude, "(?i)error", true),
		2: filterRule(2, "drop-noise", logFilterRuleExclude, "(?i)healthcheck", true),
	}
	resolved := resolveLogFilter(pointer(1), pointer(2), nil, nil, rules)
	if resolved.Include == nil || resolved.Exclude == nil {
		t.Fatalf("白名单与黑名单可以同时生效（一条日志两条规则）：%+v", resolved)
	}
	// 只关掉 exclude：include 不受影响。
	resolved = resolveLogFilter(pointer(1), pointer(2), nil, pointer(0), rules)
	if resolved.Include == nil || resolved.Exclude != nil {
		t.Fatalf("两个方向各自独立：%+v", resolved)
	}
}

func TestResolveLogFilterDegradesInsteadOfDroppingLogs(t *testing.T) {
	cases := []struct {
		name       string
		rules      map[int64]logFilterRuleRef
		templateID *int64
		wantNil    bool
		wantWarn   string
	}{
		{
			name:       "规则已被删除：降级为不过滤并告警（不能因为规则没了就不采日志）",
			rules:      map[int64]logFilterRuleRef{},
			templateID: pointer(99),
			wantNil:    true,
			wantWarn:   "已不存在",
		},
		{
			name:       "规则被停用：静默不生效（停用就是想让它不起作用）",
			rules:      map[int64]logFilterRuleRef{5: filterRule(5, "paused", logFilterRuleInclude, "x", false)},
			templateID: pointer(5),
			wantNil:    true,
		},
		{
			name: "方向不符：必须拦住并告警（白名单落进 exclude 槽会只采噪声）",
			// 规则声明是 include，却被放在 exclude 方向：按槽位硬解释会把语义反转。
			rules:      map[int64]logFilterRuleRef{7: filterRule(7, "keep-errors", logFilterRuleInclude, "(?i)error", true)},
			templateID: pointer(7),
			wantNil:    true,
			wantWarn:   "为避免语义反转已忽略该方向",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// 一律放在 exclude 方向来跑，正好覆盖"方向不符"那条。
			resolved := resolveLogFilter(nil, testCase.templateID, nil, nil, testCase.rules)
			if testCase.wantNil && resolved.Exclude != nil {
				t.Fatalf("应降级为不过滤，实际拿到 %+v", resolved.Exclude)
			}
			if testCase.wantWarn != "" {
				joined := strings.Join(resolved.Warnings, "；")
				if !strings.Contains(joined, testCase.wantWarn) {
					t.Fatalf("告警缺少 %q：%s", testCase.wantWarn, joined)
				}
			}
		})
	}
}

func TestValidFilterPatternRejectsBadPatterns(t *testing.T) {
	// 空正则：include 会放行全部、exclude 会丢弃全部，必须降级（不是"过滤生效"）。
	if _, warning := validFilterPattern(logFilterRuleExclude, filterRule(1, "empty", logFilterRuleExclude, "   ", true)); !strings.Contains(warning, "为空") {
		t.Fatalf("空正则要被拒绝并说明原因：%s", warning)
	}
	// 编译不过（RE2 不认 \u）：降级 + 说明，绝不让它进 Filebeat 配置（否则 Filebeat 起不来）。
	if _, warning := validFilterPattern(logFilterRuleInclude, filterRule(2, "java-style", logFilterRuleInclude, `\u4e00`, true)); !strings.Contains(warning, "编译不过") {
		t.Fatalf("编译不过要被降级并说明原因：%s", warning)
	}
	pattern, warning := validFilterPattern(logFilterRuleInclude, filterRule(3, "ok", logFilterRuleInclude, `(?i)(error|fatal)`, true))
	if warning != "" || pattern != `(?i)(error|fatal)` {
		t.Fatalf("正常正则原样返回：pattern=%q warning=%q", pattern, warning)
	}
}

// 渲染出来的片段要能被 Filebeat 读懂：include_lines / exclude_lines 是 input 内的列表，
// 正则用单引号包裹（YAML 单引号里反斜杠不转义，\d 保持原样），多个方向可同时出现。
func TestRenderEmitsIncludeAndExcludeLines(t *testing.T) {
	entries := []hostLogRenderInput{
		{
			Prefix: "logs", Application: "tib", Service: "tomcat-svc", ServiceID: 7,
			Project: "kul", Environment: "test", BusinessSystem: "tib", Tier: "std",
			Pipeline: "springboot-tomcat-exception", LogName: "catalina.out",
			ResolvedPath:         "/data/tomcat1/logs/catalina.out",
			FilterIncludePattern: `(?i)(error|fatal|failure)`,
			FilterExcludePattern: `(?i)healthcheck`,
		},
	}
	instances := []hostInstanceInput{{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1", HostIP: "192.168.201.211"}}
	rendered := renderHostLogConfig(entries, instances, testOutputIdentity)

	var content string
	for _, fragment := range rendered.Fragments {
		if strings.Contains(fragment.Path, "inputs.d") {
			content = fragment.Content
		}
	}
	for _, want := range []string{
		"  include_lines:",
		"    - '(?i)(error|fatal|failure)'",
		"  exclude_lines:",
		"    - '(?i)healthcheck'",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("片段缺少 %q：\n%s", want, content)
		}
	}
	// 没配过滤的日志不能凭空多出这两项（否则 Filebeat 会按"空模式匹配全部"处理）。
	withoutFilter := renderHostLogConfig([]hostLogRenderInput{{
		Prefix: "logs", Application: "tib", Service: "redis", ServiceID: 9, Project: "kul",
		Environment: "test", BusinessSystem: "tib", Tier: "std", Pipeline: "redis-log",
		LogName: "redis.log", ResolvedPath: "/var/log/redis/redis.log",
	}}, []hostInstanceInput{{Service: "redis", ServiceID: 9, Instance: "redis1", HostIP: "10.0.0.9"}}, testOutputIdentity)
	for _, fragment := range withoutFilter.Fragments {
		if strings.Contains(fragment.Content, "include_lines") || strings.Contains(fragment.Content, "exclude_lines") {
			t.Errorf("没配过滤时不该出现过滤配置：\n%s", fragment.Content)
		}
	}
}

// 多实例路径：不同实例展开出不同路径是**正常且必须支持**的（每个实例一个 filestream input），
// 但同一主机上**同一个绝对路径**只能被一个 input 监听——两个 input 各自维护 offset，
// 同一份日志会进 ES 两次（instance 字段不同），错误聚类与容量统计跟着翻倍。
// Django 版有这条守卫（seen_paths → "日志路径冲突"），迁 Go 时丢了，2026-09-19 补回。
func TestRenderKeepsDistinctPathsAndSkipsDuplicateOnSameHost(t *testing.T) {
	// 真实数据里 entry 是"每个服务×日志定义一条"（查询去重），实例级差异只在 instance.Macros 里。
	entries := []hostLogRenderInput{
		{
			Prefix: "logs", Application: "tib", Service: "tomcat-svc", ServiceID: 7,
			Project: "kul", Environment: "test", BusinessSystem: "tib", Tier: "std",
			Pipeline: "springboot-tomcat-exception", LogName: "catalina.out",
			ResolvedPath: "${APP_HOME}/logs/catalina.out",
		},
	}
	differentPaths := renderHostLogConfig(entries, []hostInstanceInput{
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1", HostIP: "10.0.0.1", Macros: map[string]string{"APP_HOME": "/home/esb/tomcat1"}},
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat2", HostIP: "10.0.0.1", Macros: map[string]string{"APP_HOME": "/home/esb/tomcat2"}},
	}, testOutputIdentity)
	var content string
	for _, fragment := range differentPaths.Fragments {
		if strings.Contains(fragment.Path, "inputs.d") {
			content = fragment.Content
		}
	}
	for _, want := range []string{
		"'/home/esb/tomcat1/logs/catalina.out'",
		"'/home/esb/tomcat2/logs/catalina.out'",
		"id: 'tib__tomcat-svc__7__catalina.out__tomcat1'",
		"id: 'tib__tomcat-svc__7__catalina.out__tomcat2'",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("缺少 %q：\n%s", want, content)
		}
	}
	if len(differentPaths.Warnings) != 0 {
		t.Fatalf("路径不同不该有任何告警：%v", differentPaths.Warnings)
	}

	// 两个实例配成同一个 APP_HOME（或都没配、都落到模板默认）→ 展开成同一路径：
	// 只保留先出现的那条，并告警说清两侧是谁。
	samePath := renderHostLogConfig(entries, []hostInstanceInput{
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1", HostIP: "10.0.0.1", Macros: map[string]string{"APP_HOME": "/home/esb/tomcat"}},
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat2", HostIP: "10.0.0.1", Macros: map[string]string{"APP_HOME": "/home/esb/tomcat"}},
	}, testOutputIdentity)
	var sameContent string
	for _, fragment := range samePath.Fragments {
		if strings.Contains(fragment.Path, "inputs.d") {
			sameContent = fragment.Content
		}
	}
	if strings.Count(sameContent, "paths:") != 1 {
		t.Fatalf("同一路径只应保留一条 input：\n%s", sameContent)
	}
	joined := strings.Join(samePath.Warnings, "\n")
	for _, want := range []string{"展开成同一路径", "tomcat1", "tomcat2", "重复读取"} {
		if !strings.Contains(joined, want) {
			t.Errorf("告警缺少 %q：%s", want, joined)
		}
	}
}

// 保存逻辑服务时的自洽性判据：**只认"硬问题"**（配置不自洽、只能回配置里改的），
// 软告警（未挂解析规则所以不采集之类，按约定允许存在）绝不能挡保存——否则"先建模板、后纳管"
// 这种正常顺序都会被拦住。
func TestCollectLogConfigProblemsOnlyReportsHardProblems(t *testing.T) {
	base := hostLogRenderInput{
		Prefix: "logs", Application: "tib", Service: "tomcat-svc", ServiceID: 7,
		Project: "kul", Environment: "test", BusinessSystem: "tib", Tier: "std",
		Pipeline: "springboot-tomcat-exception", LogName: "catalina.out",
		ResolvedPath: "${APP_HOME}/logs/catalina.out",
	}
	instances := []hostInstanceInput{
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1", HostIP: "10.0.0.1", Macros: map[string]string{"APP_HOME": "/home/esb/tomcat"}},
		{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat2", HostIP: "10.0.0.1", Macros: map[string]string{"APP_HOME": "/home/esb/tomcat"}},
	}

	// 1) 两个实例展开成同一路径 → 硬问题（重复采集，数据翻倍）。
	conflict := collectLogConfigProblems(map[int64]renderInputSet{
		1: {Entries: []hostLogRenderInput{base}, Instances: instances},
	}, []int64{1})
	if len(conflict) == 0 || !strings.Contains(strings.Join(conflict, "；"), "展开成同一路径") {
		t.Fatalf("同一路径冲突必须算硬问题：%v", conflict)
	}

	// 2) 未定义宏（实例没给 APP_HOME，模板也没给）→ 硬问题（那台主机采集不了）。
	missingMacro := collectLogConfigProblems(map[int64]renderInputSet{
		1: {Entries: []hostLogRenderInput{base}, Instances: []hostInstanceInput{
			{Service: "tomcat-svc", ServiceID: 7, Instance: "tomcat1", HostIP: "10.0.0.1"},
		}},
	}, []int64{1})
	if len(missingMacro) == 0 || !strings.Contains(strings.Join(missingMacro, "；"), "含未定义宏") {
		t.Fatalf("路径展不开必须算硬问题：%v", missingMacro)
	}

	// 3) 未挂解析规则（按约定不采集）→ **软告警**，不能挡保存。
	noPipeline := base
	noPipeline.Pipeline = ""
	soft := collectLogConfigProblems(map[int64]renderInputSet{
		1: {Entries: []hostLogRenderInput{noPipeline}, Instances: instances[:1]},
	}, []int64{1})
	if len(soft) != 0 {
		t.Fatalf("未挂规则只是「不采集」的软告警，不该挡住保存：%v", soft)
	}

	// 4) 配置正常 → 没有硬问题。
	clean := base
	clean.ResolvedPath = "/home/esb/tomcat/logs/catalina.out"
	if problems := collectLogConfigProblems(map[int64]renderInputSet{
		1: {Entries: []hostLogRenderInput{clean}, Instances: instances[:1]},
	}, []int64{1}); len(problems) != 0 {
		t.Fatalf("正常配置不该报问题：%v", problems)
	}
}
