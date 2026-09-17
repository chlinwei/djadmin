package monitor

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
func renderHostLogConfig(entries []hostLogRenderInput, instances []hostInstanceInput) renderedHostLogConfig {
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
				fmt.Sprintf("    environment: %s", yamlScalar(entry.Environment)),
			}
			if instance.HostIP != "" {
				lines = append(lines, fmt.Sprintf("    host_ip: %s", yamlScalar(instance.HostIP)))
			}
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
	digest := sha256.Sum256([]byte(fmt.Sprintf("%v", fragments)))
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

// loadHostLogRenderInput 查询主机上启用采集的服务×日志定义及全部维度码。
func (handler *Handler) loadHostLogRenderInput(context *gin.Context, hostID int64) ([]hostLogRenderInput, []hostInstanceInput, error) {
	rows, err := handler.db.QueryContext(context, `
		SELECT s.code, d.instance_name, COALESCE(d.runtime_variables, '{}'), COALESCE(t.app_home, ''), COALESCE(h.ip, '')
		FROM assets_application_service_deployment sd
		JOIN assets_application_deployment d ON d.id = sd.deployment_id
		JOIN assets_application_service s ON s.id = sd.service_id
		JOIN assets_application_deployment_template t ON t.id = s.deployment_template_id
		JOIN assets_host h ON h.id = d.host_id
		WHERE d.host_id = ? AND sd.enabled = TRUE AND d.enabled = TRUE AND s.enabled = TRUE AND s.log_collection_enabled = TRUE`, hostID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	instances := []hostInstanceInput{}
	for rows.Next() {
		var code string
		var instanceName string
		var runtimeVariablesRaw string
		var appHome string
		var hostIP string
		if err = rows.Scan(&code, &instanceName, &runtimeVariablesRaw, &appHome, &hostIP); err != nil {
			return nil, nil, err
		}
		macros := instanceMacros(runtimeVariablesRaw, appHome)
		instances = append(instances, hostInstanceInput{Service: code, Instance: instanceName, HostIP: hostIP, Macros: macros})
	}
	rows.Close()

	rows, err = handler.db.QueryContext(context, `
		SELECT DISTINCT p.code, e.code, bs.code, s.code, s.application_id,
			COALESCE(app.code, ''),
			COALESCE(tier.code, ''),
			COALESCE(rule_setting.name, rule_definition.name, ''),
			ld.name, ld.path_pattern, COALESCE(s.macro_values, '{}'),
			COALESCE(rule_setting.multiline_enabled, rule_definition.multiline_enabled, FALSE),
			COALESCE(rule_setting.start_pattern, rule_definition.start_pattern, ''),
			COALESCE(rule_setting.flush_timeout, rule_definition.flush_timeout, 2000)
		FROM assets_application_service s
		JOIN assets_business_system bs ON bs.id = s.business_system_id
		JOIN assets_project p ON p.id = bs.project_id
		JOIN assets_business_environment e ON e.id = s.environment_id
		JOIN assets_application app ON app.id = s.application_id
		JOIN assets_application_log_definition ld ON ld.deployment_template_id = s.deployment_template_id
			AND ld.collection_enabled = TRUE
		LEFT JOIN assets_application_service_log_setting ls ON ls.service_id = s.id AND ls.log_definition_id = ld.id
		LEFT JOIN monitor_log_retention_tier tier ON tier.id = COALESCE(ls.retention_tier_id, s.log_retention_tier_id)
		LEFT JOIN monitor_log_processing_rule rule_setting ON rule_setting.id = ls.processing_rule_id
		LEFT JOIN monitor_log_processing_rule rule_definition ON rule_definition.id = ld.processing_rule_id
		WHERE s.enabled = TRUE AND s.log_collection_enabled = TRUE
		AND COALESCE(ls.collection_enabled, ld.collection_enabled) = TRUE
		AND s.id IN (SELECT sd.service_id FROM assets_application_service_deployment sd JOIN assets_application_deployment d ON d.id = sd.deployment_id WHERE d.host_id = ? AND sd.enabled = TRUE AND d.enabled = TRUE)`, hostID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	entries := []hostLogRenderInput{}
	for rows.Next() {
		var entry hostLogRenderInput
		// Scan 顺序必须与 SELECT 列一致：项目/环境/业务系统/服务（此前项目与服务两个
		// 变量写反，导致流名 project 段填了服务码、service 段填了项目码）。
		var projectCode, environmentCode, businessSystemCode, serviceCode string
		var applicationID int64
		var application, tier, pipeline, logName, pathPattern, macroValuesRaw string
		var multilineEnabled bool
		var startPattern string
		var flushTimeout int64
		if err = rows.Scan(&projectCode, &environmentCode, &businessSystemCode, &serviceCode, &applicationID,
			&application, &tier, &pipeline, &logName, &pathPattern, &macroValuesRaw,
			&multilineEnabled, &startPattern, &flushTimeout); err != nil {
			return nil, nil, err
		}
		entry = hostLogRenderInput{
			Prefix: "autoadmin", Application: application, Service: serviceCode,
			Project: projectCode, Environment: environmentCode, BusinessSystem: businessSystemCode,
			Tier: tier, Pipeline: pipeline, LogName: logName,
			ResolvedPath: pathPattern, Macros: parseMacroJSON(macroValuesRaw),
			Multiline: multilineEnabled, StartPattern: startPattern, FlushTimeout: flushTimeout,
		}
		entries = append(entries, entry)
	}
	return entries, instances, nil
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
	// 与下发一致：索引前缀/数据流名/pipeline 名都用默认集群的 index_prefix。
	if cluster, clusterErr := db.New(handler.db).GetDefaultEnabledElasticsearchCluster(context); clusterErr == nil {
		for index := range entries {
			entries[index].Prefix = cluster.IndexPrefix
		}
	}
	rendered := renderHostLogConfig(entries, instances)
	response.Success(context, rendered)
}
