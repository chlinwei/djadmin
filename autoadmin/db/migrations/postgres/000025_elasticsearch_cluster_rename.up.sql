-- OpenSearch → Elasticsearch：日志存储集群表更名，功能与列不变。
-- 表名与唯一约束名同步改成 db/schema/postgres 里的命名。
ALTER TABLE monitor_opensearch_cluster RENAME TO monitor_elasticsearch_cluster;
ALTER TABLE monitor_elasticsearch_cluster
  RENAME CONSTRAINT monitor_opensearch_cluster_name TO monitor_elasticsearch_cluster_name;
