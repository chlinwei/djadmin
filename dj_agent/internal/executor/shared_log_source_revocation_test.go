package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSharedSourceRevocationIsEpochSpecificAndSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	retired := "- type: filestream\n  fields:\n    shared_source_id: '5'\n    shared_source_generation: '7'\n"
	restarted := "- type: filestream\n  fields:\n    shared_source_id: '5'\n    shared_source_generation: '8'\n"
	service := "- type: filestream\n  fields:\n    project: 'p'\n    business_system: 'imf'\n    environment: 'test'\n    service: 'mq-a'\n"
	files := map[string]string{
		"general__shared-source-5__shared-5-7__shared.yml": retired,
		"general__shared-source-5__shared-5-8__shared.yml": restarted,
		"mq__mq-a__32__error.yml":                          service,
		"AMQERR01.LOG":                                     "original file",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	token, err := sharedLogRevocationToken("5", "7")
	if err != nil {
		t.Fatal(err)
	}
	row := logRevocation{"shared_source_id": "5", "shared_source_generation": "7"}
	for range 2 {
		if err = revokeLogInputs(dir, token, row); err != nil {
			t.Fatal(err)
		}
	}
	for name := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		if name == "general__shared-source-5__shared-5-7__shared.yml" {
			if !os.IsNotExist(err) {
				t.Fatalf("retired epoch still present: %v", err)
			}
		} else if err != nil {
			t.Fatalf("unrelated input/source file %s must survive: %v", name, err)
		}
	}
	persisted, err := loadLogRevocations(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if filepath.Ext(name) != ".yml" {
			continue
		}
		blocked, err := revokedManagedLogFile(name, []byte(content), persisted)
		want := name == "general__shared-source-5__shared-5-7__shared.yml"
		if err != nil || blocked != want {
			t.Fatalf("stale apply/restarted epoch isolation %s: blocked=%v err=%v", name, blocked, err)
		}
	}
	if _, err := revokedManagedLogFile("general__shared-source-5__shared-5-8__shared.yml", []byte(retired), persisted); err == nil {
		t.Fatal("matching fields under a foreign epoch filename must fail closed")
	}
	for _, id := range []string{"../5", "05", "-1", "0"} {
		if _, err := sharedLogRevocationToken(id, "7"); err == nil {
			t.Fatalf("unsafe/noncanonical source ID accepted: %q", id)
		}
	}
}
