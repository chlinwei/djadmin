package logcollect

import (
	"fmt"
	"strings"
)

// 处理器参数的**引擎方言**：同一个参数在 OpenSearch 与 Elasticsearch 里可能叫法不同，
// 规则文本从一个引擎搬到另一个引擎时，报错只说"不支持这个参数"，看不出该改成什么。
//
// 为什么会有这种搬迁：平台早期指向 ES 7.10 的 fork（OpenSearch 系，Storage 水位还用过
// `_plugins/_ism/explain`），后来迁到真正的 Elasticsearch（`_ilm/explain`，见
// LOG_COLLECTOR_MIGRATION.md）。规则是当年在旧集群上写的，于是出现"保存时正常、
// 换集群后 400"的现场案例（2026-09-19）：三条规则里的
// `{"rename": {..., "override_target": true}}` 在新集群全部编译不过。
//
// 有意不做自动改写：改写的只是**存储的文本**，发到集群上的是另一份内容，两边不一致以后
// 更难排查（平台的"期望配置指纹"体系也建立在"存的就是发出去的"之上）。该报错就报错，
// 并把等价写法说清楚——与本仓库既有的"报错而不是猜"一致。

type pipelineParameterAlias struct {
	openSearch    string
	elasticsearch string
	note          string
}

// 已核实的跨引擎参数别名。加新条目时请**在真实集群上验证过两个名字再写进来**
// （rename 的 override 用法已用 ES 8.13 的 _simulate 验证：源字段不存在时不动目标字段，
// 存在时覆盖——与 OpenSearch 的 override_target 语义一致）。
var pipelineParameterAliases = []pipelineParameterAlias{
	{openSearch: "override_target", elasticsearch: "override", note: "目标字段已存在时覆盖"},
}

// pipelineCompatHint 把"处理器参数不被当前集群支持"的 ES 报错补一句"该改成什么"。
//
// 返回空串表示不是这类错误（调用方原样返回 ES 报错即可）。判断方向靠报错里出现的
// **是哪个名字**：报 `[override_target]` 不支持 = 当前集群是 Elasticsearch 一侧，
// 该用 `override`；反之亦然。
func pipelineCompatHint(esError string) string {
	parameters := unsupportedProcessorParameters(esError)
	if len(parameters) == 0 {
		return ""
	}
	for _, parameter := range parameters {
		for _, alias := range pipelineParameterAliases {
			switch parameter {
			case alias.openSearch:
				return fmt.Sprintf("；参数 %s 是 OpenSearch 的叫法，当前集群的 Elasticsearch 用 %s（%s）——改成 %s 后重新发布即可",
					parameter, alias.elasticsearch, alias.note, alias.elasticsearch)
			case alias.elasticsearch:
				return fmt.Sprintf("；参数 %s 是 Elasticsearch 的叫法，当前集群的 OpenSearch 用 %s（%s）——改成 %s 后重新发布即可",
					parameter, alias.openSearch, alias.note, alias.openSearch)
			}
		}
	}
	return fmt.Sprintf("；%s 不属于当前集群的处理器定义：同一参数在两个引擎里可能叫法不同（已知：rename 覆盖用 %s / %s），请按当前集群改写后重新发布",
		strings.Join(parameters, "、"), pipelineParameterAliases[0].openSearch, pipelineParameterAliases[0].elasticsearch)
}

// unsupportedProcessorParameters 从 ES 报错里取出不被支持的参数名。
//
// 报错形如：
//
//	processor [rename] doesn't support one or more provided configuration parameters [override_target]
//
// 只取 "configuration parameters" 后面那个方括号里的列表，不要把前面的处理器名
// （`[rename]`）也当成参数名——那会让提示写成"rename 不属于当前集群的处理器定义"这种错话。
// 参数可能是多个，用逗号分隔。
func unsupportedProcessorParameters(esError string) []string {
	const marker = "configuration parameters"
	at := strings.Index(esError, marker)
	if at < 0 {
		return nil
	}
	rest := esError[at+len(marker):]
	open := strings.IndexByte(rest, '[')
	if open < 0 {
		return nil
	}
	closeAt := strings.IndexByte(rest[open:], ']')
	if closeAt < 0 {
		return nil
	}
	parameters := []string{}
	for _, part := range strings.Split(rest[open+1:open+closeAt], ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parameters = append(parameters, trimmed)
		}
	}
	return parameters
}
