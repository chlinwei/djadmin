ALTER TABLE scheduler_scheduledtask
  ADD COLUMN schedule_timezone varchar(64) NOT NULL DEFAULT 'UTC';

-- The old next_run_time was an unmaintained display snapshot, not a trusted cursor.
UPDATE scheduler_scheduledtask SET next_run_time = NULL, is_running = FALSE;

CREATE TABLE scheduler_execution (
  `id` varchar(36) NOT NULL PRIMARY KEY,
  `task_id` bigint NOT NULL,
  `trigger_type` varchar(16) NOT NULL,
  `status` varchar(16) NOT NULL,
  `scheduled_at` datetime(6) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `finished_at` datetime(6) DEFAULT NULL,
  KEY scheduler_execution_pending (task_id, status, created_at),
  KEY scheduler_execution_latest (task_id, created_at, id),
  CONSTRAINT scheduler_execution_task_fk FOREIGN KEY (task_id) REFERENCES scheduler_scheduledtask (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
