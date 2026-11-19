package baseline

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestIssueHistoryScopePagination(t *testing.T) {
	for _, scope := range []string{"", "&scope=all", "&scope=milestones"} {
		t.Run(scope, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			now := time.Now().UTC()
			mock.ExpectQuery(`FROM security_issue i .*WHERE i.id =`).WithArgs(int64(42)).
				WillReturnRows(sqlmock.NewRows(append(append([]string{}, issueColumns...), "environment_id", "environment_name")).
					AddRow(42, 9, "app", "ip", 1, 2, issueRuleKey(2), "v1", "v1", "", "SSH", "high", "OS",
						"open", "fail", now, now, now, 7, 1, 0, nil, nil, ""))
			countQuery := `SELECT COUNT\(\*\) FROM security_issue_history WHERE issue_id`
			listQuery := `FROM security_issue_history WHERE issue_id = .* LIMIT`
			if scope == "&scope=milestones" {
				countQuery = `WITH history_order AS .*SELECT COUNT\(\*\) FROM history_order`
				listQuery = `WITH history_order AS .*INNER JOIN history_order .* LIMIT`
			}
			mock.ExpectQuery(countQuery).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
			args := []driver.Value{int64(42), int64(2), int64(4)}
			if reflect.TypeOf(db.ListSecurityIssueHistoryParams{}).Field(0).Name == "Limit" {
				args = []driver.Value{int64(2), int64(4), int64(42)}
			}
			mock.ExpectQuery(listQuery).WithArgs(args...).
				WillReturnRows(sqlmock.NewRows([]string{"id", "issue_id", "scan_id", "rule_version", "checked_at", "result", "transition", "applied", "expected_value", "actual_value", "message", "remediation"}).
					AddRow(1, 42, 7, "v1", now, "fail", "discovered", true, []byte(`{"target":0}`), []byte(`{"value":1}`), "不符合", "修复建议"))
			ctx, recorder := ginTestContext("GET", "/sys/security/issues/42/history/?page=3&page_size=2"+scope, nil)
			ctx.Params = gin.Params{{Key: "id", Value: "42"}}
			(&Handler{db: pool}).ListIssueHistory(ctx)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("%v; response: %s", err, recorder.Body.String())
			}
			envelope := decodeResponse(t, recorder)
			if envelope["code"] != float64(200) {
				t.Fatalf("unexpected response: %s", recorder.Body.String())
			}
			data := envelope["data"].(map[string]any)
			rows := data["results"].([]any)
			if data["count"] != float64(5) || data["pageNumber"] != float64(3) || len(rows) != 1 {
				t.Fatalf("unexpected page: %v", data)
			}
			if row := rows[0].(map[string]any); row["transition"] != "discovered" || row["actual_value"].(map[string]any)["value"] != float64(1) {
				t.Fatalf("history evidence missing: %v", row)
			}
		})
	}
}

func TestIssueLifecycleAndStaleResults(t *testing.T) {
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	issue := db.SecurityIssue{Status: "open", FirstSeen: now, LastChecked: now, LatestScanID: 1}
	tests := []struct {
		scanID                     int64
		result, transition, status string
		occurrences, reopens       int64
	}{
		{1, "fail", "discovered", "open", 1, 0},
		{2, "fail", "persistent", "open", 2, 0},
		{3, "error", "check_error", "open", 2, 0},
		{4, "pass", "closed", "closed", 2, 0},
		{5, "error", "check_error", "closed", 2, 0},
		{6, "fail", "reopened", "open", 3, 1},
	}
	for _, test := range tests {
		transition, applied := applyIssueObservation(&issue, test.scanID, now.Add(time.Duration(test.scanID)*time.Hour), test.result, "v1")
		if !applied || transition != test.transition || issue.Status != test.status ||
			issue.OccurrenceCount != test.occurrences || issue.ReopenCount != test.reopens {
			t.Fatalf("scan %d: transition=%s applied=%v issue=%+v", test.scanID, transition, applied, issue)
		}
		if (issue.Status == "closed") != issue.ClosedAt.Valid {
			t.Fatalf("closed_at inconsistent: %+v", issue)
		}
	}
	before := issue
	if transition, applied := applyIssueObservation(&issue, 2, now.Add(2*time.Hour), "pass", "v2"); applied || transition != "stale" || !reflect.DeepEqual(before, issue) {
		t.Fatalf("old pass overwrote newer failure: %+v", issue)
	}
	if _, applied := applyIssueObservation(&issue, 5, issue.LastChecked, "pass", "v2"); applied {
		t.Fatal("equal timestamp must use scan id to order observations")
	}
}

