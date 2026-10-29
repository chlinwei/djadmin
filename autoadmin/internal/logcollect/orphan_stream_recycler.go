package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

var errUnmanagedOrphanStream = errors.New("unmanaged stream")

func orphanFencePipelineName(prefix string) string {
	return buildMappingGuardPipelineName(prefix) + "-orphans"
}

// Exact stream identities are stored in ES rather than process memory so stale
// shippers remain fenced after deletion, restart, and storage bootstrap.
func (handler *Handler) reconcileOrphanStreamFence(ctx context.Context, cluster elasticsearchCluster, add string) error {
	return handler.changeOrphanStreamFence(ctx, cluster, add, "")
}

func (handler *Handler) changeOrphanStreamFence(ctx context.Context, cluster elasticsearchCluster, add, remove string) error {
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.LockElasticsearchClusterForGuard(ctx, cluster.ID); err != nil {
		return err
	}
	if err = handler.updateOrphanStreamFence(ctx, q, cluster, add, remove); err != nil {
		return err
	}
	return tx.Commit()
}

func orphanStreamUnowned(prefix, name string, owners []db.AssetsServiceDeletion) error {
	_, _, err := historicalStreamOwner(prefix, name, owners)
	if !errors.Is(err, errMissingStreamOwner) {
		return fmt.Errorf("stream is owned, ambiguous or invalid: %v", err)
	}
	// Shared source names must never be guessed as service identities, including
	// sources whose historical metadata is incomplete.
	if strings.Contains(name, "-shared-source-") || strings.Contains(name, "-shared-source:") {
		return fmt.Errorf("shared source identity requires its own revocation workflow")
	}
	return nil
}

func (handler *Handler) updateOrphanStreamFence(ctx context.Context, q *db.Queries, cluster elasticsearchCluster, add, remove string) error {
	owners, err := loadHistoricalStreamOwners(ctx, q)
	if err != nil {
		return err
	}
	if add != "" {
		if err := orphanStreamUnowned(cluster.IndexPrefix, add, owners); err != nil {
			return err
		}
	}
	name := orphanFencePipelineName(cluster.IndexPrefix)
	raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodGet, "/_ingest/pipeline/"+url.PathEscape(name), nil)
	if err != nil && !isElasticsearchNotFound(err) {
		return err
	}
	var pipelines map[string]struct {
		Meta *struct {
			Streams *[]string `json:"streams"`
		} `json:"_meta"`
	}
	if err == nil {
		if err := json.Unmarshal(raw, &pipelines); err != nil {
			return err
		}
		if entry, ok := pipelines[name]; !ok || entry.Meta == nil || entry.Meta.Streams == nil {
			return fmt.Errorf("missing orphan fence pipeline evidence")
		}
	}
	streams := []string{}
	seen := map[string]bool{}
	var previous []string
	if entry := pipelines[name]; entry.Meta != nil && entry.Meta.Streams != nil {
		previous = *entry.Meta.Streams
	}
	for _, stream := range append(previous, add) {
		if stream == "" || stream == remove || seen[stream] || orphanStreamUnowned(cluster.IndexPrefix, stream, owners) != nil {
			continue
		}
		seen[stream] = true
		streams = append(streams, stream)
	}
	body := map[string]any{
		"_meta": map[string]any{"streams": streams},
		"processors": []any{map[string]any{"drop": map[string]any{
			"tag": "orphan_stream_fence",
			"if": map[string]any{"lang": "painless", "source": `def identity = ctx._index;
if (identity.startsWith('.ds-')) {
  int generation = identity.lastIndexOf('-');
  int date = generation > 0 ? identity.lastIndexOf('-', generation - 1) : -1;
  if (date > 4 && generation - date == 11) { identity = identity.substring(4, date); }
}
for (def stream : params.streams) {
  if (identity == stream) { return true; }
}
return false;`, "params": map[string]any{"streams": streams}},
		}}},
	}
	result, err := handler.elasticsearchRequest(ctx, cluster, http.MethodPut, "/_ingest/pipeline/"+url.PathEscape(name), body)
	if err != nil {
		return err
	}
	if result["acknowledged"] != true {
		return fmt.Errorf("orphan fence was not acknowledged")
	}
	return nil
}

