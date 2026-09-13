package assets

import (
	"database/sql"
	"errors"
	"testing"
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

func TestBatchDeleteAgentResultsStructure(t *testing.T) {
	results, okCount := batchDeleteAgentResults([]int64{1, 2, 3}, func(id int64) error {
		if id == 2 {
			return sql.ErrNoRows
		}
		if id == 3 {
			return errors.New("disk error")
		}
		return nil
	})
	if okCount != 1 {
		t.Fatalf("expected 1 succeeded delete, got %d", okCount)
	}
	if len(results) != 3 {
		t.Fatalf("expected one result per id, got %d", len(results))
	}
	if results[0]["id"] != int64(1) || results[0]["ok"] != true || results[0]["message"] != "" {
		t.Fatalf("unexpected success entry: %v", results[0])
	}
	if results[1]["ok"] != false || results[1]["message"] != "resource not found" {
		t.Fatalf("unexpected missing entry: %v", results[1])
	}
	if results[2]["ok"] != false || results[2]["message"] != "disk error" {
		t.Fatalf("unexpected failure entry: %v", results[2])
	}
}
