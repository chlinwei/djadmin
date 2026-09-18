DROP INDEX monitor_hist_auto_job_id_idx ON monitor_target_install_history;

ALTER TABLE monitor_target_install_history
  DROP COLUMN automation_job_id_snapshot;

ALTER TABLE automation_execution_job
  DROP COLUMN source;
