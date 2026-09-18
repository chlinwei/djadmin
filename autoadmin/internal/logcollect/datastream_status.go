package logcollect

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/logstream"

	"github.com/gin-gonic/gin"
)

// 存储水位：data stream 运行态只读展示。物理隔离粒度是 data stream
//（项目×业务系统×环境×逻辑服务×档位，服务维度直接编码在流名中），
// 因此流级即可按逻辑服务拆分磁盘占用与文档数。

type dataStreamBackingIndex struct {
	Index    string `json:"index"`
	Health   string `json:"health"`
	Docs     float64
	Bytes    float64
	CreateAt string `json:"create_at"`
	ILMState string `json:"ilm_state"`
}

type dataStreamEntry struct {
	Name           string                   `json:"name"`
	Project        string                   `json:"project"`
	Environment    string                   `json:"environment"`
	BusinessSystem string                   `json:"business_system"`
	Service        string                   `json:"service"`
	Tier           string                   `json:"tier"`
	Health         string                   `json:"health"`
	Docs           float64                  `json:"docs"`
	Bytes          float64                  `json:"bytes"`
	ILMState       string                   `json:"ilm_state"`
	Recognized     bool                     `json:"recognized"`
	BackingIndices []dataStreamBackingIndex `json:"backing_indices"`
}

// data stream 后备索引名为 .ds-<stream>-<generation>（如 .ds-logs-kul-test-tib-hot-000015），
// 传统索引为 <stream>-<YYYY.MM.DD>；两种后缀都要剥离才能还原流名。
var backingIndexDateSuffix = regexp.MustCompile(`-\d{4}\.\d{2}\.\d{2}(-\d+)?$`)
var backingIndexGenerationSuffix = regexp.MustCompile(`-\d{5,}$`)

// parsedStreamName：流名解析结果。
type parsedStreamName struct {
	Stream, Project, Environment, BusinessSystem, Service, Tier string
	Recognized                                                  bool
}

// streamNameMatcher 基于数据库维度码做流名匹配。编码可含连字符（如服务 tomcat-svc、
// 档位 wuhan-test），纯字符串切分必有歧义，必须拿已知维度码做前缀匹配：
// 新命名 = <prefix>-<项目>-<业务系统>-<环境>-<逻辑服务>-<档位>；
// 旧命名 = <prefix>-<项目>-<环境>-<业务系统>-<档位>（无服务段，业务系统/环境段序
// 为调整前的旧段序）。
type streamNameMatcher struct {
	prefix   string
	services []streamServiceKey // 新命名候选
	legacy   []streamLegacyKey  // 旧命名候选
	tiers    map[string]bool
}

type streamServiceKey struct {
	Match                                         string // "<项目>-<业务系统>-<环境>-<逻辑服务>-"
	Project, Environment, BusinessSystem, Service string
}

type streamLegacyKey struct {
	Match                                string // "<环境>-<业务系统>-"
	Project, Environment, BusinessSystem string
}

// loadStreamDims 加载启用服务的维度码，构造流名匹配候选（新命名 + 旧命名兼容）。
func (handler *Handler) loadStreamDims(context *gin.Context, prefix string) streamNameMatcher {
	matcher := streamNameMatcher{prefix: prefix, tiers: map[string]bool{}}
	var serviceKeys []streamServiceKey
	var legacyKeys []streamLegacyKey
	queries := db.New(handler.db)
	if rows, err := queries.ListEnabledServiceStreamDims(context); err == nil {
		for _, row := range rows {
			serviceKeys = append(serviceKeys, streamServiceKey{
				Match:   strings.Join([]string{row.ProjectCode, row.BusinessSystemCode, row.EnvironmentCode, row.ServiceCode}, "-") + "-",
				Project: row.ProjectCode, Environment: row.EnvironmentCode,
				BusinessSystem: row.BusinessSystemCode, Service: row.ServiceCode,
			})
			legacyKeys = append(legacyKeys, streamLegacyKey{
				Match:   strings.Join([]string{row.EnvironmentCode, row.BusinessSystemCode}, "-") + "-",
				Project: row.ProjectCode, Environment: row.EnvironmentCode, BusinessSystem: row.BusinessSystemCode,
			})
			if row.TierCode != "" {
				matcher.tiers[row.TierCode] = true
			}
		}
	}
	// 兜底：把所有档位码也带上，旧命名流（无服务维度）至少档位能对上
	if tiers, tierErr := queries.ListEnabledRetentionTiers(context); tierErr == nil {
		for _, tier := range tiers {
			if tier.Code != "" {
				matcher.tiers[tier.Code] = true
			}
		}
	}
	matcher.services = serviceKeys
	matcher.legacy = legacyKeys
	return matcher
}

