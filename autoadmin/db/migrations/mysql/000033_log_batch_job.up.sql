-- 日志采集批量动作的进度载体（计划 LOG_COLLECTION_LIFECYCLE §8 Phase 2）。
--
-- 背景：批量下发/批量安装原来在**单个 HTTP 请求内串行**遍历目标（每台 2 次 agent gRPC，
-- 超时 60s+120s），安装/重试则在 API 进程内联对每台起一个 `go func()`、无并发上限。
-- 500–1000 台规模下前者必然超时并留下部分下发的中间态，后者会把 agent 侧打满。
--
-- 目标形态「入队 + 有界并发 + 进度可查 + 可续跑」：
--   - 入队：API 只写作业行 + item 行，然后向 worker 队列投递一条消息；
--   - 有界并发：每个 item 由 worker 内的信号量按 concurrency 限流（安装与下发分别配置）；
--   - 进度可查：item 状态与计数落库，前端轮询作业详情；
--   - 可续跑：作业被打断（进程重启/消息重投）后只处理仍是 pending 的 item，
--     已成功的不会重跑；running 的 item 在续跑时回落为 pending。
--
-- 表拆两张：头表存动作/状态/计数（列表与轮询只读它），item 表存每台主机的结果。
-- host_name/host_ip 是**快照**：主机改名或被删后进度页仍能如实展示当时的目标。
CREATE TABLE `monitor_log_batch_job` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `action` varchar(16) NOT NULL,
  `status` varchar(16) NOT NULL,
  `total_count` int NOT NULL,
  `success_count` int NOT NULL,
  `failed_count` int NOT NULL,
  `concurrency` int NOT NULL,
  `message` longtext NOT NULL,
  `requested_user_id` bigint DEFAULT NULL,
  `requested_username` varchar(150) NOT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `finished_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`id`)
);

CREATE TABLE `monitor_log_batch_job_item` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `batch_job_id` bigint NOT NULL,
  `target_id` bigint NOT NULL,
  `host_id` bigint NOT NULL,
  `host_name` varchar(128) NOT NULL,
  `host_ip` varchar(64) NOT NULL,
  `status` varchar(16) NOT NULL,
  `message` longtext NOT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `finished_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `monitor_log_batch_job_item_batch_job_id_idx` (`batch_job_id`),
  CONSTRAINT `monitor_log_batch_job_item_batch_job_id_fk` FOREIGN KEY (`batch_job_id`) REFERENCES `monitor_log_batch_job` (`id`)
);
