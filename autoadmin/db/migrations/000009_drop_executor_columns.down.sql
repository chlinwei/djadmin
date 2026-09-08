ALTER TABLE `inspection_check`
  ADD COLUMN `executor` varchar(16) NOT NULL DEFAULT 'opa',
  ADD COLUMN `execution_location` varchar(16) NOT NULL DEFAULT 'agent';

ALTER TABLE `baseline_item`
  ADD COLUMN `executor` varchar(16) NOT NULL DEFAULT 'opa';
