package database_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestNoUnreviewedDuplicateParams 扫描生成物里的 `Xxx_2` / `Xxx_3` 这类后缀字段。
//
// 这类字段来自「同一参数在查询里出现多次」：sqlc 按出现次数拆成多个参数，调用点必须把同一个值
// 重复传进去。**漏传不会报错**——字段存在、零值合法，运行时退化成 `LIKE NULL` 之类的静默失效。
// 线上真实故障：assets 域列表按编码/负责人/IP/备注搜索恒返回空（见 SQL_DESIGN §2.5）。
//
// 因此这里维护一份显式清单：清单内的结构体已经人工确认「调用点把值传给了每个后缀字段」，
// 新增后缀字段会让测试失败，逼出这次确认。
func TestNoUnreviewedDuplicateParams(t *testing.T) {
	// 结构体名 -> 已确认调用点逐个传值的原因
	reviewed := map[string]string{
		"CountDeploymentTemplatesParams": "template.go 传 Name 与 Name_2（两条查询各一个 LIKE）",
		"ListDeploymentTemplatesParams":  "template.go 传 Name 与 Name_2",
	}

	fieldPattern := regexp.MustCompile(`(?m)^\s+([A-Za-z][A-Za-z0-9]*_[2-9])\s+\S`)
	structPattern := regexp.MustCompile(`(?m)^type (\w+) struct \{`)

	generatedDir := filepath.Join(packageDir(t), "generated", "mysql")
	entries, err := os.ReadDir(generatedDir)
	if err != nil {
		t.Fatalf("读取生成物目录失败：%v", err)
	}
	scanned := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql.go") {
			continue
		}
		scanned++
		raw, err := os.ReadFile(filepath.Join(generatedDir, entry.Name()))
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", entry.Name(), err)
		}
		source := string(raw)
		for _, location := range structPattern.FindAllStringSubmatchIndex(source, -1) {
			name := source[location[2]:location[3]]
			body := source[location[1]:]
			if end := strings.Index(body, "\n}"); end >= 0 {
				body = body[:end]
			}
			fields := fieldPattern.FindAllStringSubmatch(body, -1)
			if len(fields) == 0 {
				continue
			}
			if _, ok := reviewed[name]; ok {
				continue
			}
			t.Errorf("%s 的生成结构体 %s 出现重复参数字段 %v：\n"+
				"  同一参数在查询里出现多次时 sqlc 会按次数拆成多个参数，调用点漏传就是静默失效。\n"+
				"  请改成「列统一 COALESCE(col,'') + sqlc.narg」合并为一个参数（SQL_DESIGN §2.5）；\n"+
				"  确认调用点逐字段传值后，把这个结构体加入本测试的 reviewed 清单。",
				entry.Name(), name, fieldNames(fields))
		}
	}
	if scanned == 0 {
		t.Fatal("没有扫描到生成物——检查生成路径是否变化")
	}
}

func fieldNames(fields [][]string) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field[1])
	}
	return names
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件位置")
	}
	return filepath.Dir(file)
}
