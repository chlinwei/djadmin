package assets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

const logFormatMaxAttempts = 4

type LogFormatInstanceState struct {
	DeploymentID int64                  `json:"deployment_id"`
	Name         string                 `json:"name"`
	Status       string                 `json:"status"`
	Attempts     int32                  `json:"attempts"`
	Reason       string                 `json:"reason"`
	Actor        string                 `json:"actor"`
	UpdatedAt    string                 `json:"updated_at,omitempty"`
	Report       *LogFormatVerifyReport `json:"report,omitempty"`
}

// Reconciliation, rather than individual write handlers, owns automatic verification.
// Database leases deduplicate API replicas and recover interrupted attempts after restart.
func (s *Service) StartLogFormatReconciler(ctx context.Context) {
	s.logFormatStart.Do(func() {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				if err := s.reconcileLogFormats(ctx); err != nil && ctx.Err() == nil {
					slog.Error("log format reconciliation failed", "error", err)
				}
				if err := s.runDueLogFormats(ctx); err != nil && ctx.Err() == nil {
					slog.Error("log format execution failed", "error", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func logInstanceFingerprint(base string, instance db.GetLogVerifyInstanceContextRow) (string, error) {
	// Canonical JSON avoids invalidation caused only by object-key order or whitespace.
	values := []any{base, instance.DeploymentID, instance.HostID, instance.HostInstanceName, instance.AppHome}
	for _, raw := range []string{string(instance.RuntimeVariables), string(instance.MacroDefinitions)} {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return "", fmt.Errorf("实例认证宏配置无效: %w", err)
		}
		values = append(values, value)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Service) ensureLogFormatTarget(ctx context.Context, serviceID int64, log ServiceTemplateLog, deploymentID int64) (string, error) {
	instance, err := s.repository.queries.GetLogVerifyInstanceContext(ctx, db.GetLogVerifyInstanceContextParams{
		ServiceID: serviceID, LogDefinitionID: log.LogDefinition, DeploymentID: deploymentID,
	})
	if err != nil {
		return "", err
	}
	fingerprint, err := logInstanceFingerprint(log.FormatFingerprint, instance)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if err = s.repository.queries.EnsureLogFormatTarget(ctx, db.EnsureLogFormatTargetParams{
		ServiceID: serviceID, LogDefinitionID: log.LogDefinition, DeploymentID: deploymentID,
		Fingerprint: fingerprint, NextAttemptAt: now, UpdateTime: now,
	}); err != nil {
		return "", err
	}
	err = s.repository.queries.ResetChangedLogFormatTarget(ctx, db.ResetChangedLogFormatTargetParams{
		ServiceID: serviceID, LogDefinitionID: log.LogDefinition, DeploymentID: deploymentID,
		Fingerprint: fingerprint, OldFingerprint: fingerprint, NextAttemptAt: now, UpdateTime: now,
	})
	return fingerprint, err
}

func (s *Service) reconcileLogFormats(ctx context.Context) error {
	if s.logFormatVerifier == nil {
		return nil
	}
	now := time.Now().UTC()
	if err := s.repository.queries.RecoverExhaustedLogFormatTargets(ctx, db.RecoverExhaustedLogFormatTargetsParams{
		Now: now, UpdateTime: now,
	}); err != nil {
		return err
	}
	if err := s.repository.queries.DeleteInactiveLogFormatTargets(ctx); err != nil {
		return err
	}
	services, err := s.repository.queries.ListLogFormatServices(ctx)
	if err != nil {
		return err
	}
	for _, serviceID := range services {
		if err := s.reconcileServiceLogFormats(ctx, serviceID); err != nil {
			// One invalid service must not prevent other services from being verified.
			slog.Warn("log format service reconciliation failed", "service_id", serviceID, "error", err)
		}
	}
	return ctx.Err()
}

func (s *Service) reconcileServiceLogFormats(ctx context.Context, serviceID int64) error {
	logs, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		return err
	}
	deployments, err := s.repository.queries.ListServiceDeploymentIDs(ctx, serviceID)
	if err != nil {
		return err
	}
	for _, log := range logs {
		if log.TemplateProcessingRuleID == nil || !boolValue(log.CollectionEnabled, true) {
			continue
		}
		for _, deploymentID := range deployments {
			if _, err = s.ensureLogFormatTarget(ctx, serviceID, log, deploymentID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) runDueLogFormats(ctx context.Context) error {
	targets, err := s.repository.queries.ListDueLogFormatTargets(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	var workers sync.WaitGroup
	for _, target := range targets {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := s.runLogFormatTarget(ctx, target); err != nil && ctx.Err() == nil {
				slog.Warn("log format target execution failed", "service_id", target.ServiceID,
					"log_definition_id", target.LogDefinitionID, "deployment_id", target.DeploymentID, "error", err)
			}
		}()
	}
	workers.Wait()
	return nil
}

func (s *Service) currentTargetFingerprint(ctx context.Context, target db.AssetsLogFormatTarget) (string, error) {
	enabled, _, err := s.repository.GetServiceLogDefaults(ctx, target.ServiceID)
	if err != nil || !enabled {
		return "", err
	}
	logs, err := s.repository.ListServiceTemplateLogs(ctx, target.ServiceID)
	if err != nil {
		return "", err
	}
	for _, log := range logs {
		if log.LogDefinition != target.LogDefinitionID || !boolValue(log.CollectionEnabled, true) || log.TemplateProcessingRuleID == nil {
			continue
		}
		instance, err := s.repository.queries.GetLogVerifyInstanceContext(ctx, db.GetLogVerifyInstanceContextParams{
			ServiceID: target.ServiceID, LogDefinitionID: target.LogDefinitionID, DeploymentID: target.DeploymentID,
		})
		if err != nil {
			return "", err
		}
		return logInstanceFingerprint(log.FormatFingerprint, instance)
	}
	return "", nil
}

func (s *Service) runLogFormatTarget(ctx context.Context, target db.AssetsLogFormatTarget) error {
	probeContext, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	fingerprint, err := s.currentTargetFingerprint(probeContext, target)
	if err != nil || fingerprint != target.Fingerprint {
		return err
	}
	tokenBytes := make([]byte, 16)
	if _, err = rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	now := time.Now().UTC()
	claimed, err := s.repository.queries.ClaimLogFormatTarget(probeContext, db.ClaimLogFormatTargetParams{
		ServiceID: target.ServiceID, LogDefinitionID: target.LogDefinitionID, DeploymentID: target.DeploymentID,
		Fingerprint: fingerprint, LeaseToken: token, LeaseUntil: now.Add(time.Minute), UpdateTime: now, Now: now,
	})
	if err != nil {
		return err
	}
	count, err := claimed.RowsAffected()
	if err != nil || count == 0 {
		return err
	}
	report, probeErr := s.logFormatVerifier.VerifyLogFormat(probeContext, LogFormatVerifyRequest{
		ServiceID: target.ServiceID, LogDefinitionID: target.LogDefinitionID, DeploymentID: target.DeploymentID,
		Source: LogFormatSourceInstance,
	})
	if probeErr == nil && (len(report.Targets) != 1 || report.Targets[0].DeploymentID != target.DeploymentID) {
		probeErr = LogFormatConfigurationError("真实实例认证报告缺失或目标不一致")
	}
	status, reason := logFormatOutcome(report, probeErr)
	// Re-read after the probe: old samples cannot certify a changed path/rule or a removed instance.
	fingerprint, err = s.currentTargetFingerprint(ctx, target)
	if err != nil || fingerprint != target.Fingerprint {
		return err
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return err
	}
	now = time.Now().UTC()
	_, err = s.repository.queries.FinishLogFormatTarget(ctx, db.FinishLogFormatTargetParams{
		ServiceID: target.ServiceID, LogDefinitionID: target.LogDefinitionID, DeploymentID: target.DeploymentID,
		Fingerprint: fingerprint, LeaseToken: token, Status: status, Report: encoded, Actor: target.Actor,
		Reason: reason, UpdateTime: now, NextAttemptAt: now.Add(logFormatRetryDelay(target.Attempts + 1)),
	})
	return err
}

func logFormatRetryDelay(attempt int32) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > logFormatMaxAttempts {
		attempt = logFormatMaxAttempts
	}
	return time.Minute * time.Duration(1<<uint(attempt-1))
}

func logFormatOutcome(report LogFormatVerifyReport, err error) (string, string) {
	if err != nil {
		var configurationError LogFormatConfigurationError
		if errors.As(err, &configurationError) {
			return "failed", err.Error()
		}
		return "waiting", err.Error()
	}
	for _, target := range report.Targets {
		if target.Error != "" {
			if target.FailureKind == "format" {
				return "failed", target.Error
			}
			return "waiting", target.Error
		}
	}
	if !report.Passed() {
		return "failed", "样本缺少必备字段或全部被规则丢弃"
	}
	return "verified", ""
}

func summarizeFormatInstances(instances []LogFormatInstanceState) string {
	if len(instances) == 0 {
		return "waiting"
	}
	counts := map[string]int{}
	for _, instance := range instances {
		counts[instance.Status]++
	}
	for _, state := range []string{"failed", "running", "waiting", "pending"} {
		if counts[state] > 0 {
			return state
		}
	}
	if counts["waived"] > 0 {
		return "waived"
	}
	return "verified"
}

func (s *Service) ListServiceLogFormats(ctx context.Context, serviceID int64) ([]ServiceTemplateLog, error) {
	logs, err := s.repository.ListServiceTemplateLogs(ctx, serviceID)
	if err != nil || len(logs) == 0 {
		return logs, err
	}
	if err = s.attachLogFormatStates(ctx, serviceID, logs); err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *Service) attachLogFormatStates(ctx context.Context, serviceID int64, logs []ServiceTemplateLog) error {
	stored, err := s.repository.queries.ListLogFormatTargets(ctx, serviceID)
	if err != nil {
		return err
	}
	deployments, err := s.repository.queries.ListServiceDeploymentIDs(ctx, serviceID)
	if err != nil {
		return err
	}
	byTarget := map[[2]int64]db.AssetsLogFormatTarget{}
	for _, target := range stored {
		byTarget[[2]int64{target.LogDefinitionID, target.DeploymentID}] = target
	}
	for index := range logs {
		log := &logs[index]
		log.FormatInstances = []LogFormatInstanceState{}
		// Legacy service-level and rule-fixture approvals cannot certify individual instances.
		log.FormatVerifiedAt, log.FormatVerifiedSource, log.FormatVerifiedBy = nil, "", ""
		for _, deploymentID := range deployments {
			instance, err := s.repository.queries.GetLogVerifyInstanceContext(ctx, db.GetLogVerifyInstanceContextParams{
				ServiceID: serviceID, LogDefinitionID: log.LogDefinition, DeploymentID: deploymentID,
			})
			if err != nil {
				return err
			}
			state := LogFormatInstanceState{DeploymentID: deploymentID, Name: instance.DeploymentInstanceName, Status: "pending"}
			fingerprint, fingerprintErr := logInstanceFingerprint(log.FormatFingerprint, instance)
			storedTarget, exists := byTarget[[2]int64{log.LogDefinition, deploymentID}]
			if fingerprintErr != nil {
				state.Status, state.Reason = "failed", fingerprintErr.Error()
			} else if exists && storedTarget.Fingerprint == fingerprint {
				state.Status, state.Attempts, state.Reason = storedTarget.Status, storedTarget.Attempts, storedTarget.Reason
				state.Actor, state.UpdatedAt = storedTarget.Actor, timestamp(storedTarget.UpdateTime)
				var report LogFormatVerifyReport
				if err = json.Unmarshal(storedTarget.Report, &report); err != nil {
					return fmt.Errorf("认证报告无效: %w", err)
				}
				state.Report = &report
			}
			if state.Status == "verified" {
				log.FormatPassed++
			}
			if state.Status == "waived" {
				log.FormatWaived++
			}
			log.FormatInstances = append(log.FormatInstances, state)
		}
		log.FormatTotal = len(deployments)
		log.FormatState = summarizeFormatInstances(log.FormatInstances)
	}
	return nil
}
