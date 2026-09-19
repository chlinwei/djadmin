# 监控中心 Prometheus 代理与 Explore 补全

## 入口

- 前端 Explore 页（`fronted/src/views/monitor/explore/index.vue`）使用 codemirror-promql 编辑器，自动补全请求 `<后端>/monitor/targets/prometheus/proxy/api/v1/*`（同域代理，避免跨域/鉴权问题）。补全会用到 `/api/v1/label/__name__/values`、`/api/v1/labels`、`/api/v1/series`、`/api/v1/metadata` 等端点；其中 `/labels`、`/series`、`/label/*/values` 以 POST + form 方式提交。
- Go 侧处理器：`autoadmin/internal/monitor/handler.go` 的 `PrometheusProxy`，路由注册为 `Any`（GET/POST 等均可，`router.go`）。仅放行 `/api/v1/*` 前缀；POST 请求的 form 参数会合并进 query（POST 优先覆盖同名 query 参数）后转发给上游。

## 最终逻辑

- `prometheusGet` 统一请求 Prometheus，把响应解为 `prometheusPayload`：`status`、`warnings`、`error` 为定式字段，`data` 用 `json.RawMessage` 保存原始 JSON，不预设形状。
- 原因：Prometheus 各端点的 `data` 形状不一致——`/query`、`/targets`、`/alerts`、`/rules`、`/status/*` 返回对象，`/label/<name>/values`、`/series` 返回数组。早期版本把 `data` 定为 `map[string]any`，导致 Explore 补全（数组形 data）解码失败，前端收到 `prometheus invalid json response: json: cannot unmarshal array...`，补全失效。
- `PrometheusProxy` 将 `payload.Data`（RawMessage）原样透传，格式无关，任意 `/api/v1/*` 端点均可代理。
- 需要 `data` 为对象的内部 handler（targets/alerts/rules/query/status）通过 `payload.dataMap()` 解为 `map[string]any` 使用；数组形 data 会得到空 map，这些 handler 本身不消费数组形端点。
- 非 2xx 或 `status != success` 时，`prometheusGet` 返回错误（优先取上游 `error` 字段）；代理以 HTTP 200 + `{"status":"error", ...}` 返回，保持与 Prometheus 客户端库的容错兼容。

## 双实现对齐

Django 侧已废弃（源码已移出版本库）；监控中心相关功能只以 Go 版 autoadmin 为准。
