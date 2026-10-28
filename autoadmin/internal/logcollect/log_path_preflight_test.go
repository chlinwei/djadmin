package logcollect

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
	"autoadmin/internal/assets"

	"github.com/DATA-DOG/go-sqlmock"
)

func testPathReceipt(fragments []logConfigFragment, inputs int) string {
	raw, _ := json.Marshal(fragments)
	data, _ := json.Marshal(map[string]any{"path_check": logPathCheckReceipt{
		Version: 1, Checked: true, FilesDigest: fmt.Sprintf("%x", sha256.Sum256(raw)),
		InputsChecked: inputs, MatchedFiles: inputs, UnmatchedPatterns: []string{},
	}})
	return string(data)
}

func testPathResponse(request *pb.AutomationExecuteRequest, status, data, message string) *pb.AgentFrame {
	return &pb.AgentFrame{Payload: &pb.AgentFrame_AutomationExecuteResponse{
		AutomationExecuteResponse: &pb.AutomationExecuteResponse{
			RequestId: request.RequestId, JobId: request.JobId, Status: status,
			ResultDataJson: data, ErrorMessage: message,
		},
	}}
}

func expectPathApplyCluster(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM monitor_elasticsearch_cluster").WillReturnRows(
		sqlmock.NewRows([]string{"hosts", "username", "password", "index_prefix", "verify_tls"}).
			AddRow("http://es:9200", "", "", "logs", false))
}

