-- 日志处理规则保存"在线调试"用的原始日志样例，便于下次打开编辑器直接复用。
ALTER TABLE monitor_log_processing_rule ADD COLUMN sample_log text NOT NULL DEFAULT '';
