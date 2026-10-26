package logcollect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	revocations, err := q.ListSharedLogSourceRevocations(ctx)
	if err != nil {
		return err
	}
	if len(revocations) > 0 {
		epochs := make([]any, 0, len(revocations))
		for _, row := range revocations {
			epochs = append(epochs, gin.H{"id": strconv.FormatInt(row.SourceID, 10), "generation": strconv.FormatInt(row.Generation, 10)})
		}
		barrier := gin.H{"drop": gin.H{"tag": "revoked_shared_sources", "if": gin.H{
			"lang": "painless", "source": `for (def epoch : params.epochs) {
  if (ctx.shared_source_id == epoch.id && ctx.shared_source_generation == epoch.generation) { return true; }
}
return false;`, "params": gin.H{"epochs": epochs},
		}}}
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
	if err = handler.detachServiceSharedSources(ctx, serviceID); err != nil {
		return fmt.Errorf("解除共享采集源关联失败: %w", err)
	}
	// Codes may contain hyphens: svc-* also matches svc-extra-*. Include live and
	// deleted identities so ambiguous names fail closed instead of deleting another service.
	owners, err := q.ListServiceStreamDims(ctx)
	if err != nil {
		return err
	}
	tombstones, err := q.ListServiceDeletionTombstones(ctx)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		tombstones = append(tombstones, db.AssetsServiceDeletion{
			ServiceID: owner.ServiceID, ProjectCode: owner.ProjectCode,
			BusinessSystemCode: owner.BusinessSystemCode, EnvironmentCode: owner.EnvironmentCode,
			ServiceCode: owner.ServiceCode,
		})
	}
	ids, err := q.ListServiceDeletionClusterIDs(ctx)
	if err != nil {
		return err
	}
	clusters := make([]elasticsearchCluster, 0, len(ids))
	// Install an ES-side barrier before touching any host. Offline Filebeat, queued bulk
	// requests and stale concurrent applies must not write logs after stream deletion.
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
	targets, err := listServiceLogApplyTargets(ctx, handler.db, serviceID)
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
		if err = handler.deleteServiceDataStreams(ctx, cluster, scope, tombstones); err != nil {
			return fmt.Errorf("cluster %d delete service data streams: %w", cluster.ID, err)
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

func serviceStreamPrefix(prefix string, scope db.AssetsServiceDeletion) string {
	return LogDataStreamName(prefix, scope.ProjectCode, scope.EnvironmentCode,
		scope.BusinessSystemCode, scope.ServiceCode, "")
}

func (handler *Handler) deleteServiceDataStreams(ctx context.Context, cluster elasticsearchCluster, scope db.AssetsServiceDeletion, owners []db.AssetsServiceDeletion) error {
	prefix := serviceStreamPrefix(cluster.IndexPrefix, scope)
	if err := validateCleanupStreamName(cluster.IndexPrefix, prefix); err != nil {
		return err
	}
	raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodGet,
		"/_data_stream/"+url.PathEscape(prefix)+"*?expand_wildcards=all", nil)
	if err != nil {
		if isElasticsearchNotFound(err) {
			return nil
		}
		return err
	}
	var result struct {
		Streams *[]struct {
			Name string `json:"name"`
		} `json:"data_streams"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Streams == nil {
		return fmt.Errorf("Elasticsearch did not return data_streams")
	}
	// Validate the entire set before deleting anything. Enumerate all actual tiers,
	// including historical/removed tiers, then delete exact names rather than wildcards.
	for _, stream := range *result.Streams {
		if err = validateCleanupStreamName(cluster.IndexPrefix, stream.Name); err != nil {
			return err
		}
		if !strings.HasPrefix(stream.Name, prefix) || stream.Name == prefix {
			return fmt.Errorf("data stream %q does not belong to service %d", stream.Name, scope.ServiceID)
		}
		for _, owner := range owners {
			if owner.ServiceID != scope.ServiceID && strings.HasPrefix(stream.Name, serviceStreamPrefix(cluster.IndexPrefix, owner)) {
				return fmt.Errorf("data stream %q has ambiguous service ownership; refusing deletion", stream.Name)
			}
		}
	}
	for _, stream := range *result.Streams {
		deleted, err := handler.elasticsearchRequest(ctx, cluster, http.MethodDelete,
			"/_data_stream/"+url.PathEscape(stream.Name), nil)
		if err != nil {
			if isElasticsearchNotFound(err) {
				continue
			}
			return err
		}
		if deleted["acknowledged"] != true {
			return fmt.Errorf("Elasticsearch did not acknowledge deletion of data stream %q", stream.Name)
		}
	}
	return nil
}
