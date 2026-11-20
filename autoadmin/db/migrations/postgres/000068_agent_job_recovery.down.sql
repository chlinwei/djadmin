DROP INDEX security_scan_dispatch_idx;
ALTER TABLE security_scan DROP COLUMN dispatch_snapshot;
DROP INDEX inspection_execution_dispatch_idx;
ALTER TABLE inspection_execution DROP COLUMN dispatch_snapshot;
