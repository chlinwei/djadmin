package logcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"autoadmin/internal/agent/pb"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) startSharedSourceReconciler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			sweep, cancel := context.WithTimeout(ctx, 2*time.Minute)
			if err := handler.reconcileSharedSourceStops(sweep); err != nil && ctx.Err() == nil {
				slog.Error("shared log source stop reconciliation failed", "error", err)
			}
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (handler *Handler) reconcileSharedSourceStops(ctx context.Context) error {
	q := db.New(handler.db)
	rows, err := q.ListSharedLogSourceRevocations(ctx)
	if err != nil {
		return err
	}
	pending := false
	for _, row := range rows {
		pending = pending || !row.Stopped
	}
	if !pending {
		return nil
	}
	if err = handler.installSharedSourceBarriers(ctx, q); err != nil {
		for _, row := range rows {
			if row.Stopped {
				continue
			}
			if persistErr := q.UpdateSharedLogSourceRevocation(ctx, db.UpdateSharedLogSourceRevocationParams{
				SourceID: row.SourceID, Generation: row.Generation, Stopped: false,
				LastError: "ES 写入屏障未完成: " + err.Error(),
			}); persistErr != nil {
				return fmt.Errorf("publish shared-source barrier: %v; persist stop error: %w", err, persistErr)
			}
		}
		return err
	}
	for _, row := range rows {
		if row.Stopped {
			continue
		}
		stopErr := handler.stopSharedSourceEpoch(ctx, row)
		lastError := ""
		if stopErr != nil {
			lastError = stopErr.Error()
			slog.Warn("shared log source stop pending", "source_id", row.SourceID, "generation", row.Generation, "error", stopErr)
		}
		if err = q.UpdateSharedLogSourceRevocation(ctx, db.UpdateSharedLogSourceRevocationParams{
			SourceID: row.SourceID, Generation: row.Generation, Stopped: stopErr == nil, LastError: lastError,
		}); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func (handler *Handler) installSharedSourceBarriers(ctx context.Context, q *db.Queries) error {
	// Every cluster is fenced before asking any host to stop: an offline Filebeat
	// or queued bulk request cannot keep a retired epoch alive.
	clusterIDs, err := q.ListServiceDeletionClusterIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range clusterIDs {
		cluster, err := handler.loadElasticsearchClusterByID(ctx, id)
		if err != nil {
			return err
		}
		cluster.IndexPrefix = defaultString(cluster.IndexPrefix, "autoadmin")
		if !deletionIndexPrefix.MatchString(cluster.IndexPrefix) {
			return fmt.Errorf("unsafe cluster index prefix")
		}
		if err = handler.publishDeletionAwareGuard(ctx, cluster); err != nil {
			return err
		}
		settings, err := handler.elasticsearchRequest(ctx, cluster, "PUT",
			"/"+cluster.IndexPrefix+"-*/_settings?expand_wildcards=all&allow_no_indices=true&ignore_unavailable=true",
			gin.H{"index.final_pipeline": buildMappingGuardPipelineName(cluster.IndexPrefix)})
		if err != nil {
			return err
		}
		if settings["acknowledged"] != true {
			return fmt.Errorf("cluster %d did not acknowledge shared-source write barrier", id)
		}
		if err = handler.checkServiceDeletionTemplates(ctx, cluster); err != nil {
			return err
		}
	}
	return nil
}

func (handler *Handler) stopSharedSourceEpoch(ctx context.Context, row db.ListSharedLogSourceRevocationsRow) error {
	if !handler.agentOnline(row.HostName) {
		return fmt.Errorf("主机 %s Agent 离线，等待重连后撤销采集", row.HostName)
	}
	params, err := json.Marshal(gin.H{"shared_source_id": strconv.FormatInt(row.SourceID, 10),
		"shared_source_generation": strconv.FormatInt(row.Generation, 10)})
	if err != nil {
		return err
	}
	result, err := handler.gateway.Execute(ctx, row.HostName, &pb.AutomationExecuteRequest{
		JobId: fmt.Sprintf("revoke-shared-source-%d-%d-%d", row.SourceID, row.Generation, time.Now().UnixNano()),
		Type:  "custom", Action: "revoke_shared_log_source", ParamsJson: string(params), TimeoutSeconds: 30,
	})
	if err != nil {
		return err
	}
	if result.Status != "success" {
		return fmt.Errorf("共享源停止失败（请确认 Agent 已升级）: %s %s", result.ErrorMessage, result.Stderr)
	}
	return nil
}
