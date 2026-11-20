ALTER TABLE inspection_execution ADD COLUMN dispatch_snapshot JSON NULL,
    ADD INDEX inspection_execution_dispatch_idx (status, id);
ALTER TABLE security_scan ADD COLUMN dispatch_snapshot JSON NULL,
    ADD INDEX security_scan_dispatch_idx (scan_type, status, id);
