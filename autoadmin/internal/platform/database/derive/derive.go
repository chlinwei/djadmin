// Package derive 把 db/queries/mysql 的查询机械派生为 db/queries/postgres 的查询。
//
// 派生方向是单向的：MySQL 侧是唯一人工维护来源，PG 侧是产物，禁止手改
// （由 derive_test.go 的 TestDerivedQueriesMatchRepository 守住）。
//
// 规则见 docs/architecture/SQL_DESIGN.md §4.6，实现上分三步：
//
//  1. 显式 override 表（本文件的 overrides.go）：方言构造按查询改写成 PG 写法，
//     例如 GROUP_CONCAT→string_agg、JSON_ARRAYAGG→json_agg、CAST(x AS CHAR)→CAST(x AS text)。
//  2. 反引号标识符 → 双引号。MySQL 的双引号是字符串字面量而非标识符
//     （sqlc 的 mysql 引擎实测会把 "order" 解析成字符串常量并生成 string 列），
//     所以 MySQL 源里必须用反引号、PG 侧必须换成双引号，不能两边共用一种引号。
//  3. `?` → `$n`，按出现顺序逐个编号（每个 `?` 是独立参数，不做去重）。
//     sqlc.arg/sqlc.narg 不改写：两个引擎都原生支持，且 PG 侧保留参数名
//     （改写成 $n 会让 sqlc 退回 Column1/Column2 的兜底命名，丢掉 Parameter 名）。
//  4. `:execlastid` → `:one` + 语句末尾 `RETURNING id`（自增主键的取回方式必须分叉，见
//     appendReturningID 的说明）。两侧生成的签名都是 `(int64, error)`，调用点不用变。
//  5. 可变长 `IN (sqlc.slice(x))` → `x = ANY(sqlc.arg(x)::bigint[])`（Sqlc.slice 在 PG 侧是坏的，
//     见 rewriteSlices；残留的 sqlc.slice 会让派生直接失败）。
//  6. `ON DUPLICATE KEY UPDATE a=VALUES(a)` → `ON CONFLICT (<目标>) DO UPDATE SET a=EXCLUDED.a`
//     （见 rewriteUpserts；冲突目标由源里的 `-- conflict: 列名` 注释声明，缺了直接报错）。
//  7. `INSERT IGNORE INTO t(…)` → `INSERT INTO t(…) ON CONFLICT DO NOTHING`（见 rewriteInsertIgnore）。
//
// 关于命名参数与位置参数混用：sqlc 会把 sqlc.arg/sqlc.narg 的编号排在显式 $n 之后
// （实测 `... (sqlc.narg(k) IS NULL OR s LIKE sqlc.narg(k)) AND n = $1 LIMIT $2` 里
// narg 得到 $4），因此混用不会撞号。这也意味着**不需要**把分页查询的可选过滤改成
// 纯位置参数：那样反而会让 `(? IS NULL OR ...)` 里的第一个 `?` 只出现在 IS NULL
// 位置、PG 无法推断其类型，运行时报 could not determine data type of parameter。
package derive

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// File 是一个派生结果：相对于 db/queries 的文件名（如 assets.sql）+ 内容。
type File struct {
	Name    string
	Content string
}

// header 会加在每个派生文件开头。sqlc 忽略注释，加在这里是为了让手改产物的人先看到。
const headerTemplate = `-- 本文件由 make derive 从 db/queries/mysql/%s 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

`

// Derive 读取 mysqlDir 下的所有 .sql 查询文件，返回对应的 PG 派生结果（按文件名排序）。
func Derive(mysqlDir string) ([]File, error) {
	names, err := sqlFileNames(mysqlDir)
	if err != nil {
		return nil, err
	}
	matched := make(map[string]bool, len(globalOverrides))
	files := make([]File, 0, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(mysqlDir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		content, err := deriveFile(name, string(raw), matched)
		if err != nil {
			return nil, err
		}
		files = append(files, File{Name: name, Content: fmt.Sprintf(headerTemplate, name) + content})
	}
	// override 表里任何一条都没匹配上，说明源已经改过、这一条已成噪声或写错了：
	// 静默不生效的 override 会让派生结果悄悄退回未修正的方言写法。
	for _, override := range globalOverrides {
		if !matched[override.Old] {
			return nil, fmt.Errorf("override 未匹配到任何查询，疑似源已变更或写法有误：%q", clip(override.Old))
		}
	}
	return files, nil
}

// WriteAll 把派生结果写入 postgresDir，并删除该目录下不再派生的 .sql 文件
// （避免源里删掉一个文件后产物残留）。非 .sql 文件（如 README.md）不动。
func WriteAll(postgresDir string, files []File) error {
	if err := os.MkdirAll(postgresDir, 0o775); err != nil {
		return err
	}
	existing, err := sqlFileNames(postgresDir)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(files))
	for _, file := range files {
		wanted[file.Name] = true
		if err := os.WriteFile(filepath.Join(postgresDir, file.Name), []byte(file.Content), 0o664); err != nil {
			return fmt.Errorf("write %s: %w", file.Name, err)
		}
	}
	for _, name := range existing {
		if !wanted[name] {
			if err := os.Remove(filepath.Join(postgresDir, name)); err != nil {
				return fmt.Errorf("remove stale %s: %w", name, err)
			}
		}
	}
	return nil
}

func sqlFileNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// deriveFile 按查询切分文件：注释（含 `-- name:` 行本身）原样保留，
// 只有查询体参与改写。编号按查询重置——参数编号只在单条语句内有意义。
func deriveFile(name string, raw string, matched map[string]bool) (string, error) {
	lines := strings.SplitAfter(raw, "\n")
	var out strings.Builder
	var body strings.Builder
	var current queryHeader

	flush := func() error {
		if current.name == "" {
			return nil
		}
		statement := body.String()
		returning := needsReturningID(current, statement)
		rewritten, err := rewriteQuery(current, statement, returning, matched)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		out.WriteString(current.nameLine(returning))
		out.WriteString(rewritten)
		body.Reset()
		return nil
	}

	for _, line := range lines {
		if header, ok := parseNameLine(line); ok {
			if err := flush(); err != nil {
				return "", err
			}
			current = header
			// name 行要等语句读完才知道注解是否改写（INSERT 才加 RETURNING），所以延到 flush 里写。
			continue
		}
		if current.name == "" {
			out.WriteString(line) // 文件头注释
			continue
		}
		body.WriteString(line)
	}
	if err := flush(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// queryHeader 是 `-- name: X :one` 这一行的解析结果。
type queryHeader struct {
	name       string
	annotation string // 不带冒号，如 "one" / "many" / "execlastid"
}

// nameLine 给出 PG 产物里的 name 行：需要 RETURNING 的查询在 PG 侧是 `:one`。
func (header queryHeader) nameLine(returningID bool) string {
	annotation := header.annotation
	if returningID {
		annotation = "one"
	}
	return "-- name: " + header.name + " :" + annotation + "\n"
}

// needsReturningID 判断该查询在 PG 产物里是否要改写为 `:one` + `RETURNING id`：
// `:execlastid` 与 INSERT 的 `:execresult` 都要（两者在 MySQL 侧靠 `LastInsertId()` 取主键）；
// UPDATE/DELETE 的 `:execresult` 靠 `RowsAffected()`，PG 侧原样可用，不改。
func needsReturningID(header queryHeader, body string) bool {
	switch header.annotation {
	case "execlastid":
		return true
	case "execresult":
		return firstStatementKeyword(body) == "INSERT"
	default:
		return false
	}
}

// firstStatementKeyword 取语句里第一个非注释、非空行的首个单词（大写），用于识别语句类型。
func firstStatementKeyword(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		return strings.ToUpper(fields[0])
	}
	return ""
}

func parseNameLine(line string) (queryHeader, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "-- name:") {
		return queryHeader{}, false
	}
	fields := strings.Fields(strings.TrimPrefix(trimmed, "-- name:"))
	if len(fields) == 0 {
		return queryHeader{}, false
	}
	header := queryHeader{name: fields[0]}
	if len(fields) > 1 {
		header.annotation = strings.TrimPrefix(fields[1], ":")
	}
	return header, true
}

