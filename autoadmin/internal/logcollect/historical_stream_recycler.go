package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

const historicalStreamSweepInterval = 10 * time.Minute

type historicalDataStream struct {
	Name    string `json:"name"`
	Indices []struct {
		Name string `json:"index_name"`
	} `json:"indices"`
}

// Runs in api, not scheduler/worker: host readiness depends on this process's live
// agent sessions. It never changes retention policies or deletes current streams.
func (handler *Handler) startHistoricalStreamRecycler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(historicalStreamSweepInterval)
		defer ticker.Stop()
		for {
			sweep, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := handler.recycleHistoricalStreams(sweep); err != nil && ctx.Err() == nil {
				slog.Error("historical data stream recycling failed", "error", err)
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

func (handler *Handler) recycleHistoricalStreams(ctx context.Context) error {
	q := db.New(handler.db)
	cluster, err := handler.defaultElasticsearchCluster(ctx)
	if err == sql.ErrNoRows {
		return nil // No managed storage is configured.
	}
	if err != nil {
		return err
	}
	if cluster.IndexPrefix == "" {
		cluster.IndexPrefix = "autoadmin"
	}
	if !deletionIndexPrefix.MatchString(cluster.IndexPrefix) {
		return fmt.Errorf("unsafe index prefix %q", cluster.IndexPrefix)
	}
	raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodGet,
		"/_data_stream/"+cluster.IndexPrefix+"-*?expand_wildcards=all", nil)
	if err != nil {
		if isElasticsearchNotFound(err) {
			return nil
		}
		return err
	}
	var result struct {
		Streams *[]historicalDataStream `json:"data_streams"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Streams == nil {
		return fmt.Errorf("Elasticsearch did not return data_streams")
	}
	owners, err := loadHistoricalStreamOwners(ctx, q)
	if err != nil {
		return err
	}
	active, err := q.ListServiceActiveStreamTiers(ctx)
	if err != nil {
		return err
	}
	for _, stream := range *result.Streams {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		owner, tier, ownerErr := historicalStreamOwner(cluster.IndexPrefix, stream.Name, owners)
		if ownerErr != nil {
			slog.Warn("historical data stream recycling deferred", "stream", stream.Name, "error", ownerErr)
			continue
		}
		current := false
		for _, row := range active {
			current = current || row.ServiceID == owner.ServiceID && row.TierCode == tier
		}
		if current {
			continue
		}
		attempt, cancel := context.WithTimeout(ctx, time.Minute)
		err := handler.recycleHistoricalStream(attempt, cluster, stream, owner, tier)
		cancel()
		if err != nil {
			// One offline host or malformed stream must not prevent unrelated services
			// from being collected. The reason is explicit and retried next sweep.
			slog.Warn("historical data stream recycling deferred", "stream", stream.Name, "error", err)
		}
	}
	return nil
}

func historicalStreamOwner(prefix, name string, owners []db.AssetsServiceDeletion) (db.AssetsServiceDeletion, string, error) {
	if err := validateCleanupStreamName(prefix, name); err != nil {
		return db.AssetsServiceDeletion{}, "", err
	}
	var found db.AssetsServiceDeletion
	var tier string
	for _, owner := range owners {
		base := serviceStreamPrefix(prefix, owner)
		if !strings.HasPrefix(name, base) {
			continue
		}
		if found.ServiceID != 0 && found.ServiceID != owner.ServiceID {
			return found, "", fmt.Errorf("ambiguous service ownership")
		}
		found, tier = owner, strings.TrimPrefix(name, base)
	}
	if found.ServiceID == 0 || tier == "" {
		return found, "", fmt.Errorf("missing service ownership or retention tier")
	}
	return found, tier, nil
}

func loadHistoricalStreamOwners(ctx context.Context, q *db.Queries) ([]db.AssetsServiceDeletion, error) {
	owners, err := q.ListServiceDeletionTombstones(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListServiceStreamDims(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		owners = append(owners, db.AssetsServiceDeletion{
			ServiceID: row.ServiceID, ProjectCode: row.ProjectCode, BusinessSystemCode: row.BusinessSystemCode,
			EnvironmentCode: row.EnvironmentCode, ServiceCode: row.ServiceCode,
		})
	}
	return owners, nil
}

func (handler *Handler) recycleHistoricalStream(ctx context.Context, cluster elasticsearchCluster, stream historicalDataStream, owner db.AssetsServiceDeletion, tier string) error {
	tx, err := handler.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	// The cluster lock serializes replicas and storage bootstrap. Row locks on
	// configuration protect the external delete against concurrent tier edits.
	if _, err = q.LockElasticsearchClusterForGuard(ctx, cluster.ID); err != nil {
		return err
	}
	cluster, err = handler.loadElasticsearchClusterByID(ctx, cluster.ID)
	if err != nil {
		return err
	}
	if !cluster.Enabled {
		return fmt.Errorf("storage cluster is disabled")
	}
	if cluster.IndexPrefix == "" {
		cluster.IndexPrefix = "autoadmin"
	}
	if _, err = q.LockHistoricalStreamService(ctx, owner.ServiceID); err != nil {
		return err
	}
	if _, err = q.LockHistoricalStreamDefinitions(ctx, owner.ServiceID); err != nil {
		return err
	}
	if _, err = q.LockHistoricalStreamSettings(ctx, owner.ServiceID); err != nil {
		return err
	}
	tiers, err := q.LockHistoricalStreamTiers(ctx)
	if err != nil {
		return err
	}
	if len(tiers) == 0 {
		return fmt.Errorf("no retention tier configuration; refusing automatic deletion")
	}
	if _, err = q.LockHistoricalStreamMembers(ctx, owner.ServiceID); err != nil {
		return err
	}
	if _, err = q.GetServiceDeletionTombstone(ctx, owner.ServiceID); err == nil {
		return fmt.Errorf("service deletion is in progress; use its revocation workflow")
	} else if err != sql.ErrNoRows {
		return err
	}
	owners, err := loadHistoricalStreamOwners(ctx, q)
	if err != nil {
		return err
	}
	currentOwner, currentTier, err := historicalStreamOwner(cluster.IndexPrefix, stream.Name, owners)
	if err != nil {
		return err
	}
	if currentOwner != owner || currentTier != tier {
		return fmt.Errorf("stream ownership changed")
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
	if err = handler.historicalStreamHostsReady(ctx, owner.ServiceID); err != nil {
		return err
	}
	deleted, reason, err := handler.deleteExpiredHistoricalStream(ctx, cluster, stream, time.Now().UTC())
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if deleted {
		slog.Info("historical data stream deleted", "stream", stream.Name, "service_id", owner.ServiceID, "reason", reason)
	} else {
		slog.Info("historical data stream retained", "stream", stream.Name, "service_id", owner.ServiceID, "reason", reason)
	}
	return nil
}

func (handler *Handler) historicalStreamHostsReady(ctx context.Context, serviceID int64) error {
	managed, unmanaged, err := handler.resolveServiceApplyTargets(ctx, serviceID)
	if err != nil {
		return err
	}
	refs := make([]LogConfigTargetRef, 0, len(managed))
	for _, host := range managed {
		refs = append(refs, LogConfigTargetRef{HostID: host.HostID, AppliedFingerprint: host.ConfigFingerprint})
	}
	// Cleanup requires positive whole-host evidence, not the presentation shortcut
	// where both expected/applied service fingerprints being absent means "synced".
	states, err := handler.EvaluateLogConfigStates(ctx, refs)
	if err != nil {
		return err
	}
	for index := range managed {
		managed[index].AgentOnline = handler.agentOnline(managed[index].HostInstanceName)
		state, ok := states[managed[index].HostID]
		if !ok || len(state.Warnings) != 0 {
			return fmt.Errorf("host %d has unknown or incomplete rendered configuration", managed[index].HostID)
		}
		managed[index].ConfigState = state.Status
	}
	rows, err := db.New(handler.db).ListHistoricalStreamHosts(ctx)
	if err != nil {
		return err
	}
	return checkHistoricalStreamHosts(time.Now(), serviceID, managed, unmanaged, rows)
}

func checkHistoricalStreamHosts(now time.Time, serviceID int64, managed, unmanaged []serviceApplyTarget, rows []db.ListHistoricalStreamHostsRow) error {
	if len(unmanaged) != 0 || len(managed) == 0 {
		return fmt.Errorf("host configuration cannot be confirmed: unmanaged or no service hosts")
	}
	current := map[int64]serviceApplyTarget{}
	for _, host := range managed {
		if !host.AgentOnline || host.ConfigState != LogConfigSynced {
			return fmt.Errorf("host %s is offline or configuration is not synced", host.HostInstanceName)
		}
		current[host.HostID] = host
	}
	for _, row := range rows {
		// Check former members too: their stale fragments may still write the old tier.
		var applied map[string]string
		if err := json.Unmarshal(row.ServiceFingerprints, &applied); err != nil {
			return fmt.Errorf("host %d has invalid applied fingerprints: %w", row.HostID, err)
		}
		host, exists := current[row.HostID]
		if !exists && applied[strconv.FormatInt(serviceID, 10)] == "" {
			continue
		}
		if !exists {
			return fmt.Errorf("former host %d still has applied service configuration", row.HostID)
		}
		// inputs.d reload is asynchronous; do not collect an empty old stream while
		// Filebeat is still closing the removed input after a successful apply.
		if !row.ManagedEnabled || host.HostInstanceName == "" || host.HostInstanceName != row.InstanceName.String ||
			row.ConfigFingerprint == "" || row.ConfigFingerprint != host.ConfigFingerprint ||
			!row.LastAppliedTime.Valid || now.Sub(row.LastAppliedTime.Time) < time.Minute {
			return fmt.Errorf("host %d is not ready or is within the configuration reload grace period", row.HostID)
		}
		delete(current, row.HostID)
	}
	if len(current) != 0 {
		return fmt.Errorf("missing applied configuration evidence for service hosts")
	}
	return nil
}

type historicalStreamSnapshot struct {
	Count  int64
	Latest time.Time
}

func (handler *Handler) historicalStreamSnapshot(ctx context.Context, cluster elasticsearchCluster, name string) (historicalStreamSnapshot, error) {
	var snapshot historicalStreamSnapshot
	raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodPost, "/"+url.PathEscape(name)+"/_search", historicalStreamSearchBody())
	if err != nil {
		return snapshot, err
	}
	var result struct {
		TimedOut *bool `json:"timed_out"`
		Shards   struct {
			Failed     *int64 `json:"failed"`
			Total      int64  `json:"total"`
			Successful int64  `json:"successful"`
		} `json:"_shards"`
		Hits struct {
			Total *struct {
				Value    int64  `json:"value"`
				Relation string `json:"relation"`
			} `json:"total"`
		} `json:"hits"`
		Aggregations struct {
			Latest struct {
				Value *float64 `json:"value"`
			} `json:"latest"`
			Missing struct {
				Count *int64 `json:"doc_count"`
			} `json:"missing"`
		} `json:"aggregations"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return snapshot, err
	}
	if result.TimedOut == nil || *result.TimedOut || result.Shards.Failed == nil || *result.Shards.Failed != 0 ||
		result.Shards.Total <= 0 || result.Shards.Successful != result.Shards.Total ||
		result.Hits.Total == nil || result.Hits.Total.Relation != "eq" || result.Hits.Total.Value < 0 ||
		result.Aggregations.Missing.Count == nil || *result.Aggregations.Missing.Count != 0 {
		return snapshot, fmt.Errorf("incomplete search or logs without @timestamp; refusing stream deletion")
	}
	snapshot.Count = result.Hits.Total.Value
	if snapshot.Count > 0 {
		if result.Aggregations.Latest.Value == nil || *result.Aggregations.Latest.Value <= 0 || *result.Aggregations.Latest.Value >= math.MaxInt64 {
			return snapshot, fmt.Errorf("missing latest event timestamp")
		}
		snapshot.Latest = time.UnixMilli(int64(*result.Aggregations.Latest.Value)).UTC()
	}
	return snapshot, nil
}

func historicalStreamSearchBody() map[string]any {
	return map[string]any{"size": 0, "track_total_hits": true, "aggs": map[string]any{
		"latest":  map[string]any{"max": map[string]any{"field": "@timestamp"}},
		"missing": map[string]any{"missing": map[string]any{"field": "@timestamp"}},
	}}
}

var historicalRetentionAge = regexp.MustCompile(`^([1-9][0-9]*)(ms|s|m|h|d)$`)

func parseHistoricalRetentionAge(value string) (time.Duration, error) {
	match := historicalRetentionAge.FindStringSubmatch(value)
	if match == nil {
		return 0, fmt.Errorf("unsupported ILM delete min_age %q", value)
	}
	amount, err := strconv.ParseInt(match[1], 10, 64)
	unit := map[string]time.Duration{"ms": time.Millisecond, "s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[match[2]]
	if err != nil || amount > math.MaxInt64/int64(unit) {
		return 0, fmt.Errorf("ILM delete min_age overflows %q", value)
	}
	return time.Duration(amount) * unit, nil
}

func (handler *Handler) historicalStreamDeadline(ctx context.Context, cluster elasticsearchCluster, stream historicalDataStream, latest time.Time) (time.Time, error) {
	raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodGet, "/"+url.PathEscape(stream.Name)+"/_ilm/explain", nil)
	if err != nil {
		return time.Time{}, err
	}
	var result struct {
		Indices map[string]struct {
			Managed   bool   `json:"managed"`
			Policy    string `json:"policy"`
			Created   int64  `json:"index_creation_date_millis"`
			Lifecycle int64  `json:"lifecycle_date_millis"`
			Step      string `json:"step"`
		} `json:"indices"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return time.Time{}, err
	}
	if len(stream.Indices) == 0 || len(result.Indices) != len(stream.Indices) {
		return time.Time{}, fmt.Errorf("backing indices changed or ILM evidence is incomplete")
	}
	var deadline time.Time
	periods := map[string]time.Duration{}
	for _, index := range stream.Indices {
		state, ok := result.Indices[index.Name]
		if !ok || !state.Managed || state.Policy == "" || state.Created <= 0 || state.Lifecycle <= 0 || state.Step == "ERROR" {
			return time.Time{}, fmt.Errorf("index %s has unknown, unmanaged or failed ILM state", index.Name)
		}
		period, ok := periods[state.Policy]
		if !ok {
			raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodGet, "/_ilm/policy/"+url.PathEscape(state.Policy), nil)
			if err != nil {
				return time.Time{}, err
			}
			var policies map[string]struct {
				Policy struct {
					Phases struct {
						Delete struct {
							MinAge  string                     `json:"min_age"`
							Actions map[string]json.RawMessage `json:"actions"`
						} `json:"delete"`
					} `json:"phases"`
				} `json:"policy"`
			}
			if err = json.Unmarshal(raw, &policies); err != nil {
				return time.Time{}, err
			}
			policy := policies[state.Policy].Policy.Phases.Delete
			if _, ok := policy.Actions["delete"]; !ok {
				return time.Time{}, fmt.Errorf("ILM policy %s has no delete action", state.Policy)
			}
			period, err = parseHistoricalRetentionAge(policy.MinAge)
			if err != nil {
				return time.Time{}, err
			}
			periods[state.Policy] = period
		}
		anchor := latest
		for _, millis := range []int64{state.Created, state.Lifecycle} {
			if candidate := time.UnixMilli(millis); candidate.After(anchor) {
				anchor = candidate
			}
		}
		if candidate := anchor.Add(period); candidate.After(deadline) {
			deadline = candidate
		}
	}
	return deadline, nil
}

func (handler *Handler) deleteExpiredHistoricalStream(ctx context.Context, cluster elasticsearchCluster, stream historicalDataStream, now time.Time) (deleted bool, reason string, err error) {
	if err := validateCleanupStreamName(cluster.IndexPrefix, stream.Name); err != nil {
		return false, "", err
	}
	if len(stream.Indices) == 0 {
		return false, "", fmt.Errorf("missing backing index evidence")
	}
	snapshot, err := handler.historicalStreamSnapshot(ctx, cluster, stream.Name)
	if err != nil {
		return false, "", err
	}
	reason = "empty historical stream"
	if snapshot.Count > 0 {
		deadline, err := handler.historicalStreamDeadline(ctx, cluster, stream, snapshot.Latest)
		if err != nil {
			return false, "", err
		}
		if now.Before(deadline) {
			return false, "retained until " + deadline.UTC().Format(time.RFC3339), nil
		}
		reason = "historical stream retention expired"
	}
	raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodGet,
		"/"+url.PathEscape(stream.Name)+"/_settings?filter_path=*.settings.index.blocks.write", nil)
	if err != nil {
		return false, "", err
	}
	var settings map[string]struct {
		Settings struct {
			Index struct {
				Blocks struct {
					Write string `json:"write"`
				} `json:"blocks"`
			} `json:"index"`
		} `json:"settings"`
	}
	if err = json.Unmarshal(raw, &settings); err != nil {
		return false, "", err
	}
	// A pre-existing block belongs to its original operator/workflow, not this
	// collector. Do not clear it in the cleanup failure path.
	for _, setting := range settings {
		if setting.Settings.Index.Blocks.Write != "" && setting.Settings.Index.Blocks.Write != "false" {
			return false, "", fmt.Errorf("stream has a pre-existing write block")
		}
	}
	// Block and drain in-flight index writes before refreshing/rechecking. Never
	// leave a retained stream write-blocked if a race or ES failure aborts deletion.
	defer func() {
		if deleted {
			return
		}
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		result, restoreErr := handler.elasticsearchRequest(restoreCtx, cluster, http.MethodPut, "/"+url.PathEscape(stream.Name)+"/_settings",
			map[string]any{"index.blocks.write": false})
		if restoreErr != nil {
			if !isElasticsearchNotFound(restoreErr) {
				err = errors.Join(err, fmt.Errorf("restore historical stream write setting: %w", restoreErr))
			}
		} else if result["acknowledged"] != true {
			err = errors.Join(err, fmt.Errorf("Elasticsearch did not acknowledge restoring stream write setting"))
		}
	}()
	blocked, err := handler.elasticsearchRequest(ctx, cluster, http.MethodPut, "/"+url.PathEscape(stream.Name)+"/_block/write", nil)
	if err != nil || blocked["acknowledged"] != true || blocked["shards_acknowledged"] != true {
		return false, "", errors.Join(err, fmt.Errorf("Elasticsearch did not confirm historical stream write block"))
	}
	refreshed, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodPost, "/"+url.PathEscape(stream.Name)+"/_refresh", nil)
	if err != nil {
		return false, "", err
	}
	var refresh struct {
		Shards struct {
			Failed     *int64 `json:"failed"`
			Total      int64  `json:"total"`
			Successful int64  `json:"successful"`
		} `json:"_shards"`
	}
	if err = json.Unmarshal(refreshed, &refresh); err != nil {
		return false, "", err
	}
	if refresh.Shards.Failed == nil || *refresh.Shards.Failed != 0 || refresh.Shards.Total <= 0 || refresh.Shards.Successful != refresh.Shards.Total {
		return false, "", fmt.Errorf("historical stream refresh incomplete")
	}
	final, err := handler.historicalStreamSnapshot(ctx, cluster, stream.Name)
	if err != nil {
		return false, "", err
	}
	if final.Count != snapshot.Count || !final.Latest.Equal(snapshot.Latest) {
		return false, "", fmt.Errorf("historical stream received writes during recycling; retry next sweep")
	}
	result, err := handler.elasticsearchRequest(ctx, cluster, http.MethodDelete, "/_data_stream/"+url.PathEscape(stream.Name), nil)
	if err != nil {
		if isElasticsearchNotFound(err) {
			return true, reason, nil
		}
		return false, "", err
	}
	if result["acknowledged"] != true {
		return false, "", fmt.Errorf("Elasticsearch did not acknowledge historical stream deletion")
	}
	return true, reason, nil
}
