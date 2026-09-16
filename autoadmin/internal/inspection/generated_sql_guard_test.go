package inspection

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// 守护测试：已删除的列（迁移 000005）不得再出现在 inspection 的任何生成 SQL 里。
// CountInspectionTasks 曾漏改（LEFT JOIN t.logical_service_id 残留）导致列表接口
// 500；sqlmock 单测只覆盖被显式 mock 的查询，拦不住同文件其他查询的漏改，
// 所以这里直接扫整个生成文件。
func TestGeneratedInspectionSQLHasNoDroppedColumns(t *testing.T) {
	root := findModuleRoot(t)
	// 两个方言的产物都要扫：MySQL 是当前在跑的，PG 是 -tags postgres 的潜在实现。
	paths := []string{
		filepath.Join(root, "internal", "platform", "database", "generated", "mysql", "inspection.sql.go"),
		filepath.Join(root, "internal", "platform", "database", "generated", "postgres", "inspection.sql.go"),
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read generated file: %v", err)
		}
		content := string(raw)
		for _, pattern := range []string{
			`t\.logical_service_id`,
			`t\.selected_host_ids`,
			`g\.scope`,
			`'scope', g2?\.scope`,
		} {
			if regexp.MustCompile(pattern).MatchString(content) {
				t.Errorf("%s references dropped column: %s", filepath.Base(path), pattern)
			}
		}
	}
}

// findModuleRoot 从测试工作目录向上找 go.mod。
func findModuleRoot(t *testing.T) string {
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