// resolveStreamName 按候选码匹配流名；新命名优先（更长前缀），旧命名要求剩余段恰好是已知档位。
func (m streamNameMatcher) resolveStreamName(stream string) parsedStreamName {
	var result parsedStreamName
	result.Stream = stream
	if !strings.HasPrefix(stream, m.prefix+"-") {
		return result
	}
	rest := strings.TrimPrefix(stream, m.prefix+"-")
	for _, candidate := range m.services {
		if strings.HasPrefix(rest, candidate.Match) {
			tier := strings.TrimPrefix(rest, candidate.Match)
			result.Project, result.Environment, result.BusinessSystem, result.Service, result.Tier = candidate.Project, candidate.Environment, candidate.BusinessSystem, candidate.Service, tier
			result.Recognized = true
			return result
		}
	}
	for _, candidate := range m.legacy {
		if strings.HasPrefix(rest, candidate.Match) {
			tier := strings.TrimPrefix(rest, candidate.Match)
			if m.tiers[tier] {
				result.Project, result.Environment, result.BusinessSystem, result.Tier = candidate.Project, candidate.Environment, candidate.BusinessSystem, tier
				result.Recognized = true
				return result
			}
		}
	}
	return result
}

// LogDataStreamName 构造逻辑服务级 data stream 名（logs-<项目>-<业务系统>-<环境>-<逻辑服务>-<档位>）。
// 后续生成 Filebeat 采集配置的地方必须统一调用本函数，禁止各自拼接。
func LogDataStreamName(prefix, project, environment, businessSystem, service, tier string) string {
	return logstream.Name(prefix, project, environment, businessSystem, service, tier)
}

// stripBackingIndexSuffixes 剥离 .ds- 前缀与代数/日期后缀，还原 data stream 名。
func stripBackingIndexSuffixes(prefix, index string) string {
	stream := strings.TrimPrefix(index, ".ds-")
	stream = backingIndexDateSuffix.ReplaceAllString(stream, "")
	stream = backingIndexGenerationSuffix.ReplaceAllString(stream, "")
	return stream
}

func catFloat(row map[string]any, key string) float64 {
	switch value := row[key].(type) {
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsed
	case float64:
		return value
	}
	return 0
}

func catString(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return strings.TrimSpace(value)
}

