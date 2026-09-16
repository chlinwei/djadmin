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
	if merged.Status != "stopped" || merged.InstanceName != "host-01" || merged.IP == nil || *merged.IP != "10.0.0.1" {
		t.Fatalf("status-only patch did not preserve host fields: %+v", merged)
	}
	if !merged.IsDeletedInCloud || merged.WebSSHDefaultUsername != "admin" || merged.WebSSHLoginUsers != "admin root" {
		t.Fatalf("status-only patch did not preserve scalar fields: %+v", merged)
	}
}

// instance_name 是主机的业务标识（也是 dj-agent 的全局标识），显式置 null 必须报错，
// 不能静默退化成空串。
func TestMergeHostPatchRejectsNullInstanceName(t *testing.T) {
	current := populatedHostRow()
	var patch HostPatchInput
	if err := json.Unmarshal([]byte(`{"instance_name":null}`), &patch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}

	if _, err := mergeHostPatch(current, patch); err != ErrInvalid {
		t.Fatalf("merge patch error = %v, want ErrInvalid", err)
	}
}

func TestMergeHostPatchAllowsExplicitNullForNullableField(t *testing.T) {
	current := populatedHostRow()
	var patch HostPatchInput
	if err := json.Unmarshal([]byte(`{"instance_id":null}`), &patch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}

	merged, err := mergeHostPatch(current, patch)
	if err != nil {
		t.Fatalf("merge patch: %v", err)
	}
	if merged.InstanceID != nil {
		t.Fatalf("instance_id = %v, want nil", *merged.InstanceID)
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
		EnvironmentID:         sql.NullInt64{Int64: 3, Valid: true},
	}
}
