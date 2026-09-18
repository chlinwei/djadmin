-- 作业实时输出：运行期间的 ansible 输出按块追加，供「查看日志」在作业跑完之前就能看到进度。
-- 此前输出只在作业结束后才逐台落 automation_execution_host_log，"查看日志"在运行期间一直显示
-- "等待新输出"（多主机长作业尤其明显）。
--
-- 生命周期刻意做得很短：作业结束时由执行方删除本作业的块，改读按主机的结果行——
-- 既不重复展示同一份 ansible 输出，也没有留存/清理负担。
CREATE TABLE `automation_execution_job_log` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `remark` longtext,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `job_id` bigint NOT NULL,
  `content` longtext NOT NULL,
  PRIMARY KEY (`id`),
  KEY `automation_execution_job_log_job_id_7f1c2a94` (`job_id`),
  CONSTRAINT `automation_execution_job_log_job_id_7f1c2a94_fk_automatio` FOREIGN KEY (`job_id`) REFERENCES `automation_execution_job` (`id`)
);
