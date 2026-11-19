package baseline

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

type issueFilter struct {
	status, severity, ruleKey, pattern sql.NullString
	hostID                             sql.NullInt64
	environmentID                      sql.NullInt64
	environmentUnset                   bool
	page, size                         int32
}

func issuePagination(context *gin.Context) (int32, int32, error) {
	values := []int64{1, 10}
	for index, key := range []string{"page", "page_size"} {
		if raw, present := context.GetQuery(key); present {
			value, err := strconv.ParseInt(raw, 10, 32)
			if err != nil || value < 1 {
				return 0, 0, fmt.Errorf("%s 必须是正整数", key)
			}
			values[index] = value
		}
	}
	if values[1] > 100 || (values[0]-1)*values[1] > 2147483647 {
		return 0, 0, fmt.Errorf("分页范围无效，page_size 最大为 100")
	}
	return int32(values[0]), int32(values[1]), nil
}

func parseIssueFilter(context *gin.Context) (issueFilter, error) {
	filter := issueFilter{}
	var err error
	filter.page, filter.size, err = issuePagination(context)
	if err != nil {
		return filter, err
	}
	status, severity := context.Query("status"), context.Query("severity")
	if status != "" && status != "open" && status != "closed" {
		return filter, fmt.Errorf("问题状态无效")
	}
	if severity != "" && severity != "high" && severity != "medium" && severity != "low" {
		return filter, fmt.Errorf("风险级别无效")
	}
	filter.status = sql.NullString{String: status, Valid: status != ""}
	filter.severity = sql.NullString{String: severity, Valid: severity != ""}
	if raw, present := context.GetQuery("host_id"); present {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			return filter, fmt.Errorf("主机 ID 无效")
		}
		filter.hostID = sql.NullInt64{Int64: id, Valid: true}
	}
	if raw, present := context.GetQuery("environment_id"); present {
		if raw == "unassigned" {
			filter.environmentUnset = true
		} else {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id < 1 {
				return filter, fmt.Errorf("环境 ID 无效")
			}
			filter.environmentID = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	if key := context.Query("rule_key"); key != "" {
		if _, err := hex.DecodeString(key); err != nil || len(key) != 64 {
			return filter, fmt.Errorf("规则标识无效")
		}
		filter.ruleKey = sql.NullString{String: key, Valid: true}
	}
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		filter.pattern = sql.NullString{String: "%" + search + "%", Valid: true}
	}
	return filter, nil
}

func issueJSON(issue db.SecurityIssue, environmentID sql.NullInt64, environmentName string) gin.H {
	var currentEnvironmentID *int64
	if environmentID.Valid {
		currentEnvironmentID = &environmentID.Int64
	}
	return gin.H{
		"id": issue.ID, "source": "baseline", "host_id": issue.HostID,
		"host_name": issue.HostName, "host_ip": issue.HostIp, "baseline_id": issue.BaselineID,
		"item_id": issue.ItemID, "rule_key": issue.RuleKey, "title": issue.Title,
		"last_rule_version": issue.LastRuleVersion, "failure_rule_version": issue.FailureRuleVersion,
		"closure_reason": issue.ClosureReason,
		"severity":       issue.Severity, "chapter": issue.Chapter, "status": issue.Status,
		"last_result": issue.LastResult, "first_seen": issue.FirstSeen, "last_seen": issue.LastSeen,
		"last_checked": issue.LastChecked, "latest_scan_id": issue.LatestScanID,
		"occurrence_count": issue.OccurrenceCount, "reopen_count": issue.ReopenCount,
		"closed_at":      nullTimeString(issue.ClosedAt),
		"environment_id": currentEnvironmentID, "environment_name": environmentName,
	}
}

func issueEnvironmentOptions(ctx context.Context, queries *db.Queries) ([]gin.H, error) {
	rows, err := queries.ListSecurityIssueEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	options := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		option := gin.H{"id": nil, "name": row.EnvironmentName}
		if row.EnvironmentID.Valid {
			option["id"] = row.EnvironmentID.Int64
		}
		options = append(options, option)
	}
	return options, nil
}

