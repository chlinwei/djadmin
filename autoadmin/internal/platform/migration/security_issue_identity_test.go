package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// 仅用专属连接上的随机命名临时表回放数据迁移，不读写连接所指数据库的业务表。
func TestSecurityIssueIdentityMigration(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("SECURITY_ISSUE_MIGRATION_" + strings.ToUpper(dialect) + "_DSN")
			if dsn == "" {
				t.Skip("set SECURITY_ISSUE_MIGRATION_" + strings.ToUpper(dialect) + "_DSN to exercise temporary-table migration")
			}
			for _, populated := range []bool{false, true} {
				name := "empty"
				if populated {
					name = "populated"
				}
				t.Run(name, func(t *testing.T) {
					testSecurityIssueIdentityMigration(t, dialect, dsn, populated)
				})
			}
		})
	}
}

func testSecurityIssueIdentityMigration(t *testing.T, dialect, dsn string, populated bool) {
	t.Helper()
	driver := dialect
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	prefix := fmt.Sprintf("si_%x_", time.Now().UnixNano())
	rewrite := strings.NewReplacer("security_issue", prefix+"security_issue", "baseline_scan_result", prefix+"baseline_scan_result")
	exec := func(statement string) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, rewrite.Replace(statement)); err != nil {
			t.Fatalf("temporary fixture SQL failed: %v\n%s", err, statement)
		}
	}

	old := readRepoFile(t, "db/migrations/"+dialect+"/000066_security_issues.up.sql")
	for _, table := range []string{"security_issue", "security_issue_history"} {
		block := regexp.MustCompile(`(?s)CREATE TABLE ` + table + ` \(.*?\n\);`).FindString(old)
		if block == "" {
			t.Fatalf("missing original schema for %s", table)
		}
		// MySQL 临时表不支持外键；仅移除测试表的 FK，生产迁移仍保留原约束。
		block = regexp.MustCompile(`,\n\s*CONSTRAINT security_issue_history_issue_fk[^\n]*`).ReplaceAllString(block, "")
		exec(strings.Replace(block, "CREATE TABLE", "CREATE TEMPORARY TABLE", 1))
	}
	exec(`CREATE TEMPORARY TABLE baseline_scan_result (host_id bigint, item_id bigint, scan_id bigint, rule_key varchar(64))`)
	exec(`CREATE INDEX baseline_result_rule ON baseline_scan_result (host_id, rule_key, scan_id)`)

	if populated {
		exec(`INSERT INTO security_issue
			(id, host_id, host_name, host_ip, baseline_id, item_id, rule_key, title, severity, chapter,
			 status, last_result, first_seen, last_seen, last_checked, latest_scan_id, occurrence_count, reopen_count, closed_at)
			VALUES
			(200, 9, 'host9', 'ip9', 2, 7, REPEAT('a',64), 'old', 'high', 'OS', 'open', 'fail',
			 '2026-01-01', '2026-01-03', '2026-01-03', 3, 2, 0, NULL),
			(201, 9, 'host9', 'ip9', 2, 7, REPEAT('b',64), 'corrected', 'medium', 'OS', 'closed', 'pass',
			 '2026-01-02', '2026-01-02', '2026-01-04', 4, 1, 0, '2026-01-04'),
			(210, 10, 'host10', 'ip10', 2, 7, REPEAT('c',64), 'failing', 'high', 'OS', 'open', 'fail',
			 '2026-01-03', '2026-01-03', '2026-01-03', 3, 1, 0, NULL),
			(211, 10, 'host10', 'ip10', 2, 7, REPEAT('d',64), 'error', 'high', 'OS', 'closed', 'error',
			 '2026-01-01', '2026-01-01', '2026-01-05', 5, 1, 0, '2026-01-02'),
			(220, 11, 'host11', 'ip11', 2, 8, REPEAT('e',64), 'verified', 'low', 'OS', 'closed', 'pass',
			 '2026-01-01', '2026-01-01', '2026-01-02', 2, 1, 0, '2026-01-02')`)
		exec(`INSERT INTO security_issue_history
			(id, issue_id, scan_id, checked_at, result, transition, applied, expected_value, actual_value, message, remediation)
			VALUES
			(1,200,1,'2026-01-01','fail','discovered',TRUE,'{"policy":"original"}','{"stat":"0"}','first','strict'),
			(2,200,3,'2026-01-03','fail','persistent',TRUE,'{}','{}','still failing','strict'),
			(3,201,2,'2026-01-02','fail','discovered',TRUE,'{}','{}','other version','corrected'),
			(4,201,4,'2026-01-04','pass','closed',TRUE,'{}','{}','passed','corrected'),
			(5,210,3,'2026-01-03','fail','discovered',TRUE,'{}','{}','new failure','strict'),
			(6,211,1,'2026-01-01','fail','discovered',TRUE,'{}','{}','old failure','strict'),
			(7,211,2,'2026-01-02','pass','closed',TRUE,'{}','{}','old pass','strict'),
			(8,211,5,'2026-01-05','error','check_error',TRUE,'{}','{}','offline','strict'),
			(9,220,1,'2026-01-01','fail','discovered',TRUE,'{}','{}','failure','strict'),
			(10,220,2,'2026-01-02','pass','closed',TRUE,'{}','{}','same version pass','strict')`)
		exec(`INSERT INTO baseline_scan_result VALUES (9,7,1,REPEAT('a',64)), (9,7,4,REPEAT('b',64)), (12,9,0,NULL)`)
	}
	before := readIssueMigrationHistory(t, ctx, conn, rewrite, false)
	for id, row := range before {
		if row[0] == "201" {
			row[0] = "200"
		}
		if row[0] == "211" {
			row[0] = "210"
		}
		before[id] = row
	}
	up := sqlCodeLines(readRepoFile(t, "db/migrations/"+dialect+"/000067_stable_security_issue_identity.up.sql"))
	for _, statement := range strings.Split(up, ";") {
		if strings.TrimSpace(statement) != "" {
			exec(statement)
		}
	}
	after := readIssueMigrationHistory(t, ctx, conn, rewrite, true)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("migration changed evidence or lost history:\nbefore=%v\nafter=%v", before, after)
	}

	rows, err := conn.QueryContext(ctx, rewrite.Replace(`SELECT id, rule_key, status, last_result, closure_reason,
		last_rule_version, failure_rule_version, occurrence_count, closed_at IS NULL, title,
		EXTRACT(DAY FROM first_seen), EXTRACT(DAY FROM last_seen), EXTRACT(DAY FROM last_checked), latest_scan_id
		FROM security_issue ORDER BY id`))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id, occurrences int64
		var firstDay, lastDay, checkedDay, scanID int64
		var key, status, result, reason, version, failure, title string
		var noClosure bool
		if err := rows.Scan(&id, &key, &status, &result, &reason, &version, &failure, &occurrences, &noClosure,
			&title, &firstDay, &lastDay, &checkedDay, &scanID); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%d|%t|%s|%d|%d|%d|%d",
			id, key, status, result, reason, version, failure, occurrences, noClosure, title, firstDay, lastDay, checkedDay, scanID))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var want []string
	if populated {
		want = []string{
			fmt.Sprintf("200|%064x|closed|pass|rule_changed|%s|%s|3|false|corrected|1|3|4|4", 7, strings.Repeat("b", 64), strings.Repeat("a", 64)),
			fmt.Sprintf("210|%064x|open|error||%s|%s|2|true|error|1|3|5|5", 7, strings.Repeat("d", 64), strings.Repeat("c", 64)),
			fmt.Sprintf("220|%064x|closed|pass|verified|%s|%s|1|false|verified|1|1|2|2", 8, strings.Repeat("e", 64), strings.Repeat("e", 64)),
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("incorrect consolidated state:\ngot=%v\nwant=%v", got, want)
	}
	if populated {
		var count int
		if err := conn.QueryRowContext(ctx, rewrite.Replace(`SELECT COUNT(*) FROM baseline_scan_result
			WHERE (scan_id=1 AND rule_version=REPEAT('a',64)) OR (scan_id=4 AND rule_version=REPEAT('b',64))
			   OR (scan_id=0 AND rule_version IS NULL)`)).Scan(&count); err != nil || count != 3 {
			t.Fatalf("scan version evidence changed: count=%d err=%v", count, err)
		}
	}
}

func readIssueMigrationHistory(t *testing.T, ctx context.Context, conn *sql.Conn, rewrite *strings.Replacer, migrated bool) map[int64][]string {
	t.Helper()
	version := "i.rule_key"
	if migrated {
		version = "h.rule_version"
	}
	query := `SELECT h.id, h.issue_id, h.scan_id, h.result, h.transition, h.applied,
		h.expected_value, h.actual_value, h.message, h.remediation, ` + version + `, h.checked_at
		FROM security_issue_history h JOIN security_issue i ON i.id=h.issue_id ORDER BY h.id`
	rows, err := conn.QueryContext(ctx, rewrite.Replace(query))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := make(map[int64][]string)
	for rows.Next() {
		var id int64
		values := make([]string, 11)
		args := []any{&id}
		for i := range values {
			args = append(args, &values[i])
		}
		if err := rows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		for _, i := range []int{5, 6} {
			var evidence any
			if err := json.Unmarshal([]byte(values[i]), &evidence); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(evidence)
			if err != nil {
				t.Fatal(err)
			}
			values[i] = string(encoded)
		}
		result[id] = values
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
