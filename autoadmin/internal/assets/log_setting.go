package assets

import (
	"autoadmin/internal/shared/logmacro"
	"context"
	"time"

	platformdb "autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
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
// 另带日志格式认证状态（迁移 000036，模型见 log_format_fingerprint.go 与架构文档 §4.8）：
// FormatState 是"当前配置指纹 vs 认证时指纹"的比对结果，只读库即可得出，不需要碰主机。
type ServiceTemplateLog struct {
	LogDefinition int64  `json:"log_definition"`
	Name          string `json:"name"`
	PathPattern   string `json:"path_pattern"`
	// ResolvedPath 是**尽力解析后的路径**：模板 macro_definitions 的默认值 → 服务 macro_values，
	// 外加模板 app_home 作为 APP_HOME 默认值（与下发渲染同一套合并顺序，见 shared/logmacro）。
	// 仍是"服务这一层能解析的部分"——实例 runtime_variables 只有到主机上才知道，
	// 所以剩下的宏由 PendingMacros 显式列出来，界面标成"实例上展开"，不猜值。
	ResolvedPath           string   `json:"resolved_path"`
	PendingMacros          []string `json:"pending_macros"`
	DataStream             string   `json:"data_stream"`
	RetentionTier          *int64   `json:"retention_tier"`
	CollectionEnabled      *bool    `json:"collection_enabled"`
	CollectionFilterRuleID *int64   `json:"collection_filter_rule_id"`
	// CollectionExcludeFilterRuleID 排除方向的覆盖（三态：NULL 继承模板 / 0 显式关闭 / >0 规则）。
	CollectionExcludeFilterRuleID *int64 `json:"collection_exclude_filter_rule_id"`
	// 模板级默认（来自日志定义本身）：界面用它显示"继承的是哪条"，渲染用它做兜底。
	TemplateFilterIncludeRuleID *int64 `json:"template_filter_include_rule_id"`
	TemplateFilterExcludeRuleID *int64 `json:"template_filter_exclude_rule_id"`
	// TierCode 是这条日志**当前生效**的档位编码（覆盖档位 → 服务默认档位 → is_default → 'std' 的
	// COALESCE 链，与 data_stream 的尾段一致）。页面据此区分"当前档位"与"改档位后留下的历史流"：
	// 流的档位不在生效档位集合里，就说明它已停止写入、只是数据还没到期。
	TierCode string `json:"tier_code"`
	// ServiceCode 是**本服务自己**（不是这条日志）的编码。它只是展示用的受控标识：
	// 唯一域是 (业务系统, 环境) 而非全局（见 assets 000044 迁移），所以不再是可靠的全局连接键。
	// 日志中心页按服务取水位数据改传 application_service_id（见 log-storage-overview 与 scopeIndexPattern）。
	ServiceCode                string `json:"service_code"`
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

// ServiceLogConfig 是 log-config 接口的响应体：模板日志定义（合入服务级覆盖）+ 服务级采集总开关。
//
// 总开关放在顶层而不是每行重复：它是"这个服务"的属性，而且逐条开关在总开关关闭时并不生效，
// 页面必须能区分这两层（否则用户会以为逐条都关了、其实只是总开关关了）。
type ServiceLogConfig struct {
	Logs []ServiceTemplateLog `json:"logs"`
	// LogCollectionEnabled 服务级日志采集总开关。
	LogCollectionEnabled bool `json:"log_collection_enabled"`
	// LogRetentionTier 服务级**默认保留档位**（逐条日志没覆盖时生效；为 null 时由平台默认档
	// `is_default` 决定）。界面用它把"继承服务默认"写成"继承服务默认（标准 30 天）"——
	// 只说"继承"而不说继承的是什么，用户没法判断这条日志实际保留多久。
	LogRetentionTier *int64 `json:"log_retention_tier"`
	// ServiceCode 本服务的编码。放在顶层是为了让"本服务水位"不依赖 logs 非空：
	// 模板里的日志定义被删光之后，这个服务的存量流仍要能查出来（数据还在 ES 里，
	// 页面不能因为"没有日志定义"就当作没有流）。
	ServiceCode string `json:"service_code"`
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
			RetentionTier:                 intPtr(row.RetentionTierID),
			CollectionFilterRuleID:        intPtr(row.CollectionFilterRuleID),
			CollectionExcludeFilterRuleID: intPtr(row.CollectionExcludeFilterRuleID),
			TemplateFilterIncludeRuleID:   intPtr(row.FilterIncludeRuleID),
			TemplateFilterExcludeRuleID:   intPtr(row.FilterExcludeRuleID),
			TierCode:                      row.TierCode,
			ServiceCode:                   row.ServiceCode,
			TemplateProcessingRuleID:      intPtr(row.ProcessingRuleID),
			TemplateProcessingRuleName:    row.ProcessingRuleName,
			CollectionEnabled:             row.OverrideCollectionEnabled,
		}
		// 与渲染同序：模板默认值（含 app_home 作 APP_HOME）→ 服务覆盖。未展开的留给实例变量，
		// 由 PendingMacros 如实报出来（界面标注），而不是显示一个猜出来的路径。
		item.ResolvedPath = logmacro.Resolve(
			item.PathPattern,
			logmacro.TemplateDefaults(string(row.MacroDefinitions)),
			logmacro.InstanceValues("", row.AppHome),
			logmacro.ParseValues(string(row.MacroValues)),
		)
		item.PendingMacros = logmacro.Pending(item.ResolvedPath)
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

// ServiceLogOverrideInput 按行修改服务级日志覆盖：只有"采集开关"与"保留档位"两项
// （解析规则只由部署模板决定、采集过滤规则选了不生效，都不在这里）。
//
// 两个字段都是"覆盖值"语义：nil = 不覆盖（采集默认采、档位继承服务默认），与覆盖行不存在等价。
type ServiceLogOverrideInput struct {
	LogDefinition     int64  `json:"log_definition_id"`
	CollectionEnabled *bool  `json:"collection_enabled"`
	RetentionTier     *int64 `json:"retention_tier"`
	// 采集过滤的两个方向（三态：null 继承模板 / 0 显式关闭 / >0 指定规则）。
	// 与采集开关、档位一样是"按行 upsert，不碰其他列"，所以内联改动不会影响别的日志。
	CollectionFilterRule        *int64 `json:"collection_filter_rule"`
	CollectionExcludeFilterRule *int64 `json:"collection_exclude_filter_rule"`
}

// UpsertServiceLogOverride 写回一条覆盖行（没有就插）。
//
// **不碰 format_verified_***：认证结果只由格式认证流程写。改采集开关/档位既不改日志格式、
// 也不进认证指纹（见 log_format_fingerprint.go 的输入清单），所以覆盖值更新后认证状态必须
// 原样保留——UPDATE 分支只列这两个覆盖列就是靠这一点保证的，改动这里要同步改
// TestUpsertServiceLogOverrideKeepsFormatVerification。
func (r *Repository) UpsertServiceLogOverride(ctx context.Context, serviceID int64, input ServiceLogOverrideInput) error {
	now := time.Now().UTC()
	return r.queries.UpsertServiceLogOverride(ctx, db.UpsertServiceLogOverrideParams{
		CreateTime: now, UpdateTime: now,
		CollectionEnabled:             input.CollectionEnabled,
		LogDefinitionID:               input.LogDefinition,
		RetentionTierID:               nullableInt(input.RetentionTier),
		ServiceID:                     serviceID,
		CollectionFilterRuleID:        nullableInt(input.CollectionFilterRule),
		CollectionExcludeFilterRuleID: nullableInt(input.CollectionExcludeFilterRule),
	})
}

// UpsertServiceLogOverrides 在一个事务里逐行写回覆盖值（批量路径专用）。
//
// 为什么批量要单独一条路：日志中心页的批量操作把它**同一次动作**涉及的几十行一次提交，
// 逐行发 HTTP 会让服务端对每个 id 都重算一遍模板日志与认证指纹（两次全表读），而且中途失败
// 会留下"改了一半"的界面。这里一次事务写完成，服务层只做一次校验读 + 一次回读。
func (r *Repository) UpsertServiceLogOverrides(ctx context.Context, serviceID int64, inputs []ServiceLogOverrideInput) error {
	now := time.Now().UTC()
	return platformdb.InTransaction(ctx, r.pool, func(queries *db.Queries) error {
		for _, input := range inputs {
			if err := queries.UpsertServiceLogOverride(ctx, db.UpsertServiceLogOverrideParams{
				CreateTime: now, UpdateTime: now,
				CollectionEnabled:             input.CollectionEnabled,
				LogDefinitionID:               input.LogDefinition,
				RetentionTierID:               nullableInt(input.RetentionTier),
				ServiceID:                     serviceID,
				CollectionFilterRuleID:        nullableInt(input.CollectionFilterRule),
				CollectionExcludeFilterRuleID: nullableInt(input.CollectionExcludeFilterRule),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// SaveServiceLogOverride 按行保存一条 (服务 × 日志定义) 的覆盖值，并回读该行。
//
// 与 SaveApplicationService 的 log_settings **不是同一套语义**：后者是整表替换
// （提交的集合即全量，先 DELETE 再逐行重插），只允许"完整表单"调用；按行操作必须走这里，
// 否则一次内联改动会把该服务其他日志的覆盖值一起删掉。两者边界见
// docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §9.5。
//
// 校验：这条日志定义必须属于该服务当前所用的部署模板（否则就是往别的服务的行上写），
// 档位 ID 由外键兜底（不存在会被数据库拒绝并翻译成"关联资产不存在"）。
func (s *Service) SaveServiceLogOverride(ctx context.Context, serviceID int64, input ServiceLogOverrideInput) (ServiceTemplateLog, error) {
	if serviceID < 1 || input.LogDefinition < 1 {
		return ServiceTemplateLog{}, ErrInvalid
	}
	rows, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		return ServiceTemplateLog{}, translate(err)
	}
	var target *ServiceTemplateLog
	for index := range rows {
		if rows[index].LogDefinition == input.LogDefinition {
			target = &rows[index]
			break
		}
	}
	if target == nil {
		return ServiceTemplateLog{}, ErrNotFound
	}
	if err = s.repository.UpsertServiceLogOverride(ctx, serviceID, input); err != nil {
		return ServiceTemplateLog{}, translate(err)
	}
	// 回读：覆盖值与认证状态都由后端算，前端不自己拼（与格式认证同一做法）。
	refreshed, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		return ServiceTemplateLog{}, translate(err)
	}
	for index := range refreshed {
		if refreshed[index].LogDefinition == input.LogDefinition {
			return refreshed[index], nil
		}
	}
	return *target, nil
}

// MaxBatchLogOverrides 一次批量覆盖值提交的行数上限。日志中心页的"全选"就是一屏的
// 日志定义（通常几条到几十条），上限只是防线：再多的量应该走部署模板调整，而不是批量刷覆盖值。
const MaxBatchLogOverrides = 500

// ServiceLogOverrideBatchResult 批量提交里**单条**的结果：与批量删除同口径——
// 不属于本服务模板的日志记 ok=false 并说明原因，不整体失败，其余照常写入。
type ServiceLogOverrideBatchResult struct {
	// LogDefinition 就是前端表格的行键（这条日志定义），前端用它把失败项对回到日志名。
	LogDefinition int64  `json:"id"`
	OK            bool   `json:"ok"`
	Message       string `json:"message"`
}

// BatchSaveServiceLogOverrides 批量按行保存覆盖值（日志中心页的批量操作）。
//
// 与单条 SaveServiceLogOverride **同一套按行语义**，只是共用一次校验读与一次事务：
//   - 每个 item 都是该行覆盖值的**全集**（缺列 = 清成"不覆盖"），调用方必须从服务端回读的
//     行出发构造，只覆盖这次要改的那一列——否则会把其他列静默清空，与单条接口是同一个坑。
//   - 日志定义必须属于该服务当前所用的部署模板；不属于的记 ok=false（ErrNotFound 的批量口径），
//     剩下的照写，不整体失败。
//   - 全部写入在**一个事务**里完成：批量动作要么都落库、要么都不落（事务失败时返回 error），
//     不留半套配置给界面。与"批量删除逐 id 独立"的差别在于这里没有可独立的部分——
//     同一个批量动作改的是同一列的同一个值。
//
// 不返回回读行：调用方（日志中心页）在批量后重新拉一次 log-config（那一份还带认证状态与
// 待下发判定），比在这里逐行拼更不容易出现"界面显示的和库里不一致"。
func (s *Service) BatchSaveServiceLogOverrides(ctx context.Context, serviceID int64, inputs []ServiceLogOverrideInput) ([]ServiceLogOverrideBatchResult, error) {
	if serviceID < 1 || len(inputs) == 0 || len(inputs) > MaxBatchLogOverrides {
		return nil, ErrInvalid
	}
	rows, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		return nil, translate(err)
	}
	// 本服务模板下的日志定义集合：不在其中的一律拒绝（否则就是往别的服务的行上写）。
	belongs := make(map[int64]bool, len(rows))
	for _, row := range rows {
		belongs[row.LogDefinition] = true
	}
	results := make([]ServiceLogOverrideBatchResult, 0, len(inputs))
	applicable := make([]ServiceLogOverrideInput, 0, len(inputs))
	for _, input := range inputs {
		if !belongs[input.LogDefinition] {
			results = append(results, ServiceLogOverrideBatchResult{
				LogDefinition: input.LogDefinition, OK: false,
				Message: "该日志不属于本服务的部署模板",
			})
			continue
		}
		results = append(results, ServiceLogOverrideBatchResult{LogDefinition: input.LogDefinition, OK: true})
		applicable = append(applicable, input)
	}
	if len(applicable) == 0 {
		// 一条都不适用：不是错误响应，把逐条原因照常返回（前端按 ok=false 提示）。
		return results, nil
	}
	if err = s.repository.UpsertServiceLogOverrides(ctx, serviceID, applicable); err != nil {
		return nil, translate(err)
	}
	return results, nil
}

// GetServiceLogDefaults 读服务级的两个日志默认值：采集总开关 + 默认保留档位（服务不存在时 ErrNotFound）。
//
// 两个一起读：日志中心的「日志配置」既要区分"总开关关了"与"逐条关了"，也要回答
// "**继承服务默认**的档位到底是哪一档"（现场反馈"我怎么知道默认是什么呢"）。
// 档位为空表示这条服务的有效档位由平台默认档（`is_default`）决定。
func (r *Repository) GetServiceLogDefaults(ctx context.Context, serviceID int64) (bool, *int64, error) {
	row, err := r.queries.GetApplicationServiceLogDefaults(ctx, serviceID)
	if err != nil {
		return false, nil, translate(err)
	}
	return row.LogCollectionEnabled, intPtr(row.LogRetentionTierID), nil
}

// SetServiceLogCollection 单独写服务级日志采集总开关（log_collection_enabled）。
//
// 关掉之后：该服务下**任何**日志都不再被采集（渲染层按 `s.log_collection_enabled = TRUE` 过滤），
// 逐条日志的开关随之失去意义——但逐条开关是"配置意图"，仍然保留，重新打开总开关即恢复原样。
//
// 不影响格式认证：总开关不进认证指纹，认证四列也在 log_setting 行上、不在这张表。
// 返回写入后的值（回读口径统一由后端给，前端不自己拼）。
func (s *Service) SetServiceLogCollection(ctx context.Context, serviceID int64, enabled bool) (bool, error) {
	if serviceID < 1 {
		return false, ErrInvalid
	}
	result, err := s.repository.queries.UpdateApplicationServiceLogCollection(ctx, db.UpdateApplicationServiceLogCollectionParams{
		UpdateTime: time.Now().UTC(), LogCollectionEnabled: enabled, ID: serviceID,
	})
	if err != nil {
		return false, translate(err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		// 服务不存在（或 id 写错）：必须报出来，不能让"什么都没改"看起来像成功。
		return false, ErrNotFound
	}
	return enabled, nil
}
