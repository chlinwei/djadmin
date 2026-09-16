package derive

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "重新写出 db/queries/postgres 的派生结果（make derive）")

// TestDerivedQueriesMatchRepository 断言 db/queries/postgres 的内容 == 现场重新派生的结果。
//
// PG 侧是产物而不是并行维护的第二份源：手改产物会让两侧悄悄分叉，
// 而分叉只会在运行到那条查询时才暴露。用这条测试把漂移挡在提交前。
func TestDerivedQueriesMatchRepository(t *testing.T) {
	mysqlDir, postgresDir := queryDirs(t)
	files, err := Derive(mysqlDir)
	if err != nil {
		t.Fatalf("派生失败：%v", err)
	}
	if *update {
		if err := WriteAll(postgresDir, files); err != nil {
			t.Fatalf("写出派生结果失败：%v", err)
		}
		t.Logf("已写出 %d 个派生文件到 %s", len(files), postgresDir)
		return
	}
	if len(files) == 0 {
		t.Fatal("派生结果为空——检查 db/queries/mysql 是否有查询文件")
	}
	for _, file := range files {
		onDisk, err := os.ReadFile(filepath.Join(postgresDir, file.Name))
		if err != nil {
			t.Errorf("%s：产物缺失或不可读（跑 make derive 生成）：%v", file.Name, err)
			continue
		}
		if string(onDisk) != file.Content {
			t.Errorf("%s 与派生结果不一致：PG 侧禁止手改，请改 MySQL 源后跑 make derive\n%s",
				file.Name, firstDifference(string(onDisk), file.Content))
		}
	}
	// 反向检查：产物目录里多出来的 .sql 说明源里已删掉对应文件，产物成了孤儿。
	onDisk, err := sqlFileNames(postgresDir)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", postgresDir, err)
	}
	derived := make(map[string]bool, len(files))
	for _, file := range files {
		derived[file.Name] = true
	}
	for _, name := range onDisk {
		if !derived[name] {
			t.Errorf("%s 在 db/queries/mysql 里没有对应源文件，是孤儿产物（make derive 会删除）", name)
		}
	}
}

func TestDeriveIsDeterministic(t *testing.T) {
	mysqlDir, _ := queryDirs(t)
	first, err := Derive(mysqlDir)
	if err != nil {
		t.Fatalf("派生失败：%v", err)
	}
	second, err := Derive(mysqlDir)
	if err != nil {
		t.Fatalf("第二次派生失败：%v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("两次派生的文件数不同：%d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("%s 两次派生结果不同，派生过程不稳定", first[i].Name)
		}
	}
}

// 自增主键的取回方式必须分叉：PG 侧 `:execlastid` / INSERT 的 `:execresult` → `:one` + `RETURNING id`。
// 漏掉这条改写的话，PG 变体下每一个新建接口都会在运行时抛
// `LastInsertId is not supported by this driver`（pgx 不实现 LastInsertId）。
func TestDeriveRewritesLastInsertIDToReturning(t *testing.T) {
	source := "-- name: CreateThing :execlastid\n" +
		"INSERT INTO thing(create_time, name) VALUES (?, ?);\n" +
		"\n" +
		"-- name: CreateOther :execresult\n" +
		"INSERT INTO thing(create_time, name) VALUES (?, ?);\n" +
		"\n" +
		"-- name: ClaimThing :execresult\n" +
		"UPDATE thing SET status = ? WHERE id = ? AND status = ?;\n" +
		"\n" +
		"-- name: ListThings :many\n" +
		"SELECT id FROM thing WHERE name = ?;\n"
	got, err := deriveFile("thing.sql", source, map[string]bool{})
	if err != nil {
		t.Fatalf("派生失败：%v", err)
	}
	for _, want := range []string{
		"-- name: CreateThing :one\n",
		"-- name: CreateOther :one\n",
		// UPDATE 的 :execresult 保持原样（靠 RowsAffected，PG 侧可用）。
		"-- name: ClaimThing :execresult\n",
		"-- name: ListThings :many\n",
		"VALUES ($1, $2)\nRETURNING id;\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("派生结果缺少 %q：\n%s", want, got)
		}
	}
	// 只有两条 INSERT 被追加 RETURNING。
	if count := strings.Count(got, "RETURNING id"); count != 2 {
		t.Errorf("RETURNING 应只出现在两条 INSERT 上（实得 %d）：\n%s", count, got)
	}
	if strings.Contains(got, "VALUES ($1, $2);") {
		t.Errorf("INSERT 语句尾的分号应被换成 RETURNING：\n%s", got)
	}
}

// 多行 INSERT 取不回每行的 id：必须在派生期报错，而不是产出只会取到首行 id 的 SQL。
func TestDeriveRejectsMultiRowInsertWithReturning(t *testing.T) {
	source := "-- name: CreateThings :execlastid\n" +
		"INSERT INTO thing(name) VALUES (?), (?);\n"
	if _, err := deriveFile("thing.sql", source, map[string]bool{}); err == nil {
		t.Fatal("多行 INSERT + execlastid 应报错")
	}
}

func queryDirs(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件位置")
	}
	// 本文件位于 <repo>/autoadmin/internal/platform/database/derive/
	autoadmin := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	return filepath.Join(autoadmin, "db", "queries", "mysql"),
		filepath.Join(autoadmin, "db", "queries", "postgres")
}

// firstDifference 给出首个不同行的上下文，避免整文件 dump 把失败信息淹掉。
func firstDifference(onDisk, derived string) string {
	diskLines := strings.Split(onDisk, "\n")
	derivedLines := strings.Split(derived, "\n")
	for i := 0; i < len(diskLines) || i < len(derivedLines); i++ {
		var diskLine, derivedLine string
		if i < len(diskLines) {
			diskLine = diskLines[i]
		}
		if i < len(derivedLines) {
			derivedLine = derivedLines[i]
		}
		if diskLine != derivedLine {
			return "首次差异在第 " + strconv.Itoa(i+1) + " 行：\n  产物: " + clip(diskLine) + "\n  派生: " + clip(derivedLine)
		}
	}
	return "内容长度不同但逐行相同（行尾符号不一致）"
}
