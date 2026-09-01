package assets

import (
	"database/sql"
	"encoding/json"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

func TestMergeHostPatchPreservesOmittedFields(t *testing.T) {
	current := populatedHostRow()
	var patch HostPatchInput
	if err := json.Unmarshal([]byte(`{"status":"stopped"}`), &patch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}

	merged, err := mergeHostPatch(current, patch)
	if err != nil {
		t.Fatalf("merge patch: %v", err)
	}
	if merged.Status != "stopped" || merged.InstanceName == nil || *merged.InstanceName != "host-01" || merged.IP == nil || *merged.IP != "10.0.0.1" {
		t.Fatalf("status-only patch did not preserve host fields: %+v", merged)
	}
	if !merged.IsDeletedInCloud || merged.WebSSHDefaultUsername != "admin" || merged.WebSSHLoginUsers != "admin root" {
		t.Fatalf("status-only patch did not preserve scalar fields: %+v", merged)
	}
}

func TestMergeHostPatchAllowsExplicitNullForNullableField(t *testing.T) {
	current := populatedHostRow()
	var patch HostPatchInput
	if err := json.Unmarshal([]byte(`{"agent_id":null}`), &patch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}

	merged, err := mergeHostPatch(current, patch)
	if err != nil {
		t.Fatalf("merge patch: %v", err)
	}
	if merged.AgentID != nil {
		t.Fatalf("agent_id = %v, want nil", *merged.AgentID)
	}
}

func populatedHostRow() db.GetHostRow {
	return db.GetHostRow{
		Remark:                sql.NullString{String: "remark", Valid: true},
		Status:                "running",
		InstanceID:            sql.NullString{String: "i-01", Valid: true},
		Ip:                    sql.NullString{String: "10.0.0.1", Valid: true},
		IsDeletedInCloud:      true,
		CloudAccountID:        sql.NullInt64{Int64: 1, Valid: true},
		GroupID:               sql.NullInt64{Int64: 2, Valid: true},
		InstanceName:          sql.NullString{String: "host-01", Valid: true},
		WebsshDefaultUsername: "admin",
		WebsshLoginUsers:      "admin root",
		AgentID:               sql.NullString{String: "agent-01", Valid: true},
		EnvironmentID:         sql.NullInt64{Int64: 3, Valid: true},
	}
}
