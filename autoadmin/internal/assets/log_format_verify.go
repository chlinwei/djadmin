package assets

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// Request orchestration lives in assets; logcollect owns the agent and ES probe.
// Production evidence is stored per deployment, independently of service configuration overrides.
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
	// Partial drops are expected, but an entirely dropped file is not proof of a valid stored format.
	DroppedSamples int                   `json:"dropped_samples"`
	Error          string                `json:"error,omitempty"`
	FailureKind    string                `json:"failure_kind,omitempty"`
	SampleCount    int                   `json:"sample_count"`
	Files          []LogFormatFileReport `json:"files"`
}

type LogFormatFileReport struct {
	Path           string   `json:"path"`
	SampleCount    int      `json:"sample_count"`
	DroppedSamples int      `json:"dropped_samples"`
	MissingFields  []string `json:"missing_fields"`
	SampleShapes   []string `json:"sample_shapes"`
	Error          string   `json:"error,omitempty"`
}

// LogFormatVerifyReport 一次认证的完整结果：全部目标的缺失字段并集 + 逐实例明细。
//
// MissingFields 是所有目标缺失字段的**并集**（任一实例/文件缺就算缺），供"通过 / 不通过"
// 的判定与既有前端契约使用；Targets 用来告诉人"是哪台实例、哪个文件的问题"。
type LogFormatVerifyReport struct {
	MissingFields []string `json:"missing_fields"`
	SampleCount   int      `json:"sample_count"`
	// DroppedSamples is explicit evidence; all-dropped samples cannot pass.
	DroppedSamples int                     `json:"dropped_samples"`
	Targets        []LogFormatVerifyTarget `json:"targets"`
}

// Passed 判定：没有任何缺失字段，且没有任何目标取不到样例（Error）。
// 取不到样例绝不算通过——否则会把"没验成"记成"已验证"。sample_log 依据没有实例目标，
// 其样例已在执行器里校验过，所以这里不因 Targets 为空而否定。
func (report LogFormatVerifyReport) Passed() bool {
	if report.SampleCount == 0 || report.DroppedSamples >= report.SampleCount {
		return false
	}
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

// Instance requests return queued=true; fixture tests return passed without certifying production.
type LogFormatVerifyResult struct {
	Queued               bool                    `json:"queued"`
	Passed               bool                    `json:"passed"`
	Source               string                  `json:"source"`
	MissingFields        []string                `json:"missing_fields"`
	DroppedSamples       int                     `json:"dropped_samples"`
	Targets              []LogFormatVerifyTarget `json:"targets,omitempty"`
	FormatState          string                  `json:"format_state"`
	FormatFingerprint    string                  `json:"format_fingerprint"`
	FormatVerifiedAt     *string                 `json:"format_verified_at"`
	FormatVerifiedSource string                  `json:"format_verified_source"`
	FormatVerifiedBy     string                  `json:"format_verified_by"`
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
// Instance fingerprints extend the shared rule signature with host and runtime macro inputs.
func (s *Service) VerifyServiceLogFormat(ctx context.Context, serviceID, logDefinitionID, deploymentID int64, allDeployments bool, source, actor string, reasons ...string) (LogFormatVerifyResult, error) {
	source = strings.TrimSpace(source)
	switch source {
	case LogFormatSourceInstance:
		if !allDeployments && deploymentID < 1 {
			return LogFormatVerifyResult{}, ErrLogFormatDeploymentRequired
		}
	case LogFormatSourceSampleLog:
	case LogFormatSourceWaiver:
		if len(reasons) != 1 || strings.TrimSpace(reasons[0]) == "" || len(reasons[0]) > 2000 || actor == "" {
			return LogFormatVerifyResult{}, fmt.Errorf("人工豁免必须提供原因（不超过 2000 字节）和登录操作人")
		}
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

	if source == LogFormatSourceSampleLog {
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
		result.DroppedSamples = report.DroppedSamples
		result.Passed = report.Passed()
		result.MissingFields = report.MissingFields
		// A rule fixture is a test, not evidence that any production instance is verified.
		return result, nil
	}

	if target.TemplateProcessingRuleID == nil {
		return LogFormatVerifyResult{}, fmt.Errorf("该日志未关联解析规则")
	}
	deployments := []int64{deploymentID}
	if allDeployments || source == LogFormatSourceWaiver {
		deployments, err = s.repository.queries.ListServiceDeploymentIDs(ctx, serviceID)
		if err != nil {
			return LogFormatVerifyResult{}, err
		}
	}
	if len(deployments) == 0 {
		return LogFormatVerifyResult{}, fmt.Errorf("请先绑定部署实例")
	}
	enabled, _, err := s.repository.GetServiceLogDefaults(ctx, serviceID)
	if err != nil {
		return LogFormatVerifyResult{}, err
	}
	if !enabled || !boolValue(target.CollectionEnabled, true) {
		return LogFormatVerifyResult{}, fmt.Errorf("请先开启服务及该日志的采集开关")
	}
	now := time.Now().UTC()
	status, reason := "pending", ""
	if source == LogFormatSourceWaiver {
		status, reason = "waived", strings.TrimSpace(reasons[0])
	}
	requests := make([]db.RequestLogFormatTargetParams, 0, len(deployments))
	for _, instanceID := range deployments {
		fingerprint, err := s.ensureLogFormatTarget(ctx, serviceID, *target, instanceID)
		if err != nil {
			return LogFormatVerifyResult{}, err
		}
		requests = append(requests, db.RequestLogFormatTargetParams{
			ServiceID: serviceID, LogDefinitionID: logDefinitionID, DeploymentID: instanceID,
			Fingerprint: fingerprint, Status: status, NextAttemptAt: now, UpdateTime: now, Actor: actor, Reason: reason,
		})
	}
	tx, err := s.repository.pool.BeginTx(ctx, nil)
	if err != nil {
		return LogFormatVerifyResult{}, err
	}
	defer tx.Rollback()
	for _, request := range requests {
		if err = db.New(tx).RequestLogFormatTarget(ctx, request); err != nil {
			return LogFormatVerifyResult{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return LogFormatVerifyResult{}, err
	}
	result.Queued = source == LogFormatSourceInstance
	result.FormatState = status
	result.FormatVerifiedSource = source
	result.FormatVerifiedBy = actor
	return result, nil
}

type LogFormatConfigurationError string

func (err LogFormatConfigurationError) Error() string { return string(err) }
