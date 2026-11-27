package assets

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestValidateAgentBinaryRejectsLegacyRabbitMQBuild(t *testing.T) {
	data := []byte("binary blob ... connect rabbitmq failed ... DJ_AGENT_GRPC_FILE_ADDR")
	if err := validateAgentBinary(data); err == nil {
		t.Fatal("expected legacy RabbitMQ build to be rejected")
	}
}

func TestValidateAgentBinaryRequiresGRPCMarker(t *testing.T) {
	if err := validateAgentBinary([]byte("binary blob without markers")); err == nil {
		t.Fatal("expected binary without DJ_AGENT_GRPC_FILE_ADDR marker to be rejected")
	}
}

func TestValidateAgentBinaryAcceptsCurrentBuild(t *testing.T) {
	data := []byte("binary blob with DJ_AGENT_GRPC_FILE_ADDR marker")
	if err := validateAgentBinary(data); err != nil {
		t.Fatalf("expected current build to be accepted, got %v", err)
	}
}

func TestBatchDeleteAgentPackageReportsMissing(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT id,file,sha256,size_bytes,is_active,create_time FROM agent_package").
		WithArgs(uint64(1)).WillReturnError(sql.ErrNoRows)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/test", strings.NewReader(`{"ids":[1]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	NewHandler(&Service{repository: NewRepository(pool)}, nil, "").BatchDeleteAgentPackages(ctx)
	var body struct {
		Code int `json:"code"`
		Data struct {
			Count   int `json:"count"`
			Results []struct {
				Code int  `json:"code"`
				OK   bool `json:"ok"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 404 || body.Data.Count != 0 || len(body.Data.Results) != 1 ||
		body.Data.Results[0].Code != 404 || body.Data.Results[0].OK {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