// fetchDataStreamEntries 基于 _cat/indices + _ism/explain 组装流级运行态。
func (handler *Handler) fetchDataStreamEntries(context *gin.Context, cluster elasticsearchCluster, matcher streamNameMatcher) ([]dataStreamEntry, map[string]string, error) {
	prefix := logHealthPrefix(cluster)
	indices, err := handler.elasticsearchRequestArray(context, cluster, "GET",
		"/_cat/indices/"+prefix+"-*?format=json&h=index,health,status,docs.count,store.size,creation.date.string&bytes=b")
	if err != nil {
		return nil, nil, fmt.Errorf("查询索引列表失败: %w", err)
	}
	ilmStates := map[string]string{}
	// ES 8 的 ILM explain：GET <prefix>-*/_ilm/explain -> {"indices":{"<index>":{"phase":"hot",...}}}，
	// 未挂策略/未托管的索引没有 phase（只有 step）。没有匹配索引时返回 {"indices":{}}。
	explain, explainErr := handler.elasticsearchRequest(context, cluster, "GET", "/"+prefix+"-*/_ilm/explain", nil)
	if explainErr == nil {
		if indices, ok := explain["indices"].(map[string]any); ok {
			for key, raw := range indices {
				payload, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if phase, _ := payload["phase"].(string); phase != "" {
					ilmStates[key] = phase
					continue
				}
				if step, _ := payload["step"].(string); step != "" {
					ilmStates[key] = step
				}
			}
		}
	}

	entries := map[string]*dataStreamEntry{}
	var order []string
	for _, row := range indices {
		index := catString(row, "index")
		if index == "" {
			continue
		}
		parsed := matcher.resolveStreamName(stripBackingIndexSuffixes(prefix, index))
		entry, exists := entries[parsed.Stream]
		if !exists {
			entry = &dataStreamEntry{
				Name: parsed.Stream, Project: parsed.Project, Environment: parsed.Environment,
				BusinessSystem: parsed.BusinessSystem, Service: parsed.Service, Tier: parsed.Tier,
				Recognized: parsed.Recognized,
			}
			entries[parsed.Stream] = entry
			order = append(order, parsed.Stream)
		}
		entry.Docs += catFloat(row, "docs.count")
		entry.Bytes += catFloat(row, "store.size")
		if entry.Health == "" || catString(row, "health") == "red" {
			entry.Health = catString(row, "health")
		}
		entry.BackingIndices = append(entry.BackingIndices, dataStreamBackingIndex{
			Index: index, Health: catString(row, "health"), Docs: catFloat(row, "docs.count"),
			Bytes: catFloat(row, "store.size"), CreateAt: catString(row, "creation.date.string"),
			ILMState: ilmStates[index],
		})
		// 流级 ILM 状态取任一后备索引的非空状态（同一流的策略一致）。
		if entry.ILMState == "" && ilmStates[index] != "" {
			entry.ILMState = ilmStates[index]
		}
	}
	sort.Strings(order)
	result := make([]dataStreamEntry, 0, len(order))
	for _, name := range order {
		backing := entries[name].BackingIndices
		sort.Slice(backing, func(i, j int) bool { return backing[i].Index < backing[j].Index })
		result = append(result, *entries[name])
	}
	return result, ilmStates, nil
}

