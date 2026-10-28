-- Do not restore an integer-only check on an expression column. Migration 56
-- validates numeric values before converting the column back on rollback.
SELECT 1;
