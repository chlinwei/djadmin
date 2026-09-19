package logcollect

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"autoadmin/internal/shared/logmacro"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// Filebeat 采集配置渲染：按主机上启用的 逻辑服务×日志定义×实例 生成
// /etc/filebeat/inputs.d/<app>__<svc>__<log>.yml。
//
// 职责分层：多行合并由 Filebeat filestream 的 multiline parser 在发送前完成；
// 字段提取/时间戳归一由 Elasticsearch ingest pipeline（处理规则 pipeline_body）承担；
// input 做文件监听 + 维度字段注入，index 用 LogDataStreamName（服务级数据流命名）。
// output（Elasticsearch 地址/账号）与 filebeat.config.inputs 在主配置 filebeat.yml，
// 由 agent 的 configure_filebeat_output 下发。
//
// 渲染是纯函数（数据由 loader 填充），与 SQL 解耦，便于单测。

const (
	filebeatInputsDir = "/etc/filebeat/inputs.d"
	filebeatDataDir   = "/var/lib/filebeat"
)

type logConfigFragment struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type renderedHostLogConfig struct {
	Fragments   []logConfigFragment `json:"fragments"`
	Fingerprint string              `json:"fingerprint"`
	ServiceNum  int                 `json:"service_num"`
	Warnings    []string            `json:"warnings,omitempty"`
	// Errors 是**硬问题**：配置本身不自洽，光靠"跳过并告警"下发只会让这台主机少采，
	// 必须回到配置里去改。当下包括：日志定义名含未展开宏、首行正则/采集过滤正则编译不过、
	// 实例路径含未定义宏、同主机上展开成同一路径（重复采集）。
	//
	// 为什么要单独一份而不是只看 Warnings：保存逻辑服务时要用它**拦住保存**（见
	// CheckServiceLogConfigConsistency），而 Warnings 里还有"未挂解析规则所以不采"这类
	// 按约定允许存在的状态，不能一棍子打死。同一件事仍然会同时进 Warnings，
	// 这样日志采集页/链路体检现有的展示不需要改。
	Errors []string `json:"errors,omitempty"`
}

// hostLogRenderInput 渲染所需的全部维度数据（由 loadHostLogRenderInput 查询填充）。
type hostLogRenderInput struct {
	Prefix         string
	Application    string // 应用 code
	Service        string // 逻辑服务 code
	Instance       string // 实例名（deployment.instance_name）
	Project        string
	Environment    string
	BusinessSystem string
	Tier           string // 有效档位 code（服务级 log_setting 优先，回落服务表）
	Pipeline       string // 有效处理规则 name（空=不过 pipeline）
	LogName        string // 日志定义 name
	ResolvedPath   string
	Macros         map[string]string
	Multiline      bool   // 有效处理规则开启多行合并
	StartPattern   string // 多行首行正则（处理规则）
	FlushTimeout   int64  // 多行 flush 超时（毫秒，处理规则）
	// 采集过滤（2026-09-19）：解析后的白名单/黑名单正则，空串 = 该方向不过滤。
	// 解析规则（模板默认 → 服务覆盖 → 三态）见 log_collection_filter.go；
	// 这里只拿最终结果，纯函数渲染不认识"继承"这种概念。
	FilterIncludePattern string
	FilterExcludePattern string
	// FilterWarnings 该条日志的过滤解析告警（规则不存在/停用/方向不符/正则编译不过），
	// 由渲染汇总进 warnings —— 只写服务端日志的话，用户看不到"过滤没生效"这件事。
	FilterWarnings []string
}

// configBaseName inputs.d 文件名基名：<app>__<service>__<logname>（不带后缀）。
func configBaseName(application, service, logName string) string {
	return fmt.Sprintf("%s__%s__%s", application, service, logName)
}

// resolveMacros 合并服务级与服务实例级变量，替换路径中的 ${VAR}；未定义的保持原样。
//
// 实现委托 internal/shared/logmacro：合并顺序（模板默认 → 服务覆盖 → 实例变量）与界面展示的
// "解析后路径"必须是同一份，否则会出现"界面显示的路径与主机上实际采的不一致"。
func resolveMacros(path string, macroSets ...map[string]string) string {
	return logmacro.Resolve(path, macroSets...)
}