// GetLogStorageOverview 存储水位总览：流级运行态（Elasticsearch）+ 维度数据（MySQL）一次返回，
// 由前端组装 顶层→项目→业务系统→环境→逻辑服务 的层级树。
func (handler *Handler) GetLogStorageOverview(context *gin.Context) {
	cluster, err := handler.loadElasticsearchCluster(context)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "Elasticsearch cluster not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	matcher := handler.loadStreamDims(context, logHealthPrefix(cluster))
	entries, _, err := handler.fetchDataStreamEntries(context, cluster, matcher)
	if err != nil {
		response.BusinessError(context, 502, err.Error(), nil)
		return
	}
	// 节点磁盘水位：失败不阻塞总览（旧版本/云托管可能拒绝该端点）。
	allocation, allocErr := handler.elasticsearchRequestArray(context, cluster, "GET", "/_cat/allocation?format=json&h=node,name,shards,disk.used,disk.total,disk.percent")

	type bizsysRow struct {
		ID        int64  `json:"id"`
		Code      string `json:"code"`
		Name      string `json:"name"`
		ProjectID *int64 `json:"project_id"`
	}
	type projectRow struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	type envRow struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	type serviceRow struct {
		Code           string  `json:"code"`
		Name           string  `json:"name"`
		BusinessSystem string  `json:"business_system_code"`
		Environment    *string `json:"environment_code"`
		Tier           *string `json:"retention_tier"`
		CollectEnabled bool    `json:"log_collection_enabled"`
	}
	queries := db.New(handler.db)
	projects := []projectRow{}
	if rows, queryErr := queries.ListEnabledProjects(context); queryErr == nil {
		for _, row := range rows {
			projects = append(projects, projectRow{ID: row.ID, Code: row.Code, Name: row.Name})
		}
	}
	bizsystems := []bizsysRow{}
	if rows, queryErr := queries.ListEnabledBusinessSystems(context); queryErr == nil {
		for _, row := range rows {
			var projectID *int64
			if row.ProjectID.Valid {
				value := row.ProjectID.Int64
				projectID = &value
			}
			bizsystems = append(bizsystems, bizsysRow{ID: row.ID, Code: row.Code, Name: row.Name, ProjectID: projectID})
		}
	}
	environments := []envRow{}
	if rows, queryErr := queries.ListEnabledBusinessEnvironments(context); queryErr == nil {
		for _, row := range rows {
			environments = append(environments, envRow{ID: row.ID, Code: row.Code, Name: row.Name})
		}
	}
	services := []serviceRow{}
	if rows, queryErr := queries.ListEnabledServiceStreamRows(context); queryErr == nil {
		for _, row := range rows {
			var environment, tier *string
			if row.EnvironmentCode.Valid {
				value := row.EnvironmentCode.String
				environment = &value
			}
			if row.RetentionTier.Valid {
				value := row.RetentionTier.String
				tier = &value
			}
			services = append(services, serviceRow{
				Code: row.Code, Name: row.Name, BusinessSystem: row.BusinessSystemCode,
				Environment: environment, Tier: tier, CollectEnabled: row.LogCollectionEnabled,
			})
		}
	}

	response.Success(context, gin.H{
		"cluster":      gin.H{"id": cluster.ID, "index_prefix": cluster.IndexPrefix},
		"data_streams": entries,
		"allocation":   allocation,
		"alloc_error":  errorString(allocErr),
		"dims": gin.H{
			"projects":         projects,
			"business_systems": bizsystems,
			"environments":     environments,
			"services":         services,
		},
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetLogServiceUsage 逻辑服务写入量：按 service 字段的 terms 聚合（文档数口径，
// 非磁盘占用）。点进逻辑服务层时按需调用。
func (handler *Handler) GetLogServiceUsage(context *gin.Context) {
	cluster, err := handler.loadElasticsearchCluster(context)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "Elasticsearch cluster not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	businessSystem := strings.TrimSpace(context.Query("business_system"))
	environment := strings.TrimSpace(context.Query("environment"))
	if businessSystem == "" || environment == "" {
		response.BusinessError(context, 400, "business_system 与 environment 必填", nil)
		return
	}
	if strings.ContainsAny(businessSystem, " ,\"*") || strings.ContainsAny(environment, " ,\"*") {
		response.BusinessError(context, 400, "参数包含非法字符", nil)
		return
	}
	days := 30
	if raw := context.Query("days"); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}
	prefix := logHealthPrefix(cluster)
	body := gin.H{
		"size":  0,
		"query": gin.H{"range": gin.H{"@timestamp": gin.H{"gte": fmt.Sprintf("now-%dd", days)}}},
		"aggs":  gin.H{"by_service": gin.H{"terms": gin.H{"field": "service", "size": 500}}},
	}
	result, err := handler.elasticsearchRequest(context, cluster, "POST",
		// 流名 = <prefix>-<项目>-<业务系统>-<环境>-<逻辑服务>-<档位>，项目段不参与查询条件，用 * 通配。
		"/"+prefix+"-*-"+businessSystem+"-"+environment+"-*/_search", body)
	if err != nil {
		response.BusinessError(context, 502, fmt.Sprintf("查询失败: %v", err), nil)
		return
	}
	items := []gin.H{}
	total := 0.0
	if aggregations, ok := result["aggregations"].(map[string]any); ok {
		if byService, ok := aggregations["by_service"].(map[string]any); ok {
			buckets, _ := byService["buckets"].([]any)
			for _, bucketRaw := range buckets {
				bucket, _ := bucketRaw.(map[string]any)
				name, _ := bucket["key"].(string)
				count, _ := bucket["doc_count"].(float64)
				total += count
				items = append(items, gin.H{"service": name, "docs": count})
			}
		}
	}
	response.Success(context, gin.H{
		"business_system": businessSystem,
		"environment":     environment,
		"days":            days,
		"total_docs":      total,
		"items":           items,
		"generated_at":    time.Now().UTC().Format(time.RFC3339),
	})
}
