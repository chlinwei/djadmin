package baseline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// 问题身份只取稳定条目 ID；内容哈希仅作为证据版本，不能让编辑规则制造重复问题。
func issueRuleKey(itemID int64) string {
	return fmt.Sprintf("%064x", itemID)
}

func issueRuleVersion(item db.ListBaselineItemsRow) (string, error) {
	var config map[string]any
	if err := json.Unmarshal(item.Config, &config); err != nil {
		return "", fmt.Errorf("decode baseline rule %d: %w", item.ID, err)
	}
	delete(config, "remediation")
	canonical, err := json.Marshal(struct {
		ItemID int64          `json:"item_id"`
		Config map[string]any `json:"config"`
	}{item.ID, config})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(canonical)), nil
}

// applyIssueObservation 只让较新的扫描结论改变当前状态；乱序结果仍保存证据。
func applyIssueObservation(issue *db.SecurityIssue, scanID int64, checkedAt time.Time, result, version string) (string, bool) {
	if checkedAt.Before(issue.LastChecked) || (checkedAt.Equal(issue.LastChecked) && scanID < issue.LatestScanID) {
		return "stale", false
	}
	transition := "check_error"
	switch result {
	case "fail":
		transition = "persistent"
		if issue.OccurrenceCount == 0 {
			transition = "discovered"
		} else if issue.Status == "closed" {
			transition = "reopened"
			issue.ReopenCount++
		}
		issue.Status, issue.LastSeen = "open", checkedAt
		issue.ClosedAt = sql.NullTime{}
		issue.ClosureReason = ""
		issue.FailureRuleVersion = version
		issue.OccurrenceCount++
	case "pass":
		transition = "verified"
		if issue.Status == "open" {
			transition = "closed"
			issue.ClosedAt = sql.NullTime{Time: checkedAt, Valid: true}
			issue.ClosureReason = "verified"
			// 版本不同只证明当前规则下通过，不能将标准放宽或策略纠错宣称为主机修复。
			if version != issue.FailureRuleVersion {
				transition, issue.ClosureReason = "closed_rule_changed", "rule_changed"
			}
		}
		issue.Status = "closed"
	}
	issue.LastResult, issue.LastChecked, issue.LatestScanID = result, checkedAt, scanID
	issue.LastRuleVersion = version
	return transition, true
}

func reconcileIssue(ctx context.Context, queries *db.Queries, scanID int64, checkedAt time.Time, host scanHost, item scanResultRow) error {
	issue, err := queries.GetSecurityIssueByIdentity(ctx, db.GetSecurityIssueByIdentityParams{HostID: host.HostID, ItemID: item.itemID})
	if errors.Is(err, sql.ErrNoRows) {
		if item.status != "fail" {
			return nil
		}
		id, createErr := queries.CreateSecurityIssue(ctx, db.CreateSecurityIssueParams{
			HostID: host.HostID, HostName: host.HostName, HostIp: host.HostIP,
			BaselineID: item.baselineID, ItemID: item.itemID, RuleKey: item.ruleKey,
			Title: item.itemName, Severity: item.severity, Chapter: item.chapter,
			FirstSeen: checkedAt, LastSeen: checkedAt, LastChecked: checkedAt, LatestScanID: scanID,
			LastRuleVersion: item.ruleVersion, FailureRuleVersion: item.ruleVersion,
		})
		if createErr != nil {
			return createErr
		}
		issue = db.SecurityIssue{
			ID: id, HostID: host.HostID, RuleKey: item.ruleKey, Status: "open",
			FirstSeen: checkedAt, LastSeen: checkedAt, LastChecked: checkedAt, LatestScanID: scanID,
			LastRuleVersion: item.ruleVersion, FailureRuleVersion: item.ruleVersion,
		}
	} else if err != nil {
		return err
	}
	exists, err := queries.SecurityIssueHistoryExists(ctx, db.SecurityIssueHistoryExistsParams{IssueID: issue.ID, ScanID: scanID})
	if err != nil || exists > 0 {
		return err
	}
	transition, applied := applyIssueObservation(&issue, scanID, checkedAt, item.status, item.ruleVersion)
	if err = queries.CreateSecurityIssueHistory(ctx, db.CreateSecurityIssueHistoryParams{
		IssueID: issue.ID, ScanID: scanID, CheckedAt: checkedAt, Result: item.status,
		Transition: transition, Applied: applied,
		ExpectedValue: jsonBytes(item.expected), ActualValue: jsonBytes(item.actual),
		Message: item.message, Remediation: item.remediation,
		RuleVersion: item.ruleVersion,
	}); err != nil {
		return err
	}
	if !applied {
		if item.status == "fail" {
			if checkedAt.Before(issue.FirstSeen) {
				issue.FirstSeen = checkedAt
			}
			if checkedAt.After(issue.LastSeen) {
				issue.LastSeen = checkedAt
			}
			return queries.UpdateSecurityIssueDiscoveryTimes(ctx, db.UpdateSecurityIssueDiscoveryTimesParams{
				ID: issue.ID, FirstSeen: issue.FirstSeen, LastSeen: issue.LastSeen,
			})
		}
		return nil
	}
	return queries.UpdateSecurityIssue(ctx, db.UpdateSecurityIssueParams{
		ID: issue.ID, HostName: host.HostName, HostIp: host.HostIP, Title: item.itemName,
		Severity: item.severity, Chapter: item.chapter, Status: issue.Status, LastResult: issue.LastResult,
		FirstSeen: issue.FirstSeen, LastSeen: issue.LastSeen, LastChecked: issue.LastChecked,
		LatestScanID: issue.LatestScanID, OccurrenceCount: issue.OccurrenceCount,
		ReopenCount: issue.ReopenCount, ClosedAt: issue.ClosedAt,
		LastRuleVersion: issue.LastRuleVersion, FailureRuleVersion: issue.FailureRuleVersion, ClosureReason: issue.ClosureReason,
	})
}

