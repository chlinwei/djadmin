package assets

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 守卫：**业务代码里不得再出现内联 SQL**（把 SQL 文本直接写在 QueryContext/QueryRowContext/
// ExecContext 的参数里）。
//
// 为什么需要它：内联 SQL 里「SQL 文本的列」与「Go 侧 Scan 目标」是两个独立事实，任意一处漂移
// 都不会编译报错，只在真实查询时抛 `sql: expected N destination arguments in Scan, not M`
// （曾导致 POST /api/agent/install 直接 500）。SQL 全部交给 sqlc 生成后，这类问题在编译期就不存在，
// 所以本测试从"比对内联 SQL 的列数"改成守"不要再引入内联 SQL"这条不变量。
//
// 迁移记录：2026-09 日志渲染的最后两条查询（`ListHostLogRenderInstances` /
// `ListHostLogRenderEntries`）迁入 `db/queries` 后，本仓库内联 SQL 归零。
//
// 允许的例外：查询文本来自变量（如 `database.QueryRowContext(ctx, query, args...)`，
// 文本由 sqlc 生成物或构造器提供）——本守卫只认参数位置上的反引号字面量。
func TestNoInlineSQLLiterals(t *testing.T) {
	root := findAssetsModuleRoot(t)
	packages := []string{"assets", "monitor", "logcollect", "inspection", "baseline", "automation", "identity"}

	callRe := regexp.MustCompile(`\.(Query|QueryRow|Exec)Context\(`)
	scannedFiles := 0
	offenders := []string{}

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
			scannedFiles++
			src := string(raw)
			for _, loc := range callRe.FindAllStringIndex(src, -1) {
				sql, _, ok := inlineSQLAt(src, loc[1])
				if !ok {
					continue
				}
				offenders = append(offenders, pkg+"/"+name+": "+compact(sql))
			}
		}
	}
	// 自检：扫描必须真的落到文件上，否则"零命中"是假绿（老版本靠 checked==0 做同样的自检）。
	if scannedFiles == 0 {
		t.Fatal("没有扫描到任何文件，守卫失效，请检查路径解析")
	}
	if len(offenders) > 0 {
		t.Errorf("发现内联 SQL，应改为 db/queries 下的 sqlc 查询：\n  %s", strings.Join(offenders, "\n  "))
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

func compact(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}
