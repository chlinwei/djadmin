ALTER TABLE scheduler_scheduledtask
  ADD COLUMN schedule_timezone varchar(64) NOT NULL DEFAULT 'UTC';

-- The old next_run_time was an unmaintained display snapshot, not a trusted cursor.
UPDATE scheduler_scheduledtask SET next_run_time = NULL, is_running = FALSE;

CREATE TABLE scheduler_execution (
  id varchar(36) NOT NULL PRIMARY KEY,
  task_id bigint NOT NULL,
  trigger_type varchar(16) NOT NULL,
  status varchar(16) NOT NULL,
  scheduled_at timestamp(6) NOT NULL,
  created_at timestamp(6) NOT NULL,
  finished_at timestamp(6) DEFAULT NULL,
  CONSTRAINT scheduler_execution_task_fk FOREIGN KEY (task_id) REFERENCES scheduler_scheduledtask (id)
);
CREATE INDEX scheduler_execution_pending ON scheduler_execution (task_id, status, created_at);
CREATE INDEX scheduler_execution_latest ON scheduler_execution (task_id, created_at, id);
