package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"autoadmin/internal/api/response"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestRequiredIDListQuery(t *testing.T) {
	tests := []struct {
		query string
		want  []int64
	}{
		{query: "ids=1", want: []int64{1}},
		{query: "ids=1,2,9223372036854775807", want: []int64{1, 2, 9223372036854775807}},
		{query: "ids=%201%20,%202%20", want: []int64{1, 2}},
		{query: ""},
		{query: "ids="},
		{query: "ids=0"},
		{query: "ids=-1"},
		{query: "ids=1,invalid"},
		{query: "ids=1,,2"},
		{query: "ids=1,"},
		{query: "ids=1.5"},
		{query: "ids=9223372036854775808"},
		{query: "ids=1&ids=2"},
		{query: "ids[]=1"},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodGet, "/?"+test.query, nil)
			got, err := requiredIDListQuery(context, "ids")
			if test.want == nil {
				if err == nil {
					t.Fatalf("invalid ID list accepted: %v", got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v, %v; want %v", got, err, test.want)
			}
		})
	}
}

func TestDeploymentRuntimeObservationRejectsInvalidIDs(t *testing.T) {
	for _, query := range []string{"", "?ids=", "?ids=0", "?ids=1,bad", "?ids=1&ids=2"} {
		t.Run(query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/?"+strings.TrimPrefix(query, "?"), strings.NewReader(`{"ids":[60]}`))
			(&Handler{}).RefreshApplicationDeploymentRuntimeStatus(c)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != 400 || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("invalid observation accepted: %s", recorder.Body.String())
			}
		})
	}
}

func TestDeploymentRuntimeObservationReportsMissingInstance(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("WHERE d.id =").WithArgs(int64(60)).WillReturnError(sql.ErrNoRows)
	handler := &Handler{service: newTestService(t, NewRepository(database))}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/?ids=60,60", nil)
	handler.RefreshApplicationDeploymentRuntimeStatus(c)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Total   int                        `json:"total"`
			Summary map[string]int             `json:"summary"`
			Results []deploymentRuntimeOutcome `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || envelope.Data.Total != 1 || envelope.Data.Summary["error"] != 1 ||
		len(envelope.Data.Results) != 1 || envelope.Data.Results[0].Error == "" {
		t.Fatalf("missing instance not reported explicitly: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRuntimeObservationPreservesDisabledAndReportsCheckFailures(t *testing.T) {
	for _, test := range []struct {
		name        string
		enabled     bool
		persistFail bool
	}{
		{name: "disabled"},
		{name: "failed-check", enabled: true},
		{name: "failed-snapshot-write", enabled: true, persistFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			expectSnapshot := func(status, output string) {
				mock.ExpectQuery("WHERE d.id =").WithArgs(int64(60)).
					WillReturnRows(applicationDeploymentRow(60).AddRow(
						60, testRuleUpdatedAt, testRuleUpdatedAt, nil, "nacos", test.enabled, 107, "192.168.201.107",
						status, output, testRuleUpdatedAt, "unknown", []byte(`{}`), int64(8),
					))
				mock.ExpectQuery("FROM assets_application_service_deployment").
					WithArgs(serviceDeploymentLinkArgs(60)...).
					WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "service_id"}).AddRow(60, 42))
			}
			expectSnapshot("running", "old snapshot")
			if test.enabled {
				mock.ExpectBegin().WillReturnError(errors.New("runtime check unavailable"))
				update := mock.ExpectExec("UPDATE assets_application_deployment")
				if test.persistFail {
					update.WillReturnError(errors.New("database write unavailable"))
					expectSnapshot("running", "old snapshot")
				} else {
					update.WillReturnResult(sqlmock.NewResult(0, 1))
					expectSnapshot("error", "runtime check unavailable")
				}
			}
			handler := &Handler{service: newTestService(t, NewRepository(database))}
			result := handler.refreshOneDeploymentRuntimeStatus(context.Background(), 60)
			if result.ID != 60 || result.LastStatusCheckTime == nil {
				t.Fatalf("snapshot fields lost: %+v", result)
			}
			if test.enabled {
				if result.RuntimeStatus != "error" || result.Error == "" || result.RuntimeStatusOutput == "old snapshot" {
					t.Fatalf("failed check disguised as cached success: %+v", result)
				}
			} else if result.RuntimeStatus != "running" || result.Error != "" {
				t.Fatalf("disabled instance should only return cached data: %+v", result)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCollectDeploymentRuntimeStatusesDeduplicatesAndBoundsConcurrency(t *testing.T) {
	ids := make([]int64, 0, 25)
	for id := int64(1); id <= 24; id++ {
		ids = append(ids, id)
	}
	ids = append(ids, 1)
	started := make(chan int64, 24)
	release := make(chan struct{})
	done := make(chan []deploymentRuntimeOutcome, 1)
	var active, maximum atomic.Int64
	go func() {
		done <- collectDeploymentRuntimeStatuses(context.Background(), ids, func(_ context.Context, id int64) deploymentRuntimeOutcome {
			current := active.Add(1)
			for previous := maximum.Load(); current > previous; previous = maximum.Load() {
				if maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- id
			<-release
			active.Add(-1)
			return deploymentRuntimeOutcome{ID: id, RuntimeStatus: "running"}
		})
	}()
	for range 8 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("eight concurrent checks did not start")
		}
	}
	close(release)
	select {
	case results := <-done:
		if len(results) != 24 || maximum.Load() != 8 {
			t.Fatalf("results=%d max concurrency=%d", len(results), maximum.Load())
		}
		for index, result := range results {
			if result.ID != int64(index+1) || result.RuntimeStatus != "running" {
				t.Fatalf("unexpected result %+v", result)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("checks did not finish")
	}
}

func TestBatchRefreshHostInfoRejectsMissingOrInvalidQueryIDs(t *testing.T) {
	for _, query := range []string{"", "?ids=", "?ids=1,bad", "?ids=0", "?ids=1&ids=2"} {
		t.Run(query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			// A GET body must not restore the removed POST query contract.
			context.Request = httptest.NewRequest(http.MethodGet, "/assets/hosts/refresh-info/"+query, strings.NewReader(`{"ids":[1]}`))
			context.Request.Header.Set("Content-Type", "application/json")
			handler := &Handler{}
			handler.BatchRefreshHostInfo(context)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || envelope.Code != 400 {
				t.Fatalf("invalid query was not rejected: %s", recorder.Body.String())
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("live host observation must not be cached")
			}
		})
	}
}