func TestLogPathPreflightFailsBeforeAnySideEffect(t *testing.T) {
	for _, failure := range []string{"overlap", "unsupported", "missing-receipt", "malformed-receipt", "wrong-digest"} {
		t.Run(failure, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			requests := []string{}
			gateway := startTestAgent(t, func(frame *pb.ServerFrame) *pb.AgentFrame {
				request := frame.GetAutomationExecuteRequest()
				if request == nil {
					return nil
				}
				requests = append(requests, request.Action)
				status, data, message := "failed", "", failure
				switch failure {
				case "missing-receipt":
					status, data = "success", "{}"
				case "malformed-receipt":
					status, data = "success", "{"
				case "wrong-digest":
					status, data = "success", testPathReceipt([]logConfigFragment{}, 0)
				}
				return testPathResponse(request, status, data, message)
			})
			expectPathApplyCluster(mock)
			expectServiceConflictInputs(mock, "/data/app/logs/other.log")
			handler := &Handler{db: pool, gateway: gateway}
			result, err := handler.applyLogTargetConfigRow(context.Background(), logTargetRow{
				ID: 1, HostID: 42, HostName: "host-01",
			})
			if err == nil || result != nil || !strings.Contains(err.Error(), "已拒绝下发") ||
				len(requests) != 1 || requests[0] != "check_filebeat_paths" {
				t.Fatalf("preflight failure must prevent ES/config/fingerprint changes: %v %v requests=%v", result, err, requests)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLogPathPreflightRunsBeforeFingerprintSkip(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	requests := []string{}
	gateway := startTestAgent(t, func(frame *pb.ServerFrame) *pb.AgentFrame {
		request := frame.GetAutomationExecuteRequest()
		if request == nil {
			return nil
		}
		requests = append(requests, request.Action)
		return testPathResponse(request, "success", testPathReceipt([]logConfigFragment{}, 0), "")
	})
	expectPathApplyCluster(mock)
	mock.ExpectQuery("FROM assets_application_service_deployment").WillReturnRows(sqlmock.NewRows([]string{"host_id"}))
	mock.ExpectQuery("FROM assets_application_service s").WillReturnRows(sqlmock.NewRows([]string{"host_id"}))
	identity, err := filebeatOutputIdentity("http://es:9200", "", false)
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderHostLogConfig(nil, nil, identity)
	mock.ExpectQuery("SELECT COALESCE\\(config_fingerprint").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"config_fingerprint"}).AddRow(rendered.Fingerprint))
	mock.ExpectQuery("SELECT COALESCE\\(service_fingerprints").
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"service_fingerprints"}).AddRow([]byte(`{}`)))
	mock.ExpectExec("UPDATE monitor_log_collection_target SET last_applied_time").
		WillReturnResult(sqlmock.NewResult(0, 1))
	result, err := (&Handler{db: pool, gateway: gateway}).applyLogTargetConfigRow(context.Background(),
		logTargetRow{ID: 1, HostID: 42, HostName: "host-01"})
	if err != nil || result["skipped"] != true || len(requests) != 1 || requests[0] != "check_filebeat_paths" ||
		result["path_check"] == nil {
		t.Fatalf("unchanged fingerprint must still be checked: %v %v requests=%v", result, err, requests)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLogPathCheckedApplyUsesOneConfigAction(t *testing.T) {
	for _, failedRecheck := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "apply-recheck-fails"}[failedRecheck], func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			requests := []string{}
			gateway := startTestAgent(t, func(frame *pb.ServerFrame) *pb.AgentFrame {
				request := frame.GetAutomationExecuteRequest()
				if request == nil {
					return nil
				}
				requests = append(requests, request.Action)
				var params struct {
					Files  []logConfigFragment   `json:"files"`
					Output *filebeatOutputParams `json:"output"`
				}
				if err := json.Unmarshal([]byte(request.ParamsJson), &params); err != nil {
					t.Error(err)
				}
				if request.Action == "apply_filebeat_config" {
					if params.Output == nil || params.Output.URL != "http://es:9200" {
						t.Error("main output config must accompany inputs in a single checked apply")
					}
					if failedRecheck {
						return testPathResponse(request, "failed", "", "log file overlap after preflight")
					}
				} else if request.Action != "check_filebeat_paths" {
					t.Errorf("unexpected config action: %s", request.Action)
				}
				return testPathResponse(request, "success", testPathReceipt(params.Files, 2), "")
			})
			expectPathApplyCluster(mock)
			expectServiceConflictInputs(mock, "/data/app/logs/other.log")
			for _, serviceID := range []int64{10, 20} {
				mock.ExpectQuery("name: ListServiceExtraPipelineRows").
					WithArgs(serviceID).WillReturnRows(sqlmock.NewRows([]string{"service_id"}))
			}
			mock.ExpectQuery("SELECT COALESCE\\(config_fingerprint").
				WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"config_fingerprint"}).AddRow(""))
			mock.ExpectQuery("SELECT COALESCE\\(service_fingerprints").
				WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"service_fingerprints"}).AddRow([]byte(`{}`)))
			if !failedRecheck {
				mock.ExpectExec("UPDATE monitor_log_collection_target SET last_applied_time").
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			secrets, err := assets.NewSecretEncryptor("", "test-secret")
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&Handler{db: pool, gateway: gateway, secrets: secrets}).applyLogTargetConfigRow(
				context.Background(), logTargetRow{ID: 1, HostID: 42, HostName: "host-01"})
			if failedRecheck {
				if err == nil || result != nil || !strings.Contains(err.Error(), "未记录同步指纹") {
					t.Fatalf("failed apply-time check must not mark synced: %v %v", result, err)
				}
			} else if err != nil || result["skipped"] != false || result["path_check"] == nil {
				t.Fatalf("checked apply failed: %v %v", result, err)
			}
			if len(requests) != 2 || requests[0] != "check_filebeat_paths" || requests[1] != "apply_filebeat_config" {
				t.Fatalf("expected read-only preflight then one checked apply, got %v", requests)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLogPathPreflightRejectsOfflineAndCanceled(t *testing.T) {
	gateway := agent.NewGateway(nil, nil)
	_, err := (&Handler{gateway: gateway}).applyLogTargetConfigRow(context.Background(), logTargetRow{HostName: "offline"})
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("offline host must reject apply: %v", err)
	}
	gateway = startTestAgent(t, func(*pb.ServerFrame) *pb.AgentFrame { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Handler{gateway: gateway}).checkLogTargetPaths(ctx, logTargetRow{HostName: "host-01"}, []logConfigFragment{}); err == nil {
		t.Fatal("canceled preflight must not pass")
	}
}

func TestLogPathReceiptRequiresCompleteMatchingCheck(t *testing.T) {
	files := []logConfigFragment{{Path: "/etc/filebeat/inputs.d/a.yml",
		Content: "- type: filestream\n  id: a\n  paths: ['/logs/app.log']\n- type: filestream\n  id: b\n  paths: ['/logs/other.log']\n"}}
	valid := testPathReceipt(files, 2)
	for _, invalid := range []string{
		`{}`, `{"path_check":null}`,
		strings.Replace(valid, `"checked":true`, `"checked":false`, 1),
		strings.Replace(valid, `"version":1`, `"version":2`, 1),
		strings.Replace(valid, `"inputs_checked":2`, `"inputs_checked":1`, 1),
		strings.Replace(valid, `"inputs_checked":2`, `"inputs_checked":3`, 1),
		strings.Replace(valid, `"matched_files":2`, `"matched_files":-1`, 1),
		strings.Replace(valid, `"unmatched_patterns":[]`, `"unmatched_patterns":null`, 1),
	} {
		if _, err := validateLogPathReceipt(&pb.AutomationExecuteResponse{Status: "success", ResultDataJson: invalid}, files); err == nil {
			t.Errorf("incomplete or mismatched receipt must fail: %s", invalid)
		}
	}
	receipt, err := validateLogPathReceipt(&pb.AutomationExecuteResponse{Status: "success", ResultDataJson: valid}, files)
	if err != nil || !receipt.Checked || receipt.InputsChecked != 2 {
		t.Fatalf("complete receipt rejected: %+v %v", receipt, err)
	}
	receipt.UnmatchedPatterns = []string{"/logs/future.log"}
	if warnings := logPathCheckWarnings(receipt); len(warnings) != 1 || !strings.Contains(warnings[0], "/logs/future.log") {
		t.Fatalf("unmatched patterns must be explicitly surfaced: %v", warnings)
	}
}