func TestRuleEditsKeepIdentityAndVersionEvidenceSeparate(t *testing.T) {
	item := db.ListBaselineItemsRow{ID: 9, Name: "old", Config: json.RawMessage(`{"policy":"p","input_files":[{"key":"ssh","path":"/etc/ssh"}],"remediation":"old"}`)}
	key := issueRuleKey(item.ID)
	first, err := issueRuleVersion(item)
	if err != nil {
		t.Fatal(err)
	}
	item.Name, item.Category = "new", "renamed"
	item.Config = json.RawMessage(`{"remediation":"new","input_files":[{"path":"/etc/ssh","key":"ssh"}],"policy":"p"}`)
	same, err := issueRuleVersion(item)
	if err != nil || same != first {
		t.Fatalf("display/order change split identity: %s %s %v", first, same, err)
	}
	item.Config = json.RawMessage(`{"policy":"changed"}`)
	changed, err := issueRuleVersion(item)
	if err != nil || changed == first {
		t.Fatal("changed policy reused old evidence version")
	}
	if issueRuleKey(item.ID) != key {
		t.Fatal("editing a policy split its stable identity")
	}
	item.ID++
	if issueRuleKey(item.ID) == key {
		t.Fatal("recreated rule reused old identity")
	}
}

func TestChangedRuleClosesOriginalIssueWithoutClaimingHostRepair(t *testing.T) {
	now := time.Now().UTC()
	issue := db.SecurityIssue{Status: "open", FirstSeen: now, LastChecked: now, LatestScanID: 1}
	applyIssueObservation(&issue, 1, now, "fail", "old-policy")
	applyIssueObservation(&issue, 2, now.Add(time.Minute), "error", "corrected-policy")
	if issue.Status != "open" || issue.FailureRuleVersion != "old-policy" {
		t.Fatal("an error must not close or replace the failure evidence")
	}
	transition, applied := applyIssueObservation(&issue, 3, now.Add(2*time.Minute), "pass", "corrected-policy")
	if !applied || transition != "closed_rule_changed" || issue.Status != "closed" || issue.ClosureReason != "rule_changed" ||
		issue.LastRuleVersion != "corrected-policy" || issue.FailureRuleVersion != "old-policy" {
		t.Fatalf("cross-version verification was lost or mislabeled: %+v transition=%s", issue, transition)
	}
	applyIssueObservation(&issue, 4, now.Add(3*time.Minute), "pass", "corrected-policy")
	if issue.ClosureReason != "rule_changed" {
		t.Fatal("later passes must not rewrite why the issue was closed")
	}
	applyIssueObservation(&issue, 5, now.Add(4*time.Minute), "fail", "corrected-policy")
	if issue.Status != "open" || issue.ClosureReason != "" || issue.ReopenCount != 1 {
		t.Fatal("recurrence must reopen the same issue and clear the previous closure")
	}
	applyIssueObservation(&issue, 6, now.Add(5*time.Minute), "pass", "corrected-policy")
	if issue.ClosureReason != "verified" {
		t.Fatal("same-version verification was mislabeled as a rule change")
	}
}

