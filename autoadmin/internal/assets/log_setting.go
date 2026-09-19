package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"autoadmin/internal/shared/logstream"
)

// ServiceTemplateLog：编辑逻辑服务弹窗"模板日志"表格行——部署模板的日志定义 + 服务级覆盖。
//
// 两条"只归一侧"的字段，前端分别是只读展示与可编辑：
//   - **解析规则只有模板一处**：TemplateProcessingRule* 是模板日志定义上的规则，服务侧只读
//     （迁移 000035 删掉了服务级覆盖 processing_rule_id——同一模板的日志格式相同，规则就该相同；
//     要不同就另建模板/另建日志定义）。
//   - **采集开关只有服务一处**：CollectionEnabled 是服务级覆盖值，null/无覆盖行 = 采（默认采、按需关），
//     false = 不采（迁移 000034 删掉了模板级开关）。
//
// 另带日志格式认证状态（迁移 000036，模型见 log_format_fingerprint.go 与架构文档 §6）：
// FormatState 是"当前配置指纹 vs 认证时指纹"的比对结果，只读库即可得出，不需要碰主机。
type ServiceTemplateLog struct {
	LogDefinition              int64  `json:"log_definition"`
	Name                       string `json:"name"`
	PathPattern                string `json:"path_pattern"`
	ResolvedPath               string `json:"resolved_path"`
	DataStream                 string `json:"data_stream"`
	RetentionTier              *int64 `json:"retention_tier"`
	CollectionEnabled          *bool  `json:"collection_enabled"`
	CollectionFilterRuleID     *int64 `json:"collection_filter_rule_id"`
	TemplateProcessingRuleID   *int64 `json:"template_processing_rule_id"`
	TemplateProcessingRuleName string `json:"template_processing_rule_name"`
	// 格式认证：state ∈ unverified / verified / needs_recheck；fingerprint 是**当前**配置指纹
	// （验证接口回写的就是它），verified_* 是上一次认证的时间/依据/操作人。
	FormatState          string  `json:"format_state"`
	FormatFingerprint    string  `json:"format_fingerprint"`
	FormatVerifiedAt     *string `json:"format_verified_at"`
	FormatVerifiedSource string  `json:"format_verified_source"`
	FormatVerifiedBy     string  `json:"format_verified_by"`
}

// ListServiceTemplateLogs 返回服务所属部署模板的全部日志定义，并合入该服务的覆盖值
// （无覆盖行时覆盖字段为 null，前端按"跟随默认"展示；采集默认是"采"）。
// resolved_path 用服务级 macro_values 替换 ${VAR} 后回显（实例级宏因逐实例而异不在
// 此处展开）；data_stream 按 monitor.LogDataStreamName 的同一构造规则生成
// （logs-<项目>-<业务系统>-<环境>-<服务>-<有效档位>，档位 = 覆盖档位 → 服务默认档位
// → is_default 档位 → 'std'），与 Filebeat 下发的 Index 命名保持一致。
func (r *Repository) ListServiceTemplateLogs(ctx context.Context, serviceID int64) ([]ServiceTemplateLog, error) {
	rows, err := r.queries.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	items := make([]ServiceTemplateLog, 0, len(rows))
	for _, row := range rows {
		item := ServiceTemplateLog{
			LogDefinition: row.ID, Name: row.Name, PathPattern: row.PathPattern,
			RetentionTier:              intPtr(row.RetentionTierID),
			CollectionFilterRuleID:     intPtr(row.CollectionFilterRuleID),
			TemplateProcessingRuleID:   intPtr(row.ProcessingRuleID),
			TemplateProcessingRuleName: row.ProcessingRuleName,
			CollectionEnabled:          row.OverrideCollectionEnabled,
		}
		item.ResolvedPath = resolveServiceMacros(item.PathPattern, string(row.MacroValues))
		item.DataStream = logstream.Name("autoadmin", row.ProjectCode, row.EnvironmentCode.String, row.BusinessSystemCode, row.ServiceCode, row.TierCode)
		// 格式认证状态：当前指纹现场算、与认证时存下的比（纯读库，不碰主机）。
		item.FormatFingerprint = logFormatFingerprintOf(logFormatFingerprintInput{
			LogDefinition: row.ID, LogName: row.Name, PathPattern: row.PathPattern,
			RuleID: row.ProcessingRuleID.Int64, RuleUpdatedAt: row.ProcessingRuleUpdateTime,
			MacroValues: string(row.MacroValues), ApplicationVersion: row.ApplicationVersionID,
		})
		item.FormatState = formatStateUnverified
		if stored := row.FormatVerifiedFingerprint.String; stored != "" {
			if stored == item.FormatFingerprint {
				item.FormatState = formatStateVerified
			} else {
				item.FormatState = formatStateNeedsRecheck
			}
		}
		if row.FormatVerifiedAt.Valid {
			formatted := timestamp(row.FormatVerifiedAt.Time)
			item.FormatVerifiedAt = &formatted
		}
		item.FormatVerifiedSource = row.FormatVerifiedSource.String
		item.FormatVerifiedBy = row.FormatVerifiedBy.String
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

// ListServiceLogSettings 读回服务的日志覆盖行（只有采集开关/档位/过滤规则三项，
// 解析规则属于模板日志定义，不在覆盖里）。
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
		})
	}
	return items, nil
}
