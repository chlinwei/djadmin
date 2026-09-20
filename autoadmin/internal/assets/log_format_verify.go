package assets

import (
	"context"
	"database/sql"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"log/slog"
)

// 日志格式认证（架构文档 §4.8）：把"这条日志的格式能否被模板上的规则解析出必备字段"
// 在交付时回答一次，结果落在 assets_application_service_log_setting 的 format_verified_* 四列。
//
// 本文件只负责**认证的编排与落库**：校验谁、写什么指纹、状态怎么变。真正"取样例 + 跑 ES"
// 由 LogFormatVerifier 实现（logcollect 侧——它才持有 agent 文件通道与 ES 客户端）。
// 这样分层的原因：logcollect 已经依赖 assets（凭证解密、主机读取），反向依赖会成环。
const (
	// LogFormatSourceInstance：取某个部署实例上日志文件的最近若干行真实日志。
	LogFormatSourceInstance = "instance"
	// LogFormatSourceSampleLog：用处理规则自带的样例日志。
	LogFormatSourceSampleLog = "sample_log"
	// LogFormatSourceWaiver：人工确认豁免——不做校验，直接记指纹与操作人。
	LogFormatSourceWaiver = "waiver"
)

// LogFormatVerifyRequest 认证执行器（取样例 + 跑 ES 校验）的输入。
type LogFormatVerifyRequest struct {
	ServiceID       int64
	LogDefinitionID int64
	// DeploymentID 仅 source=instance 且 AllDeployments=false 时使用：从哪个实例上取真实日志。
	DeploymentID int64
	// AllDeployments 仅 source=instance 时有效：true = 认证该服务的**全部承载实例**，
	// false = 只认证 DeploymentID 指定的那一个。两种情况下，实例上**所有匹配到的日志文件**
	// 都会被逐一认证（见 logcollect 侧）。
	AllDeployments bool
	Source         string
}

// LogFormatVerifyTarget 一台承载实例的认证结果：该实例上匹配到的每个日志文件都参与认证，
// 缺必备字段即该实例不通过；某台主机离线/取不到样例时只在这一项写 Error，不影响其他实例。
type LogFormatVerifyTarget struct {
	DeploymentID     int64    `json:"deployment_id"`
	DeploymentName   string   `json:"deployment_instance_name"`
	HostInstanceName string   `json:"host_instance_name"`
	LogFiles         []string `json:"log_files"`
	MissingFields    []string `json:"missing_fields"`
	Error            string   `json:"error,omitempty"`
}

// LogFormatVerifyReport 一次认证的完整结果：全部目标的缺失字段并集 + 逐实例明细。
//
// MissingFields 是所有目标缺失字段的**并集**（任一实例/文件缺就算缺），供"通过 / 不通过"
// 的判定与既有前端契约使用；Targets 用来告诉人"是哪台实例、哪个文件的问题"。
type LogFormatVerifyReport struct {
	MissingFields []string                `json:"missing_fields"`
	Targets       []LogFormatVerifyTarget `json:"targets"`
}

// Passed 判定：没有任何缺失字段，且没有任何目标取不到样例（Error）。
// 取不到样例绝不算通过——否则会把"没验成"记成"已验证"。sample_log 依据没有实例目标，
// 其样例已在执行器里校验过，所以这里不因 Targets 为空而否定。
func (report LogFormatVerifyReport) Passed() bool {
	if len(report.MissingFields) > 0 {
		return false
	}
	for _, target := range report.Targets {
		if target.Error != "" {
			return false
		}
	}
	return true
}

// LogFormatVerifier 由 logcollect 侧实现：取样例并跑规则校验。
//
// 返回**完整报告**（缺失字段 + 逐实例明细）。取不到样例（agent 离线、文件不存在、
// 规则没配样例）必须返回 error，不能返回空报告——否则会把"没验成"误记成"认证通过"。
// 逐实例的软失败（某台主机离线）记在 Target.Error 上，不阻断其他实例。
type LogFormatVerifier interface {
	VerifyLogFormat(ctx context.Context, request LogFormatVerifyRequest) (LogFormatVerifyReport, error)
}

