package monitor

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

type openSearchCluster struct {
	ID                                             int64
	Hosts, Username, Password, CACert, IndexPrefix string
	VerifyTLS                                      bool
	Timeout                                        int
}

func (handler *Handler) loadOpenSearchCluster(context *gin.Context) (openSearchCluster, error) {
	var cluster openSearchCluster
	err := handler.db.QueryRowContext(context, `SELECT id,hosts,username,password,verify_tls,ca_cert,index_prefix,request_timeout FROM monitor_opensearch_cluster WHERE id=?`, parseID(context.Param("id"))).Scan(&cluster.ID, &cluster.Hosts, &cluster.Username, &cluster.Password, &cluster.VerifyTLS, &cluster.CACert, &cluster.IndexPrefix, &cluster.Timeout)
	if err != nil {
		return cluster, err
	}
	cluster.Password, err = handler.secrets.Decrypt(cluster.Password)
	return cluster, err
}

func (handler *Handler) openSearchRequest(context *gin.Context, cluster openSearchCluster, method, path string, body any) (map[string]any, error) {
	var rawBody []byte
	var err error
	if body != nil {
		rawBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if !cluster.VerifyTLS {
		tlsConfig.InsecureSkipVerify = true
	} // Explicit cluster setting for self-signed deployments.
	if cluster.VerifyTLS && strings.TrimSpace(cluster.CACert) != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cluster.CACert)) {
			return nil, fmt.Errorf("invalid CA certificate")
		}
		tlsConfig.RootCAs = pool
	}
	timeout := cluster.Timeout
	if timeout < 1 {
		timeout = 10
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig}}
	var lastError error
	for _, host := range strings.Split(cluster.Hosts, ",") {
		host = strings.TrimRight(strings.TrimSpace(host), "/")
		if host == "" {
			continue
		}
		request, requestErr := http.NewRequestWithContext(context, method, host+"/"+strings.TrimLeft(path, "/"), bytes.NewReader(rawBody))
		if requestErr != nil {
			return nil, requestErr
		}
		request.Header.Set("Content-Type", "application/json")
		if cluster.Username != "" {
			request.SetBasicAuth(cluster.Username, cluster.Password)
		}
		upstream, requestErr := client.Do(request)
		if requestErr != nil {
			lastError = requestErr
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(upstream.Body, 16<<20))
		upstream.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
			return nil, fmt.Errorf("opensearch %s: %s", upstream.Status, strings.TrimSpace(string(payload)))
		}
		if len(payload) == 0 {
			return map[string]any{}, nil
		}
		var result map[string]any
		if err = json.Unmarshal(payload, &result); err != nil {
			return nil, fmt.Errorf("invalid OpenSearch JSON: %w", err)
		}
		return result, nil
	}
	if lastError == nil {
		lastError = fmt.Errorf("no OpenSearch hosts configured")
	}
	return nil, lastError
}

func (handler *Handler) TestOpenSearchConnection(context *gin.Context) {
	cluster, err := handler.loadOpenSearchCluster(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	info, err := handler.openSearchRequest(context, cluster, http.MethodGet, "/", nil)
	now := time.Now().UTC()
	if err != nil {
		_, _ = handler.db.ExecContext(context, `UPDATE monitor_opensearch_cluster SET last_check_time=?,last_check_success=FALSE,last_check_message=?,update_time=? WHERE id=?`, now, err.Error(), now, cluster.ID)
		response.BusinessError(context, 400, "connection failed: "+err.Error(), nil)
		return
	}
	health, err := handler.openSearchRequest(context, cluster, http.MethodGet, "/_cluster/health", nil)
	if err != nil {
		response.BusinessError(context, 400, "connection failed: "+err.Error(), nil)
		return
	}
	version, _ := info["version"].(map[string]any)
	result := gin.H{"cluster_name": info["cluster_name"], "distribution": version["distribution"], "version": version["number"], "status": health["status"], "number_of_nodes": health["number_of_nodes"]}
	message := fmt.Sprintf("%v %v / %v / %v", result["distribution"], result["version"], result["cluster_name"], result["status"])
	_, _ = handler.db.ExecContext(context, `UPDATE monitor_opensearch_cluster SET last_check_time=?,last_check_success=TRUE,last_check_message=?,update_time=? WHERE id=?`, now, message, now, cluster.ID)
	response.Success(context, result)
}

func parseLogWindow(context *gin.Context) (time.Time, time.Time, error) {
	end := time.Now().UTC()
	var err error
	if raw := strings.TrimSpace(context.Query("end")); raw != "" {
		end, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("end must be ISO8601")
		}
	}
	start := end.Add(-time.Hour)
	if raw := strings.TrimSpace(context.Query("start")); raw != "" {
		start, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("start must be ISO8601")
		}
	}
	if !start.Before(end) {
		return start, end, fmt.Errorf("start must be before end")
	}
	if end.Sub(start) > 30*24*time.Hour {
		return start, end, fmt.Errorf("time range cannot exceed 30 days")
	}
	return start, end, nil
}

