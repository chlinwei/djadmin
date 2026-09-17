-- OpenSearch → Elasticsearch：日志存储集群表更名，功能与列不变。
-- 表名从 monitor_opensearch_cluster 改为 monitor_elasticsearch_cluster，与 db/schema 对齐。
RENAME TABLE `monitor_opensearch_cluster` TO `monitor_elasticsearch_cluster`;
