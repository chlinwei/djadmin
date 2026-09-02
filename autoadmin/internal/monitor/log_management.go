package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// OpenSearch 日志存储管理面：索引命名、index template、ISM policy、ingest pipeline
// 的期望态构建与下发编排，对应 Django monitor/log_management.py。
// 构建器全部为纯函数便于单测；bootstrapOpenSearchStorage 是唯一执行写入的编排入口。

var indexSegmentPattern = regexp.MustCompile(`[^a-z0-9_-]+`)

// safeIndexSegment 索引名段只允许小写字母/数字/下划线/连字符，避免非法索引名。
func safeIndexSegment(value string) string {
	segment := strings.Trim(indexSegmentPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-"), "-")
	if segment == "" {
		return "unknown"
	}
	return segment
}

func buildIndexTemplateName(indexPrefix string) string {
	return safeIndexSegment(indexPrefix) + "-template"
}

// buildIndexTemplateBody data stream 模板：单分片、限制字段总数、标准字段之外不再自动建 mapping。
// 字段契约与 Django log_schema.STANDARD_LOG_FIELDS 一致。
func buildIndexTemplateBody(indexPrefix string) gin.H {
	prefix := safeIndexSegment(indexPrefix)
	return gin.H{
		"index_patterns": []string{prefix + "-*"},
		"data_stream":    gin.H{},
		"template": gin.H{
			"settings": gin.H{
				"number_of_shards":                 1,
				"number_of_replicas":               0,
				"index.refresh_interval":           "10s",
				"index.mapping.total_fields.limit": 2000,
			},
			"mappings": gin.H{
				"dynamic":    false,
				"properties": standardLogFields,
			},
		},
	}
}

var standardLogFields = gin.H{
	"@timestamp":        gin.H{"type": "date"},
	"message":           gin.H{"type": "text"},
	"project":           gin.H{"type": "keyword"},
	"business_system":   gin.H{"type": "keyword"},
	"environment":       gin.H{"type": "keyword"},
	"service":           gin.H{"type": "keyword"},
	"application":       gin.H{"type": "keyword"},
	"instance":          gin.H{"type": "keyword"},
	"host_ip":           gin.H{"type": "keyword"},
	"log_name":          gin.H{"type": "keyword"},
	"log_path":          gin.H{"type": "keyword"},
	"log_level":         gin.H{"type": "keyword"},
	"log_time":          gin.H{"type": "keyword"},
	"log_message":       gin.H{"type": "text"},
	"error_fingerprint": gin.H{"type": "keyword"},
	"app_fields":        gin.H{"type": "flat_object"},
}

func buildISMPolicyName(indexPrefix, tierCode string) string {
	return safeIndexSegment(indexPrefix) + "-" + safeIndexSegment(tierCode) + "-retention"
}

type retentionTierRow struct {
	Code                 string
	RetentionDays        int64
	DailySizeGB          float64
	RolloverMinIndexAge  string
}

// buildISMPolicyBody 按档位生成 ISM policy，经 ism_template 按索引名后缀自动挂载。
// 滚动阈值与 Django 一致：单分片不小于 1gb，避免小档位产生大量碎索引。
func buildISMPolicyBody(indexPrefix string, tier retentionTierRow) gin.H {
	prefix := safeIndexSegment(indexPrefix)
	code := safeIndexSegment(tier.Code)
	rolloverSize := fmt.Sprintf("%dgb", int(math.Max(1, math.Round(tier.DailySizeGB))))
	estimatedTotal := math.Round(tier.DailySizeGB*float64(tier.RetentionDays)*100) / 100
	return gin.H{"policy": gin.H{
		"description": fmt.Sprintf("%s *-%s 索引保留 %d 天（预估 %.0fGB/天，合计 %.2fGB）",
			prefix, code, tier.RetentionDays, tier.DailySizeGB, estimatedTotal),
		"default_state": "hot",
		"states": []any{
			gin.H{
				"name": "hot",
				"actions": []any{gin.H{"rollover": gin.H{
					"min_primary_shard_size": rolloverSize,
					"min_index_age":          tier.RolloverMinIndexAge,
				}}},
				"transitions": []any{gin.H{
					"state_name": "delete",
					"conditions": gin.H{"min_index_age": fmt.Sprintf("%dd", tier.RetentionDays)},
				}},
			},
			gin.H{"name": "delete", "actions": []any{gin.H{"delete": gin.H{}}}},
		},
		"ism_template": []any{gin.H{
			"index_patterns": []any{fmt.Sprintf("%s-*-%s", prefix, code)},
			"priority":       100,
		}},
	}}
}

// pipelineSignature 只取行为字段：processors 有序直接序列化，描述等非行为字段不参与比对。
// encoding/json 对 map 按 key 排序输出，与 Django json.dumps(sort_keys=True) 一致。
func pipelineSignature(body any) string {
	var decoded map[string]any
	switch typed := body.(type) {
	case map[string]any:
		decoded = typed
	case gin.H:
		decoded = typed
	case json.RawMessage:
		_ = json.Unmarshal(typed, &decoded)
	case []byte:
		_ = json.Unmarshal(typed, &decoded)
	}
	processors, onFailure := []any{}, []any{}
	if decoded != nil {
		if items, ok := decoded["processors"].([]any); ok {
			processors = items
		}
		if items, ok := decoded["on_failure"].([]any); ok {
			onFailure = items
		}
	}
	encoded, _ := json.Marshal(gin.H{"processors": processors, "on_failure": onFailure})
	return string(encoded)
}

// bootstrapOpenSearchStorage 确保 index template 与各保留档位的 ISM policy 存在（幂等）。
func (handler *Handler) bootstrapOpenSearchStorage(context *gin.Context, cluster openSearchCluster) error {
	prefix := cluster.IndexPrefix
	if strings.TrimSpace(prefix) == "" {
		prefix = "logs"
	}
	if _, err := handler.openSearchRequest(context, cluster, "PUT", "/_index_template/"+safeIndexSegment(prefix), buildIndexTemplateBody(prefix)); err != nil {
		return err
	}
	tiers, err := handler.loadEnabledRetentionTiers(context)
	if err != nil {
		return err
	}
	for _, tier := range tiers {
		name := buildISMPolicyName(prefix, tier.Code)
		body := buildISMPolicyBody(prefix, tier)
		// 覆盖已存在的 ISM policy 必须带乐观锁参数，否则 OpenSearch 返回 409。
		path := "/_plugins/_ism/policies/" + name
		if existing, getErr := handler.openSearchRequest(context, cluster, "GET", path, nil); getErr == nil {
			seqNo, primaryTerm := existing["_seq_no"], existing["_primary_term"]
			if seqNo != nil && primaryTerm != nil {
				path = fmt.Sprintf("%s?if_seq_no=%v&if_primary_term=%v", path, seqNo, primaryTerm)
			}
		}
		if _, err := handler.openSearchRequest(context, cluster, "PUT", path, body); err != nil {
			return err
		}
	}
	return nil
}

func (handler *Handler) loadEnabledRetentionTiers(context *gin.Context) ([]retentionTierRow, error) {
	rows, err := handler.db.QueryContext(context, `SELECT code,retention_days,daily_size_gb,rollover_min_index_age FROM monitor_log_retention_tier WHERE enabled=TRUE ORDER BY retention_days,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tiers := make([]retentionTierRow, 0, 4)
	for rows.Next() {
		var tier retentionTierRow
		if err := rows.Scan(&tier.Code, &tier.RetentionDays, &tier.DailySizeGB, &tier.RolloverMinIndexAge); err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}
	return tiers, rows.Err()
}

// syncClusterLogStorage 对单个启用集群下发模板与保留策略，并回写 storage_sync_* 状态，
// 对应 Django 的 celery 任务 sync_log_storage（失败不抛出，只落状态供前端展示）。
func (handler *Handler) syncClusterLogStorage(clusterID int64) {
	ginContext, _ := gin.CreateTestContext(nil)
	if _, err := handler.db.ExecContext(ginContext, `UPDATE monitor_opensearch_cluster SET storage_sync_status='pending',storage_sync_error='',storage_sync_time=NULL,update_time=? WHERE id=?`, time.Now().UTC(), clusterID); err != nil {
		return
	}
	cluster, err := handler.loadOpenSearchClusterByID(ginContext, clusterID)
	switch {
	case err != nil:
		_, _ = handler.db.ExecContext(ginContext, `UPDATE monitor_opensearch_cluster SET storage_sync_status='failed',storage_sync_error=?,storage_sync_time=?,update_time=? WHERE id=?`, err.Error(), time.Now().UTC(), time.Now().UTC(), clusterID)
	case !cluster.Enabled:
		_, _ = handler.db.ExecContext(ginContext, `UPDATE monitor_opensearch_cluster SET storage_sync_status='failed',storage_sync_error='集群未启用，跳过同步',storage_sync_time=?,update_time=? WHERE id=?`, time.Now().UTC(), time.Now().UTC(), clusterID)
	default:
		if bootstrapErr := handler.bootstrapOpenSearchStorage(ginContext, cluster); bootstrapErr != nil {
			_, _ = handler.db.ExecContext(ginContext, `UPDATE monitor_opensearch_cluster SET storage_sync_status='failed',storage_sync_error=?,storage_sync_time=?,update_time=? WHERE id=?`, truncateOpenSearchError(bootstrapErr), time.Now().UTC(), time.Now().UTC(), clusterID)
		} else {
			_, _ = handler.db.ExecContext(ginContext, `UPDATE monitor_opensearch_cluster SET storage_sync_status='success',storage_sync_error='',storage_sync_time=?,update_time=? WHERE id=?`, time.Now().UTC(), time.Now().UTC(), clusterID)
		}
	}
}

// syncAllClusterLogStorage 对所有启用集群异步下发，对应 Django 档位改动后的 _apply_policies。
func (handler *Handler) syncAllClusterLogStorage() {
	ginContext, _ := gin.CreateTestContext(nil)
	rows, err := handler.db.QueryContext(ginContext, `SELECT id FROM monitor_opensearch_cluster WHERE enabled=TRUE`)
	if err != nil {
		return
	}
	ids := make([]int64, 0, 2)
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		handler.syncClusterLogStorage(id)
	}
}

func (handler *Handler) loadOpenSearchClusterByID(context *gin.Context, id int64) (openSearchCluster, error) {
	var cluster openSearchCluster
	err := handler.db.QueryRowContext(context, `SELECT id,hosts,username,password,verify_tls,ca_cert,index_prefix,request_timeout,enabled FROM monitor_opensearch_cluster WHERE id=?`, id).Scan(&cluster.ID, &cluster.Hosts, &cluster.Username, &cluster.Password, &cluster.VerifyTLS, &cluster.CACert, &cluster.IndexPrefix, &cluster.Timeout, &cluster.Enabled)
	if err != nil {
		return cluster, err
	}
	cluster.Password, err = handler.secrets.Decrypt(cluster.Password)
	return cluster, err
}

func truncateOpenSearchError(err error) string {
	message := err.Error()
	if len(message) > 500 {
		return message[:500]
	}
	return message
}

// publishProcessingPipeline 把解析规则的 pipeline 发布到集群（Django 规则增改的同步行为）。
func (handler *Handler) publishProcessingPipeline(context *gin.Context, clusterID int64, name string, pipelineBody any) error {
	cluster, err := handler.loadOpenSearchClusterByID(context, clusterID)
	if err != nil {
		return err
	}
	if !cluster.Enabled {
		return fmt.Errorf("OpenSearch 集群未启用")
	}
	if _, err := handler.openSearchRequest(context, cluster, "PUT", "/_ingest/pipeline/"+name, pipelineBody); err != nil {
		return err
	}
	return nil
}

func (handler *Handler) deleteProcessingPipeline(context *gin.Context, clusterID int64, name string) error {
	cluster, err := handler.loadOpenSearchClusterByID(context, clusterID)
	if err != nil {
		return err
	}
	if !cluster.Enabled {
		return nil
	}
	if _, err := handler.openSearchRequest(context, cluster, "DELETE", "/_ingest/pipeline/"+name, nil); err != nil {
		return err
	}
	return nil
}