func (handler *Handler) recycleOrphanStream(ctx context.Context, cluster elasticsearchCluster, stream historicalDataStream) (err error) {
	// Publish the same final guard as service deletion before acquiring its lock.
	// Future rollovers and auto-created streams must inherit the persistent fence.
	if err := handler.publishDeletionAwareGuard(ctx, cluster); err != nil {
		return err
	}
	tx, err := handler.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fenced, deleted := false, false
	defer func() {
		if !fenced || deleted {
			return
		}
		// ES changes are not rolled back with SQL. A deferred/failed deletion must
		// not leave a retained stream dropping events through our final pipeline.
		_ = tx.Rollback()
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		err = errors.Join(err, handler.changeOrphanStreamFence(cleanup, cluster, "", stream.Name))
	}()
	q := db.New(tx)
	if _, err = q.LockElasticsearchClusterForGuard(ctx, cluster.ID); err != nil {
		return err
	}
	current, err := handler.loadElasticsearchClusterByID(ctx, cluster.ID)
	if err != nil {
		return err
	}
	if current.IndexPrefix == "" {
		current.IndexPrefix = "autoadmin"
	}
	if !current.Enabled || current.IndexPrefix != cluster.IndexPrefix || current.Hosts != cluster.Hosts {
		return fmt.Errorf("storage configuration changed")
	}
	checkOwner := func() error {
		owners, err := loadHistoricalStreamOwners(ctx, q)
		if err != nil {
			return err
		}
		return orphanStreamUnowned(cluster.IndexPrefix, stream.Name, owners)
	}
	if err := checkOwner(); err != nil {
		return err
	}
	for _, index := range stream.Indices {
		if index.Name == "" || index.UUID == "" {
			return fmt.Errorf("orphan backing index identity is incomplete")
		}
	}
	prepare := func() error {
		fenced = true
		if err := handler.updateOrphanStreamFence(ctx, q, cluster, stream.Name, ""); err != nil {
			return err
		}
		if err := handler.checkServiceDeletionTemplates(ctx, cluster); err != nil {
			return err
		}
		result, err := handler.elasticsearchRequest(ctx, cluster, http.MethodPut, "/"+url.PathEscape(stream.Name)+"/_settings",
			map[string]any{"index.final_pipeline": buildMappingGuardPipelineName(cluster.IndexPrefix)})
		if err != nil {
			return err
		}
		if result["acknowledged"] != true {
			return fmt.Errorf("orphan final pipeline was not acknowledged")
		}
		raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodGet, "/_data_stream/"+url.PathEscape(stream.Name), nil)
		if err != nil {
			return err
		}
		var current struct {
			Streams []historicalDataStream `json:"data_streams"`
		}
		if err := json.Unmarshal(raw, &current); err != nil {
			return err
		}
		if len(current.Streams) != 1 || !reflect.DeepEqual(current.Streams[0], stream) {
			return fmt.Errorf("orphan backing indices changed during recycling")
		}
		return checkOwner()
	}
	var reason string
	deleted, reason, err = handler.deleteExpiredStream(ctx, cluster, stream, time.Now().UTC(), true, prepare)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("orphan data stream retention evaluated", "stream", stream.Name, "deleted", deleted, "reason", reason)
	return nil
}

func (handler *Handler) orphanStreamSnapshot(ctx context.Context, cluster elasticsearchCluster, name string) (historicalStreamSnapshot, error) {
	body := historicalStreamSearchBody()
	body["size"] = 10
	// Aggregations cover the entire stream; the ten hits are actually latest,
	// not ten arbitrary documents that happen to look old.
	body["sort"] = []any{map[string]any{"@timestamp": map[string]any{"order": "desc", "unmapped_type": "date"}}}
	snapshot, err := handler.readHistoricalStreamSnapshot(ctx, cluster, name, body)
	if err == nil {
		return snapshot, nil
	}
	if !errors.Is(err, errMissingEventTime) {
		return snapshot, err
	}
	// An aggregation failure is never proof of expiry. Exhaustively scan in
	// batches of ten; unlike an arbitrary sample this establishes a global max.
	return handler.scanOrphanStream(ctx, cluster, name)
}

