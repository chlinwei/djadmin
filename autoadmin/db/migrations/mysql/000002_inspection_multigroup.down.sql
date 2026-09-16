ALTER TABLE `inspection_result`
  DROP COLUMN `group_id`,
  DROP COLUMN `group_name`;

DELETE tg FROM `inspection_task_group` tg;

DROP TABLE `inspection_task_group`;

ALTER TABLE `inspection_group`
  DROP COLUMN `application_id`,
  DROP COLUMN `category`;
