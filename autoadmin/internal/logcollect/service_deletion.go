package logcollect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

var deletionIndexPrefix = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func serviceDeletionScope(row db.AssetsServiceDeletion) gin.H {
	return gin.H{"bool": gin.H{"filter": []any{
		gin.H{"term": gin.H{"project": row.ProjectCode}},
		gin.H{"term": gin.H{"business_system": row.BusinessSystemCode}},
		gin.H{"term": gin.H{"environment": row.EnvironmentCode}},
		gin.H{"term": gin.H{"service": row.ServiceCode}},
	}}}
}

// Keep ownership at document level, never delete a stream/index: older/shared indices
// may contain several services and a service code alone is not globally unique.
// Parameterize one script instead of compiling one unique script per deleted service:
// a large tombstone set must not exhaust ES's global script compilation budget.
const serviceDeletionCondition = `
for (def owner : params.owners) {
  if (ctx.project == owner.project && ctx.business_system == owner.business_system &&
      ctx.environment == owner.environment && ctx.service == owner.service) {
    return true;
  }
}
return false;`

// All final-guard publishers take the cluster row lock and read tombstones *after* it.
// Bootstrap or a concurrent deletion must not overwrite a newer revocation list.
func (handler *Handler) publishDeletionAwareGuard(ctx context.Context, cluster elasticsearchCluster) error {
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.LockElasticsearchClusterForGuard(ctx, cluster.ID); err != nil {
		return err
	}
	rows, err := q.ListServiceDeletionTombstones(ctx)
	if err != nil {
		return err
	}
	body := buildMappingGuardPipelineBody(handler.mappingGuardMode())
	owners := make([]any, 0, len(rows))
	for _, row := range rows {
		owners = append(owners, gin.H{"project": row.ProjectCode, "business_system": row.BusinessSystemCode,
			"environment": row.EnvironmentCode, "service": row.ServiceCode})
	}
	if len(owners) > 0 {
		barrier := gin.H{"drop": gin.H{
			"if":  gin.H{"lang": "painless", "source": serviceDeletionCondition, "params": gin.H{"owners": owners}},
			"tag": "revoked_services",
		}}
		body["processors"] = append([]any{barrier}, body["processors"].([]any)...)
	}
	result, err := handler.elasticsearchRequest(ctx, cluster, "PUT",
		"/_ingest/pipeline/"+url.PathEscape(buildMappingGuardPipelineName(cluster.IndexPrefix)), body)
	if err != nil {
		return err
	}
	if result["acknowledged"] != true {
		return fmt.Errorf("Elasticsearch did not acknowledge service write barrier")
	}
	return tx.Commit()
}

