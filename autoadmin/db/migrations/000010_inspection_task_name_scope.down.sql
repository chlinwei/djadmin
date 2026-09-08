-- 回滚恢复全局唯一。注意：若已存在同名跨组任务，需先手工去重再执行。
ALTER TABLE `inspection_task` ADD UNIQUE KEY `name` (`name`);
