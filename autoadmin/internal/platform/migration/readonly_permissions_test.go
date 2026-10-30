package migration

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var readonlyWritePermissions = []string{
	"assets:hosts:execute",
	"system:scheduler:update", "system:scheduler:execute",
	"sys:config:update",
	"monitor:create", "monitor:update", "monitor:delete", "monitor:execute", "monitor:logs:delete",
	"monitor:log_collect:create", "monitor:log_collect:execute", "monitor:log_collect:delete",
}

func readonlyMigrationSQL(t *testing.T, dialect, direction string) string {
	t.Helper()
	return strings.TrimSpace(sqlCodeLines(readRepoFile(t,
		"db/migrations/"+dialect+"/000061_readonly_permissions."+direction+".sql")))
}

func TestReadonlyPermissionsMigrationDialectsMatch(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		if readonlyMigrationSQL(t, "mysql", direction) != readonlyMigrationSQL(t, "postgres", direction) {
			t.Fatalf("%s readonly permission SQL must have identical semantics in both dialects", direction)
		}
	}
}

func TestReadonlyPermissionsMigrationCreatesUniqueAssignableActions(t *testing.T) {
	up := readonlyMigrationSQL(t, "mysql", "up")
	insert := strings.Split(up, ";")[0]
	// Match only action rows, not their parent read permissions.
	rows := regexp.MustCompile(`SELECT '[^']+'(?: AS name)?, '([^']+)'(?: AS perms)?, '([^']+)'(?: AS parent_perms)?, '([^']*)'`).
		FindAllStringSubmatch(insert, -1)
	var permissions []string
	for _, row := range rows {
		permissions = append(permissions, row[1])
		switch {
		case row[1] == "monitor:logs:delete":
			if row[2] != "monitor:view" || row[3] != "monitor/log-center/index" {
				t.Fatal("log deletion must be under the visible log center page")
			}
		case strings.HasPrefix(row[1], "monitor:log_collect:"):
			if row[2] != "monitor:log_collect:view" || row[3] != "monitor/log-collectors/index" {
				t.Fatal("collection actions must be under the collection read page")
			}
		case strings.HasPrefix(row[1], "monitor:"):
			if row[2] != "monitor:view" || row[3] != "monitor/index" {
				t.Fatal("shared monitor actions must be under the intelligent monitoring page")
			}
		}
	}
	if !reflect.DeepEqual(permissions, readonlyWritePermissions) {
		t.Fatalf("expected exactly one assignable node per new action: got %v", permissions)
	}
	for _, required := range []string{
		"name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded",
		"p.id, t.order_num, '', NULL, 'F', t.perms",
		"p.location, FALSE",
		"candidate.menu_type = 'C' AND candidate.perms = t.parent_perms",
		"candidate.component = t.parent_component",
		"ORDER BY candidate.id LIMIT 1",
		"NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.perms = t.perms)",
	} {
		if !strings.Contains(insert, required) {
			t.Fatalf("missing schema-safe, unique dynamic parent/action selection: %s", required)
		}
	}
	if regexp.MustCompile(`(?i)\b(?:id|parent_id)\s*=\s*\d+`).MatchString(insert) {
		t.Fatal("parent menu IDs must never be hardcoded")
	}
}

func TestReadonlyPermissionsMigrationNeverPromotesReaders(t *testing.T) {
	up := readonlyMigrationSQL(t, "mysql", "up")
	statements := strings.Split(up, ";")
	if len(statements) != 5 {
		t.Fatalf("expected only action insertion, admin grants, baseline read rename and baseline read grants; got %d statements", len(statements)-1)
	}
	adminGrant := strings.TrimSpace(statements[1])
	for _, required := range []string{
		"INSERT INTO sys_role_menu (role_id, menu_id)",
		"SELECT r.id, m.id FROM sys_role r",
		"m.menu_type = 'F' AND m.remark = 'migration:000061:readonly_permissions'",
		"WHERE r.code = 'admin'",
		"NOT EXISTS",
	} {
		if !strings.Contains(adminGrant, required) {
			t.Fatalf("write grants must be restricted to admin and migration-owned nodes: missing %s", required)
		}
	}
	for _, forbidden := range []string{"sys_role_menu rm", ":view", ":list", "'common'", "'test'"} {
		if strings.Contains(adminGrant, forbidden) {
			t.Fatalf("existing readers or ordinary roles must not gain write grants: %s", forbidden)
		}
	}
	rename := strings.TrimSpace(statements[2])
	if rename != "UPDATE sys_menu SET perms = 'baseline:view', update_time = CURRENT_DATE\nWHERE menu_type = 'C' AND perms = 'baseline:manage'" {
		t.Fatalf("only baseline C pages may change permission; F manage must survive: %s", rename)
	}
	readGrant := strings.TrimSpace(statements[3])
	for _, required := range []string{
		"SELECT DISTINCT rm.role_id, page.id",
		"granted.menu_type = 'F' AND granted.perms = 'baseline:manage'",
		"page.id = granted.parent_id AND page.menu_type = 'C' AND page.perms = 'baseline:view'",
		"NOT EXISTS",
	} {
		if !strings.Contains(readGrant, required) {
			t.Fatalf("baseline backfill must only grant page reads to existing managers: missing %s", required)
		}
	}
}

func TestReadonlyPermissionsRollbackPreservesExistingPermissions(t *testing.T) {
	down := readonlyMigrationSQL(t, "mysql", "down")
	statements := strings.Split(down, ";")
	if len(statements) != 4 ||
		!strings.HasPrefix(statements[0], "DELETE FROM sys_role_menu") ||
		!strings.HasPrefix(strings.TrimSpace(statements[1]), "DELETE FROM sys_menu") {
		t.Fatal("rollback must clear role references before removing only migration-created menus")
	}
	for _, statement := range statements[:2] {
		if !strings.Contains(statement, "menu_type = 'F' AND remark = 'migration:000061:readonly_permissions'") {
			t.Fatal("rollback must preserve preexisting same-code permissions")
		}
		matches := regexp.MustCompile(`'(assets:[^']+|system:[^']+|sys:[^']+|monitor:[^']+)'`).FindAllStringSubmatch(statement, -1)
		var removed []string
		for _, match := range matches {
			removed = append(removed, match[1])
		}
		if !reflect.DeepEqual(removed, readonlyWritePermissions) {
			t.Fatalf("rollback may delete only new actions, not old view/manage/scan grants: %v", removed)
		}
	}
	restore := strings.TrimSpace(statements[2])
	if restore != "UPDATE sys_menu SET perms = 'baseline:manage', update_time = CURRENT_DATE\nWHERE menu_type = 'C' AND perms = 'baseline:view'" {
		t.Fatalf("rollback must restore only baseline page permissions: %s", restore)
	}
}
