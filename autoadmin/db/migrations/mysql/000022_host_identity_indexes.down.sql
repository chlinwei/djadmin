-- 回滚主机身份键索引。加索引不改数据，回滚是纯结构变更。
-- 注意：回滚后 instance_name 的唯一性重新只由服务层保证（存在并发竞态与
-- 重复导致握手 UPDATE 命中多行的风险），不要长期停留在回滚态。

ALTER TABLE `assets_host` DROP KEY `assets_host_ip_idx`;
ALTER TABLE `assets_host` DROP KEY `assets_host_instance_name_uniq`;
