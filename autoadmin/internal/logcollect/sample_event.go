package logcollect

// 样例事件（规则调试页的「试算」与日志格式认证的输入）该长什么样 —— 2026-09-19 现场教训。
//
// **现场**（kul 的 tomcat 服务）：采集链路全绿（agent 在线、Filebeat 运行、配置已下发且一致、
// pipeline 在集群上也存在），规则"试算/格式认证"全部通过，但 ES 里一条数据都没有。原因在规则的
// 第 1 个处理器 `rename log → message`——那是 Fluent Bit 时代的写法（当年 tail 出来的原始行就在
// `log` 字段里，所以要搬到 `message` 给 grok 用）。换成 Filebeat 后原始行是 `message`，而 `log`
// 变成了 Filebeat 自己的文件元信息**对象** `{file.path, offset}`：这个 rename 把 `message` 覆盖成
// 一个对象，后面的 grok 报 `field [message] of type java.util.HashMap cannot be cast to String`，
// ES 最终以 `document_parsing_exception`（400）拒收**每一条**事件，Filebeat 只能丢弃
// （主机日志里刷 `Cannot index event (status=400): dropping event!`），data stream 永远是 0 条。
//
// **为什么平台上完全看不出来**：试算与认证喂给 `_simulate` 的样例文档只有 `{"message": <原始行>}`，
// 没有 Filebeat 自己的字段。而 `ignore_missing: true` 的 rename 在"`log` 不存在"时是**静默跳过**的，
// 于是同一条规则在平台上跑得好好的、在主机上 100% 丢数据 —— 校验的输入不真实，绿就是噪声。
//
// 所以：样例文档必须带上 Filebeat 在每个事件上都会写的字段；判定口径（必备字段、schema 违规）
// 要按"真实事件"而不是"理想事件"来算。

// filebeatEventFields 返回 Filebeat filestream 输入在**每个事件**上都会写的字段。
//
// 值只要**形态正确**即可（`log` / `host` / `agent` 是对象，不是字符串）：处理器把它们当字符串用
// （`rename log → message`）本身就是错的，与具体值无关。
//
// 平台自己注入的维度字段（下发片段的 `fields_under_root`）也一并带上：真实事件里它们一定存在，
// 规则里写 `{{log_name}}` 这类引用的处理器在样例里也必须取得到值，否则会反过来冤枉合法规则。
func filebeatEventFields() map[string]any {
	return map[string]any{
		"@timestamp": "2026-01-01T00:00:00.000Z",
		"log": map[string]any{
			"file":   map[string]any{"path": "/var/log/example.log"},
			"offset": 0,
		},
		"host":  map[string]any{"name": "example-host", "hostname": "example-host", "architecture": "x86_64"},
		"agent": map[string]any{"type": "filebeat", "version": "8.13.0", "id": "00000000-0000-0000-0000-000000000000"},
		"ecs":   map[string]any{"version": "8.0.0"},
		"event": map[string]any{"module": "filestream", "dataset": "generic"},
		"input": map[string]any{"type": "filestream"},

		"service":         "example-service",
		"instance":        "example-instance",
		"application":     "example-application",
		"business_system": "example-bs",
		"project":         "example-project",
		"environment":     "example-env",
		"host_ip":         "192.0.2.1",
		"log_name":        "example.log",
		"log_path":        "/var/log/example.log",
	}
}

// filebeatOwnedFieldNames 是"Filebeat 自己写、而平台索引模板有意不映射"的顶层字段。
//
// schema_violations 判定必须跳过它们：`dynamic: false` 下这些字段被静默丢弃是**设计如此**
// （模板只映射平台要检索的字段），不是规则写错了字段。不豁免的话，因为样例文档现在带了真实载荷，
// 每条规则的试算结果都会平白多出 6 条"字段不符合标准字段规范"，把真正的违规淹掉。
func filebeatOwnedFieldNames() map[string]bool {
	return map[string]bool{"log": true, "host": true, "agent": true, "ecs": true, "event": true, "input": true}
}

// withFilebeatEventFields 给样例文档补齐 Filebeat 自己的字段：**只补调用方没给的键**。
//
// 调用方（调试页的"样例文档 JSON"模式、多行还原出来的记录）给的值优先，不被覆盖——
// 它们的 `message` 才是这条样例真正的日志内容。
func withFilebeatEventFields(doc map[string]any) map[string]any {
	merged := filebeatEventFields()
	for key, value := range doc {
		merged[key] = value
	}
	return merged
}

// clobberedMessageHint 看试跑输出里 `message` 还是不是字符串，是的话返回空串。
//
// 这是现场那条报错的最短解释链：`message` 被处理器改成了对象 → grok 取不到原始行 → 必备字段全缺
// → ES 拒收。判定层只报"缺 log_level、log_message"的话，人还得自己猜是哪一步动的；直接把这一句
// 给出来，省掉一次"登主机翻 Filebeat 日志"。
func clobberedMessageHint(result map[string]any, rawMessage string) string {
	docs, _ := result["docs"].([]any)
	for _, rawDoc := range docs {
		doc, _ := rawDoc.(map[string]any)
		detail, _ := doc["doc"].(map[string]any)
		source, _ := detail["_source"].(map[string]any)
		if source == nil {
			continue
		}
		switch source["message"].(type) {
		case nil:
			return "处理器把 message 删掉了（原始行没了）"
		case string:
			continue
		default:
			// 原始行本身就不是字符串（调用方没给 message）时不算"被覆盖"。
			if rawMessage == "" {
				continue
			}
			return "message 被处理器改成了非字符串（典型现场：`rename log → message` 把 Filebeat 的 log 元信息对象搬到了 message，原始行丢失）"
		}
	}
	return ""
}
