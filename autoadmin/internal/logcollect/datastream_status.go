package logcollect

import (
	"context"
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
	Name           string  `json:"name"`
	Project        string  `json:"project"`
	Environment    string  `json:"environment"`
	BusinessSystem string  `json:"business_system"`
	Service        string  `json:"service"`
	Tier           string  `json:"tier"`
	Health         string  `json:"health"`
	Docs           float64 `json:"docs"`
	Bytes          float64 `json:"bytes"`
	ILMState       string  `json:"ilm_state"`
	Recognized     bool    `json:"recognized"`
	// Historical 改档位后留下的历史流：属于某个服务，但它的档位已不在该服务的生效集合里
	// （已停止写入，数据按原档位保留到期）。由后端判定，前端不再自己推。
	Historical bool `json:"historical"`
	// 服务级采集开关（配置事实，直接来自逻辑服务行）：页面据此标注"已停用 / 未开启采集"，
	// 表示这条流不会再被写入新的配置，存量数据按档位保留到期（计划 §3）。
	ServiceEnabled     bool                     `json:"service_enabled"`
	CollectedByService bool                     `json:"service_collection_enabled"`
	BackingIndices     []dataStreamBackingIndex `json:"backing_indices"`
}

// data stream 后备索引名为 .ds-<stream>-<generation>（如 .ds-logs-kul-test-tib-hot-000015），
// 传统索引为 <stream>-<YYYY.MM.DD>；两种后缀都要剥离才能还原流名。
var backingIndexDateSuffix = regexp.MustCompile(`-\d{4}\.\d{2}\.\d{2}(-\d+)?$`)
var backingIndexGenerationSuffix = regexp.MustCompile(`-\d{5,}$`)

// parsedStreamName：流名解析结果。ServiceEnabled / ServiceCollectEnabled 只在识别成功时有效，
// 取自逻辑服务行（DISTINCT 维度码已按服务去重，同码服务视为同一服务）。
type parsedStreamName struct {
	Stream, Project, Environment, BusinessSystem, Service, Tier string
	// ServiceID 是匹配到的逻辑服务主键（服务编码不再全局唯一，判定"生效档位"必须按 id）。
	ServiceID             int64
	Recognized            bool
	ServiceEnabled        bool
	ServiceCollectEnabled bool
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
	// activeTiers：逻辑服务 id → 该服务**当前生效**的档位集合。用于判定一条已有的流是不是
	// "改档位留下的历史流"（不在集合里 = 已停止写入）。判定放后端做，全局视图才推得出来。
	// 没有任何日志定义的服务不在 map 里 → 它的所有档位都不生效 → 存量流全判为历史流。
	// 用 id 而不是编码：编码现在允许跨业务/环境重复（见 000044 迁移）。
	activeTiers map[int64]map[string]bool
}

// isHistoricalStream 流是否已停止写入：属于某个服务，但它的档位不在该服务的生效档位集合里。
func (m streamNameMatcher) isHistoricalStream(serviceID int64, tierCode string) bool {
	if serviceID == 0 || tierCode == "" {
		return false
	}
	return !m.activeTiers[serviceID][tierCode]
}

type streamServiceKey struct {
	Match                                         string // "<项目>-<业务系统>-<环境>-<逻辑服务>-"
	Project, Environment, BusinessSystem, Service string
	ServiceID                                     int64
	// 服务级采集开关，随解析结果一路带到页面（见 dataStreamEntry 的说明）。
	Enabled, CollectEnabled bool
}

type streamLegacyKey struct {
	Match                                string // "<环境>-<业务系统>-"
	Project, Environment, BusinessSystem string
}

// scopeIndexPattern 计算索引通配模式：服务为空＝全量 `<prefix>-*`；
// 非空＝按该服务的维度段收窄成 `<prefix>-<项目>-<业务系统>-<环境>-<服务>-*`。
//
// 优先按 serviceID 匹配：编码允许跨业务/环境重复，只凭 code 会命中错的维度段。
// serviceCode 仅作旧调用方的兜底（取第一个匹配）。第一个返回值是通配模式；
// 第二个返回值表示该服务是否存在于维度表——不存在时调用方必须返回空视图，不能回落到全量。
func scopeIndexPattern(prefix string, serviceID int64, serviceCode string, matcher streamNameMatcher) (string, bool) {
	if serviceID <= 0 && strings.TrimSpace(serviceCode) == "" {
		return prefix + "-*", true
	}
	for _, key := range matcher.services {
		if serviceID > 0 && key.ServiceID == serviceID {
			return prefix + "-" + key.Match + "*", true
		}
		if serviceID <= 0 && key.Service == serviceCode {
			return prefix + "-" + key.Match + "*", true
		}
	}
	return "", false
}

