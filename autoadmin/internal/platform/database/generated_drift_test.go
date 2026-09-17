package database_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestGeneratedCodeMatchesQueries 是本仓库"生成物漂移"的守卫（P3-2）。
//
// 背景：`internal/platform/database/generated/` 下的 sqlc 产物过去靠手工与 `.sql` 同步，
// 已经漂移过一次（BUG_SQLC_NULLABLE_FILTER.md 教训 2）。只跑 `make generate` 并不能
// 发现问题——真正的风险是"有人改了查询却没重新生成"，而生成物照样能编译。
//
// 做法：把 `db/schema`、`db/queries` 复制到临时目录，用同一份 `sqlc.yaml` 生成到
// 临时目录，再与仓库里已提交的产物逐字节比对。不改动工作区、不依赖数据库，
// 且走的是与 `make generate` 相同的 `go tool sqlc`（版本由 go.mod 的 tool 指令锁定）。
//
// 快速跳过：`go test -short`。
func TestGeneratedCodeMatchesQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("-short：跳过需要运行 sqlc 的生成物漂移检查")
	}
	autoadmin := autoadminRoot(t)

	sandbox := t.TempDir()
	for _, dir := range []string{"schema", "queries"} {
		if err := copyTree(filepath.Join(autoadmin, "db", dir), filepath.Join(sandbox, dir)); err != nil {
			t.Fatalf("复制 db/%s 到沙箱失败：%v", dir, err)
		}
	}

	config, err := os.ReadFile(filepath.Join(autoadmin, "sqlc.yaml"))
	if err != nil {
		t.Fatalf("读取 sqlc.yaml 失败：%v", err)
	}
	// sqlc 把配置里的相对路径解析为"相对于配置文件所在目录"，所以复制到沙箱后
	// 只需把三个路径族重写到沙箱内的等价位置。
	rewrites := []string{
		`"db/schema/mysql"`, `"schema/mysql"`,
		`"db/schema/postgres"`, `"schema/postgres"`,
		`"db/queries/mysql"`, `"queries/mysql"`,
		`"db/queries/postgres"`, `"queries/postgres"`,
		`"internal/platform/database/generated/mysql"`, `"out/mysql"`,
		`"internal/platform/database/generated/postgres"`, `"out/postgres"`,
	}
	sandboxConfig := strings.NewReplacer(rewrites...).Replace(string(config))
	configPath := filepath.Join(sandbox, "sqlc.yaml")
	if err := os.WriteFile(configPath, []byte(sandboxConfig), 0o644); err != nil {
		t.Fatalf("写入沙箱 sqlc.yaml 失败：%v", err)
	}

	generate := exec.Command("go", "tool", "sqlc", "generate", "-f", configPath)
	generate.Dir = autoadmin
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("sqlc generate 失败：%v\n%s", err, output)
	}

	for _, dialect := range []string{"mysql", "postgres"} {
		compareGeneratedTree(t, dialect,
			filepath.Join(sandbox, "out", dialect),
			filepath.Join(autoadmin, "internal", "platform", "database", "generated", dialect))
	}
}

// compareGeneratedTree 逐文件比对沙箱新生成的产物与仓库已提交的产物，并反向检查孤儿文件。
func compareGeneratedTree(t *testing.T, dialect, freshly, committed string) {
	t.Helper()
	freshFiles, err := treeFiles(freshly)
	if err != nil {
		t.Fatalf("[%s] 读取新生成产物失败：%v", dialect, err)
	}
	committedFiles, err := treeFiles(committed)
	if err != nil {
		t.Fatalf("[%s] 读取已提交产物失败：%v", dialect, err)
	}

	committedSet := make(map[string]bool, len(committedFiles))
	for _, name := range committedFiles {
		committedSet[name] = true
	}
	freshSet := make(map[string]bool, len(freshFiles))
	for _, name := range freshFiles {
		freshSet[name] = true
	}

	for _, name := range freshFiles {
		if !committedSet[name] {
			t.Errorf("[%s] %s 是新增产物但未提交（跑 make generate）", dialect, name)
			continue
		}
		want, err := os.ReadFile(filepath.Join(freshly, name))
		if err != nil {
			t.Errorf("[%s] 读取新生成 %s 失败：%v", dialect, name, err)
			continue
		}
		got, err := os.ReadFile(filepath.Join(committed, name))
		if err != nil {
			t.Errorf("[%s] 读取已提交 %s 失败：%v", dialect, name, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("[%s] %s 与 db/queries + db/schema 重新生成的结果不一致："+
				"改了查询/schema 却没跑 make generate（或反过来），产物漂移。\n%s",
				dialect, name, firstLineDifference(string(got), string(want)))
		}
	}
	for _, name := range committedFiles {
		if !freshSet[name] {
			t.Errorf("[%s] %s 是孤儿产物（查询/schema 里已无对应源），跑 make generate 会删除", dialect, name)
		}
	}
}

// treeFiles 返回目录下所有文件的相对路径（排序）。
func treeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	sort.Strings(files)
	return files, err
}

// copyTree 递归复制目录，保留文件内容（权限统一为 0644）。
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// firstLineDifference 给出首个不同行的上下文，避免整文件 dump 淹掉失败信息。
func firstLineDifference(got, want string) string {
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		var gotLine, wantLine string
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if gotLine != wantLine {
			return "首次差异在第 " + strconv.Itoa(i+1) + " 行：\n  已提交: " + clipLine(gotLine) + "\n  新生成: " + clipLine(wantLine)
		}
	}
	return "内容长度不同但逐行相同（行尾符号不一致）"
}

func clipLine(line string) string {
	const max = 200
	if len(line) > max {
		return line[:max] + "…"
	}
	return line
}

// autoadminRoot 定位 <repo>/autoadmin。
func autoadminRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件位置")
	}
	// 本文件位于 <repo>/autoadmin/internal/platform/database/
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}