func (handler *Handler) RevokeServiceLogs(ctx context.Context, serviceID int64) error {
	q := db.New(handler.db)
	scope, err := q.GetServiceDeletionTombstone(ctx, serviceID)
	if err != nil {
		return err
	}
	ids, err := q.ListServiceDeletionClusterIDs(ctx)
	if err != nil {
		return err
	}
	clusters := make([]elasticsearchCluster, 0, len(ids))
	// Install an ES-side barrier before touching any host. Offline Filebeat, queued bulk
	// requests and stale concurrent applies must not recreate data after the purge.
	for _, id := range ids {
		cluster, err := handler.loadElasticsearchClusterByID(ctx, id)
		if err != nil {
			return err
		}
		if cluster.IndexPrefix == "" {
			cluster.IndexPrefix = "autoadmin"
		}
		if !deletionIndexPrefix.MatchString(cluster.IndexPrefix) {
			return fmt.Errorf("cluster %d has an unsafe index prefix", id)
		}
		if err = handler.publishDeletionAwareGuard(ctx, cluster); err != nil {
			return fmt.Errorf("cluster %d revoke ingestion: %w", id, err)
		}
		// Older indices also need the final guard; changing the template only covers future rollovers.
		settings, err := handler.elasticsearchRequest(ctx, cluster, "PUT",
			"/"+cluster.IndexPrefix+"-*/_settings?expand_wildcards=all&allow_no_indices=true&ignore_unavailable=true",
			gin.H{"index.final_pipeline": buildMappingGuardPipelineName(cluster.IndexPrefix)})
		if err != nil {
			return fmt.Errorf("cluster %d install write barrier: %w", id, err)
		}
		if settings["acknowledged"] != true {
			return fmt.Errorf("cluster %d did not acknowledge index write barrier", id)
		}
		if err = handler.checkServiceDeletionTemplates(ctx, cluster); err != nil {
			return err
		}
		clusters = append(clusters, cluster)
	}
	targets, err := q.ListServiceLogApplyTargets(ctx, serviceID)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	var revokeErrors []error
	for _, target := range targets {
		if seen[target.HostID] {
			continue
		}
		seen[target.HostID] = true
		if !handler.agentOnline(target.HostInstanceName) {
			revokeErrors = append(revokeErrors, fmt.Errorf("host %s agent offline; ingestion is fenced, retry when online", target.HostInstanceName))
			continue
		}
		params, _ := json.Marshal(gin.H{"service_id": strconv.FormatInt(serviceID, 10),
			"project": scope.ProjectCode, "business_system": scope.BusinessSystemCode,
			"environment": scope.EnvironmentCode, "service": scope.ServiceCode})
		result, err := handler.gateway.Execute(ctx, target.HostInstanceName, &pb.AutomationExecuteRequest{
			JobId: fmt.Sprintf("revoke-service-logs-%d-%d", serviceID, time.Now().UnixNano()),
			Type:  "custom", Action: "revoke_service_logs", ParamsJson: string(params), TimeoutSeconds: 30,
		})
		if err != nil {
			revokeErrors = append(revokeErrors, err)
			continue
		}
		if result.Status != "success" {
			revokeErrors = append(revokeErrors, fmt.Errorf("host %s collection revoke failed: %s %s", target.HostInstanceName, result.ErrorMessage, result.Stderr))
		}
	}
	for _, cluster := range clusters {
		// delete_by_query uses a search snapshot. Flush previously unrefreshed writes into
		// that snapshot after the ingestion barrier, otherwise recent logs can escape it.
		refreshed, err := handler.elasticsearchRequest(ctx, cluster, "POST",
			"/"+cluster.IndexPrefix+"-*/_refresh?expand_wildcards=all&allow_no_indices=true&ignore_unavailable=true", nil)
		if err != nil {
			return fmt.Errorf("cluster %d refresh before purge: %w", cluster.ID, err)
		}
		if shards, ok := refreshed["_shards"].(map[string]any); ok {
			if failed, ok := shards["failed"].(float64); ok && failed > 0 {
				return fmt.Errorf("cluster %d refresh has failed shards; retry deletion", cluster.ID)
			}
		}
		result, err := handler.elasticsearchRequest(ctx, cluster, "POST",
			"/"+cluster.IndexPrefix+"-*/_delete_by_query?wait_for_completion=true&conflicts=abort&refresh=true&expand_wildcards=all&allow_no_indices=true&ignore_unavailable=true",
			gin.H{"query": serviceDeletionScope(scope)})
		if err != nil {
			return fmt.Errorf("cluster %d purge service logs: %w", cluster.ID, err)
		}
		if err = checkServicePurgeResult(result); err != nil {
			return fmt.Errorf("cluster %d: %w", cluster.ID, err)
		}
	}
	return errors.Join(revokeErrors...)
}

// Updating existing indices alone is insufficient: a rollover or delayed Filebeat
// auto-creation must inherit the same final guard. Refuse completion until storage
// bootstrap has installed it on the platform's base and tier templates.
func (handler *Handler) checkServiceDeletionTemplates(ctx context.Context, cluster elasticsearchCluster) error {
	result, err := handler.elasticsearchRequest(ctx, cluster, "GET",
		"/_index_template/"+cluster.IndexPrefix+"-*", nil)
	if err != nil {
		return fmt.Errorf("cluster %d verify future-index barrier: %w", cluster.ID, err)
	}
	raw, _ := json.Marshal(result)
	var state struct {
		Templates []struct {
			Template struct {
				Patterns []string `json:"index_patterns"`
				Body     struct {
					Settings map[string]any `json:"settings"`
				} `json:"template"`
			} `json:"index_template"`
		} `json:"index_templates"`
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return err
	}
	baseCovered := false
	for _, entry := range state.Templates {
		settings := entry.Template.Body.Settings
		final := settings["index.final_pipeline"]
		if nested, ok := settings["index"].(map[string]any); ok {
			final = nested["final_pipeline"]
		}
		if final != buildMappingGuardPipelineName(cluster.IndexPrefix) {
			return fmt.Errorf("cluster %d index template lacks final write barrier; bootstrap storage then retry", cluster.ID)
		}
		for _, pattern := range entry.Template.Patterns {
			baseCovered = baseCovered || pattern == cluster.IndexPrefix+"-*"
		}
	}
	if !baseCovered {
		return fmt.Errorf("cluster %d has no guarded base index template; bootstrap storage then retry", cluster.ID)
	}
	return nil
}

func checkServicePurgeResult(result map[string]any) error {
	raw, _ := json.Marshal(result)
	var status struct {
		TimedOut  bool              `json:"timed_out"`
		Conflicts int64             `json:"version_conflicts"`
		Failures  []json.RawMessage `json:"failures"`
		Total     *int64            `json:"total"`
		Deleted   *int64            `json:"deleted"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return err
	}
	if status.Total == nil || status.Deleted == nil || status.TimedOut || status.Conflicts != 0 ||
		len(status.Failures) > 0 || *status.Total != *status.Deleted {
		return fmt.Errorf("历史日志清理未完成，请重试: %s", strings.TrimSpace(string(raw)))
	}
	return nil
}
