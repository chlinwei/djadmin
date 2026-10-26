package logcollect

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

func (handler *Handler) recycleSharedHistoricalStream(ctx context.Context, tx *sql.Tx, q *db.Queries,
	cluster elasticsearchCluster, stream historicalDataStream, owner db.AssetsServiceDeletion, tier string) error {
	source, err := q.LockSharedLogSource(ctx, -owner.ServiceID)
	if err != nil {
		return err
	}
	tiers, err := q.LockHistoricalStreamTiers(ctx)
	if err != nil {
		return err
	}
	if len(tiers) == 0 {
		return fmt.Errorf("no retention policies; refusing shared-source deletion")
	}
	owners, err := loadHistoricalStreamOwners(ctx, q)
	if err != nil {
		return err
	}
	found, foundTier, err := historicalStreamOwner(cluster.IndexPrefix, stream.Name, owners)
	if err != nil || found != owner || foundTier != tier {
		return fmt.Errorf("shared-source stream ownership changed: %v", err)
	}
	active, err := q.ListServiceActiveStreamTiers(ctx)
	if err != nil {
		return err
	}
	for _, row := range active {
		if row.ServiceID == owner.ServiceID && row.TierCode == tier {
			return nil
		}
	}
	revocations, err := q.ListSharedLogSourceRevocations(ctx)
	if err != nil {
		return err
	}
	for _, row := range revocations {
		if row.SourceID == source.ID && !row.Stopped {
			return fmt.Errorf("shared source %d is still waiting for collection revocation", source.ID)
		}
	}
	members, err := q.ListSharedLogSourceServiceIDs(ctx, source.ID)
	if err != nil {
		return err
	}
	if len(members) > 0 {
		// Changing a tier does not stop the source; require the host to have
		// positively applied the new tier before collecting its old write stream.
		if err = handler.sharedSourceHostReady(ctx, source.HostID); err != nil {
			return err
		}
	}
	deleted, reason, err := handler.deleteExpiredHistoricalStream(ctx, cluster, stream, time.Now().UTC())
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	slog.Info("shared-source historical stream retention evaluated", "source_id", source.ID,
		"stream", stream.Name, "deleted", deleted, "reason", reason)
	return nil
}

func (handler *Handler) sharedSourceHostReady(ctx context.Context, hostID int64) error {
	rows, err := db.New(handler.db).ListHistoricalStreamHosts(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.HostID != hostID {
			continue
		}
		if !row.ManagedEnabled || !handler.agentOnline(row.InstanceName.String) ||
			!row.LastAppliedTime.Valid || time.Since(row.LastAppliedTime.Time) < time.Minute {
			return fmt.Errorf("shared-source host %d is offline, unmanaged or within reload grace period", hostID)
		}
		states, err := handler.EvaluateLogConfigStates(ctx, []LogConfigTargetRef{
			{HostID: hostID, AppliedFingerprint: row.ConfigFingerprint},
		})
		if err != nil {
			return err
		}
		state, exists := states[hostID]
		if !exists || state.Status != LogConfigSynced || len(state.Warnings) > 0 {
			return fmt.Errorf("shared-source host %d configuration is not confirmed synced", hostID)
		}
		return nil
	}
	return fmt.Errorf("shared-source host %d has no applied configuration evidence", hostID)
}
