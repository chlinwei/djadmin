package logcollect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/assets"
	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestLogSearchTwelveHourScopedIndices(t *testing.T) {
	cases := []struct {
		name, field, options string
		multi                bool
		filterPath           bool
		shared               bool
		tailReady            bool
		tailMode             string
	}{
		{name: "single service logs"},
		{name: "multiple service logs", multi: true},
		{name: "full statistics", field: "service", multi: true},
		{name: "explicit full statistics", field: "service", options: "false"},
		{name: "level options", field: "log_level", options: "true"},
		{name: "instance options", field: "instance", options: "true", multi: true},
		{name: "path options", field: "log_path", options: "true", multi: true},
		{name: "logs filtered by real path", filterPath: true},
		{name: "levels filtered by real path", field: "log_level", options: "true", filterPath: true},
		{name: "single service includes shared events", shared: true},
		{name: "multiple members share one event", multi: true, shared: true},
		{name: "multiple members aggregate shared events once", field: "log_level", multi: true, shared: true},
		{name: "live tail default enabled logs", tailReady: true},
		{name: "live tail disabled logs", tailReady: true, tailMode: "disabled"},
		{name: "live tail default enabled statistics", field: "service", tailReady: true},
		{name: "live tail disabled statistics", field: "service", tailReady: true, tailMode: "disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOG_LIVE_TAIL_MODE", tc.tailMode)
			type capturedRequest struct {
				path, query string
				body        map[string]any
			}
			requests := make(chan capturedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode ES request: %v", err)
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- capturedRequest{request.URL.Path, request.URL.RawQuery, body}
				writer.Header().Set("Content-Type", "application/json")
				if tc.field == "" && tc.shared {
					_, _ = writer.Write([]byte(`{"hits":{"total":{"value":1},"hits":[{"_id":"shared-event","_source":{"shared_source_id":"5","shared_source_name":"MQ error"}}]}}`))
				} else if tc.field == "" {
					_, _ = writer.Write([]byte(`{"hits":{"total":{"value":0},"hits":[]}}`))
				} else if tc.options == "true" {
					_, _ = writer.Write([]byte(`{"aggregations":{"by_field":{"buckets":[{"key":"ERROR","doc_count":7}]}}}`))
				} else {
					_, _ = writer.Write([]byte(`{"aggregations":{"by_field":{"buckets":[{"key":"ERROR","doc_count":7,"sample":{"hits":{"hits":[{"_id":"sample","_source":{"service":"aos"}}]}},"trend":{"buckets":[{"key_as_string":"2026-10-12T00:00:00Z","doc_count":7}]}}]}}}`))
				}
			}))
			defer server.Close()
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectQuery(regexp.QuoteMeta(elasticsearchClusterQuery)).WillReturnRows(sqlmock.NewRows(
				[]string{"id", "hosts", "username", "password", "verify_tls", "ca_cert", "index_prefix", "request_timeout", "enabled"},
			).AddRow(int64(1), server.URL, "", "", false, "", "ops", 5, true))
			if tc.multi {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT s.id, p.code AS project_code")).
					WithArgs(int64(24), int64(25)).
					WillReturnRows(sqlmock.NewRows([]string{"id", "project_code", "environment_code", "business_system_code", "service_code"}).
						AddRow(int64(24), "yilake", "test", "gateway", "aos").
						AddRow(int64(25), "other", "test", "gateway", "aos"))
			} else {
				mock.ExpectQuery(regexp.QuoteMeta(logRefreshDimsQuery)).
					WithArgs(int64(24)).
					WillReturnRows(sqlmock.NewRows([]string{"project_code", "environment_code", "business_system_code", "service_code"}).
						AddRow("yilake", "test", "gateway", "aos"))
			}
			scopeRows := sqlmock.NewRows([]string{"id", "project_code", "business_system_code", "environment_code", "tier_code"})
			if tc.shared {
				scopeRows.AddRow(5, "yilake", "gateway", "test", "hot")
			}
			mock.ExpectQuery("SELECT DISTINCT x.id,p.code").WillReturnRows(scopeRows)
			secrets, err := assets.NewSecretEncryptor("", "test-secret")
			if err != nil {
				t.Fatal(err)
			}
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			handler := &Handler{db: database, secrets: secrets}
			if tc.tailReady {
				handler.ConfigureLiveTail(identity.NewTokenManager("test-secret", time.Hour), []string{"*"})
			}
			endpoint := "/clusters/:id/log-search/"
			action := handler.ElasticsearchLogSearch
			if tc.field != "" {
				endpoint = "/clusters/:id/log-facet-stats/"
				action = handler.ElasticsearchLogFacetStats
			}
			engine.GET(endpoint, action)
			uri := strings.Replace(endpoint, ":id", "1", 1) + "?start=2026-10-11T12:00:00Z&end=2026-10-12T00:00:00Z"
			if tc.multi {
				uri += "&application_service_ids=24,25"
			} else {
				uri += "&application_service_id=24"
			}
			if tc.field != "" {
				uri += "&field=" + tc.field
			}
			if tc.options != "" {
				uri += "&options_only=" + tc.options
			}
			if tc.filterPath {
				uri += "&log_path=/logs/app.log"
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, uri, nil))
			var result struct {
				Code int `json:"code"`
				Data struct {
					LiveTailEnabled *bool            `json:"live_tail_enabled"`
					Results         []map[string]any `json:"results"`
					Count           int              `json:"count"`
					Buckets         []struct {
						Value  string           `json:"value"`
						Count  int              `json:"count"`
						Sample map[string]any   `json:"sample"`
						Trend  []map[string]any `json:"trend"`
					} `json:"buckets"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.Code != 200 {
				t.Fatalf("API response = %s, decode error = %v", recorder.Body.String(), err)
			}
			if result.Data.LiveTailEnabled == nil || *result.Data.LiveTailEnabled != (tc.tailReady && tc.tailMode != "disabled") {
				t.Fatalf("live_tail_enabled must reflect server availability: %s", recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			var captured capturedRequest
			select {
			case captured = <-requests:
			default:
				t.Fatal("no Elasticsearch request received")
			}
			wantPath := "/ops-yilake-gateway-test-aos-*/_search"
			if tc.multi {
				wantPath = "/ops-yilake-gateway-test-aos-*,ops-other-gateway-test-aos-*/_search"
			}
			if tc.shared {
				wantPath = strings.TrimSuffix(wantPath, "/_search") + ",ops-yilake-gateway-test-shared-source-5-*/_search"
			}
			if captured.path != wantPath || !strings.Contains(captured.query, "allow_no_indices=true") ||
				!strings.Contains(captured.query, "ignore_unavailable=true") {
				t.Fatalf("search target = %s?%s, want %s with empty-index handling", captured.path, captured.query, wantPath)
			}
			query, err := json.Marshal(captured.body["query"])
			if err != nil {
				t.Fatal(err)
			}
			if tc.shared && strings.Count(string(query), `"shared_source_id":["5"]`) != 1 {
				t.Fatalf("shared scope must occur once across selected services: %s", query)
			}
			for _, want := range []string{`"gte":"2026-10-11T12:00:00Z"`, `"lte":"2026-10-12T00:00:00Z"`, `"project":"yilake"`, `"service":"aos"`} {
				if !strings.Contains(string(query), want) {
					t.Errorf("query lost %s: %s", want, query)
				}
				if tc.filterPath && !strings.Contains(string(query), `"log_path":"/logs/app.log"`) {
					t.Fatalf("path filtering must use the indexed real path: %s", query)
				}
				if captured.body["runtime_mappings"] != nil {
					t.Fatal("path queries must use the keyword index, not runtime source scans")
				}
			}
			if tc.field == "" {
				if tc.shared && (result.Data.Count != 1 || len(result.Data.Results) != 1 ||
					result.Data.Results[0]["shared_source_name"] != "MQ error") {
					t.Fatalf("one shared event with its source label expected: %+v", result.Data)
				}
				return
			}
			aggs := captured.body["aggs"].(map[string]any)
			fieldAgg := aggs["by_field"].(map[string]any)
			details, hasDetails := fieldAgg["aggs"]
			if hasDetails != (tc.options != "true") {
				t.Fatalf("nested sample/trend aggregations = %v, options_only=%s", hasDetails, tc.options)
			}
			if hasDetails {
				inner := details.(map[string]any)
				if inner["sample"] == nil || inner["trend"] == nil {
					t.Fatal("statistics lost sample or trend")
				}
			}
			if captured.body["track_total_hits"] != false {
				t.Fatal("facets must not calculate unused total hits")
			}
			if len(result.Data.Buckets) != 1 || result.Data.Buckets[0].Value != "ERROR" || result.Data.Buckets[0].Count != 7 {
				t.Fatalf("unexpected buckets: %+v", result.Data.Buckets)
			}
			bucket := result.Data.Buckets[0]
			if tc.options == "true" {
				if bucket.Sample != nil || bucket.Trend == nil || len(bucket.Trend) != 0 {
					t.Fatalf("options response must retain null sample and empty trend: %+v", bucket)
				}
			} else if bucket.Sample["id"] != "sample" || len(bucket.Trend) != 1 {
				t.Fatalf("full statistics lost sample/trend: %+v", bucket)
			}
			if tc.field == "log_path" && fieldAgg["terms"].(map[string]any)["field"] != "log_path" {
				t.Fatal("path options must aggregate the log_path keyword index")
			}
		})
	}
}

func TestLogFacetRejectsInvalidOptionsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/facet", (&Handler{}).ElasticsearchLogFacetStats)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/facet?field=service&options_only=invalid", nil))
	if !strings.Contains(recorder.Body.String(), "options_only must be a boolean") {
		t.Fatalf("invalid flag was not rejected: %s", recorder.Body.String())
	}
}
