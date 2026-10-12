package logcollect

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/logstream"

	"github.com/gin-gonic/gin"
)

type elasticsearchCluster struct {
	ID                                             int64
	Hosts, Username, Password, CACert, IndexPrefix string
	VerifyTLS                                      bool
	Timeout                                        int
	Enabled                                        bool
}

// loadElasticsearchCluster 读当前请求 URL 上的集群（管理端按 id 操作），并解密密文密码。
func (handler *Handler) loadElasticsearchCluster(context *gin.Context) (elasticsearchCluster, error) {
	return handler.loadElasticsearchClusterByID(context, parseID(context.Param("id")))
}

// loadElasticsearchClusterByID 与上面的唯一差别是 id 来源（后台任务没有请求上下文）。
// 参数类型是 context.Context 而不是 *gin.Context：格式认证等后台任务要复用这条链路。
func (handler *Handler) loadElasticsearchClusterByID(context context.Context, id int64) (elasticsearchCluster, error) {
	row, err := db.New(handler.db).GetElasticsearchClusterConnection(context, id)
	if err != nil {
		return elasticsearchCluster{}, err
	}
	cluster := elasticsearchCluster{
		ID: row.ID, Hosts: row.Hosts, Username: row.Username, Password: row.Password,
		VerifyTLS: row.VerifyTls, CACert: row.CaCert, IndexPrefix: row.IndexPrefix,
		Timeout: int(row.RequestTimeout), Enabled: row.Enabled,
	}
	cluster.Password, err = handler.secrets.Decrypt(cluster.Password)
	return cluster, err
}

// defaultElasticsearchCluster 取"启用的默认集群"（is_default 优先，否则第一个启用的）。
//
// 这个才是**日志实际写到哪**：Filebeat 的 output 由它决定（见 log_config_render.go 与
// log_target_actions.go 的下发路径），索引前缀与数据流名也取自它。所以"最近有没有在写"
// 这类问题必须以它为准，不能用 URL 上的集群（那个是管理端在按 id 操作某个集群）。
func (handler *Handler) defaultElasticsearchCluster(context context.Context) (elasticsearchCluster, error) {
	row, err := db.New(handler.db).GetDefaultEnabledElasticsearchClusterConnection(context)
	if err != nil {
		return elasticsearchCluster{}, err
	}
	cluster := elasticsearchCluster{
		ID: row.ID, Hosts: row.Hosts, Username: row.Username, Password: row.Password,
		VerifyTLS: row.VerifyTls, CACert: row.CaCert, IndexPrefix: row.IndexPrefix,
		Timeout: int(row.RequestTimeout), Enabled: row.Enabled,
	}
	cluster.Password, err = handler.secrets.Decrypt(cluster.Password)
	return cluster, err
}

// elasticsearchRequestRaw 发送请求并返回原始 JSON（对象或数组，_cat 系列端点返回数组）。
func (handler *Handler) elasticsearchRequestRaw(context context.Context, cluster elasticsearchCluster, method, path string, body any) (json.RawMessage, error) {
	var rawBody []byte
	var err error
	if body != nil {
		rawBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if !cluster.VerifyTLS {
		tlsConfig.InsecureSkipVerify = true
	} // Explicit cluster setting for self-signed deployments.
	if cluster.VerifyTLS && strings.TrimSpace(cluster.CACert) != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cluster.CACert)) {
			return nil, fmt.Errorf("invalid CA certificate")
		}
		tlsConfig.RootCAs = pool
	}
	timeout := cluster.Timeout
	if timeout < 1 {
		timeout = 10
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig}}
	var lastError error
	for _, host := range strings.Split(cluster.Hosts, ",") {
		host = strings.TrimRight(strings.TrimSpace(host), "/")
		if host == "" {
			continue
		}
		request, requestErr := http.NewRequestWithContext(context, method, host+"/"+strings.TrimLeft(path, "/"), bytes.NewReader(rawBody))
		if requestErr != nil {
			return nil, requestErr
		}
		request.Header.Set("Content-Type", "application/json")
		if cluster.Username != "" {
			request.SetBasicAuth(cluster.Username, cluster.Password)
		}
		upstream, requestErr := client.Do(request)
		if requestErr != nil {
			lastError = requestErr
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(upstream.Body, 16<<20))
		upstream.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
			return nil, fmt.Errorf("elasticsearch %s: %s", upstream.Status, strings.TrimSpace(string(payload)))
		}
		return json.RawMessage(payload), nil
	}
	if lastError == nil {
		lastError = fmt.Errorf("no Elasticsearch hosts configured")
	}
	return nil, lastError
}

