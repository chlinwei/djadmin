package migration

import (
	"strings"
	"testing"
)

func TestFlatHostGroupMigrationPreservesIdentitiesAndHostBindings(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		schema := readRepoFile(t, "db/schema/"+dialect+"/002_assets.sql")
		group := createTableBlock(t, dialect, schema, "assets_hostgroup")
		if strings.Contains(group, "parent_id") || strings.Contains(group, "parent_fk") {
			t.Fatalf("%s host groups must have no hierarchy", dialect)
		}
		up := sqlCodeLines(readRepoFile(t, "db/migrations/"+dialect+"/000060_flat_host_groups.up.sql"))
		if !strings.Contains(up, "DROP COLUMN parent_id") ||
			!strings.Contains(up, "sys.assets.hostgroup.max_tree_depth") {
			t.Fatalf("%s migration must remove hierarchy and its setting", dialect)
		}
		for _, forbidden := range []string{"DELETE FROM assets_", "DROP TABLE", "UPDATE assets_host", "CASCADE"} {
			if strings.Contains(up, forbidden) {
				t.Fatalf("%s migration must preserve groups and host bindings: %s", dialect, forbidden)
			}
		}
	}
	mysql := sqlCodeLines(readRepoFile(t, "db/migrations/mysql/000060_flat_host_groups.up.sql"))
	for _, required := range []string{"information_schema.KEY_COLUMN_USAGE", "CONSTRAINT_SCHEMA = DATABASE()",
		"TABLE_NAME = 'assets_hostgroup'", "COLUMN_NAME = 'parent_id'", "REPLACE(@hostgroup_parent_fk", "DROP FOREIGN KEY"} {
		if !strings.Contains(mysql, required) {
			t.Fatalf("MySQL must discover and escape the actual foreign-key name: missing %s", required)
		}
	}
}
