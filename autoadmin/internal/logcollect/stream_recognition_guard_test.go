package logcollect

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 守卫测试：构造流名识别候选的查询**不得**按 `enabled` 过滤。
//
// 2026-09-18 现象：逻辑服务 `yilake nginx` 被停用后，它的索引在「日志存储 → 存储水位」里
// 从"属于某个业务系统/环境/服务"变成「未识别」。根因是 `ListServiceStreamDims` 当时带
// `WHERE s.enabled = TRUE`，识别候选集里没有停用服务的维度码，`resolveStreamName` 匹配不到
// 任何候选就返回 `Recognized=false`。
//
// 这条规则必须钉住，因为它与两条设计原则直接冲突：
//   - 计划 §0：**停止采集 ≠ 删除数据**（停用是可逆的，存量流还在写或保留到 ILM 到期）；
//   - 计划 §2.3：`orphan`（孤儿流）指的是"**维度已删/改名**"，`Recognized=false` 是为它准备的信号。
//     把"已停用"混进孤儿信号，会让 Phase 3 的"孤儿流清理入口"去删还在保留期内的数据。
//
// 对照：**下发**路径（ListHostLogRenderEntries）照旧过滤 `s.enabled = TRUE`，那是正确的——
// 停用的服务不该再往主机推采集片段。所以这里只针对识别路径的三条查询。
//
// 纯文本解析（不连库）：sqlc 不校验语义，改回过滤也不会编译报错，只有真跑才暴露。
func TestStreamRecognitionQueriesDoNotFilterByEnabled(t *testing.T) {
	queries := readMonitorQueries(t)
	// 识别路径：流名候选维度码、档位码全集。
	// （早先还有一条 ListServiceStreamRows 给 dims.services 用，那份 payload 前端从未消费，
	//   2026-09-18 连同查询一起删除；服务级采集开关现在随每条流返回。）
	recognitionQueries := []string{"ListServiceStreamDims", "ListRetentionTierCodes"}
	// 下发路径：作为对照，必须保留过滤（防止把这条守卫误改成"所有查询都不许提 enabled"）。
	dispatchQueries := []string{"ListHostLogRenderEntries"}

	enabledFilter := regexp.MustCompile(`(?i)\benabled\s*=\s*(TRUE|1|true)`)
	for _, name := range recognitionQueries {
		body, ok := queryBody(queries, name)
		if !ok {
			t.Fatalf("db/queries/mysql/monitor.sql 里找不到查询 %s（改名后请同步这条守卫）", name)
		}
		if match := enabledFilter.FindString(body); match != "" {
			t.Errorf(`查询 %s 是流名识别路径，不得按 enabled 过滤（发现 %q）：停用服务/档位不是"维度已删"，
停用后既有流必须仍被识别（见本用例注释与计划 §0/§2.3）`, name, strings.TrimSpace(match))
		}
	}
	for _, name := range dispatchQueries {
		body, ok := queryBody(queries, name)
		if !ok {
			t.Fatalf("db/queries/mysql/monitor.sql 里找不到查询 %s", name)
		}
		if !enabledFilter.MatchString(body) {
			t.Errorf("查询 %s 是下发路径，必须继续按 enabled 过滤：停用的服务不应再往主机推采集片段", name)
		}
	}
}

// queryBody 截取 `-- name: <name> :...` 到下一个 `-- name:` 之间的语句文本。
func queryBody(content string, name string) (string, bool) {
	marker := "-- name: " + name + " "
	start := strings.Index(content, marker)
	if start < 0 {
		return "", false
	}
	rest := content[start+len(marker):]
	if end := strings.Index(rest, "-- name: "); end >= 0 {
		rest = rest[:end]
	}
	return rest, true
}

func readMonitorQueries(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for depth := 0; depth < 6; depth++ {
		candidate := filepath.Join(dir, "db", "queries", "mysql", "monitor.sql")
		if _, err := os.Stat(candidate); err == nil {
			raw, readErr := os.ReadFile(candidate)
			if readErr != nil {
				t.Fatalf("read monitor.sql: %v", readErr)
			}
			return string(raw)
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("未找到 db/queries/mysql/monitor.sql（从测试目录向上 6 层）")
	return ""
}
