package database_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// schemaColumn 是 db/schema/mysql 里一列的快照（只保留 INSERT 完整性校验需要的属性）。
type schemaColumn struct {
	Name          string
	NotNull       bool
	HasDefault    bool
	AutoIncrement bool
}

// Required 表示"INSERT 必须显式给值"的列：NOT NULL、库级无默认值、且不是自增列
// （自增列由数据库生成，information_schema 里 COLUMN_DEFAULT 也是 NULL，必须排除）。
func (c schemaColumn) Required() bool {
	return c.NotNull && !c.HasDefault && !c.AutoIncrement
}

// TestInsertStatementsCoverRequiredColumns 是本仓库 INSERT 列集完整性的守卫（P4-10）。
//
// 背景：`monitor_opensearch_cluster` 的建集群 INSERT 漏写了三列 NOT NULL 且无默认值的列，
// 在严格模式下恒报 1364（P5 陷阱 27）。这类错误在宽松模式下会静默写错值，编译器与
// sqlc 都不会拦。守卫的做法是**纯文本解析**（不连数据库）：
//   - 从 `db/schema/mysql/*.sql` 的 CREATE TABLE 解析每列的 NOT NULL / DEFAULT / AUTO_INCREMENT；
//   - 从 `db/queries/mysql/*.sql` 解析每条 INSERT 的显式列表；
//   - 任何一条 INSERT 缺少其目标表的"必填列"即失败。
//
// 局限：只校验**显式列了列名**的 INSERT；`INSERT INTO t VALUES(...)` 不含列名，无法校验。
func TestInsertStatementsCoverRequiredColumns(t *testing.T) {
	root := autoadminRoot(t)
	schema, err := parseSchemaColumns(filepath.Join(root, "db", "schema", "mysql"))
	if err != nil {
		t.Fatalf("解析 schema 失败：%v", err)
	}
	inserts, err := parseInserts(filepath.Join(root, "db", "queries", "mysql"))
	if err != nil {
		t.Fatalf("解析查询失败：%v", err)
	}
	if len(schema) == 0 || len(inserts) == 0 {
		t.Fatalf("解析结果为空（schema %d 表 / insert %d 条），解析器大概率失效", len(schema), len(inserts))
	}

	checked := 0
	for _, insert := range inserts {
		columns, ok := schema[insert.Table]
		if !ok {
			t.Errorf("%s:%d INSERT 的目标表 %s 在 db/schema/mysql 里没有定义",
				insert.File, insert.Line, insert.Table)
			continue
		}
		if insert.Columns == nil {
			continue // 没列列名的 INSERT（或解析不到列表）：无法校验
		}
		present := make(map[string]bool, len(insert.Columns))
		for _, name := range insert.Columns {
			present[name] = true
		}
		var missing []string
		for _, column := range columns {
			if column.Required() && !present[column.Name] {
				missing = append(missing, column.Name)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("%s:%d INSERT INTO %s 缺列 %v —— 这些列 NOT NULL 且无库级默认值，"+
				"严格模式下报 1364（P5 陷阱 27）",
				insert.File, insert.Line, insert.Table, missing)
		}
		checked++
	}
	t.Logf("已校验 %d 条含显式列名的 INSERT（共解析到 %d 条）", checked, len(inserts))
}

type insertStatement struct {
	File    string
	Line    int
	Table   string
	Columns []string // nil 表示未列列名
}

var (
	createTableRe = regexp.MustCompile("(?i)^\\s*CREATE TABLE\\s+`?([A-Za-z0-9_]+)`?\\s*\\(\\s*$")
	insertIntoRe  = regexp.MustCompile("(?i)\\bINSERT\\s+INTO\\s+`?([A-Za-z0-9_]+)`?")
)

// parseSchemaColumns 解析 CREATE TABLE，返回 表名 → 列列表。
func parseSchemaColumns(dir string) (map[string][]schemaColumn, error) {
	files, err := sqlFiles(dir)
	if err != nil {
		return nil, err
	}
	tables := make(map[string][]schemaColumn)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var current string
		for _, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimRight(raw, "\r")
			trimmed := strings.TrimSpace(line)
			if current == "" {
				if match := createTableRe.FindStringSubmatch(line); match != nil {
					current = match[1]
				}
				continue
			}
			// CREATE TABLE 体内：注释与空行跳过，遇到顶层 `)` 结束。
			if trimmed == "" || strings.HasPrefix(trimmed, "--") {
				continue
			}
			if strings.HasPrefix(trimmed, ")") {
				current = ""
				continue
			}
			if !strings.HasPrefix(trimmed, "`") {
				continue // PRIMARY KEY / KEY / CONSTRAINT 等表级约束
			}
			column, ok := parseColumnLine(trimmed)
			if !ok {
				return nil, fmt.Errorf("%s: 无法解析列定义：%s", path, trimmed)
			}
			tables[current] = append(tables[current], column)
		}
	}
	return tables, nil
}

// parseColumnLine 解析形如 “ `name` int NOT NULL AUTO_INCREMENT, “ 的一行列定义。
// 约定：每个列定义占一行（本仓库 schema 快照满足）。
func parseColumnLine(line string) (schemaColumn, bool) {
	end := strings.Index(line[1:], "`")
	if end < 0 {
		return schemaColumn{}, false
	}
	name := line[1 : 1+end]
	definition := line[1+end+1:]
	upper := strings.ToUpper(definition)
	return schemaColumn{
		Name:          name,
		NotNull:       strings.Contains(upper, "NOT NULL"),
		HasDefault:    strings.Contains(upper, "DEFAULT") || strings.Contains(upper, "GENERATED"),
		AutoIncrement: strings.Contains(upper, "AUTO_INCREMENT"),
	}, true
}

// parseInserts 解析所有 INSERT INTO 语句的显式列列表。
func parseInserts(dir string) ([]insertStatement, error) {
	files, err := sqlFiles(dir)
	if err != nil {
		return nil, err
	}
	var inserts []insertStatement
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		text := stripLineComments(string(data))
		for _, loc := range insertIntoRe.FindAllStringSubmatchIndex(text, -1) {
			table := text[loc[2]:loc[3]]
			rest := text[loc[1]:]
			columns := parseColumnList(rest)
			line := 1 + strings.Count(text[:loc[0]], "\n")
			inserts = append(inserts, insertStatement{
				File:    filepath.Base(path),
				Line:    line,
				Table:   table,
				Columns: columns,
			})
		}
	}
	return inserts, nil
}

// parseColumnList 在 INSERT INTO <table> 之后读取可选的 `(col, col, ...)` 列表。
// 没有括号（如 `INSERT INTO t VALUES ...`）返回 nil。
func parseColumnList(rest string) []string {
	rest = strings.TrimLeft(rest, " \t\r\n")
	if !strings.HasPrefix(rest, "(") {
		return nil
	}
	depth := 0
	end := -1
	for i, r := range rest {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil
	}
	inner := rest[1:end]
	parts := strings.Split(inner, ",")
	columns := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		name = strings.Trim(name, "`")
		if name != "" {
			columns = append(columns, name)
		}
	}
	if len(columns) == 0 {
		return nil
	}
	return columns
}

// stripLineComments 去掉 `--` 行注释，避免把文档里的 INSERT 例句当成真语句。
func stripLineComments(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}

func sqlFiles(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}
