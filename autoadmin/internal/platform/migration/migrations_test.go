package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestPostgresMigrationsMirrorMySQLVersions 守住"两侧迁移版本一一对应"（P1-7 的验收项之一）。
//
// 为什么要有它：PG 侧迁移是从 MySQL 侧翻译出来的增量，版本号必须逐个对齐 —— 两侧错位意味着
// "同一个版本号在两库上做的是不同的事"，而 golang-migrate 只看版本号，不会发现语义错位。
// 这条守卫不需要数据库，所以每次 go test 都会跑；**迁移内容**的正确性靠另一条证据：
// 在真 PG 上"从折叠 schema 反向回放 down、再正向回放 up"，累积效果必须与
// db/schema/postgres 完全一致（做法与实测结果见 SQL_DESIGN §4.7 与计划 P1-7）。
func TestPostgresMigrationsMirrorMySQLVersions(t *testing.T) {
	mysqlDir, postgresDir := migrationDirs(t)
	mysql := migrationFileNames(t, mysqlDir)
	postgres := migrationFileNames(t, postgresDir)

	if len(mysql) == 0 {
		t.Fatal("没有找到 mysql 迁移文件——检查目录是否变化")
	}
	if len(mysql) != len(postgres) {
		t.Fatalf("两侧迁移文件数不同：mysql=%d postgres=%d\n  只在 mysql: %v\n  只在 postgres: %v",
			len(mysql), len(postgres), missing(mysql, postgres), missing(postgres, mysql))
	}
	for index := range mysql {
		if mysql[index] != postgres[index] {
			t.Fatalf("第 %d 个迁移文件名不一致：mysql=%s postgres=%s", index, mysql[index], postgres[index])
		}
	}

	// 版本号必须唯一且 up/down 成对（golang-migrate 要求同一版本只能有一对）。
	paired := map[int64]map[string]bool{}
	versionPattern := regexp.MustCompile(`^(\d{6})_.+\.(up|down)\.sql$`)
	for _, name := range mysql {
		match := versionPattern.FindStringSubmatch(name)
		if match == nil {
			t.Fatalf("迁移文件名不符合 NNNNNN_name.(up|down).sql：%s", name)
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			t.Fatalf("解析版本号失败 %s：%v", name, err)
		}
		if paired[version] == nil {
			paired[version] = map[string]bool{}
		}
		if paired[version][match[2]] {
			t.Fatalf("版本 %d 的 %s 迁移重复：%s", version, match[2], name)
		}
		paired[version][match[2]] = true
	}
	for version, directions := range paired {
		if !directions["up"] || !directions["down"] {
			t.Fatalf("版本 %d 的 up/down 不成对：%v", version, directions)
		}
	}
}

func migrationDirs(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件位置")
	}
	// 本文件位于 <repo>/autoadmin/internal/platform/migration/
	autoadmin := filepath.Join(filepath.Dir(file), "..", "..", "..")
	return filepath.Join(autoadmin, "db", "migrations", "mysql"),
		filepath.Join(autoadmin, "db", "migrations", "postgres")
}

func migrationFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func missing(from, against []string) []string {
	present := make(map[string]bool, len(against))
	for _, name := range against {
		present[name] = true
	}
	diff := make([]string, 0)
	for _, name := range from {
		if !present[name] {
			diff = append(diff, name)
		}
	}
	return diff
}
