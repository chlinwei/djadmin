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
	root := findAssetsModuleRoot(t)
	packages := []string{"assets", "monitor", "inspection", "baseline", "automation"}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`h\.agent_id`),
		regexp.MustCompile(`COALESCE\(agent_id`),
		regexp.MustCompile(`assets_host\.agent_id`),
	}
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