func rewriteQuery(header queryHeader, body string, returningID bool, matched map[string]bool) (string, error) {
	rewritten, err := rewriteInsertIgnore(rewriteSlices(body))
	if err != nil {
		return "", fmt.Errorf("查询 %s: %w", header.name, err)
	}
	if rewritten, err = rewriteUpserts(rewritten); err != nil {
		return "", fmt.Errorf("查询 %s: %w", header.name, err)
	}
	for _, override := range perQueryOverrides[header.name] {
		if !strings.Contains(rewritten, override.Old) {
			return "", fmt.Errorf("查询 %s 的 override 未匹配：%q", header.name, clip(override.Old))
		}
		rewritten = strings.ReplaceAll(rewritten, override.Old, override.New)
	}
	for _, override := range globalOverrides {
		if strings.Contains(rewritten, override.Old) {
			matched[override.Old] = true
			rewritten = strings.ReplaceAll(rewritten, override.Old, override.New)
		}
	}
	// sqlc.slice 是两侧生成物不等价的写法：MySQL 生成 `/*SLICE:x*/?` 标记 + 运行时替换占位符，
	// PG 引擎把标记直接渲染成 `IN ($1)`，而生成代码里的替换仍在找 MySQL 的那个标记 —— 传 N>1 个
	// 值时占位符与参数个数不符，查询报错（P4-7：inspection 的业务链路快照曾因此静默变空）。
	// 上面 rewriteSlices 已把标准写法改成 PG 的数组参数，这里是兜底：还有残留说明写法不在覆盖
	// 范围内（例如不是 `IN (sqlc.slice(x))` 的形状），直接报错而不是产出一份 PG 上跑不通的 SQL。
	if strings.Contains(rewritten, "sqlc.slice(") {
		return "", fmt.Errorf("查询 %s 里还有未被改写的 sqlc.slice：PG 产物会坏（P4-7），请改用 `IN (sqlc.slice(x))` 的标准写法", header.name)
	}
	if strings.Count(stripLiteralsAndComments(rewritten), upsertMarker) > 0 {
		return "", fmt.Errorf("查询 %s 里还有未被改写的 %s（一条语句只支持一个 UPSERT）", header.name, upsertMarker)
	}
	if strings.Contains(stripLiteralsAndComments(rewritten), insertIgnorePrefix) {
		return "", fmt.Errorf("查询 %s 里还有未被改写的 %s（必须写成 `INSERT IGNORE INTO`，见 rewriteInsertIgnore）", header.name, insertIgnorePrefix)
	}
	syntaxRewritten, err := rewriteSyntax(rewritten)
	if err != nil {
		return "", err
	}
	if !returningID {
		return syntaxRewritten, nil
	}
	return appendReturningID(syntaxRewritten)
}

// insertIgnorePrefix 是 MySQL 的"插入冲突即跳过"写法。
const insertIgnorePrefix = "INSERT IGNORE INTO"

// rewriteInsertIgnore 把 `INSERT IGNORE INTO t(…) VALUES(…)` 改写成
// `INSERT INTO t(…) VALUES(…) ON CONFLICT DO NOTHING`。
//
// **语义差异（不要当等价）**：MySQL 的 INSERT IGNORE 会吞掉所有"可忽略错误"（唯一冲突、外键、
// 部分类型错误），PG 的 ON CONFLICT DO NOTHING 只处理唯一/排他冲突——外键类错误在 PG 侧会照常
// 报错。仓库里的用法（monitor_target 的 (host_id, exporter_type) 唯一键：已纳管就跳过）正好落在
// 唯一冲突这一类，所以这个改写是安全的；新增用法时要确认一下被吞掉的到底是哪类错误。
func rewriteInsertIgnore(body string) (string, error) {
	if !strings.Contains(stripLiteralsAndComments(body), insertIgnorePrefix) {
		return body, nil
	}
	if strings.Contains(stripLiteralsAndComments(body), upsertMarker) {
		return "", fmt.Errorf("%s 与 %s 不能同时用（一条语句只能有一个冲突子句）", insertIgnorePrefix, upsertMarker)
	}
	rewritten := strings.Replace(body, insertIgnorePrefix, "INSERT INTO", 1)
	before, after := splitAtStatementEnd(rewritten)
	// 语句尾的空白与注释（同查询体内、属于下一条语句的文档注释）原样留在子句之后。
	if strings.HasSuffix(strings.TrimRight(before, " \t\r\n"), ";") {
		end := strings.TrimRight(before, " \t\r\n")
		statement := strings.TrimSuffix(end, ";")
		return statement + " ON CONFLICT DO NOTHING;" + before[len(end):] + after, nil
	}
	return before + " ON CONFLICT DO NOTHING" + after, nil
}

// splitAtStatementEnd 把查询体切成「第一条语句（含结束分号）」与「其后的剩余文本」
// （空行、以及写在语句之后、下一条 `-- name:` 之前的注释——它们属于本查询体但不属于这条语句）。
// 找不到结束分号时整体算语句，剩余为空。
//
// 为什么要切：冲突子句 / RETURNING 必须紧贴语句尾插入。若按"整个查询体的末尾"插入，
// 语句后紧跟的注释会把子句吃到注释里，产出一份被注释掉的坏 SQL（`-- 说明… ON CONFLICT DO NOTHING`）。
// 这类错误 sqlc 解析不出来（注释对它是透明文本，语句照旧成立），只有真跑才会炸。
func splitAtStatementEnd(body string) (string, string) {
	for i := 0; i < len(body); {
		switch {
		case strings.HasPrefix(body[i:], "--"):
			end := strings.IndexByte(body[i:], '\n')
			if end < 0 {
				return body, ""
			}
			i += end + 1
		case body[i] == '\'':
			end, err := scanStringLiteral(body, i)
			if err != nil {
				return body, ""
			}
			i = end
		case body[i] == ';':
			return body[:i+1], body[i+1:]
		default:
			i++
		}
	}
	return body, ""
}

