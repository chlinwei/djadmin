package logcollect

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestStreamTierCapacity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		daily      float64
		retention  uint32
		unit       string
		primary    float64
		missing    bool
		recognized bool
		tier       string
		status     string
		capacity   float64
		percent    float64
	}{
		{"days", 5, 30, "d", 60 << 30, false, true, "std", "available", 150 << 30, 40},
		{"hours", 2, 12, "h", 1 << 29, false, true, "std", "available", 1 << 30, 50},
		{"over budget", 1, 1, "d", 1.5 * (1 << 30), false, true, "std", "available", 1 << 30, 150},
		{"empty stream", 1, 1, "d", 0, false, true, "std", "available", 1 << 30, 0},
		{"unrounded small budget", .001, 12, "h", .00025 * (1 << 30), false, true, "std", "available", .0005 * (1 << 30), 50},
		{"missing primary", 5, 30, "d", 0, true, true, "std", "primary_size_unavailable", 150 << 30, 0},
		{"missing tier", 5, 30, "d", 0, false, true, "deleted", "tier_missing", 0, 0},
		{"unrecognized", 5, 30, "d", 0, false, false, "std", "unrecognized", 0, 0},
		{"zero daily size", 0, 30, "d", 0, false, true, "std", "invalid_budget", 0, 0},
		{"zero retention", 1, 0, "d", 0, false, true, "std", "invalid_budget", 0, 0},
		{"invalid unit", 1, 30, "m", 0, false, true, "std", "invalid_budget", 0, 0},
		{"nonfinite budget", math.Inf(1), 30, "d", 0, false, true, "std", "invalid_budget", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := dataStreamEntry{
				Recognized: tc.recognized, Tier: tc.tier, PrimaryBytes: &tc.primary,
				// 副本占用不参与预算；历史流仍按当前档位参数参考。
				Bytes: tc.primary * 2, Historical: true,
			}
			if tc.missing {
				entry.PrimaryBytes = nil
			}
			applyStreamTierCapacity(&entry, map[string]db.MonitorLogRetentionTier{
				"std": {Code: "std", DailySizeGb: tc.daily, RetentionValue: tc.retention, RetentionUnit: tc.unit, Enabled: false},
			})
			if entry.TierCapacityStatus != tc.status {
				t.Fatalf("status = %s, want %s", entry.TierCapacityStatus, tc.status)
			}
			if tc.capacity == 0 {
				if entry.TierCapacityBytes != nil {
					t.Fatalf("invalid capacity must be null: %+v", entry)
				}
			} else if entry.TierCapacityBytes == nil || math.Abs(*entry.TierCapacityBytes-tc.capacity) > .000001 {
				t.Fatalf("capacity = %v, want %v", entry.TierCapacityBytes, tc.capacity)
			}
			if tc.status == "available" {
				if entry.TierUsagePercent == nil || math.Abs(*entry.TierUsagePercent-tc.percent) > .000001 {
					t.Fatalf("usage = %v, want %v", entry.TierUsagePercent, tc.percent)
				}
			} else if entry.TierUsagePercent != nil {
				t.Fatal("unavailable usage must be null, not zero")
			}
		})
	}
}

func TestCatPrimaryBytes(t *testing.T) {
	for _, raw := range []any{nil, "", "-", "NaN", "Inf", "-1", math.NaN(), math.Inf(1), -1.0, true} {
		if got := catPrimaryBytes(map[string]any{"pri.store.size": raw}); got != nil {
			t.Errorf("%v must be unavailable, got %v", raw, *got)
		}
	}
	for _, raw := range []any{"0", float64(0), " 125 ", float64(125)} {
		if got := catPrimaryBytes(map[string]any{"pri.store.size": raw}); got == nil {
			t.Errorf("%v must be available", raw)
		}
	}
}

func TestFetchStreamPrimaryBytesRequiresEveryBackingIndex(t *testing.T) {
	for _, second := range []string{`"30"`, `null`, `"-"`} {
		t.Run(second, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "_cat/indices") {
					if !strings.Contains(r.URL.Query().Get("h"), "pri.store.size") || r.URL.Query().Get("bytes") != "b" {
						t.Error("must request primary storage in bytes")
					}
					fmt.Fprintf(w, `[
						{"index":".ds-logs-p-b-e-s-std-000001","store.size":"40","pri.store.size":"20"},
						{"index":".ds-logs-p-b-e-s-std-000002","store.size":"60","pri.store.size":%s}
					]`, second)
					return
				}
				fmt.Fprint(w, `{"indices":{}}`)
			}))
			defer server.Close()
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			entries, _, err := (&Handler{}).fetchDataStreamEntries(ctx,
				elasticsearchCluster{Hosts: server.URL, Timeout: 5}, streamNameMatcher{prefix: "logs"}, "logs-*")
			if err != nil || len(entries) != 1 {
				t.Fatalf("entries = %+v, error = %v", entries, err)
			}
			if entries[0].Bytes != 100 {
				t.Fatal("existing replica-inclusive storage must remain unchanged")
			}
			if second == `"30"` {
				if entries[0].PrimaryBytes == nil || *entries[0].PrimaryBytes != 50 {
					t.Fatal("must sum all primary shards")
				}
			} else if entries[0].PrimaryBytes != nil {
				t.Fatal("partial primary storage must not be reported as complete")
			}
		})
	}
}

func TestEnrichStreamTierCapacityReadsAllPagesAndReturnsErrors(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler := &Handler{db: pool}
	columns := []string{"id", "create_time", "update_time", "code", "name", "daily_size_gb", "retention_value", "retention_unit", "rollover_min_index_age", "enabled", "is_default", "remark"}
	firstPage := sqlmock.NewRows(columns)
	now := time.Now()
	for i := 0; i < 200; i++ {
		firstPage.AddRow(i+1, now, now, fmt.Sprintf("tier-%d", i), "tier", 1, 1, "d", "", false, false, "")
	}
	mock.ExpectQuery("(?s)SELECT .*FROM monitor_log_retention_tier").
		WithArgs(nil, nil, nil, nil, nil, nil, nil, nil, 200, 0).WillReturnRows(firstPage)
	mock.ExpectQuery("(?s)SELECT .*FROM monitor_log_retention_tier").
		WithArgs(nil, nil, nil, nil, nil, nil, nil, nil, 200, 200).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(201, now, now, "std", "standard", 5, 30, "d", "", false, false, ""))
	primary := float64(60 << 30)
	entries := []dataStreamEntry{{Recognized: true, Tier: "std", PrimaryBytes: &primary}}
	if err := handler.enrichStreamTierCapacity(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if entries[0].TierUsagePercent == nil || *entries[0].TierUsagePercent != 40 {
		t.Fatalf("disabled tier on second page must provide capacity: %+v", entries[0])
	}
	mock.ExpectQuery("(?s)SELECT .*FROM monitor_log_retention_tier").WillReturnError(errors.New("database unavailable"))
	if err := handler.enrichStreamTierCapacity(ctx, entries); err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("database error must be surfaced, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
