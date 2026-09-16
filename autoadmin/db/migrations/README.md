# Migration policy

The existing Django-managed MySQL schema is the initial baseline. Do not run a second set of create-table migrations against production.

Adoption sequence:

1. Record the latest applied Django migration set.
2. Export and checksum the resulting schema.
3. Create a no-op Go migration baseline at that version.
4. Run all later schema changes through `golang-migrate` after the corresponding Go domain owns production traffic.
5. Keep rollback SQL for reversible changes; document irreversible data migrations explicitly.

## Dialect layout

Both dialects carry the same migration set, one directory each:

| Directory | Content |
|---|---|
| `mysql/` | hand-maintained source: `000001…000022`, each with `.up.sql` / `.down.sql` (44 files). `MIGRATION_SOURCE_URL=file://db/migrations/mysql` for the default build. |
| `postgres/` | the PostgreSQL translation of the same 44 files — **identical version numbers and file names**. `MIGRATION_SOURCE_URL=file://db/migrations/postgres` once the `migrate` role registers the PG driver (still pending, SQL_DESIGN §5.1). |

Rules for the PostgreSQL set:

- Every change to `mysql/` must be translated into `postgres/` in the same commit; a one-sided change is a half-done change.
- `down` is the strict inverse of the matching `up` (drop / delete / restore exactly what the up added or changed).
- DDL follows the folded PostgreSQL schema (`db/schema/postgres`): same type names, same quoting, same constraint and index naming. PostgreSQL constraint/index names are unique **per database** (MySQL's are per table), so generic names get a table prefix (`inspection_task_name`, `monitor_alert_route_name`); reserved words used as column names must stay double-quoted (`"order"`, `"limit"`).
- Statement-level differences (no session variables → subqueries, `UPDATE ... FROM` instead of multi-table `UPDATE ... JOIN`, `ON CONFLICT DO NOTHING` for `INSERT IGNORE`, `RENAME COLUMN` instead of `CHANGE COLUMN`, booleans written as `TRUE`/`FALSE` instead of `1`/`0`, no `AFTER`, `DROP CONSTRAINT` instead of `DROP INDEX`/`DROP FOREIGN KEY`, backslashes in literals written once) are listed in the mapping table of [SQL_DESIGN §4.7](../../../docs/architecture/SQL_DESIGN.md); each affected file starts with a `-- PG 侧差异：` note.
- Strings seeded by migrations (playbook template content, menu names/paths) must be byte-identical to what MySQL stores.