// upsertMarker 是 MySQL 的 UPSERT 子句开头。
const upsertMarker = "ON DUPLICATE KEY UPDATE"

// conflictPattern 匹配 UPSERT 的冲突目标声明：源里的一行 `-- conflict: host_id`。
var conflictPattern = regexp.MustCompile(`(?m)^[ \t]*--[ \t]*conflict:[ \t]*([\w", ]+?)[ \t]*$`)

// valuesColumnPattern 匹配 MySQL UPSERT 子句里的 `VALUES(col)`（INSERT 的值列表写作
// `VALUES (…)`、带空格，且这里只作用于子句内部，不会误伤）。
var valuesColumnPattern = regexp.MustCompile(`VALUES\((\w+)\)`)

// rewriteUpserts 把 `ON DUPLICATE KEY UPDATE a=VALUES(a), …` 改写成
// `ON CONFLICT (a) DO UPDATE SET a=EXCLUDED.a, …`。
//
// 为什么冲突目标只能由源显式声明：MySQL 的 ON DUPLICATE KEY 对"表上任意一个唯一键"生效，
// 语句本身看不出打在哪个键上；而 PG 的 ON CONFLICT 必须写出目标列。猜错等于改了语义
// （可能命中另一条唯一键），所以缺 `-- conflict: <列名>` 注释时直接报错。
func rewriteUpserts(body string) (string, error) {
	// 只在"去掉注释与字面量"的文本上判断有没有 UPSERT：注释里提到这个关键字
	// （例如说明这条规则的文档注释）不能被当成真语句。
	if !strings.Contains(stripLiteralsAndComments(body), upsertMarker) {
		return body, nil
	}
	index := strings.Index(body, upsertMarker)
	if index < 0 {
		return body, nil
	}
	match := conflictPattern.FindStringSubmatch(body)
	if match == nil {
		return "", fmt.Errorf("UPSERT 缺少冲突目标声明，请在该查询上写一行 `-- conflict: <列名>`")
	}
	head, clause := body[:index], body[index+len(upsertMarker):]
	return head + "ON CONFLICT (" + strings.TrimSpace(match[1]) + ") DO UPDATE SET" +
		valuesColumnPattern.ReplaceAllString(clause, "EXCLUDED.$1"), nil
}

// slicePattern 匹配可变长 IN 的 MySQL 写法 `IN (sqlc.slice(host_ids))`。
var slicePattern = regexp.MustCompile(`IN\s*\(\s*sqlc\.slice\((\w+)\)\s*\)`)

// rewriteSlices 把可变长 IN 改写成 PG 的数组参数：`IN (sqlc.slice(host_ids))` →
// `h.id = ANY(sqlc.arg(host_ids)::bigint[])`。
//
// 为什么要分叉：MySQL 的 IN 只能接一组占位符（sqlc 用 slice 标记 + 运行时展开实现），
// PG 侧的等价能力是数组参数。改成 `= ANY(...)` 后两侧都走索引，且生成的 Go 签名都是
// `[]int64`，调用点不需要分叉（实测见 TestDeriveRewritesSliceToArrayParameter）。
// 列不是 bigint 时这个 CAST 会在 PG 侧运行时报类型错——是响的失败，不静默。
func rewriteSlices(body string) string {
	return slicePattern.ReplaceAllString(body, "= ANY(sqlc.arg($1)::bigint[])")
}

