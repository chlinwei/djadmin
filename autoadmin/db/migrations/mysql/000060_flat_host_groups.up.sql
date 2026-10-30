-- Historical installations use different names for the parent foreign key.
-- Drop the actual constraint before its column; group IDs and host bindings stay.
SET @hostgroup_parent_fk = (
  SELECT CONSTRAINT_NAME FROM information_schema.KEY_COLUMN_USAGE
  WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'assets_hostgroup'
    AND COLUMN_NAME = 'parent_id' AND REFERENCED_TABLE_NAME IS NOT NULL
  LIMIT 1
);
SET @drop_hostgroup_parent_fk = IF(
  @hostgroup_parent_fk IS NULL,
  'SELECT 1',
  CONCAT('ALTER TABLE assets_hostgroup DROP FOREIGN KEY `', REPLACE(@hostgroup_parent_fk, '`', '``'), '`')
);
PREPARE drop_hostgroup_parent_fk FROM @drop_hostgroup_parent_fk;
EXECUTE drop_hostgroup_parent_fk;
DEALLOCATE PREPARE drop_hostgroup_parent_fk;
ALTER TABLE assets_hostgroup DROP COLUMN parent_id;
DELETE FROM sys_config WHERE `key` = 'sys.assets.hostgroup.max_tree_depth';