// LogFormatVerifyResult 一次认证的结果。passed=false 时不会写库（指纹保持原样，
// 状态仍是 unverified / needs_recheck），前端据此提示缺哪些字段、哪台实例有问题。
type LogFormatVerifyResult struct {
	Passed               bool                    `json:"passed"`
	Source               string                  `json:"source"`
	MissingFields        []string                `json:"missing_fields"`
	Targets              []LogFormatVerifyTarget `json:"targets,omitempty"`
	FormatState          string                  `json:"format_state"`
	FormatFingerprint    string                  `json:"format_fingerprint"`
	FormatVerifiedAt     *string                 `json:"format_verified_at"`
	FormatVerifiedSource string                  `json:"format_verified_source"`
	FormatVerifiedBy     string                  `json:"format_verified_by"`
}

// UpsertLogSettingFormatVerified 写回一次认证结果。必须是 upsert：认证状态按
// (服务 × 日志定义) 读，而覆盖行只在"有覆盖"时才存在，UPDATE-only 会静默写空。
func (r *Repository) UpsertLogSettingFormatVerified(ctx context.Context, serviceID, logDefinitionID int64, fingerprint, source, actor string, at time.Time) error {
	return r.queries.UpsertLogSettingFormatVerified(ctx, db.UpsertLogSettingFormatVerifiedParams{
		CreateTime: at, UpdateTime: at, LogDefinitionID: logDefinitionID, ServiceID: serviceID,
		FormatVerifiedAt:          sql.NullTime{Time: at, Valid: true},
		FormatVerifiedFingerprint: fingerprint, FormatVerifiedSource: source, FormatVerifiedBy: actor,
	})
}

// SetLogFormatVerifier 注入认证执行器。未注入时只有 waiver 依据可用（其余返回 ErrLogFormatUnavailable），
// 测试与最小部署不必拉起 agent/ES 通道。
func (s *Service) SetLogFormatVerifier(verifier LogFormatVerifier) {
	s.logFormatVerifier = verifier
}

// VerifyServiceLogFormat 对一条 (逻辑服务 × 日志定义) 执行一次格式认证。
//
// allDeployments 仅在 source=instance 时有意义：false 只认证 deploymentID 指定的实例，
// true 认证该服务全部承载实例。**无论哪种范围，实例上所有匹配到的日志文件都参与认证**
// （通配路径展开出的每个文件都要能解析出必备字段）。
//
// 指纹用**展示口径的同一个函数**算（ListServiceTemplateLogs 的 FormatFingerprint）——
// 认证写入的指纹与弹窗上比对的指纹必须逐字节相同，否则认证通过后状态还是 needs_recheck。
// 因此这里直接复用 ListServiceTemplateLogs 的返回行，而不是另写一遍指纹输入。
func (s *Service) VerifyServiceLogFormat(ctx context.Context, serviceID, logDefinitionID, deploymentID int64, allDeployments bool, source, actor string) (LogFormatVerifyResult, error) {
	source = strings.TrimSpace(source)
	switch source {
	case LogFormatSourceInstance:
		if !allDeployments && deploymentID < 1 {
			return LogFormatVerifyResult{}, ErrLogFormatDeploymentRequired
		}
	case LogFormatSourceSampleLog:
	case LogFormatSourceWaiver:
	default:
		return LogFormatVerifyResult{}, ErrLogFormatSourceInvalid
	}

	rows, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		return LogFormatVerifyResult{}, translate(err)
	}
	var target *ServiceTemplateLog
	for index := range rows {
		if rows[index].LogDefinition == logDefinitionID {
			target = &rows[index]
			break
		}
	}
	if target == nil {
		return LogFormatVerifyResult{}, ErrNotFound
	}

	result := LogFormatVerifyResult{
		Source: source, FormatState: target.FormatState, FormatFingerprint: target.FormatFingerprint,
		FormatVerifiedAt: target.FormatVerifiedAt, FormatVerifiedSource: target.FormatVerifiedSource,
		FormatVerifiedBy: target.FormatVerifiedBy,
	}

	// waiver 不做校验（人工确认豁免），但仍然记当前指纹与操作人——它同样是"这一次配置已确认"。
	if source != LogFormatSourceWaiver {
		if s.logFormatVerifier == nil {
			return LogFormatVerifyResult{}, ErrLogFormatUnavailable
		}
		report, verifyErr := s.logFormatVerifier.VerifyLogFormat(ctx, LogFormatVerifyRequest{
			ServiceID: serviceID, LogDefinitionID: logDefinitionID, DeploymentID: deploymentID,
			AllDeployments: allDeployments, Source: source,
		})
		if verifyErr != nil {
			return LogFormatVerifyResult{}, verifyErr
		}
		result.Targets = report.Targets
		if !report.Passed() {
			result.Passed = false
			result.MissingFields = report.MissingFields
			return result, nil
		}
	}

	now := time.Now().UTC()
	if err = s.repository.UpsertLogSettingFormatVerified(ctx, serviceID, logDefinitionID, target.FormatFingerprint, source, actor, now); err != nil {
		return LogFormatVerifyResult{}, translate(err)
	}
	formatted := timestamp(now)
	result.Passed = true
	result.MissingFields = []string{}
	result.FormatState = formatStateVerified
	result.FormatVerifiedAt = &formatted
	result.FormatVerifiedSource = source
	result.FormatVerifiedBy = actor
	return result, nil
}

