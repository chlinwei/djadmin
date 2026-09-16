-- 用户-告警媒介绑定的服务树订阅范围（路线三）。
-- scope 语义：NULL = 全局订阅（收到其媒介命中的所有告警）；
-- [{"type":"service|environment|business|project","id":<bigint>}] = 仅订阅归属命中任一节点的告警（数组内 OR）。
--
-- PG 侧差异：JSON NULL → jsonb（类型映射与 db/schema/postgres 一致）。

ALTER TABLE monitor_user_alert_media_binding ADD COLUMN scope jsonb NULL;