func (handler *Handler) buildLogQuery(context *gin.Context) (openSearchCluster, string, []any, error) {
	cluster, err := handler.loadOpenSearchCluster(context)
	if err != nil {
		return cluster, "", nil, err
	}
	serviceID := parseID(context.Query("application_service_id"))
	if serviceID == 0 {
		return cluster, "", nil, fmt.Errorf("application_service_id is required")
	}
	var serviceCode string
	if err = handler.db.QueryRowContext(context, `SELECT code FROM assets_application_service WHERE id=?`, serviceID).Scan(&serviceCode); err != nil {
		return cluster, "", nil, fmt.Errorf("application service not found")
	}
	start, end, err := parseLogWindow(context)
	if err != nil {
		return cluster, "", nil, err
	}
	filters := []any{gin.H{"term": gin.H{"service": serviceCode}}, gin.H{"range": gin.H{"@timestamp": gin.H{"gte": start.Format(time.RFC3339Nano), "lte": end.Format(time.RFC3339Nano)}}}}
	for _, field := range []string{"instance", "host_ip", "log_name", "error_fingerprint"} {
		if value := strings.TrimSpace(context.Query(field)); value != "" {
			filters = append(filters, gin.H{"term": gin.H{field: value}})
		}
	}
	if levels := strings.TrimSpace(context.Query("log_level")); levels != "" {
		filters = append(filters, gin.H{"terms": gin.H{"log_level": strings.Split(levels, ",")}})
	}
	must := []any{gin.H{"match_all": gin.H{}}}
	if keyword := strings.TrimSpace(context.Query("keyword")); keyword != "" {
		if len(keyword) > 500 {
			keyword = keyword[:500]
		}
		must = []any{gin.H{"match": gin.H{"log_message": keyword}}}
	}
	return cluster, serviceCode, []any{filters, must}, nil
}

func (handler *Handler) OpenSearchLogSearch(context *gin.Context) {
	cluster, _, parts, err := handler.buildLogQuery(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	size := queryInt(context, "size", 100)
	if size < 1 {
		size = 1
	}
	if size > 200 {
		size = 200
	}
	offset := queryInt(context, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	if offset+size > 2000 {
		response.BusinessError(context, 400, "offset+size cannot exceed 2000", nil)
		return
	}
	body := gin.H{"from": offset, "size": size, "track_total_hits": 2000, "sort": []any{gin.H{"@timestamp": "desc"}}, "_source": []string{"@timestamp", "log_level", "service", "instance", "host_ip", "log_name", "log_path", "log_message", "error_fingerprint", "app_fields"}, "query": gin.H{"bool": gin.H{"filter": parts[0], "must": parts[1]}}}
	data, err := handler.openSearchRequest(context, cluster, http.MethodPost, "/"+url.PathEscape(defaultString(cluster.IndexPrefix, "logs"))+"-*/_search", body)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	hits, _ := data["hits"].(map[string]any)
	rawHits, _ := hits["hits"].([]any)
	results := make([]gin.H, 0, len(rawHits))
	for _, raw := range rawHits {
		hit, _ := raw.(map[string]any)
		source, _ := hit["_source"].(map[string]any)
		item := gin.H{"id": hit["_id"]}
		for key, value := range source {
			item[key] = value
		}
		results = append(results, item)
	}
	count := intValue(hits["total"])
	if total, ok := hits["total"].(map[string]any); ok {
		count = intValue(total["value"])
	}
	response.Success(context, gin.H{"results": results, "count": count, "size": size, "offset": offset})
}

func (handler *Handler) OpenSearchLogFacetStats(context *gin.Context) {
	allowed := map[string]bool{"log_level": true, "instance": true, "host_ip": true, "log_name": true, "error_fingerprint": true}
	field := strings.TrimSpace(context.Query("field"))
	if !allowed[field] {
		response.BusinessError(context, 400, "invalid facet field", nil)
		return
	}
	cluster, _, parts, err := handler.buildLogQuery(context)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	size := queryInt(context, "size", 20)
	if size < 1 {
		size = 1
	}
	if size > 50 {
		size = 50
	}
	interval := queryInt(context, "interval_minutes", 2)
	if interval < 1 {
		interval = 1
	}
	body := gin.H{"size": 0, "query": gin.H{"bool": gin.H{"filter": parts[0], "must": parts[1]}}, "aggs": gin.H{"by_field": gin.H{"terms": gin.H{"field": field, "size": size, "order": gin.H{"_count": "desc"}}, "aggs": gin.H{"sample": gin.H{"top_hits": gin.H{"size": 1, "sort": []any{gin.H{"@timestamp": "desc"}}}}, "trend": gin.H{"date_histogram": gin.H{"field": "@timestamp", "fixed_interval": fmt.Sprintf("%dm", interval)}}}}}}
	data, err := handler.openSearchRequest(context, cluster, http.MethodPost, "/"+url.PathEscape(defaultString(cluster.IndexPrefix, "logs"))+"-*/_search", body)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	aggregations, _ := data["aggregations"].(map[string]any)
	byField, _ := aggregations["by_field"].(map[string]any)
	buckets, _ := byField["buckets"].([]any)
	response.Success(context, gin.H{"field": field, "interval_minutes": interval, "buckets": buckets})
}

func queryInt(context *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(context.Query(name))
	if err != nil {
		return fallback
	}
	return value
}

var _ = sql.ErrNoRows
