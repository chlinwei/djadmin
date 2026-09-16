-- 补齐主机身份键的索引：instance_name 加唯一索引、ip 加普通索引。
--
-- 背景：主机身份从 agent_id 改为 instance_name 时，删掉了 assets_host.agent_id 的
-- UNIQUE KEY，但没有给 instance_name 补上等价约束，导致两个问题：
--
-- 1) 正确性：instance_name 是 agent 的全局标识，唯一性此前只在服务层
--    （checkInstanceNameUnique 的 check-then-insert）保证，并发创建存在竞态；
--    且一旦出现重复，握手落库的
--      UPDATE assets_host SET agent_online=TRUE ... WHERE instance_name=?
--    会一次命中多行，把两台主机的在线状态一起改掉。
-- 2) 性能：instance_name 与 ip 都没有任何索引，上述 UPDATE 与
--    HostIPExists / InstanceNameExists 的 COUNT(*) 校验、以及主机的
--    instance_name / ip 模糊搜索全部退化为全表扫描。19 行时无感，
--    但这几个操作在每次 agent 握手、每次主机增改时都会执行。
--
-- 执行前校验（本次已在生产库确认）：两列均无 NULL、无空串、无重复值，
-- 故可直接建唯一索引。若其他环境存在重复，需先人工合并再执行本迁移。
--
-- PG 侧差异：MySQL 的 ADD UNIQUE KEY / ADD KEY 在 PG 分别是 ADD CONSTRAINT ... UNIQUE
-- 与独立的 CREATE INDEX（名字沿用 db/schema/postgres：assets_host_instance_name_uniq / assets_host_ip_idx）。

ALTER TABLE assets_host
  ADD CONSTRAINT assets_host_instance_name_uniq UNIQUE (instance_name);

CREATE INDEX assets_host_ip_idx ON assets_host (ip);