func (handler *Handler) elasticsearchRequest(context context.Context, cluster elasticsearchCluster, method, path string, body any) (map[string]any, error) {
	payload, err := handler.elasticsearchRequestRaw(context, cluster, method, path, body)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err = json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("invalid Elasticsearch JSON: %w", err)
	}
	return result, nil
}

// elasticsearchRequestArray：_cat 系列端点返回 JSON 数组，解码为 []map[string]any。
func (handler *Handler) elasticsearchRequestArray(context context.Context, cluster elasticsearchCluster, method, path string) ([]map[string]any, error) {
	payload, err := handler.elasticsearchRequestRaw(context, cluster, method, path, nil)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return []map[string]any{}, nil
	}
	var result []map[string]any
	if err = json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("invalid Elasticsearch JSON: %w", err)
	}
	return result, nil
}

func (handler *Handler) TestElasticsearchConnection(context *gin.Context) {
	cluster, err := handler.loadElasticsearchCluster(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	info, err := handler.elasticsearchRequest(context, cluster, http.MethodGet, "/", nil)
	now := time.Now().UTC()
	if err != nil {
		_ = db.New(handler.db).MarkElasticsearchClusterCheckFailed(context, db.MarkElasticsearchClusterCheckFailedParams{
			LastCheckTime: sql.NullTime{Time: now, Valid: true}, LastCheckMessage: err.Error(),
			UpdateTime: now, ID: cluster.ID,
		})
		response.BusinessError(context, 400, "connection failed: "+err.Error(), nil)
		return
	}
	health, err := handler.elasticsearchRequest(context, cluster, http.MethodGet, "/_cluster/health", nil)
	if err != nil {
		response.BusinessError(context, 400, "connection failed: "+err.Error(), nil)
		return
	}
	version, _ := info["version"].(map[string]any)
	// Elasticsearch 的 version 对象没有 distribution 字段（那是 Elasticsearch 的），回落 build_flavor。
	distribution := version["distribution"]
	if distribution == nil || fmt.Sprint(distribution) == "<nil>" {
		distribution = version["build_flavor"]
	}
	if distribution == nil || fmt.Sprint(distribution) == "<nil>" {
		distribution = "elasticsearch"
	}
	result := gin.H{"cluster_name": info["cluster_name"], "distribution": distribution, "version": version["number"], "status": health["status"], "number_of_nodes": health["number_of_nodes"]}
	message := fmt.Sprintf("%v %v / %v / %v", result["distribution"], result["version"], result["cluster_name"], result["status"])
	_ = db.New(handler.db).MarkElasticsearchClusterCheckSuccess(context, db.MarkElasticsearchClusterCheckSuccessParams{
		LastCheckTime: sql.NullTime{Time: now, Valid: true}, LastCheckMessage: message,
		UpdateTime: now, ID: cluster.ID,
	})
	response.Success(context, result)
}

func parseLogWindow(context *gin.Context) (time.Time, time.Time, error) {
	end := time.Now().UTC()
	var err error
	if raw := strings.TrimSpace(context.Query("end")); raw != "" {
		end, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("end must be ISO8601")
		}
	}
	start := end.Add(-time.Hour)
	if raw := strings.TrimSpace(context.Query("start")); raw != "" {
		start, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("start must be ISO8601")
		}
	}
	if !start.Before(end) {
		return start, end, fmt.Errorf("start must be before end")
	}
	if end.Sub(start) > 30*24*time.Hour {
		return start, end, fmt.Errorf("time range cannot exceed 30 days")
	}
	return start, end, nil
}

// MaxLogSearchServices 跨服务检索一次允许的逻辑服务数上限。日志中心的范围选择
// （业务系统/环境节点）通常是个位数到几十个服务；上限只是防线，再大的范围应该
// 逐层下钻或用时间收窄（SQL IN 与 ES should 子句都会随服务数线性膨胀）。
const MaxLogSearchServices = 50

