package assets

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 通用守卫：内联 SQL 的 SELECT 列数必须等于紧随其后的 rows.Scan 目标数。
//
// 为什么需要它：手写 SQL 里「SQL 文本的列」与「Go 侧 Scan 目标」是两个独立事实，
// 任意一处漂移都不会编译报错，只在真实查询时抛
// `sql: expected N destination arguments in Scan, not M`（曾导致
// POST /api/agent/install 直接 500）。sqlc 生成的查询没有这个问题，所以本守卫
// 只针对内联 SQL；等这些查询迁到 sqlc 后，本测试覆盖的集合会自然缩小。
//
// 覆盖不到的情况一律跳过（宁可漏报也不误报）：
//   - SQL 由变量拼接（`...`+where+`...`）：列数在编译期不固定
//   - select 列表含 `*`
//   - Scan 目标里含嵌套括号导致无法可靠数顶层逗号
func TestInlineSQLColumnArityMatchesScan(t *testing.T) {
	root := findAssetsModuleRoot(t)
	packages := []string{"assets", "monitor", "inspection", "baseline", "automation", "identity"}

	callRe := regexp.MustCompile(`\.(Query|QueryRow)Context\(`)
	checked, skipped := 0, 0

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
			src := string(raw)
			for _, loc := range callRe.FindAllStringIndex(src, -1) {
				sql, callEnd, ok := inlineSQLAt(src, loc[1])
				if !ok {
					skipped++
					continue
				}
				if _, hasStar := selectListHasStar(sql); hasStar {
					skipped++
					continue
				}
				columns, ok := selectListColumnCount(sql)
				if !ok {
					skipped++
					continue
				}
				targets, ok := scanTargetCount(src, callEnd)
				if !ok {
					skipped++
					continue
				}
				checked++
				if columns != targets {
					t.Errorf("%s/%s: SELECT 列数 %d != Scan 目标数 %d\n  SQL: %s",
						pkg, name, columns, targets, compact(sql))
				}
			}
		}
	}
	t.Logf("内联 SELECT 已核对 %d 条，跳过（不可静态判定）%d 条", checked, skipped)
	if checked == 0 {
		t.Fatal("没有核对到任何内联 SELECT，守卫失效，请检查解析逻辑")
	}
	if skipped > checked*3 {
		t.Errorf("跳过比例过高（checked=%d skipped=%d），守卫可能已失效", checked, skipped)
	}
}

// interpolated 代表反引号字面量之间被拼接进去的表达式（如 IN (`+join+`)）。
// 它不含逗号，因此不影响 select 列表的列数统计；只要它出现在 select 列表内部，
// 才认为列数不可静态判定。
const interpolated = "\x00EXPR\x00"

// inlineSQLAt 取调用起点之后的 SQL 表达式：从第一个反引号到最后一个反引号，
// 中间被拼接的表达式片段用 interpolated 占位。
//
// 关键：拼接通常发生在 WHERE（如 `IN (`+strings.Join(placeholders, ",")+`)`），
// 此时 select 列表仍是完整的字面量，列数照样可判——早期版本因为「见到拼接就跳过」
// 恰好漏掉了出过错的那条语句，所以这里只对 select 列表内部的拼接放弃。
func inlineSQLAt(src string, callStart int) (sql string, callEnd int, ok bool) {
	end := matchingParen(src, callStart-1)
	if end < 0 {
		return "", 0, false
	}
	call := src[callStart:end]
	first := strings.Index(call, "`")
	last := strings.LastIndex(call, "`")
	if first < 0 || last <= first {
		return "", 0, false
	}
	expr := call[first : last+1]

	var b strings.Builder
	i := 0
	for i < len(expr) {
		if expr[i] == '`' {
			closeRel := strings.IndexByte(expr[i+1:], '`')
			if closeRel < 0 {
				return "", 0, false
			}
			b.WriteString(expr[i+1 : i+1+closeRel])
			i += closeRel + 2
			continue
		}
		nextRel := strings.IndexByte(expr[i:], '`')
		if nextRel < 0 {
			break
		}
		b.WriteString(interpolated)
		i += nextRel
	}
	return strings.Join(strings.Fields(b.String()), " "), end, true
}

// matchingParen 返回与 open（'(' 的下标）配对的 ')' 下标，忽略引号内的括号。
func matchingParen(src string, open int) int {
	depth, i := 0, open
	var quote byte
	for i < len(src) {
		ch := src[i]
		if quote != 0 {
			if ch == '\\' {
				i += 2
				continue
			}
			if ch == quote {
				quote = 0
			}
		} else {
			switch ch {
			case '"', '`', '\'':
				quote = ch
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return i
				}
			}
		}
		i++
	}
	return -1
}

// topLevelParts 按顶层逗号切分，忽略括号与引号内部。
func topLevelParts(s string) []string {
	parts := make([]string, 0, 8)
	depth, i, start := 0, 0, 0
	var quote byte
	for i < len(s) {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' {
				i += 2
				continue
			}
			if ch == quote {
				quote = 0
			}
		} else {
			switch ch {
			case '"', '`', '\'':
				quote = ch
			case '(', '[':
				depth++
			case ')', ']':
				depth--
			case ',':
				if depth == 0 {
					parts = append(parts, s[start:i])
					start = i + 1
				}
			}
		}
		i++
	}
	parts = append(parts, s[start:])
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

func selectListHasStar(sql string) (string, bool) {
	list, ok := selectList(sql)
	if !ok {
		return "", false
	}
	return list, regexp.MustCompile(`(^|[\s,(])[*]`).MatchString(list)
}

// selectList 截取 SELECT 与第一个顶层 FROM 之间的列表。
func selectList(sql string) (string, bool) {
	idx := strings.Index(strings.ToUpper(sql), "SELECT")
	if idx < 0 {
		return "", false
	}
	rest := sql[idx+len("SELECT"):]
	depth, i := 0, 0
	var quote byte
	for i < len(rest) {
		ch := rest[i]
		if quote != 0 {
			if ch == '\\' {
				i += 2
				continue
			}
			if ch == quote {
				quote = 0
			}
		} else {
			switch ch {
			case '"', '`', '\'':
				quote = ch
			case '(':
				depth++
			case ')':
				depth--
			case ' ':
				if depth == 0 && strings.HasPrefix(strings.ToUpper(rest[i:]), " FROM ") {
					return rest[:i], true
				}
			}
		}
		i++
	}
	return "", false
}

func selectListColumnCount(sql string) (int, bool) {
	list, ok := selectList(sql)
	if !ok {
		return 0, false
	}
	// select 列表内部有拼接 → 列数在编译期不确定，放弃。
	if strings.Contains(list, interpolated) {
		return 0, false
	}
	parts := topLevelParts(list)
	if len(parts) == 0 {
		return 0, false
	}
	return len(parts), true
}

// scanTargetCount 找调用之后最近的 .Scan(...) 并数顶层参数个数。
func scanTargetCount(src string, callEnd int) (int, bool) {
	rest := src[callEnd:]
	loc := regexp.MustCompile(`\.\s*Scan\(`).FindStringIndex(rest)
	if loc == nil {
		return 0, false
	}
	open := callEnd + loc[1] - 1
	end := matchingParen(src, open)
	if end < 0 {
		return 0, false
	}
	args := topLevelParts(src[open+1 : end])
	if len(args) == 0 {
		return 0, false
	}
	return len(args), true
}

func compact(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}