// loadStreamDims 加载**全部**逻辑服务的维度码，构造流名匹配候选（新命名 + 旧命名兼容）。
//
// 这里刻意不按 `enabled` 过滤：识别回答的是"这条已有的流属于哪个已知服务"，
// 停用不影响归属，只影响下发（见 ListServiceStreamDims 查询上的说明）。
func (handler *Handler) loadStreamDims(context *gin.Context, prefix string) streamNameMatcher {
	matcher := streamNameMatcher{prefix: prefix, tiers: map[string]bool{}}
	var serviceKeys []streamServiceKey
	var legacyKeys []streamLegacyKey
	queries := db.New(handler.db)
	if rows, err := queries.ListServiceStreamDims(context); err == nil {
		for _, row := range rows {
			serviceKeys = append(serviceKeys, streamServiceKey{
				Match:   strings.Join([]string{row.ProjectCode, row.BusinessSystemCode, row.EnvironmentCode, row.ServiceCode}, "-") + "-",
				Project: row.ProjectCode, Environment: row.EnvironmentCode,
				BusinessSystem: row.BusinessSystemCode, Service: row.ServiceCode, ServiceID: row.ServiceID,
				Enabled: row.ServiceEnabled, CollectEnabled: row.LogCollectionEnabled,
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
	// 兜底：把所有档位码也带上，旧命名流（无服务维度）至少档位能对上。
	// 用不过滤 enabled 的 ListRetentionTierCodes：停用一个档位不该让既有旧命名流变成"未识别"
	//（下拉/管理路径要的"只列启用档位"仍用 ListEnabledRetentionTiers）。
	if tierCodes, tierErr := queries.ListRetentionTierCodes(context); tierErr == nil {
		for _, code := range tierCodes {
			if code != "" {
				matcher.tiers[code] = true
			}
		}
	}
	matcher.services = serviceKeys
	matcher.legacy = legacyKeys
	matcher.activeTiers = loadActiveStreamTiers(context, queries)
	return matcher
}

// loadActiveStreamTiers 逻辑服务 id → 生效档位集合（查询失败时返回空 map：判不出就都不标，
// 宁可少标"历史流"，也不要把正在写的流错标成停写）。
func loadActiveStreamTiers(context context.Context, queries *db.Queries) map[int64]map[string]bool {
	active := map[int64]map[string]bool{}
	rows, err := queries.ListServiceActiveStreamTiers(context)
	if err != nil {
		return active
	}
	for _, row := range rows {
		if row.ServiceID <= 0 || row.TierCode == "" {
			continue
		}
		if active[row.ServiceID] == nil {
			active[row.ServiceID] = map[string]bool{}
		}
		active[row.ServiceID][row.TierCode] = true
	}
	return active
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
			result.ServiceID = candidate.ServiceID
			result.Recognized = true
			result.ServiceEnabled = candidate.Enabled
			result.ServiceCollectEnabled = candidate.CollectEnabled
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
//
// indexPattern 是索引通配模式（含前缀与 `*`），由调用方给：全量视图传 `<prefix>-*`，
// 按服务收窄的视图传 `<prefix>-<项目>-<业务系统>-<环境>-<服务>-*`。收窄放在 ES 查询里
// 而不是查完再过滤，因为 `_cat/indices` 是这一页最大的成本（全集群索引 + ILM explain）。
func (handler *Handler) fetchDataStreamEntries(context *gin.Context, cluster elasticsearchCluster, matcher streamNameMatcher, indexPattern string) ([]dataStreamEntry, map[string]string, error) {
	indices, err := handler.elasticsearchRequestArray(context, cluster, "GET",
		"/_cat/indices/"+indexPattern+"?format=json&h=index,health,status,docs.count,store.size,creation.date.string&bytes=b")
	if err != nil {
		return nil, nil, fmt.Errorf("查询索引列表失败: %w", err)
	}
	ilmStates := map[string]string{}
	// ES 8 的 ILM explain：GET <prefix>-*/_ilm/explain -> {"indices":{"<index>":{"phase":"hot",...}}}，
	// 未挂策略/未托管的索引没有 phase（只有 step）。没有匹配索引时返回 {"indices":{}}。
	explain, explainErr := handler.elasticsearchRequest(context, cluster, "GET", "/"+indexPattern+"/_ilm/explain", nil)
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
		parsed := matcher.resolveStreamName(stripBackingIndexSuffixes(matcher.prefix, index))
		entry, exists := entries[parsed.Stream]
		if !exists {
			entry = &dataStreamEntry{
				Name: parsed.Stream, Project: parsed.Project, Environment: parsed.Environment,
				BusinessSystem: parsed.BusinessSystem, Service: parsed.Service, Tier: parsed.Tier,
				Recognized: parsed.Recognized,
				// 旧命名流没有服务段，识别不出服务，两个开关都没有意义（保持 false，页面不标注）。
				ServiceEnabled: parsed.ServiceEnabled, CollectedByService: parsed.ServiceCollectEnabled,
				// 历史流判定：服务认得出来 + 档位不在该服务的生效集合里（= 改档位留下的旧流）。
				Historical: matcher.isHistoricalStream(parsed.ServiceID, parsed.Tier),
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
//
// 可选参数 `application_service_id`：只取该逻辑服务的流（优先；编码可跨业务/环境重复，用 id 才无歧义）。
// 兼容旧的 `service_code` 参数（按编码取第一个匹配）。给出口径一致的服务级视图（日志中心页的
// 「本服务水位」tab）用，并且是**收窄 ES 查询**而不是查完再过滤——见 scopeIndexPattern。
// 不带这两个参数时行为与以前完全一致（全量视图）。
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
	prefix := logHealthPrefix(cluster)
	matcher := handler.loadStreamDims(context, prefix)
	// 优先用 application_service_id：编码允许跨业务/环境重复，只凭 service_code 可能命中错的维度段。
	serviceID := parseID(context.Query("application_service_id"))
	serviceCode := strings.TrimSpace(context.Query("service_code"))
	indexPattern, scopeFound := scopeIndexPattern(prefix, serviceID, serviceCode, matcher)
	if !scopeFound {
		// 给了服务但这个服务不在维度表里（已删除/编码写错）：返回空视图，**不能回落到全量**，
		// 否则调用方以为看到的是"这个服务的流"，实际拿到整个集群。
		response.Success(context, gin.H{
			"data_streams": []any{}, "dims": gin.H{"projects": []any{}, "business_systems": []any{}, "environments": []any{}},
			"scope": gin.H{"service_code": serviceCode, "application_service_id": serviceID, "found": false},
		})
		return
	}
	entries, _, err := handler.fetchDataStreamEntries(context, cluster, matcher, indexPattern)
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
	// serviceRow：服务维度。2026-09-18 曾因"前端从未消费"删掉过这份 payload，现在日志中心的
	// 容量统计要"把每一层的成员都列全（含没有任何日志的）"，以及反推"某业务系统下的环境"
	// （环境是服务上的属性，不挂在业务系统下）——有了真正的消费方，所以加回来。
	type serviceRow struct {
		ID             int64  `json:"id"`
		Code           string `json:"code"`
		Name           string `json:"name"`
		BusinessSystem string `json:"business_system"`
		Environment    string `json:"environment"`
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
	if rows, queryErr := queries.ListEnabledServiceStreamDims(context); queryErr == nil {
		for _, row := range rows {
			services = append(services, serviceRow{
				ID: row.ID, Code: row.Code, Name: row.Name,
				BusinessSystem: row.BusinessSystem, Environment: row.Environment,
			})
		}
	}
	response.Success(context, gin.H{
		"cluster":      gin.H{"id": cluster.ID, "index_prefix": cluster.IndexPrefix},
		"data_streams": entries,
		"allocation":   allocation,
		"alloc_error":  errorString(allocErr),
		// dims：树的层级元数据，四层都给（projects / business_systems / environments / services）。
		// services 曾在 2026-09-18 因"前端从未消费"删除；现在日志中心的容量统计需要"把每一层的
		// 成员都列全（含没有任何日志的）"，且"某业务系统下的环境"只能由它名下服务反推
		// （环境是服务上的属性，不挂在业务系统下）——消费方出现了，所以连同查询一起加回来。
		// 服务级的采集开关仍随每条流返回：service_enabled / service_collection_enabled。
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
		// 按 [service, project] 多键聚合：业务系统编码只在项目内唯一、服务编码又只在
		// (业务系统, 环境) 内唯一（000044 迁移），单按 service 会把不同项目下的同名服务合并。
		"aggs": gin.H{"by_service": gin.H{"multi_terms": gin.H{
			"terms": []any{gin.H{"field": "service"}, gin.H{"field": "project"}},
			"size":  500,
		}}},
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
				name, project := "", ""
				if key, ok := bucket["key"].([]any); ok {
					if len(key) > 0 {
						name, _ = key[0].(string)
					}
					if len(key) > 1 {
						project, _ = key[1].(string)
					}
				}
				count, _ := bucket["doc_count"].(float64)
				total += count
				items = append(items, gin.H{"service": name, "project": project, "docs": count})
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
