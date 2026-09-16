-- 通知策略出口增加用户组维度：策略命中后仅投递出口用户组成员的媒介绑定。
-- user_group_ids NULL = 继承父节点的用户组限制；[] = 显式不限组（该媒介上的全部绑定都能收）；
-- [id...] = 仅这些组的成员可收。与 media_ids 的继承语义一致（沿路径取最后一个显式设置）。
-- 注意与 media_ids 相反的默认：根节点 NULL 时按「不限组」处理，保持"一媒介一绑定"用法的直觉。
--
-- PG 侧差异：JSON NULL → jsonb。
ALTER TABLE monitor_notification_policy ADD COLUMN user_group_ids jsonb NULL;