func (handler *Handler) ListIssues(context *gin.Context) {
	filter, err := parseIssueFilter(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	queries := db.New(handler.db)
	total, err := queries.CountSecurityIssues(context, db.CountSecurityIssuesParams{
		Status: filter.status, Severity: filter.severity, HostID: filter.hostID, RuleKey: filter.ruleKey, Pattern: filter.pattern,
		EnvironmentID: filter.environmentID, EnvironmentUnset: filter.environmentUnset,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListSecurityIssues(context, db.ListSecurityIssuesParams{
		Status: filter.status, Severity: filter.severity, HostID: filter.hostID, RuleKey: filter.ruleKey, Pattern: filter.pattern,
		EnvironmentID: filter.environmentID, EnvironmentUnset: filter.environmentUnset,
		Limit: filter.size, Offset: (filter.page - 1) * filter.size,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, issueJSON(row.SecurityIssue, row.EnvironmentID, row.EnvironmentName))
	}
	environments, err := issueEnvironmentOptions(context, queries)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.PaginatedWith(context, items, total, filter.page, filter.size, gin.H{"environments": environments})
}

// sqlc 的聚合列为 interface{}；显式转换驱动值，避免 MySQL []byte 被编码成 base64。
func issueAggregateText(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", fmt.Errorf("unexpected security issue aggregate text %T", value)
	}
}

func issueAggregateTime(value any) (time.Time, error) {
	if typed, ok := value.(time.Time); ok {
		return typed.UTC(), nil
	}
	text, err := issueAggregateText(value)
	if err != nil {
		return time.Time{}, err
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid security issue aggregate time %q", text)
}

func (handler *Handler) ListIssueGroups(context *gin.Context) {
	filter, err := parseIssueFilter(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	queries := db.New(handler.db)
	total, err := queries.CountSecurityIssueGroups(context, db.CountSecurityIssueGroupsParams{
		Status: filter.status, Severity: filter.severity, HostID: filter.hostID, RuleKey: filter.ruleKey, Pattern: filter.pattern,
		EnvironmentID: filter.environmentID, EnvironmentUnset: filter.environmentUnset,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListSecurityIssueGroups(context, db.ListSecurityIssueGroupsParams{
		Status: filter.status, Severity: filter.severity, HostID: filter.hostID, RuleKey: filter.ruleKey, Pattern: filter.pattern,
		EnvironmentID: filter.environmentID, EnvironmentUnset: filter.environmentUnset,
		Limit: filter.size, Offset: (filter.page - 1) * filter.size,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		title, titleErr := issueAggregateText(row.Title)
		severity, severityErr := issueAggregateText(row.Severity)
		firstSeen, firstErr := issueAggregateTime(row.FirstSeen)
		lastChecked, lastErr := issueAggregateTime(row.LastChecked)
		if err := errors.Join(titleErr, severityErr, firstErr, lastErr); err != nil {
			response.Error(context, err)
			return
		}
		items = append(items, gin.H{
			"rule_key": row.RuleKey, "title": title, "severity": severity, "host_count": row.HostCount,
			"open_count": row.OpenCount, "closed_count": row.ClosedCount, "unverified_count": row.UnverifiedCount,
			"first_seen": firstSeen, "last_checked": lastChecked,
		})
	}
	environments, err := issueEnvironmentOptions(context, queries)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.PaginatedWith(context, items, total, filter.page, filter.size, gin.H{"environments": environments})
}

func (handler *Handler) GetIssue(context *gin.Context) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BusinessError(context, 400, "安全问题 ID 无效", nil)
		return
	}
	issue, err := db.New(handler.db).GetSecurityIssue(context, id)
	if errors.Is(err, sql.ErrNoRows) {
		response.BusinessError(context, 404, "安全问题不存在", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, issueJSON(issue.SecurityIssue, issue.EnvironmentID, issue.EnvironmentName))
}

func (handler *Handler) ListIssueHistory(context *gin.Context) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.BusinessError(context, 400, "安全问题 ID 无效", nil)
		return
	}
	page, size, err := issuePagination(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	scope := context.DefaultQuery("scope", "all")
	if scope != "all" && scope != "milestones" {
		response.BusinessError(context, 400, "scope 必须为 all 或 milestones", nil)
		return
	}
	queries := db.New(handler.db)
	if _, err = queries.GetSecurityIssue(context, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.BusinessError(context, 404, "安全问题不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	count := queries.CountSecurityIssueHistory
	if scope == "milestones" {
		count = queries.CountSecurityIssueMilestones
	}
	total, err := count(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	offset := (page - 1) * size
	var rows []db.SecurityIssueHistory
	if scope == "milestones" {
		rows, err = queries.ListSecurityIssueMilestones(context, db.ListSecurityIssueMilestonesParams{
			IssueID: id, Limit: size, Offset: offset,
		})
	} else {
		rows, err = queries.ListSecurityIssueHistory(context, db.ListSecurityIssueHistoryParams{
			IssueID: id, Limit: size, Offset: offset,
		})
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Paginated(context, rows, total, page, size)
}
