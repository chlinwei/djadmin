ALTER TABLE inspection_execution ADD COLUMN dispatch_snapshot jsonb;
CREATE INDEX inspection_execution_dispatch_idx ON inspection_execution (status, id);
ALTER TABLE security_scan ADD COLUMN dispatch_snapshot jsonb;
CREATE INDEX security_scan_dispatch_idx ON security_scan (scan_type, status, id);
