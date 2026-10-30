-- Restores the column, not the discarded hierarchy or retired depth setting.
ALTER TABLE assets_hostgroup
  ADD COLUMN parent_id bigint DEFAULT NULL,
  ADD CONSTRAINT assets_hostgroup_parent_fk FOREIGN KEY (parent_id) REFERENCES assets_hostgroup (id);
CREATE INDEX assets_hostgroup_parent_id_idx ON assets_hostgroup (parent_id);