// yamlScalar 用单引号包裹 YAML 标量（单引号转义为两个），保证路径/正则/维度值里的
// 反斜杠、冒号、引号等不会被 YAML 误解析。
func yamlScalar(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// renderHostLogConfig 纯函数：每个 服务×日志定义 一个 inputs.d/<base>.yml，文件内每个实例一个
// filestream input（instance/host_ip 等维度必须按实例区分，不能合并成单 input 多 paths）。
//
// outputIdentity 是**输出段**对指纹的贡献（Elasticsearch 地址/账号/TLS，见 filebeatOutputIdentity）。
// 它不参与片段渲染，但必须进指纹：主配置 filebeat.yml 的内容由它决定，漏掉就会出现
// 「只改集群地址 → 指纹没变 → 下发被跳过 → 主配置停留在旧值」。
func renderHostLogConfig(entries []hostLogRenderInput, instances []hostInstanceInput, outputIdentity string) renderedHostLogConfig {
	type fragmentPair struct {
		base   string
		inputs []string
	}
	pairs := map[string]*fragmentPair{}
	order := []string{}
	warnings := []string{}
	errors := []string{}
	// 同时进 warnings（既有展示不动）与 errors（保存校验用）的硬问题。
	recordHardProblem := func(message string) {
		warnings = append(warnings, message)
		errors = append(errors, message)
	}
	// 本主机上已被占用的绝对路径 → 占用者描述（同一路径重复监听会导致日志翻倍，见下）。
	// 渲染是以**单台主机**为单位调用的，所以这个集合天然就是"主机级"的。
	claimedPaths := map[string]string{}

	instancesByService := map[string][]hostInstanceInput{}
	for _, instance := range instances {
		instancesByService[instance.Service] = append(instancesByService[instance.Service], instance)
	}
	for key := range instancesByService {
		list := instancesByService[key]
		sort.Slice(list, func(i, j int) bool { return list[i].Instance < list[j].Instance })
		instancesByService[key] = list
	}

	for _, entry := range entries {
		base := configBaseName(entry.Application, entry.Service, entry.LogName)
		// 日志定义名称直接作为文件名/维度值，不参与宏展开；名称里带 ${...} 说明配置写错位置，
		// 继续下发会生成监听不到文件的坏片段，这里跳过并告警（宏应写在 path_pattern 里）。
		if strings.Contains(base, "${") {
			recordHardProblem(fmt.Sprintf(
				"日志定义名称含未展开宏，已跳过（宏应写在路径里）: %s", base))
			continue
		}
		// 未关联处理规则 = 没有 pipeline，日志不会被解析：按约定直接不采集（跳过并告警），
		// 避免"采进来了但查不到 log_level/log_message/error_fingerprint"这种半成品数据。
		if strings.TrimSpace(entry.Pipeline) == "" {
			warnings = append(warnings, fmt.Sprintf(
				"服务 %s 日志 %s 未关联处理规则，已跳过（未挂 pipeline 的日志不采集）",
				entry.Service, entry.LogName))
			continue
		}
		// 首行正则落到主机上是由 Filebeat 编译的（Go RE2）：编译不过的片段会让 Filebeat 起不来，
		// **整台主机的日志一起停**——比"少采一个文件"严重得多，所以与上面两条同样"跳过并告警"。
		// 保存路径已按 RE2 校验（见 regex_pattern.go），这里兜的是历史数据与其他环境的存量规则。
		// 过滤解析的告警先冒出来：用户最需要知道的是"我配了过滤但没生效"，而不是先看到别的。
		for _, warning := range entry.FilterWarnings {
			if strings.TrimSpace(warning) == "" {
				continue
			}
			recordHardProblem(fmt.Sprintf("服务 %s 日志 %s：%s", entry.Service, entry.LogName, warning))
		}
		if entry.Multiline {
			if problem := validateRulePattern("首行正则", entry.StartPattern); problem != "" {
				recordHardProblem(fmt.Sprintf(
					"服务 %s 日志 %s 的处理规则 %s 首行正则 Filebeat 编译不过，已跳过（否则 Filebeat 无法启动，该主机所有日志都会停）：%s",
					entry.Service, entry.LogName, entry.Pipeline, problem))
				continue
			}
		}
		pair, exists := pairs[base]
		if !exists {
			pair = &fragmentPair{base: base}
			pairs[base] = pair
			order = append(order, base)
		}
		indexName := LogDataStreamName(entry.Prefix, entry.Project, entry.Environment, entry.BusinessSystem, entry.Service, entry.Tier)
		for _, instance := range instancesByService[entry.Service] {
			resolved := resolveMacros(entry.ResolvedPath, entry.Macros, instance.Macros)
			// 含未展开宏的路径会导致 Filebeat 监听不到任何文件，必须跳过该实例并记录告警。
			if strings.Contains(resolved, "${") {
				recordHardProblem(fmt.Sprintf(
					"服务 %s 实例 %s 日志 %s 的路径含未定义宏，已跳过: %s",
					entry.Service, instance.Instance, entry.LogName, resolved))
				continue
			}
			// 同一主机上**同一个绝对路径**只能被一个 filestream input 监听：两个 input 各自维护 offset，
			// 同一份日志会进 ES 两次（instance 字段不同），下游的错误聚类与容量统计跟着翻倍。
			// 多实例服务最容易踩：实例没配各自的 APP_HOME，展开出来就是同一条路径。
			// 保留先出现的那个（entries 按查询顺序、instances 按实例名排序，结果是确定的），跳过并告警——
			// 与"未展开宏""未挂规则"同一处理：不带病下发，也绝不静默重复采集。
			if owner, exists := claimedPaths[resolved]; exists {
				recordHardProblem(fmt.Sprintf(
					"服务 %s 实例 %s 日志 %s 与 %s 在同一主机上展开成同一路径 %s，已跳过（否则 Filebeat 会重复读取，同一条日志进 ES 两次）：请给实例配置各自的宏（如 APP_HOME）或拆成不同日志定义",
					entry.Service, instance.Instance, entry.LogName, owner, resolved))
				continue
			}
			claimedPaths[resolved] = fmt.Sprintf("服务 %s 实例 %s 日志 %s", entry.Service, instance.Instance, entry.LogName)
			lines := []string{
				"- type: filestream",
				fmt.Sprintf("  id: %s", yamlScalar(base+"__"+instance.Instance)),
				"  enabled: true",
				"  paths:",
				fmt.Sprintf("    - %s", yamlScalar(resolved)),
				"  fields_under_root: true",
				"  fields:",
				fmt.Sprintf("    service: %s", yamlScalar(entry.Service)),
				fmt.Sprintf("    instance: %s", yamlScalar(instance.Instance)),
				fmt.Sprintf("    application: %s", yamlScalar(entry.Application)),
				fmt.Sprintf("    log_name: %s", yamlScalar(entry.LogName)),
				fmt.Sprintf("    business_system: %s", yamlScalar(entry.BusinessSystem)),
				fmt.Sprintf("    project: %s", yamlScalar(entry.Project)),
				fmt.Sprintf("    environment: %s", yamlScalar(entry.Environment)),
			}
			// host_ip 是索引模板里的必备字段之一：主机资产没采到 IP 时也写空串，
			// 保证字段一定存在（否则"少一个字段都不行"的判定会把整台主机的日志判违规）。
			lines = append(lines, fmt.Sprintf("    host_ip: %s", yamlScalar(instance.HostIP)))
			// log_path 是该实例实际监听的绝对路径，便于按文件定位日志来源
			//（索引模板 standardLogFields 已声明 log_path/project 为 keyword，此前从未注入导致两列恒空）。
			lines = append(lines, fmt.Sprintf("    log_path: %s", yamlScalar(resolved)))
			lines = append(lines, fmt.Sprintf("  index: %s", yamlScalar(indexName)))
			if entry.Pipeline != "" {
				// pipeline id 与发布/删除/体检统一：<前缀>-<应用 code|general>-<规则名>。
				lines = append(lines, fmt.Sprintf("  pipeline: %s", yamlScalar(processingPipelineName(entry.Prefix, entry.Application, entry.Pipeline))))
			}
			// 采集过滤：写进 input 的 include_lines / exclude_lines（Filebeat 上是行级正则，
			// 且在 multiline 合并**之后**评估，语义与 Fluent Bit 时代的 [FILTER] grep 一致）。
			// 正则编译不过时上面已经把它降级成空串并在 warnings 里说明，所以这里直接写。
			if pattern := strings.TrimSpace(entry.FilterIncludePattern); pattern != "" {
				lines = append(lines, "  include_lines:", "    - "+yamlScalar(pattern))
			}
			if pattern := strings.TrimSpace(entry.FilterExcludePattern); pattern != "" {
				lines = append(lines, "  exclude_lines:", "    - "+yamlScalar(pattern))
			}
			if entry.Multiline && strings.TrimSpace(entry.StartPattern) != "" {
				timeout := entry.FlushTimeout
				if timeout <= 0 {
					timeout = 2000
				}
				lines = append(lines,
					"  parsers:",
					"    - multiline:",
					fmt.Sprintf("        pattern: %s", yamlScalar(entry.StartPattern)),
					"        negate: true",
					"        match: after",
					fmt.Sprintf("        timeout: %s", yamlScalar(fmt.Sprintf("%dms", timeout))),
				)
			}
			pair.inputs = append(pair.inputs, strings.Join(lines, "\n")+"\n")
		}
	}

	fragments := []logConfigFragment{}
	for _, base := range order {
		pair := pairs[base]
		if len(pair.inputs) == 0 {
			continue
		}
		fragments = append(fragments, logConfigFragment{Path: filebeatInputsDir + "/" + base + ".yml", Content: strings.Join(pair.inputs, "\n")})
	}
	sort.Slice(fragments, func(i, j int) bool { return fragments[i].Path < fragments[j].Path })
	// registry/data 目录由 agent 对文件路径做 MkdirAll 时顺带建出。
	if len(fragments) > 0 {
		fragments = append(fragments, logConfigFragment{Path: filebeatDataDir + "/.keep", Content: ""})
	}
	// 指纹 = 输出段标识 + 全部片段内容。两者任一变化都必须触发重新下发。
	digest := sha256.Sum256([]byte(fmt.Sprintf("output:%s\n%v", outputIdentity, fragments)))
	return renderedHostLogConfig{
		Fragments:   fragments,
		Fingerprint: fmt.Sprintf("%x", digest),
		ServiceNum:  len(instancesByService),
		Warnings:    warnings,
		Errors:      errors,
	}
}

type hostInstanceInput struct {
	Service  string
	Instance string
	HostIP   string
	Macros   map[string]string
}

// renderInputSet 单台主机的渲染输入：服务×日志定义（entries）与服务×实例（instances）。
type renderInputSet struct {
	Entries   []hostLogRenderInput
	Instances []hostInstanceInput
}

// loadHostLogRenderInputs 批量装载若干主机的渲染输入：**两条查询覆盖全部入参主机**，
// 不按主机循环查库（500–1000 台规模下这是硬约束，见计划文档 §2.4 的算力约束）。
// 返回按 host_id 分组的输入；没有任何采集配置的主机不会出现在 map 里。
//
// 行序不影响结果：片段最终按路径排序、实例按名称排序后才参与渲染与指纹。
func (handler *Handler) loadHostLogRenderInputs(context context.Context, hostIDs []int64) (map[int64]renderInputSet, error) {
	return loadHostLogRenderInputsPool(context, handler.db, hostIDs)
}

// loadHostLogRenderInputsPool 与上面的唯一差别是取数走传入的 pool（*sql.DB 或事务）：
// 保存逻辑服务时的"配置自洽性校验"要在**同一个事务里**读到刚写入的状态（见 CheckServiceLogConfigConsistency）。
func loadHostLogRenderInputsPool(context context.Context, pool db.DBTX, hostIDs []int64) (map[int64]renderInputSet, error) {
	sets := map[int64]renderInputSet{}
	if len(hostIDs) == 0 {
		return sets, nil
	}
	queries := db.New(pool)

	instanceRows, err := queries.ListHostLogRenderInstances(context, hostIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range instanceRows {
		set := sets[row.HostID]
		set.Instances = append(set.Instances, hostInstanceInput{
			Service: row.ServiceCode, Instance: row.InstanceName, HostIP: row.HostIp,
			Macros: instanceMacros(string(row.RuntimeVariables), row.AppHome),
		})
		sets[row.HostID] = set
	}

	entryRows, err := queries.ListHostLogRenderEntries(context, hostIDs)
	if err != nil {
		return nil, err
	}
	// 采集过滤：先把这批条目引用到的规则一次性取回来，再逐条解析（模板默认 → 服务覆盖三态）。
	// 规则数量很少，一次查询就够；放这里是因为渲染要的是"最终生效的两条正则"，
	// 而不是"哪个 id"——解析与降级逻辑集中在 resolveLogFilter 里，便于单测。
	ruleIDs := []int64{}
	for _, row := range entryRows {
		for _, id := range []sql.NullInt64{row.FilterIncludeRuleID, row.FilterExcludeRuleID, row.CollectionFilterRuleID, row.CollectionExcludeFilterRuleID} {
			if id.Valid && id.Int64 > 0 {
				ruleIDs = append(ruleIDs, id.Int64)
			}
		}
	}
	rules, err := loadLogFilterRulesPool(context, pool, ruleIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range entryRows {
		set := sets[row.HostID]
		resolved := resolveLogFilter(
			nullableToPointer(row.FilterIncludeRuleID), nullableToPointer(row.FilterExcludeRuleID),
			nullableToPointer(row.CollectionFilterRuleID), nullableToPointer(row.CollectionExcludeFilterRuleID),
			rules,
		)
		includePattern, warning := "", ""
		if resolved.Include != nil {
			includePattern, warning = validFilterPattern(logFilterRuleInclude, *resolved.Include)
		}
		excludePattern := ""
		if resolved.Exclude != nil {
			excludePattern, warning = validFilterPattern(logFilterRuleExclude, *resolved.Exclude)
		}
		set.Entries = append(set.Entries, hostLogRenderInput{
			Prefix: "autoadmin", Application: row.ApplicationCode, Service: row.ServiceCode,
			Project: row.ProjectCode, Environment: row.EnvironmentCode,
			BusinessSystem: row.BusinessSystemCode, Tier: row.TierCode,
			Pipeline: row.PipelineName, LogName: row.LogName, ResolvedPath: row.PathPattern,
			// 宏优先级：部署模板 macro_definitions 作默认值，服务级 macro_values 覆盖同名项。
			Macros: mergeMacroValues(
				templateMacroDefaults(string(row.MacroDefinitions)),
				parseMacroJSON(string(row.MacroValues)),
			),
			Multiline: row.MultilineEnabled, StartPattern: row.StartPattern,
			FlushTimeout: int64(row.FlushTimeout),
			// 坏规则/规则被删/方向不符：降级成"该方向不过滤"并把原因带给用户（见文件头取舍）。
			FilterIncludePattern: includePattern, FilterExcludePattern: excludePattern,
			FilterWarnings: append(append([]string{}, resolved.Warnings...), warning),
		})
		sets[row.HostID] = set
	}
	return sets, nil
}

// loadHostLogRenderInput 单台主机的渲染输入（下发、预览用）；批量场景用 loadHostLogRenderInputs。
func (handler *Handler) loadHostLogRenderInput(context context.Context, hostID int64) ([]hostLogRenderInput, []hostInstanceInput, error) {
	sets, err := handler.loadHostLogRenderInputs(context, []int64{hostID})
	if err != nil {
		return nil, nil, err
	}
	set := sets[hostID]
	return set.Entries, set.Instances, nil
}

// instanceMacros 构造实例级宏：运行时变量为基底，部署模板 app_home 作为 APP_HOME
// 默认值（实例变量显式配置的值优先）。APP_HOME 的权威来源是部署模板（与巡检模块
// 一致）；未接入该来源会导致日志路径 ${APP_HOME} 永远无法展开。
func instanceMacros(runtimeVariablesRaw, appHome string) map[string]string {
	return logmacro.InstanceValues(runtimeVariablesRaw, appHome)
}

func parseMacroJSON(raw string) map[string]string {
	return logmacro.ParseValues(raw)
}

// templateMacroDefaults 摊平部署模板的 macro_definitions 作为默认值（服务级覆盖同名项）。
func templateMacroDefaults(raw string) map[string]string {
	return logmacro.TemplateDefaults(raw)
}

func mergeMacroValues(macroSets ...map[string]string) map[string]string {
	return logmacro.Merge(macroSets...)
}

func (handler *Handler) GetHostLogConfigPreview(context *gin.Context) {
	row, err := loadLogTarget(context, handler.db, parseID(context.Param("id")))
	if err != nil {
		response.Error(context, err)
		return
	}
	entries, instances, err := handler.loadHostLogRenderInput(context, row.HostID)
	if err != nil {
		response.Error(context, err)
		return
	}
	// 与下发一致：索引前缀/数据流名/pipeline 名都用默认集群的 index_prefix；
	// 输出段标识也取自同一个集群，这样预览指纹与下发时写入的指纹可比。
	outputIdentity := ""
	if cluster, clusterErr := db.New(handler.db).GetDefaultEnabledElasticsearchCluster(context); clusterErr == nil {
		for index := range entries {
			entries[index].Prefix = cluster.IndexPrefix
		}
		outputIdentity, _ = filebeatOutputIdentity(cluster.Hosts, cluster.Username, cluster.VerifyTls)
	}
	rendered := renderHostLogConfig(entries, instances, outputIdentity)
	response.Success(context, rendered)
}