// autoVerifyDeploymentAttempts 自动认证时最多试几个实例：实例级 runtime_variables 不进指纹，
// 所以不同实例的宏可能不同（文件路径不同）。逐个试到第一次成功为止，次数封顶避免
// 一个挂了很多实例的服务把后台任务拖长。
const autoVerifyDeploymentAttempts = 3

// MaybeAutoVerifyServiceLogFormats 新增服务 / 开启采集时抽样认证一次（架构文档 §4.8）。
//
// 以下情况不触发，都属于"现在没有可校验的样例"，而不是漏做：
//   - 服务没开采集（log_collection_enabled=false）：此时根本没有"要采的日志"；
//   - 该行已经 verified：认证是幂等的，"一次认证"不该被重复触发（改配置会让指纹变化，
//     状态自己回到 needs_recheck，那时才需要重新认证）。
//
// 依据优先级：先试 instance（真实文件，最贴近实际采集），全部实例都取不到样例时
// 退到 sample_log（规则自带样例）；都失败就保持未认证，由人在弹窗里手动处理。
//
// best-effort：整个流程在后台跑，任何失败只记日志，绝不影响服务保存的返回结果。
func (s *Service) MaybeAutoVerifyServiceLogFormats(ctx context.Context, serviceID int64) {
	if s.logFormatVerifier == nil {
		return
	}
	// 请求上下文在响应返回后就会被取消，后台任务必须脱钩；超时兜住卡住的主机。
	background, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	go func() {
		defer cancel()
		s.autoVerifyServiceLogFormats(background, serviceID)
	}()
}

func (s *Service) autoVerifyServiceLogFormats(ctx context.Context, serviceID int64) {
	rows, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		slog.Warn("auto verify log format: load service logs failed", "service_id", serviceID, "error", err)
		return
	}
	// 实例列表按需加载：全都已认证时（保存服务是高频操作）不该多查一次库。
	var deployments []int64
	loadedDeployments := false
	for _, row := range rows {
		if row.FormatState == formatStateVerified {
			continue
		}
		if !loadedDeployments {
			loadedDeployments = true
			deployments, err = s.repository.queries.ListServiceDeploymentIDs(ctx, serviceID)
			if err != nil {
				slog.Warn("auto verify log format: load service deployments failed", "service_id", serviceID, "error", err)
				deployments = nil
			}
		}
		if s.autoVerifyOneLogFormat(ctx, serviceID, row.LogDefinition, deployments) {
			continue
		}
		slog.Info("auto verify log format: kept unverified",
			"service_id", serviceID, "log_definition_id", row.LogDefinition)
	}
}

// autoVerifyOneLogFormat 对一条日志定义按 instance → sample_log 顺序尝试，返回是否认证成功。
func (s *Service) autoVerifyOneLogFormat(ctx context.Context, serviceID, logDefinitionID int64, deployments []int64) bool {
	attempts := 0
	for _, deploymentID := range deployments {
		if attempts >= autoVerifyDeploymentAttempts {
			break
		}
		attempts++
		result, err := s.VerifyServiceLogFormat(ctx, serviceID, logDefinitionID, deploymentID, false, LogFormatSourceInstance, "")
		if err == nil && result.Passed {
			slog.Info("auto verify log format: verified from instance",
				"service_id", serviceID, "log_definition_id", logDefinitionID, "deployment_id", deploymentID)
			return true
		}
	}
	result, err := s.VerifyServiceLogFormat(ctx, serviceID, logDefinitionID, 0, false, LogFormatSourceSampleLog, "")
	if err == nil && result.Passed {
		slog.Info("auto verify log format: verified from rule sample log",
			"service_id", serviceID, "log_definition_id", logDefinitionID)
		return true
	}
	return false
}