// appendReturningID 把 `INSERT … VALUES (…);` 变成 `INSERT … VALUES (…)\nRETURNING id;`。
//
// 为什么必须分叉：MySQL 的自增主键靠 `LastInsertId()` 取回（`:execlastid` 生成的代码就是这么做的，
// `:execresult` 的调用点也这么用），而 PostgreSQL 的驱动层根本不实现它——pgx 的 database/sql
// 适配器对普通 Exec 返回的是 `driver.RowsAffected`，其 `LastInsertId()` 恒返回
// `LastInsertId is not supported by this driver`（见 go/src/database/sql/driver/driver.go）。
// 这不是配置问题，无法绕过：PG 侧只能 INSERT … RETURNING。
//
// 主键列名假定为 `id`（本仓库所有表都是）；多行 INSERT 取不回"每行的 id"，
// 直接在派生期报错，而不是生成一份只会取到首行 id 的 SQL。
func appendReturningID(body string) (string, error) {
	plain := stripLiteralsAndComments(body)
	if strings.Contains(plain, "),(") || strings.Contains(plain, "), (") {
		return "", fmt.Errorf("多行 INSERT 不能用 RETURNING 取主键 id：改成 :execrows，或逐条插入（见 SQL_DESIGN §4.3）")
	}
	before, after := splitAtStatementEnd(body)
	end := strings.TrimRight(before, " \t\r\n")
	if !strings.HasSuffix(end, ";") {
		// 没有结束分号的语句（源里允许省略）：直接在语句文本后补 RETURNING。
		return before + "\nRETURNING id" + after, nil
	}
	// 分号要去掉再接 RETURNING，但末尾必须补回来：sqlc 靠分号切分查询，
	// PG 解析器遇到"没有终结符就跟着下一条语句"会报 syntax error。
	statement := strings.TrimSuffix(end, ";")
	// 语句后的空白与注释（见 splitAtStatementEnd）原样保留，产物文件的分隔与其它查询一致。
	return statement + "\nRETURNING id;" + before[len(end):] + after, nil
}

// stripLiteralsAndComments 去掉字符串字面量与注释（保留其它字符），供关键字/形状判断使用：
// `),(` 出现在字面量里时会误判成多行 VALUES。
func stripLiteralsAndComments(body string) string {
	var out strings.Builder
	for i := 0; i < len(body); {
		switch {
		case strings.HasPrefix(body[i:], "--"):
			end := strings.IndexByte(body[i:], '\n')
			if end < 0 {
				return out.String()
			}
			i += end + 1
		case body[i] == '\'':
			end, err := scanStringLiteral(body, i)
			if err != nil {
				return out.String()
			}
			out.WriteString("''")
			i = end
		default:
			out.WriteByte(body[i])
			i++
		}
	}
	return out.String()
}

// rewriteSyntax 做纯语法层面的改写：反引号标识符 → 双引号，`?` → `$n`。
// 字符串字面量与注释原样保留：`?` 出现在 '%x%' 这类字面量里不是占位符。
func rewriteSyntax(body string) (string, error) {
	var out strings.Builder
	next := 1

	for i := 0; i < len(body); {
		switch {
		case strings.HasPrefix(body[i:], "--"):
			end := strings.IndexByte(body[i:], '\n')
			if end < 0 {
				out.WriteString(body[i:])
				i = len(body)
				continue
			}
			out.WriteString(body[i : i+end+1])
			i += end + 1
		case body[i] == '\'':
			end, err := scanStringLiteral(body, i)
			if err != nil {
				return "", err
			}
			out.WriteString(body[i:end])
			i = end
		case body[i] == '`':
			end := strings.IndexByte(body[i+1:], '`')
			if end < 0 {
				return "", fmt.Errorf("反引号未闭合：%q", clip(body[i:]))
			}
			out.WriteString(`"` + body[i+1:i+1+end] + `"`)
			i += end + 2
		case body[i] == '?':
			fmt.Fprintf(&out, "$%d", next)
			next++
			i++
		case strings.HasPrefix(body[i:], "sqlc.arg(") || strings.HasPrefix(body[i:], "sqlc.narg("):
			// 命名参数原样保留（sqlc 两侧都支持，且 PG 侧靠它保住参数名）。
			// 不占用 $n 编号：sqlc 会把命名参数排在显式编号之后，两者互不干扰。
			close := strings.IndexByte(body[i:], ')') + i
			if close < i {
				return "", fmt.Errorf("命名参数未闭合：%q", clip(body[i:]))
			}
			out.WriteString(body[i : close+1])
			i = close + 1
		default:
			out.WriteByte(body[i])
			i++
		}
	}
	return out.String(), nil
}

// scanStringLiteral 返回单引号字面量结束后的下标，兼容 ” 与 \' 两种转义。
func scanStringLiteral(body string, start int) (int, error) {
	for i := start + 1; i < len(body); {
		switch body[i] {
		case '\\':
			i += 2
		case '\'':
			if i+1 < len(body) && body[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1, nil
		default:
			i++
		}
	}
	return 0, fmt.Errorf("字符串字面量未闭合：%q", clip(body[start:]))
}

func clip(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 60 {
		return value[:60] + "…"
	}
	return value
}
