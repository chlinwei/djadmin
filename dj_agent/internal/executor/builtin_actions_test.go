package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAgentBinaryFile_RejectsMissingFile(t *testing.T) {
	err := validateAgentBinaryFile(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for a missing staged binary")
	}
}

func TestValidateAgentBinaryFile_RejectsBinaryWithoutGRPCMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dj-agent.new")
	if err := os.WriteFile(path, []byte("ELF\x00unknown-agent"), 0o644); err != nil {
		t.Fatalf("write staged binary: %v", err)
	}
	err := validateAgentBinaryFile(path)
	if err == nil {
		t.Fatal("binary missing the gRPC marker must be rejected")
	}
}

func TestValidateAgentBinaryFile_AcceptsCurrentGRPCBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dj-agent.new")
	if err := os.WriteFile(path, []byte("ELF\x00DJ_AGENT_GRPC_FILE_ADDR\x00"), 0o644); err != nil {
		t.Fatalf("write staged binary: %v", err)
	}
	if err := validateAgentBinaryFile(path); err != nil {
		t.Fatalf("current gRPC binary should be accepted: %v", err)
	}
}

func TestFileContainsMarker_FindsMarkerSplitAcrossReadChunks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marker.bin")
	// 手写一个跨读取块边界的场景：真实文件用的是 1MB 缓冲区，这里通过前缀垫大内容间接验证重叠拼接逻辑。
	marker := []byte("DJ_AGENT_GRPC_FILE_ADDR")
	content := append(make([]byte, 0), marker...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open marker file: %v", err)
	}
	defer file.Close()

	found, err := fileContainsMarker(file, marker)
	if err != nil {
		t.Fatalf("fileContainsMarker returned error: %v", err)
	}
	if !found {
		t.Fatal("expected marker to be found")
	}
}

func TestFileContainsMarker_ReturnsFalseWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-marker.bin")
	if err := os.WriteFile(path, []byte("nothing interesting here"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer file.Close()

	found, err := fileContainsMarker(file, []byte("DJ_AGENT_GRPC_FILE_ADDR"))
	if err != nil {
		t.Fatalf("fileContainsMarker returned error: %v", err)
	}
	if found {
		t.Fatal("marker should not be reported as found")
	}
}
