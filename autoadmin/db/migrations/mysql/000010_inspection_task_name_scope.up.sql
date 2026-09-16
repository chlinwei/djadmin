-- 任务名称唯一性从全局收窄为"同巡检组内唯一"：
-- 任务列表/执行记录都会展示所属组，歧义只存在于同组内；不同组允许复用通用名（如"日巡检"）。
ALTER TABLE `inspection_task` DROP INDEX `name`;