// parseIDList 解析逗号分隔的 id 列表（去空、保序、去重）。解析不出数字的段直接忽略——
// 与 parseID 的"静默 0"同一取舍：调用方对空列表给出明确报错即可。
func parseIDList(raw string) []int64 {
	seen := map[int64]bool{}
	ids := []int64{}
	for _, part := range strings.Split(raw, ",") {
		id := parseID(strings.TrimSpace(part))
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// serviceDimsFilters 单个逻辑服务的四元组过滤（服务编码允许跨业务/环境重复，见 000044）。
func serviceDimsFilters(dims db.GetApplicationServiceStreamDimsRow) []any {
	return []any{
		gin.H{"term": gin.H{"service": dims.ServiceCode}},
		gin.H{"term": gin.H{"project": dims.ProjectCode}},
		gin.H{"term": gin.H{"business_system": dims.BusinessSystemCode}},
		gin.H{"term": gin.H{"environment": dims.EnvironmentCode}},
	}
}

// multiServiceScopeFilter 多个逻辑服务的范围过滤：每个服务一个四元组子句，
// should + minimum_should_match=1（命中任一服务即算范围内）。纯函数便于单测。
func multiServiceScopeFilter(dimsList []db.ListApplicationServiceStreamDimsByIDsRow) gin.H {
	should := make([]any, 0, len(dimsList))
	for _, dims := range dimsList {
		should = append(should, gin.H{"bool": gin.H{"filter": []any{
			gin.H{"term": gin.H{"service": dims.ServiceCode}},
			gin.H{"term": gin.H{"project": dims.ProjectCode}},
			gin.H{"term": gin.H{"business_system": dims.BusinessSystemCode}},
			gin.H{"term": gin.H{"environment": dims.EnvironmentCode}},
		}}})
	}
	return gin.H{"bool": gin.H{"should": should, "minimum_should_match": 1}}
}

func (handler *Handler) buildLogQuery(context *gin.Context) (elasticsearchCluster, string, []any, error) {
	cluster, err := handler.loadElasticsearchCluster(context)
	if err != nil {
		return cluster, "", nil, err
	}
	// 两种范围：单个逻辑服务（application_service_id，逻辑服务页沿用）或多个
	//（application_service_ids，日志中心上层节点的跨服务检索，2026-09-28）。
	// 两者都把范围折算成"四元组过滤"，后续的关键词/级别/实例等过滤完全共用。
	var scopeFilter any
	prefix := defaultString(cluster.IndexPrefix, "autoadmin")
	var indexPatterns []string
	if rawIDs := strings.TrimSpace(context.Query("application_service_ids")); rawIDs != "" {
		ids := parseIDList(rawIDs)
		if len(ids) == 0 {
			return cluster, "", nil, fmt.Errorf("application_service_ids is invalid")
		}
		if len(ids) > MaxLogSearchServices {
			return cluster, "", nil, fmt.Errorf("一次最多查询 %d 个逻辑服务，请收窄范围", MaxLogSearchServices)
		}
		dimsList, err := db.New(handler.db).ListApplicationServiceStreamDimsByIDs(context, ids)
		if err != nil || len(dimsList) == 0 {
			return cluster, "", nil, fmt.Errorf("application service not found")
		}
		// 请求里的 id 可能有不存在的：按实际查到的 dims 收窄（缺的服务静默忽略，
		// 与"范围由前端从已列出的服务里选"一致）。
		scopeFilter = multiServiceScopeFilter(dimsList)
		for _, dims := range dimsList {
			indexPatterns = append(indexPatterns, logstream.Name(prefix, dims.ProjectCode, dims.EnvironmentCode, dims.BusinessSystemCode, dims.ServiceCode, "*"))
		}
	} else {
		serviceID := parseID(context.Query("application_service_id"))
		if serviceID == 0 {
			return cluster, "", nil, fmt.Errorf("application_service_id is required")
		}
		// 逻辑服务的编码允许跨业务/环境重复（见 000044 迁移），只按 service 一个字段过滤会串数据：
		// 文档已带 project/business_system/environment（见 log_config_render 的 fields），按完整维度收窄。
		dims, err := db.New(handler.db).GetApplicationServiceStreamDims(context, serviceID)
		if err != nil {
			return cluster, "", nil, fmt.Errorf("application service not found")
		}
		scopeFilter = gin.H{"bool": gin.H{"filter": serviceDimsFilters(dims)}}
		indexPatterns = []string{logstream.Name(prefix, dims.ProjectCode, dims.EnvironmentCode, dims.BusinessSystemCode, dims.ServiceCode, "*")}
	}
	start, end, err := parseLogWindow(context)
	if err != nil {
		return cluster, "", nil, err
	}
	filters := []any{
		scopeFilter,
		gin.H{"range": gin.H{"@timestamp": gin.H{"gte": start.Format(time.RFC3339Nano), "lte": end.Format(time.RFC3339Nano)}}},
	}
	for _, field := range []string{"project", "business_system", "environment", "service", "instance", "host_ip", "log_name", "error_fingerprint"} {
		if value := strings.TrimSpace(context.Query(field)); value != "" {
			filters = append(filters, gin.H{"term": gin.H{field: value}})
		}
	}
	// log_path 在写入时已统一为真实文件路径，直接使用 keyword 索引，禁止逐条读取 _source 过滤。
	if value := strings.TrimSpace(context.Query("log_path")); value != "" {
		filters = append(filters, gin.H{"term": gin.H{"log_path": value}})
	}
	if levels := strings.TrimSpace(context.Query("log_level")); levels != "" {
		filters = append(filters, gin.H{"terms": gin.H{"log_level": strings.Split(levels, ",")}})
	}
	// 关键词框有两种模式（由前端切换，见日志查询面板的「正文 / Lucene」）：
	//   message（默认）：**只搜日志正文**——default_field=log_message，并把冒号转义，
	//     免得 query_string 的显式字段前缀绕过"只搜正文"（default_field 管不住 `field:value`）。
	//   lucene：完整 Lucene 语法，字段过滤可用（`host_ip:"192.168.201.209"`、
	//     `log_level:ERROR AND timeout`…），此时**不转义**。
	// 两种模式的裸词都仍然落在 log_message 上（default_field 保持不变）：否则 `timeout AND error`
	// 这种写法的语义会随"默认字段是整个文档"漂移，用户很难预期。
	// 两种模式都保留 allow_leading_wildcard=false 与 500 字符上限——开"能按字段查"的口子，
	// 不等于也允许 `*foo` 这类前置通配（ES 侧是纯扫描，代价与收益不成比例）。
	keywordMode := strings.ToLower(strings.TrimSpace(context.Query("keyword_mode")))
	if keywordMode != keywordModeLucene {
		keywordMode = keywordModeMessage
	}
	must := []any{gin.H{"match_all": gin.H{}}}
	if clause := keywordQuery(context.Query("keyword"), keywordMode); clause != nil {
		must = []any{*clause}
	}
	// 文档过滤保证范围正确，索引范围同时收窄才能避免让无关服务的全部分片参与查询。
	// 档位用通配符，保留当前及历史档位的日志；没有匹配流时返回空结果，不扩大全量范围。
	searchPath := "/" + url.PathEscape(strings.Join(indexPatterns, ",")) + "/_search?ignore_unavailable=true&allow_no_indices=true"
	return cluster, searchPath, []any{filters, must}, nil
}

// keywordQuery 把关键词与模式折算成 query_string 子句（关键词为空时返回 nil）。
//
// 抽成纯函数是为了能直接测两种模式的分叉：面板上那个框写着 Lucene 就必须真的支持 Lucene，
// 而"默认只搜正文"这条也不能被顺手弄丢（两侧任一搞错，都要等用户查不到数据才发现）。
func keywordQuery(keyword, mode string) *gin.H {
	trimmed := strings.TrimSpace(keyword)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > 500 {
		trimmed = trimmed[:500]
	}
	if mode != keywordModeLucene {
		// 非 lucene（含没传/未知）一律按"只搜正文"处理：安全的一侧做默认。
		trimmed = escapeLuceneFieldColon(trimmed)
	}
	clause := gin.H{"query_string": gin.H{
		"query":                  trimmed,
		"default_field":          "log_message",
		"default_operator":       "AND",
		"allow_leading_wildcard": false,
		"lenient":                true,
	}}
	return &clause
}

// 关键词框的两种模式（与前端 filters.keywordMode 一一对应）。
const (
	keywordModeMessage = "message"
	keywordModeLucene  = "lucene"
)

// escapeLuceneFieldColon 转义用户输入里的冒号，把查询锁在 log_message 上：
// query_string 的 default_field 只影响没有字段前缀的 term，`field:value` 想查哪个字段就查哪个字段。
// 「正文」模式下要的是"搜内容"，所以把冒号转义掉；要按字段查就切到 Lucene 模式。
func escapeLuceneFieldColon(keyword string) string {
	return strings.ReplaceAll(keyword, ":", "\\:")
}

func (handler *Handler) ElasticsearchLogSearch(context *gin.Context) {
	cluster, searchPath, parts, err := handler.buildLogQuery(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	size := queryInt(context, "size", 100)
	if size < 1 {
		size = 1
	}
	if size > 200 {
		size = 200
	}
	offset := queryInt(context, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	if offset+size > 2000 {
		response.BusinessError(context, 400, "offset+size cannot exceed 2000", nil)
		return
	}
	// _source 带上 project/business_system/environment：跨服务检索（application_service_ids）的
	// 列表要按层级显示"业务系统/环境/逻辑服务"列，文档上的维度编码是唯一的来源。
	body := gin.H{"from": offset, "size": size, "track_total_hits": 2000, "sort": []any{gin.H{"@timestamp": "desc"}}, "_source": []string{"@timestamp", "log_level", "service", "instance", "host_ip", "log_name", "log_path", "message", "log_message", "error_fingerprint", "app_fields", "log.file.path", "project", "business_system", "environment"}, "query": gin.H{"bool": gin.H{"filter": parts[0], "must": parts[1]}}}
	data, err := handler.elasticsearchRequest(context, cluster, http.MethodPost, searchPath, body)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	hits, _ := data["hits"].(map[string]any)
	rawHits, _ := hits["hits"].([]any)
	results := make([]gin.H, 0, len(rawHits))
	for _, raw := range rawHits {
		hit, _ := raw.(map[string]any)
		source, _ := hit["_source"].(map[string]any)
		item := gin.H{"id": hit["_id"]}
		for key, value := range source {
			item[key] = value
		}
		// 日志路径以 Filebeat 的 `log.file.path`（该事件实际来自的文件）为准。
		applyLogPathFromFilebeat(item, source)
		results = append(results, item)
	}
	count := intValue(hits["total"])
	if total, ok := hits["total"].(map[string]any); ok {
		count = intValue(total["value"])
	}
	response.Success(context, gin.H{"results": results, "count": count, "size": size, "offset": offset})
}

// nestedFilePath 取 Filebeat 写在事件里的真实文件路径 `log.file.path`；缺失返回空串。
//
// 这是"这条日志到底来自哪个文件"的权威值：filestream 会为每个事件写入它，而采集配置里的
// `paths` 可能是通配模式（`/home/esb/data/logs/*/log_error.log`），不能拿来当具体路径显示。
func nestedFilePath(source map[string]any) string {
	logField, _ := source["log"].(map[string]any)
	if logField == nil {
		return ""
	}
	fileField, _ := logField["file"].(map[string]any)
	if fileField == nil {
		return ""
	}
	path, _ := fileField["path"].(string)
	return path
}

// applyLogPathFromFilebeat 把事件里的 log_path 换成真实文件路径（若有）。
func applyLogPathFromFilebeat(item gin.H, source map[string]any) {
	if actual := nestedFilePath(source); actual != "" {
		item["log_path"] = actual
	}
}

// logFacetAllowedFields 统计面板允许的分组维度。新增维度必须同时改前端
// statsFieldOptions 与 FACET_FILTER_KEY（并把字段加进 buildLogQuery 的过滤白名单），
// 否则"能统计但不能下钻"。
//
// 2026-09-28 加上 project/business_system/environment/service：日志查询在**上层节点**（项目/
// 业务系统/环境/全部）改为"概览优先"，最自然的问题就是"哪个服务/业务系统错误最多"——
// 这四个维度既是文档上的 keyword 字段（log_config_render 注入），也是范围四元组本身。
var logFacetAllowedFields = map[string]bool{
	"log_level": true, "instance": true, "host_ip": true, "log_name": true, "log_path": true, "error_fingerprint": true,
	"service": true, "project": true, "business_system": true, "environment": true,
}

func (handler *Handler) ElasticsearchLogFacetStats(context *gin.Context) {
	field := strings.TrimSpace(context.Query("field"))
	if !logFacetAllowedFields[field] {
		response.BusinessError(context, 400, "invalid facet field", nil)
		return
	}
	optionsOnly := false
	if value, present := context.GetQuery("options_only"); present {
		var err error
		optionsOnly, err = strconv.ParseBool(value)
		if err != nil {
			response.BusinessError(context, 400, "options_only must be a boolean", nil)
			return
		}
	}
	cluster, searchPath, parts, err := handler.buildLogQuery(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	size := queryInt(context, "size", 20)
	if size < 1 {
		size = 1
	}
	if size > 50 {
		size = 50
	}
	interval := queryInt(context, "interval_minutes", 2)
	if interval < 1 {
		interval = 1
	}
	aggField := facetAggField(field)
	fieldAgg := gin.H{"terms": gin.H{"field": aggField, "size": size, "order": gin.H{"_count": "desc"}}}
	// 下拉候选只需要值与计数；样例排序和每桶时间趋势只由统计视图请求，避免长窗口重复聚合。
	if !optionsOnly {
		fieldAgg["aggs"] = gin.H{
			"sample": gin.H{"top_hits": gin.H{"size": 1, "sort": []any{gin.H{"@timestamp": "desc"}}}},
			"trend":  gin.H{"date_histogram": gin.H{"field": "@timestamp", "fixed_interval": fmt.Sprintf("%dm", interval)}},
		}
	}
	body := gin.H{"size": 0, "track_total_hits": false, "query": gin.H{"bool": gin.H{"filter": parts[0], "must": parts[1]}}, "aggs": gin.H{"by_field": fieldAgg}}
	data, err := handler.elasticsearchRequest(context, cluster, http.MethodPost, searchPath, body)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	aggregations, _ := data["aggregations"].(map[string]any)
	byField, _ := aggregations["by_field"].(map[string]any)
	rawBuckets, _ := byField["buckets"].([]any)
	// 与 Django MonitorViewSet.log_facet_stats 保持同一响应结构：
	// 原始 Elasticsearch bucket 需要拍平成 {value,count,sample,trend:[{timestamp,count}]}，
	// 否则前端 LogQueryPanel 按 bucket.trend 数组渲染趋势图会因为拿到聚合桶对象而报错。
	buckets := make([]gin.H, 0, len(rawBuckets))
	for _, raw := range rawBuckets {
		bucket, _ := raw.(map[string]any)
		var sample gin.H
		if sampleAgg, ok := bucket["sample"].(map[string]any); ok {
			if hits, ok := sampleAgg["hits"].(map[string]any); ok {
				if hitList, ok := hits["hits"].([]any); ok && len(hitList) > 0 {
					if hit, ok := hitList[0].(map[string]any); ok {
						sample = gin.H{"id": hit["_id"]}
						if source, ok := hit["_source"].(map[string]any); ok {
							for key, value := range source {
								sample[key] = value
							}
							// 样例详情也要显示具体文件，而不是路径模式。
							applyLogPathFromFilebeat(sample, source)
						}
					}
				}
			}
		}
		trend := []gin.H{}
		if trendAgg, ok := bucket["trend"].(map[string]any); ok {
			if trendBuckets, ok := trendAgg["buckets"].([]any); ok {
				for _, rawPoint := range trendBuckets {
					point, _ := rawPoint.(map[string]any)
					trend = append(trend, gin.H{"timestamp": point["key_as_string"], "count": point["doc_count"]})
				}
			}
		}
		item := gin.H{"value": bucket["key"], "count": bucket["doc_count"], "sample": sample, "trend": trend}
		buckets = append(buckets, item)
	}
	response.Success(context, gin.H{"field": field, "interval_minutes": interval, "buckets": buckets})
}

// facetAggField 的维度名与索引字段一致；真实路径统一存储在 log_path keyword 字段中。
func facetAggField(field string) string {
	return field
}

func queryInt(context *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(context.Query(name))
	if err != nil {
		return fallback
	}
	return value
}

var _ = sql.ErrNoRows
