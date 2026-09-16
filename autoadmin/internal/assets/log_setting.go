package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"autoadmin/internal/shared/logstream"
)

// ServiceTemplateLog：编辑逻辑服务弹窗"模板日志"表格行——部署模板的日志定义 + 服务级覆盖。
type ServiceTemplateLog struct {
	LogDefinition             int64  `json:"log_definition"`
	Name                      string `json:"name"`
	PathPattern               string `json:"path_pattern"`
	ResolvedPath              string `json:"resolved_path"`
	DataStream                string `json:"data_stream"`
	TemplateCollectionEnabled bool   `json:"template_collection_enabled"`
	RetentionTier             *int64 `json:"retention_tier"`
	CollectionEnabled         *bool  `json:"collection_enabled"`
	CollectionFilterRuleID    *int64 `json:"collection_filter_rule_id"`
	ProcessingRuleID          *int64 `json:"processing_rule_id"`
}

// ListServiceTemplateLogs 返回服务所属部署模板的全部日志定义，并合入该服务的覆盖值
// （无覆盖行时三个覆盖字段为 null，前端按"跟随模板"三态展示）。
// resolved_path 用服务级 macro_values 替换 ${VAR} 后回显（实例级宏因逐实例而异不在
// 此处展开）；data_stream 按 monitor.LogDataStreamName 的同一构造规则生成
// （logs-<项目>-<业务系统>-<环境>-<服务>-<有效档位>，档位 = 覆盖档位 → 服务默认档位
// → is_default 档位 → 'std'），与 Fluent Bit 下发的 Index 命名保持一致。
func (r *Repository) ListServiceTemplateLogs(ctx context.Context, serviceID int64) ([]ServiceTemplateLog, error) {
	rows, err := r.pool.QueryContext(ctx, `
		SELECT ld.id, ld.name, ld.path_pattern, ld.collection_enabled,
		       ls.retention_tier_id, ls.collection_enabled, ls.collection_filter_rule_id, ls.processing_rule_id,
		       s.code, p.code, e.code, bs.code,
		       COALESCE(s.macro_values, '{}'),
		       COALESCE(tier.code, (SELECT code FROM monitor_log_retention_tier WHERE is_default = TRUE ORDER BY id LIMIT 1), 'std')
		FROM assets_application_log_definition ld
		JOIN assets_application_service s ON s.id = ?
		LEFT JOIN assets_application_service_log_setting ls
			ON ls.log_definition_id = ld.id AND ls.service_id = s.id
		JOIN assets_business_system bs ON bs.id = s.business_system_id
		JOIN assets_project p ON p.id = bs.project_id
		LEFT JOIN assets_business_environment e ON e.id = s.environment_id
		LEFT JOIN monitor_log_retention_tier tier ON tier.id = COALESCE(ls.retention_tier_id, s.log_retention_tier_id)
		WHERE ld.deployment_template_id = s.deployment_template_id
		ORDER BY ld.id`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ServiceTemplateLog, 0)
	for rows.Next() {
		var item ServiceTemplateLog
		var retention, filterRule, processing sql.NullInt64
		var overrideCollection sql.NullBool
		var serviceCode, projectCode, businessSystemCode, tierCode, macroValuesRaw string
		var environmentCode sql.NullString
		if err = rows.Scan(&item.LogDefinition, &item.Name, &item.PathPattern, &item.TemplateCollectionEnabled,
			&retention, &overrideCollection, &filterRule, &processing,
			&serviceCode, &projectCode, &environmentCode, &businessSystemCode,
			&macroValuesRaw, &tierCode); err != nil {
			return nil, err
		}
		item.RetentionTier = intPtr(retention)
		item.CollectionFilterRuleID = intPtr(filterRule)
		item.ProcessingRuleID = intPtr(processing)
		if overrideCollection.Valid {
			item.CollectionEnabled = &overrideCollection.Bool
		}
		item.ResolvedPath = resolveServiceMacros(item.PathPattern, macroValuesRaw)
		item.DataStream = logstream.Name("logs", projectCode, environmentCode.String, businessSystemCode, serviceCode, tierCode)
		items = append(items, item)
	}
	return items, rows.Err()
}

// resolveServiceMacros 用服务级宏替换路径里的 ${VAR}，未定义的保持原样以暴露数据缺口。
func resolveServiceMacros(path, macroValuesRaw string) string {
	trimmed := strings.TrimSpace(macroValuesRaw)
	if trimmed == "" || trimmed == "{}" {
		return path
	}
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return path
	}
	result := path
	for key, value := range decoded {
		result = strings.ReplaceAll(result, "${"+key+"}", strings.TrimSpace(fmt.Sprint(value)))
	}
	return result
}

func (r *Repository) ListServiceLogSettings(ctx context.Context, serviceID int64) ([]ServiceLogSettingInput, error) {
	rows, err := r.pool.QueryContext(ctx, `SELECT log_definition_id,retention_tier_id,collection_enabled,collection_filter_rule_id,processing_rule_id FROM assets_application_service_log_setting WHERE service_id=? ORDER BY log_definition_id`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ServiceLogSettingInput, 0)
	for rows.Next() {
		var item ServiceLogSettingInput
		var retention, filterRule, processing sql.NullInt64
		var collection sql.NullBool
		if err = rows.Scan(&item.LogDefinition, &retention, &collection, &filterRule, &processing); err != nil {
			return nil, err
		}
		item.RetentionTier = intPtr(retention)
		item.CollectionFilterRule = intPtr(filterRule)
		item.ProcessingRule = intPtr(processing)
		if collection.Valid {
			item.CollectionEnabled = &collection.Bool
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
