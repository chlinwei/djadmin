package derive

// Override 表：无法机械替换的方言构造在这里显式改写。
//
// 每条的语义都是「在 MySQL 源里找到 Old，在 PG 产物里写 New」：
//   - globalOverrides 在**所有**查询上尝试替换，全部处理完后要求每一条至少命中一次。
//     命中数为零说明源已改写（或改成别的写法），此时派生直接报错而不是悄悄跳过：
//     漏掉一条就是产出一份跑的起来但语义错的 SQL。
//   - perQueryOverrides 按查询名精确替换，未命中同样报错。
//
// 顺序：perQuery 先于 global（perQuery 的 Old 按 MySQL 原文书写，此时 global 还没动过它）。
type Override struct {
	Old string
	New string
}

var globalOverrides = []Override{
	// CAST(x AS CHAR) 在 PG 里是 char(1)，会把值截断成首字符；目标类型要写 text。
	// 这类 CAST 只用于把参数类型钉成文本，去掉会触发 sqlc 的类型冲突
	// （"named param X has incompatible types"），所以 MySQL 源里保留、PG 侧换目标类型。
	// 注意 `CAST(sqlc.arg(pattern) AS CHAR)` 那一族已经整体改成
	// `COALESCE(col,'') LIKE sqlc.narg(pattern)`：sqlc.arg 在列可空性不一致时无法合并同名参数，
	// 会生成 Pattern/Pattern_2/… 多个参数，调用点漏设就变成 LIKE NULL（真库上复现过搜索失效）。
	{Old: "CAST(sqlc.narg(label_value) AS CHAR)", New: "CAST(sqlc.narg(label_value) AS text)"},
	// SIGNED 是 MySQL 专有类型名，PG 用 bigint；两侧都落到 Go 的 int64。
	{Old: "CAST(COALESCE(MAX(sort), -1) AS SIGNED)", New: "CAST(COALESCE(MAX(sort), -1) AS bigint)"},

	// json_agg/json_build_object 对应 JSON_ARRAYAGG/JSON_OBJECT。MySQL 的 JSON_ARRAY()
	// 空数组字面量在 PG 里是 '[]'::json（json_agg 对零行返回 NULL，COALESCE 兜住）。
	{Old: "JSON_ARRAYAGG(JSON_OBJECT(", New: "json_agg(json_build_object("},
	{Old: ", JSON_ARRAY())", New: ", '[]'::json)"},

	// JSON_UNQUOTE(JSON_EXTRACT(doc, '$.name')) 与 jsonb 的 ->> 等价。
	// 取字面量键时直接写键名；键来自参数时（告警标签过滤）PG 的 ->> 直接接参数，
	// 不需要像 MySQL 那样用 CONCAT 拼 '$.' 路径前缀。
	{Old: "JSON_UNQUOTE(JSON_EXTRACT(e.service_snapshot,'$.name'))", New: "e.service_snapshot->>'name'"},
	{
		Old: "JSON_UNQUOTE(JSON_EXTRACT(ah.labels, CONCAT('$.', sqlc.arg(label_key))))",
		New: "ah.labels->>CAST(sqlc.arg(label_key) AS text)",
	},
}

var perQueryOverrides = map[string][]Override{
	// GROUP_CONCAT → string_agg：分隔符从 SEPARATOR 尾缀变成第二个参数，
	// ORDER BY 的位置也从函数内移到聚合参数之后。bs.id 是 bigint，string_agg 只接受
	// text，需要 ::text（MySQL 的 GROUP_CONCAT 对整数列直接转十进制文本）。
	"ListProjects": {
		{Old: "GROUP_CONCAT(bs.name ORDER BY bs.id SEPARATOR '||')", New: "string_agg(bs.name, '||' ORDER BY bs.id)"},
		{Old: "GROUP_CONCAT(bs.id ORDER BY bs.id SEPARATOR '||')", New: "string_agg(bs.id::text, '||' ORDER BY bs.id)"},
	},
	"GetProject": {
		{Old: "GROUP_CONCAT(bs.name ORDER BY bs.id SEPARATOR '||')", New: "string_agg(bs.name, '||' ORDER BY bs.id)"},
		{Old: "GROUP_CONCAT(bs.id ORDER BY bs.id SEPARATOR '||')", New: "string_agg(bs.id::text, '||' ORDER BY bs.id)"},
	},
	// 单地址投递的 get-or-create：MySQL 用 `id=LAST_INSERT_ID(id)` 把既有行的主键
	// 变成 LastInsertId。PG 没有这个函数，等价写法是 `id = <表>.id`（一个保持原值的空操作）——
	// 写成 EXCLUDED.id 会命中这条 INSERT 自己算出的下一个序列值，等于把主键改成新值。
	// 冲突目标 (event_id,media_id,user_id,address) 由源里 `-- conflict:` 注释提供。
	"CreateAlertNotificationDeliveryOrGetID": {
		{
			Old: "id=LAST_INSERT_ID(id)",
			New: "id=monitor_alert_notification_delivery.id",
		},
	},
}
