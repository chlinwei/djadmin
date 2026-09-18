-- 回滚：两张表都是新表，直接删除（先删 item，外键指向头表）。
DROP TABLE monitor_log_batch_job_item;
DROP TABLE monitor_log_batch_job;
