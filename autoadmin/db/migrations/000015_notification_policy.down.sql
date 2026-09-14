-- 回滚 000015：恢复旧路由表与绑定 scope 列（数据不可恢复，仅恢复结构）。
ALTER TABLE `monitor_user_alert_media_binding` ADD COLUMN `scope` JSON NULL;

DROP TABLE IF EXISTS `monitor_notification_policy`;

CREATE TABLE `monitor_alert_route` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `matchers` json NOT NULL,
  `notify_on_firing` BOOLEAN NOT NULL,
  `notify_on_resolved` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

CREATE TABLE `monitor_alert_route_media` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `alertroute_id` bigint NOT NULL,
  `alertmedia_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `alertroute_alertmedia` (`alertroute_id`, `alertmedia_id`)
);
