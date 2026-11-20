ALTER TABLE security_scan DROP INDEX security_scan_dispatch_idx, DROP COLUMN dispatch_snapshot;
ALTER TABLE inspection_execution DROP INDEX inspection_execution_dispatch_idx, DROP COLUMN dispatch_snapshot;
