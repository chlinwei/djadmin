# Schema baseline

This directory is the schema input for sqlc. Do not hand-copy Django models into approximate DDL.

Before the first generated query package:

1. Apply every Django migration to the migration database.
2. Export the resulting MySQL schema without data.
3. Normalize the export into versioned SQL files in this directory.
4. Compare table names, columns, indexes, foreign keys and defaults against `docs/BACKEND_MODULE_MAP.md`.
5. Run `make generate` and commit the generated Go package.

The Go rewrite must initially use the existing tables in place. Renaming tables or columns belongs to a later, explicit migration.