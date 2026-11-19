DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE remark = 'migration:000066:security_issues');
DELETE FROM sys_menu WHERE remark = 'migration:000066:security_issues';
DROP TABLE security_issue_history;
DROP TABLE security_issue;
DROP INDEX baseline_result_rule;
ALTER TABLE baseline_scan_result DROP COLUMN rule_key;
