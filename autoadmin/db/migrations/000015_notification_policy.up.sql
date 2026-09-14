-- 通知策略树（对齐 Grafana notification policy）：
-- - 根节点 parent_id IS NULL，matchers=[] 恒命中，是唯一不可删除节点；
-- - matchers JSON：[{"type":"label","label":"severity","operator":"=","value":"critical"},
--   {"type":"tree","node_type":"business","id":3}]，节点内条件 AND；
-- - media_ids JSON：NULL=继承父节点出口，[]=显式不通知（静音）；
-- - 路由：从根向下，每层取 position/id 顺序下第一条命中的子策略，最深命中节点决定出口。
CREATE TABLE IF NOT EXISTS monitor_notification_policy (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `parent_id` bigint DEFAULT NULL,
  `name` varchar(128) NOT NULL,
  `position` int NOT NULL DEFAULT 0,
  `matchers` json NOT NULL,
  `media_ids` json DEFAULT NULL,
  `notify_on_firing` BOOLEAN NOT NULL DEFAULT TRUE,
  `notify_on_resolved` BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (`id`),
  KEY `idx_notification_policy_parent` (`parent_id`),
  CONSTRAINT `fk_notification_policy_parent` FOREIGN KEY (`parent_id`) REFERENCES `monitor_notification_policy` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO monitor_notification_policy(create_time,update_time,remark,parent_id,name,position,matchers,media_ids,notify_on_firing,notify_on_resolved)
SELECT NOW(6), NOW(6), '默认通知策略（匹配一切告警）', NULL, '默认策略', 0, '[]', '[]', TRUE, TRUE
WHERE NOT EXISTS (SELECT 1 FROM monitor_notification_policy WHERE parent_id IS NULL);

-- 策略树取代旧告警路由（monitor_alert_route / monitor_alert_route_media）；
-- 用户绑定退化为纯收件配置，订阅范围列一并移除。
DROP TABLE IF EXISTS `monitor_alert_route_media`;
DROP TABLE IF EXISTS `monitor_alert_route`;

ALTER TABLE `monitor_user_alert_media_binding` DROP COLUMN `scope`;

-- 菜单适配：旧「告警路由」菜单/按钮改为「通知策略」页。
UPDATE sys_menu SET name='通知策略', path='/monitor/notification-policies', component='monitor/notification-policies/index', update_time=NOW(6)
WHERE path LIKE '%alert-routes%' OR component LIKE '%alert-routes%';
