package logcollect

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
}

// configBaseName inputs.d 文件名基名：<app>__<service>__<logname>（不带后缀）。
func configBaseName(application, service, logName string) string {
	return fmt.Sprintf("%s__%s__%s", application, service, logName)
}

// resolveMacros 合并服务级与服务实例级变量，替换路径中的 ${VAR}；未定义的保持原样。
func resolveMacros(path string, macroSets ...map[string]string) string {
	merged := map[string]string{}
	for _, set := range macroSets {
		for key, value := range set {
			merged[key] = value
		}
	}
	result := path
	for key, value := range merged {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	return result
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
			warnings = append(warnings, fmt.Sprintf(
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
				warnings = append(warnings, fmt.Sprintf(
					"服务 %s 实例 %s 日志 %s 的路径含未定义宏，已跳过: %s",
					entry.Service, instance.Instance, entry.LogName, resolved))
				continue
			}
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
			if instance.HostIP != "" {
				lines = append(lines, fmt.Sprintf("    host_ip: %s", yamlScalar(instance.HostIP)))
			}
			// log_path 是该实例实际监听的绝对路径，便于按文件定位日志来源
			//（索引模板 standardLogFields 已声明 log_path/project 为 keyword，此前从未注入导致两列恒空）。
			lines = append(lines, fmt.Sprintf("    log_path: %s", yamlScalar(resolved)))
			lines = append(lines, fmt.Sprintf("  index: %s", yamlScalar(indexName)))
			if entry.Pipeline != "" {
				// pipeline id 与发布/删除/体检统一：<前缀>-<应用 code|general>-<规则名>。
				lines = append(lines, fmt.Sprintf("  pipeline: %s", yamlScalar(processingPipelineName(entry.Prefix, entry.Application, entry.Pipeline))))
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
func (handler *Handler) loadHostLogRenderInputs(context *gin.Context, hostIDs []int64) (map[int64]renderInputSet, error) {
	sets := map[int64]renderInputSet{}
	if len(hostIDs) == 0 {
		return sets, nil
	}
	queries := db.New(handler.db)

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
	for _, row := range entryRows {
		set := sets[row.HostID]
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
		})
		sets[row.HostID] = set
	}
	return sets, nil
}

// loadHostLogRenderInput 单台主机的渲染输入（下发、预览用）；批量场景用 loadHostLogRenderInputs。
func (handler *Handler) loadHostLogRenderInput(context *gin.Context, hostID int64) ([]hostLogRenderInput, []hostInstanceInput, error) {
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
	macros := parseMacroJSON(runtimeVariablesRaw)
	if appHome = strings.TrimSpace(appHome); appHome != "" {
		if _, exists := macros["APP_HOME"]; !exists {
			macros["APP_HOME"] = appHome
		}
	}
	return macros
}

func parseMacroJSON(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return map[string]string{}
	}
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return map[string]string{}
	}
	result := map[string]string{}
	for key, value := range decoded {
		result[key] = strings.TrimSpace(fmt.Sprint(value))
	}
	return result
}

// templateMacroDefaults 把部署模板的 macro_definitions（[{name,value,description}]）摊平成
// name→value，作为宏解析的默认值；服务级 macro_values 覆盖同名项。
// 与服务弹窗展示口径一致（弹窗显示 服务覆盖 ?? 模板默认），避免前端看着有值、后端展不开。
func templateMacroDefaults(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" {
		return map[string]string{}
	}
	var definitions []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(trimmed), &definitions); err != nil {
		return map[string]string{}
	}
	result := map[string]string{}
	for _, definition := range definitions {
		name := strings.TrimSpace(definition.Name)
		if name == "" {
			continue
		}
		result[name] = strings.TrimSpace(definition.Value)
	}
	return result
}

// mergeMacroValues 合并宏集合：后面的覆盖前面的（默认值在前、覆盖值在后）。
func mergeMacroValues(sets ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, set := range sets {
		for key, value := range set {
			merged[key] = value
		}
	}
	return merged
}

// GetHostLogConfigPreview 预览采集目标主机的 Filebeat inputs 片段（不下发）：路由参数为采集目标 id。
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
