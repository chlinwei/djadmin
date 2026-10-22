package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogRevocationRejectsForeignServiceBeforeRemovingAnyInput(t *testing.T) {
	dir := t.TempDir()
	row := logRevocation{"project": "nkg", "business_system": "imf", "environment": "test", "service": "imf-mysql"}
	input := "- type: filestream\n  fields:\n    project: nkg\n    business_system: imf\n    environment: test\n    service: imf-mysql\n"
	names := []string{"mysql__imf-mysql__29__error.yml", "imf-adapter__imf-adapteri__31__error_log.yml"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := revokeLogInputs(dir, "29", row); err == nil {
		t.Fatal("foreign service filename must prevent removal despite matching fields")
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("preflight failure must preserve %s: %v", name, err)
		}
	}
}

func TestLogRevocationPreservesOtherIMFServices(t *testing.T) {
	dir := t.TempDir()
	row := logRevocation{"project": "nkg", "business_system": "imf", "environment": "test", "service": "imf-mysql"}
	services := map[string]string{
		"mysql__imf-mysql__29__error.yml":              "imf-mysql",
		"imf-adapter__imf-adapterp__30__error_log.yml": "imf-adapterp",
		"imf-adapter__imf-adapteri__31__error_log.yml": "imf-adapteri",
	}
	for name, service := range services {
		raw := "- type: filestream\n  fields:\n    project: nkg\n    business_system: imf\n    environment: test\n    service: " + service + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := revokeLogInputs(dir, "29", row); err != nil {
			t.Fatal(err)
		}
	}
	for name, service := range services {
		_, err := os.Stat(filepath.Join(dir, name))
		if service == "imf-mysql" {
			if !os.IsNotExist(err) {
				t.Fatalf("target input must be removed: %v", err)
			}
		} else if err != nil {
			t.Fatalf("other service %s must survive: %v", service, err)
		}
	}
}

func TestLogRevocationPreservesSharedHostInputsAndSurvivesReload(t *testing.T) {
	dir, err := os.MkdirTemp(".", "service-revocation-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	row := logRevocation{"project": "p", "business_system": "b", "environment": "prod", "service": "SVC"}
	input := "- type: filestream\n  fields:\n    project: 'p'\n    business_system: 'b'\n    environment: 'prod'\n    service: 'SVC'\n"
	other := "- type: filestream\n  fields:\n    project: 'p'\n    business_system: 'other'\n    environment: 'prod'\n    service: 'SVC'\n"
	for name, value := range map[string]string{"app__SVC__7__error.yml": input, "app__SVC__8__error.yml": other, "source.log": "original log"} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err = revokeLogInputs(dir, "7", row); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "app__SVC__7__error.yml")); !os.IsNotExist(err) {
		t.Fatal("revoked input still exists")
	}
	for _, name := range []string{"app__SVC__8__error.yml", "source.log"} {
		if _, err = os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("unrelated file removed: %s: %v", name, err)
		}
	}
	persisted, err := loadLogRevocations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if revoked, err := revokedLogFragment([]byte(input), persisted); err != nil || !revoked {
		t.Fatalf("stale apply after agent restart must remain revoked: %v %v", revoked, err)
	}
	if revoked, err := revokedLogFragment([]byte(other), persisted); err != nil || revoked {
		t.Fatalf("other service must remain collectable: %v %v", revoked, err)
	}
	if revoked, err := revokedManagedLogFile("app__SVC__7__error.yml", []byte(input), persisted); err != nil || !revoked {
		t.Fatalf("stale target must remain revoked: %v %v", revoked, err)
	}
	if _, err := revokedManagedLogFile("app__SVC__8__error.yml", []byte(input), persisted); err == nil {
		t.Fatal("stale apply must reject foreign filename with revoked fields")
	}
	if err = revokeLogInputs(dir, "7", row); err != nil {
		t.Fatalf("retry must be idempotent: %v", err)
	}
	if _, err = revokedLogFragment([]byte(input+other), persisted); err == nil {
		t.Fatal("mixed ownership must fail safely")
	}
}
