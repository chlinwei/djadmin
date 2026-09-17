-- 回滚：monitor_elasticsearch_cluster → monitor_opensearch_cluster（仅表名）。
RENAME TABLE `monitor_elasticsearch_cluster` TO `monitor_opensearch_cluster`;
