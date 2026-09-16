package assets

import (
	"context"
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
	rows, err := r.queries.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	items := make([]ServiceTemplateLog, 0, len(rows))
	for _, row := range rows {
		item := ServiceTemplateLog{
			LogDefinition: row.ID, Name: row.Name, PathPattern: row.PathPattern,
			TemplateCollectionEnabled: row.CollectionEnabled,
			RetentionTier:             intPtr(row.RetentionTierID),
			CollectionFilterRuleID:    intPtr(row.CollectionFilterRuleID),
			ProcessingRuleID:          intPtr(row.ProcessingRuleID),
			CollectionEnabled:         row.OverrideCollectionEnabled,
		}
		item.ResolvedPath = resolveServiceMacros(item.PathPattern, string(row.MacroValues))
		item.DataStream = logstream.Name("logs", row.ProjectCode, row.EnvironmentCode.String, row.BusinessSystemCode, row.ServiceCode, row.TierCode)
		items = append(items, item)
	}
	return items, nil
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
	rows, err := r.queries.ListServiceLogSettings(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	items := make([]ServiceLogSettingInput, 0, len(rows))
	for _, row := range rows {
		items = append(items, ServiceLogSettingInput{
			LogDefinition:        row.LogDefinitionID,
			RetentionTier:        intPtr(row.RetentionTierID),
			CollectionEnabled:    row.CollectionEnabled,
			CollectionFilterRule: intPtr(row.CollectionFilterRuleID),
			ProcessingRule:       intPtr(row.ProcessingRuleID),
		})
	}
	return items, nil
}
