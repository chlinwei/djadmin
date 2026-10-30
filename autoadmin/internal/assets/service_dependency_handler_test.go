package assets

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/api/response"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestDependencyHandlersRejectInvalidInputBeforeDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&Service{}, nil, "")
	for _, tt := range []struct {
		name, method, path, body string
		handler                  gin.HandlerFunc
	}{
		{"missing topology scope", "GET", "/test/", "", h.GetServiceDependencyTopology},
		{"bad filter", "GET", "/test/?strength=medium", "", h.ListServiceDependencies},
		{"negative filter", "GET", "/test/?project=-1", "", h.ListServiceDependencies},
		{"bad enabled", "GET", "/test/?enabled=maybe", "", h.ListServiceDependencyTypes},
		{"empty enabled", "GET", "/test/?enabled=", "", h.ListServiceDependencyTypes},
		{"self edge", "POST", "/test/", `{"source_service_id":1,"target_service_id":1}`, h.CreateServiceDependency},
		{"duplicate ids", "POST", "/test/", `{"ids":[1,1]}`, h.BatchDeleteServiceDependencies},
		{"negative id", "POST", "/test/", `{"ids":[-1]}`, h.BatchDeleteServiceDependencyTypes},
		{"empty ids", "POST", "/test/", `{"ids":[]}`, h.BatchDeleteServiceDependencies},
		{"invalid type code", "POST", "/test/", `{"code":"Invalid Code","name":"type"}`, h.CreateServiceDependencyType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine := gin.New()
			engine.Handle(tt.method, "/test/", tt.handler)
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(recorder, req)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code == 200 || recorder.Code != 200 {
				t.Fatalf("expected business error envelope: %s", recorder.Body.String())
			}
		})
	}
}

func TestDependencyTopologyReadOnlyHTTP(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("FROM assets_project p").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "name", "code", "owner", "enabled", "business_system_names", "business_system_ids"}).
			AddRow(3, now, now, nil, "P", "p", "", true, "B", "10"))
	mock.ExpectQuery("FROM assets_business_environment WHERE").WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "name", "code", "order", "owner", "enabled"}).
			AddRow(4, now, now, nil, "生产", "prod", 0, "", true))
	mock.ExpectQuery("SELECT s.id,s.name,s.code,s.enabled,s.business_system_id,b.name").
		WithArgs(int64(3), int64(4)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "enabled", "business_system_id", "business_system_name"}).
		AddRow(1, "API", "api", true, 10, "B").AddRow(2, "DB", "db", true, 20, "Shared"))
	mock.ExpectQuery("FROM assets_service_dependency d").WillReturnRows(dependencyGetRows(11, 1, 2))
	mock.ExpectQuery("FROM assets_application_service_deployment m").
		WithArgs(int64(3), int64(4)).WillReturnRows(sqlmock.NewRows([]string{"service_id", "id", "instance_name", "host_id", "host_ip", "runtime_status", "runtime_status_output", "last_status_check_time", "ha_role"}).
		AddRow(1, 111, "api-01", 50, "10.0.0.1", "error", "agent unavailable", now, ""))
	mock.ExpectCommit()
	engine := gin.New()
	engine.GET("/topology/", NewHandler(&Service{repository: NewRepository(pool)}, nil, "").GetServiceDependencyTopology)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/topology/?project=3&environment=4&service=1", nil))
	var envelope struct {
		Code int                `json:"code"`
		Data DependencyTopology `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || len(envelope.Data.Nodes) != 2 || len(envelope.Data.Edges) != 1 {
		t.Fatalf("response: %s", recorder.Body.String())
	}
	if envelope.Data.Nodes[0].RuntimeSummary.Error != 1 || envelope.Data.Nodes[0].RuntimeSummary.Stopped != 0 {
		t.Fatal("check error must not be interpreted as stopped")
	}
	if !envelope.Data.Nodes[1].ExternalToSelection || envelope.Data.Nodes[1].Instances == nil {
		t.Fatal("external node/empty array semantics lost")
	}
	if envelope.Data.StateSource != "stored_runtime_snapshot" || envelope.Data.Nodes[0].Instances[0].LastStatusCheckTime == nil {
		t.Fatal("snapshot provenance missing")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
