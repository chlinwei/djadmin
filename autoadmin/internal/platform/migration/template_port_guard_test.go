package migration

import (
	"strings"
	"testing"
)

func TestTemplatePortMigrationRemovesOnlyLegacyNumericCheck(t *testing.T) {
	mysql := sqlCodeLines(readRepoFile(t, "db/migrations/mysql/000057_template_port_legacy_check.up.sql"))
	for _, required := range []string{
		"information_schema.CHECK_CONSTRAINTS", "cc.CONSTRAINT_SCHEMA = DATABASE()",
		"tc.TABLE_NAME = 'assets_application_port'", "tc.CONSTRAINT_TYPE = 'CHECK'",
		"IN ('port>=0', '0<=port')", "'SELECT 1'", "PREPARE", "DEALLOCATE PREPARE",
	} {
		if !strings.Contains(mysql, required) {
			t.Fatalf("legacy-check migration lacks %q", required)
		}
	}
	if hardcodedConstraintName.MatchString(mysql) {
		t.Fatal("generated check names must be discovered, not hardcoded")
	}
	pg := sqlCodeLines(readRepoFile(t, "db/migrations/postgres/000057_template_port_legacy_check.up.sql"))
	for _, required := range []string{
		"'assets_application_port'::regclass", "con.contype = 'c'",
		"pg_get_expr", "IN ('port>=0', '0<=port')", "DROP CONSTRAINT %I",
	} {
		if !strings.Contains(pg, required) {
			t.Fatalf("PostgreSQL legacy-check migration lacks %q", required)
		}
	}
	for _, dialect := range []string{"mysql", "postgres"} {
		schema := readRepoFile(t, "db/schema/"+dialect+"/002_assets.sql")
		block := createTableBlock(t, dialect, schema, "assets_application_port")
		if !strings.Contains(block, "varchar(64)") || strings.Contains(block, "CHECK") {
			t.Fatalf("%s snapshot must preserve expression ports without the obsolete check", dialect)
		}
	}
}