func TestScanErrorsAndMissingResultsCannotBecomeIssuesOrPasses(t *testing.T) {
	items := []db.ListBaselineItemsRow{
		{ID: 1, Config: json.RawMessage(`{}`)}, {ID: 2, Config: json.RawMessage(`{}`)},
		{ID: 3, Config: json.RawMessage(`{}`)}, {ID: 4, Config: json.RawMessage(`{}`)},
		{ID: 5, Config: json.RawMessage(`{}`)},
	}
	results, err := buildScanResults(7, items, map[string]scanCheck{
		"baseline:7:0": {Status: "error", Message: "permission denied"},
		"baseline:7:1": {Status: "pass", Actual: map[string]any{"details": []any{}}},
		"baseline:7:2": {Status: "fail"},
		"baseline:7:3": {Status: "pass", Actual: map[string]any{"details": []any{map[string]any{"successful": true}}}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, result := range results {
		got = append(got, result.status)
	}
	if !reflect.DeepEqual(got, []string{"error", "error", "fail", "pass", "error"}) {
		t.Fatalf("statuses: %v", got)
	}
}

var issueColumns = []string{
	"id", "host_id", "host_name", "host_ip", "baseline_id", "item_id", "rule_key",
	"last_rule_version", "failure_rule_version", "closure_reason", "title", "severity", "chapter",
	"status", "last_result", "first_seen", "last_seen", "last_checked", "latest_scan_id", "occurrence_count", "reopen_count", "closed_at",
}

func TestIssueObservationIsIdempotentAndErrorsDoNotCreateIssues(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	item := scanResultRow{itemID: 2, ruleKey: issueRuleKey(2), ruleVersion: strings.Repeat("a", 64), status: "fail"}
	mock.ExpectQuery("SELECT .* FROM security_issue WHERE host_id").
		WithArgs(int64(9), int64(2)).
		WillReturnRows(sqlmock.NewRows(issueColumns).AddRow(3, 9, "host", "ip", 1, 2, item.ruleKey,
			item.ruleVersion, item.ruleVersion, "", "SSH", "high", "OS", "open", "fail", now, now, now, 7, 1, 0, nil))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM security_issue_history").
		WithArgs(int64(3), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	if err := reconcileIssue(context.Background(), db.New(database), 7, now, scanHost{HostID: 9}, item); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"error", "pass"} {
		mock.ExpectQuery("SELECT .* FROM security_issue WHERE host_id").WillReturnError(sql.ErrNoRows)
		item.status = status
		if err := reconcileIssue(context.Background(), db.New(database), 8, now, scanHost{HostID: 9}, item); err != nil {
			t.Fatal(err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHostResultsRollbackAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprint(canceled), func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			now := time.Now().UTC()
			status := "running"
			if canceled {
				status = "canceled"
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT create_time, status FROM security_scan").
				WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"create_time", "status"}).AddRow(now, status))
			if !canceled {
				mock.ExpectQuery("SELECT id FROM assets_host").WithArgs(int64(9)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
				mock.ExpectQuery("SELECT id, host_id, host_name.*FROM security_scan_target").
					WillReturnRows(sqlmock.NewRows([]string{"id", "host_id", "host_name", "host_ip", "instance_name_snapshot", "status", "passed_items", "failed_items", "compliance_rate", "error_message"}).
						AddRow(1, 9, "host", "ip", "host", "pending", 0, 0, "0.00", ""))
				mock.ExpectExec("INSERT INTO baseline_scan_result").WillReturnError(fmt.Errorf("write failed"))
			}
			mock.ExpectRollback()
			err = (&Handler{db: database}).persistHostResults(context.Background(), 7, scanHost{HostID: 9}, 1, "failed", "", []scanResultRow{{itemID: 2, status: "fail"}})
			if (err == nil) != canceled {
				t.Fatalf("canceled=%v err=%v", canceled, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLateFirstFailureUsesAlreadyPersistedNewerPass(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	key := issueRuleKey(2)
	version, newerVersion := strings.Repeat("a", 64), strings.Repeat("b", 64)
	host := scanHost{HostID: 9, HostName: "app", HostIP: "10.0.0.9"}
	item := scanResultRow{baselineID: 1, itemID: 2, ruleKey: key, ruleVersion: version, itemName: "SSH", severity: "high", chapter: "OS", status: "fail"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT create_time, status FROM security_scan").
		WithArgs(int64(10)).WillReturnRows(sqlmock.NewRows([]string{"create_time", "status"}).AddRow(now, "running"))
	mock.ExpectQuery("SELECT id FROM assets_host").WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectQuery("SELECT id, host_id, host_name.*FROM security_scan_target").
		WillReturnRows(sqlmock.NewRows([]string{"id", "host_id", "host_name", "host_ip", "instance_name_snapshot", "status", "passed_items", "failed_items", "compliance_rate", "error_message"}).
			AddRow(1, 9, "app", "10.0.0.9", "app", "pending", 0, 0, "0.00", ""))
	mock.ExpectExec("INSERT INTO baseline_scan_result").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT .* FROM security_issue WHERE host_id").WillReturnError(sql.ErrNoRows)
	expectCreateReturnsID(mock, "INSERT INTO security_issue", []driver.Value{
		int64(9), "app", "10.0.0.9", int64(1), int64(2), key, "SSH", "high", "OS", now, now, now, int64(10), version, version,
	}, 42)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM security_issue_history").
		WithArgs(int64(42), int64(10)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO security_issue_history").
		WithArgs(int64(42), int64(10), now, "fail", "discovered", true, []byte("null"), []byte("null"), "", "", version).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE security_issue SET host_name").
		WithArgs("app", "10.0.0.9", "SSH", "high", "OS", "open", "fail", now, now, now, int64(10), int64(1), int64(0), nil, version, version, "", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT r.scan_id, s.create_time AS checked_at").
		WillReturnRows(sqlmock.NewRows([]string{"scan_id", "checked_at", "status", "rule_version", "item_name", "severity", "chapter", "expected_value", "actual_value", "message", "remediation"}).
			AddRow(20, later, "pass", newerVersion, "SSH corrected", "medium", "Access", []byte("null"), []byte("null"), "verified", ""))
	mock.ExpectQuery("SELECT .* FROM security_issue WHERE host_id").
		WithArgs(int64(9), int64(2)).
		WillReturnRows(sqlmock.NewRows(issueColumns).AddRow(42, 9, "app", "10.0.0.9", 1, 2, key,
			version, version, "", "SSH", "high", "OS", "open", "fail", now, now, now, 10, 1, 0, nil))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM security_issue_history").
		WithArgs(int64(42), int64(20)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO security_issue_history").
		WithArgs(int64(42), int64(20), later, "pass", "closed_rule_changed", true, []byte("null"), []byte("null"), "verified", "", newerVersion).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("UPDATE security_issue SET host_name").
		WithArgs("app", "10.0.0.9", "SSH corrected", "medium", "Access", "closed", "pass", now, now, later, int64(20), int64(1), int64(0), later, newerVersion, version, "rule_changed", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE security_scan_target").
		WithArgs("failed", int32(0), int32(1), "0.00", "", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := (&Handler{db: database}).persistHostResults(context.Background(), 10, host, 1, "failed", "", []scanResultRow{item}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIssueListPaginationAndValidation(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := &Handler{db: database}
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM security_issue").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery("SELECT .* FROM security_issue").
		WillReturnRows(sqlmock.NewRows(issueColumns))
	mock.ExpectQuery("SELECT DISTINCT h.environment_id").
		WillReturnRows(sqlmock.NewRows([]string{"environment_id", "environment_name"}))
	ctx, recorder := ginTestContext("GET", "/sys/security/issues/?page=2&page_size=10&status=open&search=ssh", nil)
	handler.ListIssues(ctx)
	data := decodeResponse(t, recorder)["data"].(map[string]any)
	if data["count"] != float64(21) || data["pageNumber"] != float64(2) || data["totalPages"] != float64(3) || len(data["results"].([]any)) != 0 {
		t.Fatalf("invalid pagination response: %v", data)
	}
	for _, query := range []string{"status=invalid", "severity=invalid", "host_id=-1", "environment_id=-1", "environment_id=0",
		"environment_id=", "environment_id=invalid", "rule_key=bad", "page=0", "page_size=101", "page=2147483647&page_size=100"} {
		ctx, recorder := ginTestContext("GET", "/sys/security/issues/?"+query, nil)
		handler.ListIssues(ctx)
		if decodeResponse(t, recorder)["code"] != float64(400) {
			t.Fatalf("accepted %s", query)
		}
	}
	ctx, recorder = ginTestContext("GET", "/sys/security/issues/invalid/", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "invalid"}}
	handler.GetIssue(ctx)
	if decodeResponse(t, recorder)["code"] != float64(400) {
		t.Fatal("invalid id accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIssueGroupsConvertDriverValues(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(DISTINCT i.rule_key)")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT i.rule_key, MAX\\(i.title\\)").
		WillReturnRows(sqlmock.NewRows([]string{"rule_key", "title", "severity", "host_count", "open_count", "closed_count", "unverified_count", "first_seen", "last_checked"}).
			AddRow(strings.Repeat("a", 64), []byte("SSH"), []byte("high"), 3, 2, 1, 1, []byte("2026-11-17 00:00:00"), []byte("2026-11-17 01:00:00")))
	mock.ExpectQuery("SELECT DISTINCT h.environment_id").
		WillReturnRows(sqlmock.NewRows([]string{"environment_id", "environment_name"}))
	ctx, recorder := ginTestContext("GET", "/sys/security/issues/groups/", nil)
	(&Handler{db: database}).ListIssueGroups(ctx)
	data := decodeResponse(t, recorder)["data"].(map[string]any)
	row := data["results"].([]any)[0].(map[string]any)
	if row["title"] != "SSH" || row["first_seen"] != "2026-11-17T00:00:00Z" || row["host_count"] != float64(3) {
		t.Fatalf("invalid aggregate response: %v", row)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIssueEnvironmentFilterAppliesBeforeCountsGroupsAndPagination(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		for _, unassigned := range []bool{false, true} {
			t.Run(fmt.Sprintf("grouped=%t/unassigned=%t", grouped, unassigned), func(t *testing.T) {
				pool, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer pool.Close()
				now := time.Now().UTC()
				key := strings.Repeat("a", 64)
				var environmentID driver.Value = int64(2)
				rawEnvironment, environmentName := "2", "生产"
				if unassigned {
					environmentID, rawEnvironment, environmentName = nil, "unassigned", ""
				}
				args := []driver.Value{
					"open", "open", "high", "high", int64(9), int64(9), key, key,
					"%ssh%", "%ssh%", "%ssh%", "%ssh%", environmentID, environmentID, unassigned,
				}
				environmentClause := regexp.QuoteMeta("AND (h.environment_id = ? OR ? IS NULL) AND (? = FALSE OR h.environment_id IS NULL)")
				countPrefix, listPrefix := regexp.QuoteMeta("SELECT COUNT(*)"), "SELECT i.id,"
				if grouped {
					countPrefix, listPrefix = regexp.QuoteMeta("SELECT COUNT(DISTINCT i.rule_key)"), regexp.QuoteMeta("SELECT i.rule_key, MAX(i.title)")
				}
				mock.ExpectQuery(countPrefix + ".*LEFT JOIN assets_host h ON h.id = i.host_id.*" + environmentClause).
					WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
				listArgs := append(append([]driver.Value{}, args...), int64(10), int64(10))
				listRows := sqlmock.NewRows(append(append([]string{}, issueColumns...), "environment_id", "environment_name")).
					AddRow(42, 9, "app", "10.0.0.9", 1, 2, key, "v1", "v1", "", "SSH", "high", "OS",
						"open", "fail", now, now, now, 7, 1, 0, nil, environmentID, environmentName)
				if grouped {
					listRows = sqlmock.NewRows([]string{"rule_key", "title", "severity", "host_count", "open_count", "closed_count", "unverified_count", "first_seen", "last_checked"}).
						AddRow(key, "SSH", "high", 3, 3, 0, 0, now, now)
				}
				mock.ExpectQuery(listPrefix + ".*LEFT JOIN assets_host h ON h.id = i.host_id.*" + environmentClause + ".*LIMIT .* OFFSET").
					WithArgs(listArgs...).WillReturnRows(listRows)
				mock.ExpectQuery("SELECT DISTINCT h.environment_id").WithArgs().
					WillReturnRows(sqlmock.NewRows([]string{"environment_id", "environment_name"}).
						AddRow(nil, "").AddRow(1, "测试").AddRow(2, "生产"))
				ctx, recorder := ginTestContext("GET", "/sys/security/issues/?page=2&status=open&severity=high&host_id=9&search=ssh&rule_key="+key+"&environment_id="+rawEnvironment, nil)
				handler := &Handler{db: pool}
				if grouped {
					handler.ListIssueGroups(ctx)
				} else {
					handler.ListIssues(ctx)
				}
				envelope := decodeResponse(t, recorder)
				if envelope["code"] != float64(200) {
					t.Fatalf("query failed: %s", recorder.Body.String())
				}
				data := envelope["data"].(map[string]any)
				if data["count"] != float64(21) || data["pageNumber"] != float64(2) ||
					data["totalPages"] != float64(3) || len(data["environments"].([]any)) != 3 {
					t.Fatalf("filter metadata or server pagination mismatch: %v", data)
				}
				row := data["results"].([]any)[0].(map[string]any)
				if grouped {
					if row["host_count"] != float64(3) || row["open_count"] != float64(3) {
						t.Fatalf("aggregate mismatch: %v", row)
					}
				} else if row["environment_name"] != environmentName ||
					(unassigned && row["environment_id"] != nil) || (!unassigned && row["environment_id"] != float64(2)) {
					t.Fatalf("current host environment mismatch: %v", row)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestIssueDetailReadsCurrentEnvironmentWithoutChangingIssue(t *testing.T) {
	for _, environmentID := range []driver.Value{int64(2), nil} {
		t.Run(fmt.Sprint(environmentID), func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			now := time.Now().UTC()
			mock.ExpectQuery("FROM security_issue i LEFT JOIN assets_host h ON h.id = i.host_id LEFT JOIN assets_business_environment e ON e.id = h.environment_id").
				WithArgs(int64(42)).
				WillReturnRows(sqlmock.NewRows(append(append([]string{}, issueColumns...), "environment_id", "environment_name")).
					AddRow(42, 9, "snapshot-host", "snapshot-ip", 1, 2, issueRuleKey(2), "v1", "v1", "", "SSH", "high", "OS",
						"open", "fail", now, now, now, 7, 1, 0, nil, environmentID, "当前名称"))
			ctx, recorder := ginTestContext("GET", "/sys/security/issues/42/", nil)
			ctx.Params = gin.Params{{Key: "id", Value: "42"}}
			(&Handler{db: pool}).GetIssue(ctx)
			data := decodeResponse(t, recorder)["data"].(map[string]any)
			if data["environment_name"] != "当前名称" || (environmentID == nil && data["environment_id"] != nil) ||
				(environmentID != nil && data["environment_id"] != float64(2)) ||
				data["host_name"] != "snapshot-host" || data["status"] != "open" {
				t.Fatalf("environment read changed issue evidence: %v", data)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIssueEnvironmentOptionsFailureIsNotEmptySuccess(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM security_issue").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM security_issue").
		WillReturnRows(sqlmock.NewRows(issueColumns))
	mock.ExpectQuery("SELECT DISTINCT h.environment_id").WillReturnError(sql.ErrConnDone)
	ctx, recorder := ginTestContext("GET", "/sys/security/issues/", nil)
	(&Handler{db: pool}).ListIssues(ctx)
	if decodeResponse(t, recorder)["code"] != float64(600) {
		t.Fatalf("environment query failure hidden: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
