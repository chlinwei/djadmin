-- PostgreSQL removes the column's own foreign key and index automatically.
-- No CASCADE: unrelated dependencies must not be silently removed.
ALTER TABLE assets_hostgroup DROP COLUMN parent_id;
DELETE FROM sys_config WHERE key = 'sys.assets.hostgroup.max_tree_depth';