// 扫描结果、问题及历史同事务落库，无异步双写窗口；主机锁保护“首次发现”的并发创建。
// 先锁扫描父行，与取消互斥，取消之后到达的结果不能更新安全台账。
func (handler *Handler) persistHostResults(ctx context.Context, scanID int64, host scanHost, targetID int64, status, message string, results []scanResultRow) error {
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := db.New(tx)
	scan, err := queries.GetSecurityIssueScanForShare(ctx, scanID)
	if err != nil {
		return err
	}
	if scan.Status == "canceled" {
		return nil
	}
	if scan.Status != "running" {
		return fmt.Errorf("scan %d is not running: %s", scanID, scan.Status)
	}
	if _, err = queries.LockSecurityIssueHost(ctx, host.HostID); err != nil {
		return err
	}
	// 同一目标事务的重试不能重复插入报告明细或累计问题次数。
	target, err := queries.GetBaselineScanTarget(ctx, db.GetBaselineScanTargetParams{ScanID: scanID, HostID: host.HostID})
	if err != nil {
		return err
	}
	if target.Status != "pending" && target.Status != "running" {
		return nil
	}
	if err = flushResults(ctx, tx, scanID, host.HostID, results); err != nil {
		return err
	}
	passed, failed := 0, 0
	for _, result := range results {
		if err = reconcileIssue(ctx, queries, scanID, scan.CreateTime, host, result); err != nil {
			return err
		}
		latest, latestErr := queries.GetLatestSecurityRuleObservation(ctx, db.GetLatestSecurityRuleObservationParams{
			HostID: host.HostID, ItemID: result.itemID,
		})
		if latestErr != nil {
			return latestErr
		}
		if latest.ScanID != scanID {
			newer := result
			if !latest.RuleVersion.Valid || latest.RuleVersion.String == "" {
				return fmt.Errorf("scan %d has no rule version", latest.ScanID)
			}
			newer.ruleVersion = latest.RuleVersion.String
			newer.itemName, newer.severity, newer.chapter = latest.ItemName, latest.Severity, latest.Chapter
			newer.status, newer.message, newer.remediation = latest.Status, latest.Message, latest.Remediation
			if err = json.Unmarshal(latest.ExpectedValue, &newer.expected); err != nil {
				return err
			}
			if err = json.Unmarshal(latest.ActualValue, &newer.actual); err != nil {
				return err
			}
			if err = reconcileIssue(ctx, queries, latest.ScanID, latest.CheckedAt, host, newer); err != nil {
				return err
			}
		}
		if result.status == "pass" {
			passed++
		} else if result.status == "fail" {
			failed++
		}
	}
	compliance := 0.0
	if len(results) > 0 {
		compliance = float64(passed) / float64(len(results)) * 100
	}
	if err = queries.FinishBaselineScanTarget(ctx, db.FinishBaselineScanTargetParams{
		Status: status, PassedItems: int32(passed), FailedItems: int32(failed),
		ComplianceRate: decimalString(compliance), ErrorMessage: message, ID: targetID,
	}); err != nil {
		return err
	}
	return tx.Commit()
}
