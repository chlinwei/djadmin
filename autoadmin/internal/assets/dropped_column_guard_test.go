package assets

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 守卫测试：assets_host.agent_id 列已由迁移 000020 删除，主机身份只有
// assets_host.instance_name。内联 SQL 里若再出现读该列，编译期不会报错，
// 只会在真实查询时抛 `Unknown column 'agent_id'`——所以这里直接扫源码拦下来。
//
// 覆盖曾经读过该列的包；测试文件不扫（sqlmock 断言要求与真实 SQL 逐字一致，
// SQL 写错时那些用例本身就会失败）。
func TestNoInlineSQLReferencesDroppedHostAgentIDColumn(t *testing.T) {
	assertNoSourceMatches(t,
		[]string{"assets", "monitor", "inspection", "baseline", "automation"},
		[]*regexp.Regexp{
			regexp.MustCompile(`h\.agent_id`),
			regexp.MustCompile(`COALESCE\(agent_id`),
			regexp.MustCompile(`assets_host\.agent_id`),
		})
}

// 守卫测试：assets_application_log_definition.collection_enabled 已由迁移 000034 删除
// （采集开关下沉到逻辑服务，见 docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §6）。
// 这条列在渲染查询里曾写成 JOIN 条件 `ld.collection_enabled = TRUE`，若内联 SQL 里残留，
// 同样是"编译通过、运行时 Unknown column"。sqlc 产物由 schema 保证，这里只管内联 SQL。
func TestNoInlineSQLReferencesDroppedLogDefinitionCollectionSwitch(t *testing.T) {
	assertNoSourceMatches(t,
		[]string{"assets", "monitor", "inspection", "baseline", "automation", "logcollect"},
		[]*regexp.Regexp{
			regexp.MustCompile(`ld\.collection_enabled`),
			regexp.MustCompile(`log_definition\.collection_enabled`),
		})
}

// 守卫测试：assets_application_service_log_setting.processing_rule_id 已由迁移 000035 删除
// （解析规则只由部署模板的日志定义决定，服务侧只读，见
// docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §6）。这条列在渲染查询里曾以
// `rule_setting` 别名 JOIN 进来做 COALESCE，残留同样是"编译通过、运行时 Unknown column"。
func TestNoInlineSQLReferencesDroppedServiceLogRuleOverride(t *testing.T) {
	assertNoSourceMatches(t,
		[]string{"assets", "monitor", "inspection", "baseline", "automation", "logcollect"},
		[]*regexp.Regexp{
			regexp.MustCompile(`ls\.processing_rule_id`),
			regexp.MustCompile(`rule_setting\.`),
		})
}

// assertNoSourceMatches 扫这些包下所有非测试 go 源码，命中任一正则即报错。
func assertNoSourceMatches(t *testing.T, packages []string, patterns []*regexp.Regexp) {
	t.Helper()
	root := findAssetsModuleRoot(t)
	for _, pkg := range packages {
		dir := filepath.Join(root, "internal", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			content := string(raw)
			for _, pattern := range patterns {
				if match := pattern.FindString(content); match != "" {
					t.Errorf("internal/%s/%s 仍引用已删除的列: %q", pkg, name, match)
				}
			}
		}
	}
}

// findAssetsModuleRoot 从测试工作目录向上找 go.mod。
func findAssetsModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found")
	return ""
}