func (handler *Handler) scanOrphanStream(ctx context.Context, cluster elasticsearchCluster, name string) (historicalStreamSnapshot, error) {
	var snapshot historicalStreamSnapshot
	var scrollID string
	defer func() {
		if scrollID != "" {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = handler.elasticsearchRequestRaw(cleanup, cluster, http.MethodDelete, "/_search/scroll", map[string]any{"scroll_id": []string{scrollID}})
		}
	}()
	path := "/" + url.PathEscape(name) + "/_search?scroll=1m"
	body := map[string]any{"size": 10, "track_total_hits": true, "sort": []string{"_doc"}}
	var total int64 = -1
	for {
		raw, err := handler.elasticsearchRequestRaw(ctx, cluster, http.MethodPost, path, body)
		if err != nil {
			return snapshot, err
		}
		var page struct {
			ScrollID string `json:"_scroll_id"`
			TimedOut *bool  `json:"timed_out"`
			Shards   struct {
				Total      int64  `json:"total"`
				Successful int64  `json:"successful"`
				Failed     *int64 `json:"failed"`
			} `json:"_shards"`
			Hits struct {
				Total struct {
					Value    int64  `json:"value"`
					Relation string `json:"relation"`
				} `json:"total"`
				Hits []struct {
					Source map[string]any `json:"_source"`
				} `json:"hits"`
			} `json:"hits"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return snapshot, err
		}
		if page.ScrollID != "" {
			scrollID = page.ScrollID
		}
		if page.TimedOut == nil || *page.TimedOut || page.Shards.Failed == nil || *page.Shards.Failed != 0 ||
			page.Shards.Total <= 0 || page.Shards.Total != page.Shards.Successful ||
			page.Hits.Total.Relation != "eq" || page.Hits.Total.Value < 0 {
			return snapshot, fmt.Errorf("incomplete orphan timestamp scan")
		}
		if total == -1 {
			total = page.Hits.Total.Value
		}
		if page.Hits.Total.Value != total {
			return snapshot, fmt.Errorf("orphan scan total changed")
		}
		for _, hit := range page.Hits.Hits {
			event, err := orphanEventTime(hit.Source)
			if err != nil {
				return snapshot, err
			}
			snapshot.Count++
			if event.After(snapshot.Latest) {
				snapshot.Latest = event
			}
		}
		if snapshot.Count > total {
			return snapshot, fmt.Errorf("orphan scan exceeds total")
		}
		if len(page.Hits.Hits) == 0 {
			if snapshot.Count != total {
				return snapshot, fmt.Errorf("incomplete orphan scan")
			}
			return snapshot, nil
		}
		if scrollID == "" {
			return snapshot, fmt.Errorf("orphan scan has no cursor")
		}
		path, body = "/_search/scroll", map[string]any{"scroll": "1m", "scroll_id": scrollID}
	}
}

func orphanEventTime(source map[string]any) (time.Time, error) {
	// Only explicit timezone-bearing timestamps are safe without the removed
	// owner's parser/timezone configuration. Never infer a timezone or use now.
	for _, field := range []string{"@timestamp", "timestamp", "time"} {
		if value, ok := source[field]; ok {
			text, ok := value.(string)
			if ok {
				parsed, err := time.Parse(time.RFC3339Nano, text)
				if err == nil && parsed.UnixMilli() > 0 {
					return parsed.UTC(), nil
				}
			}
			return time.Time{}, fmt.Errorf("unparseable orphan timestamp field %s", field)
		}
	}
	return time.Time{}, fmt.Errorf("orphan log has no reliable timestamp")
}
