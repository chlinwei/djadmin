package inspection

import "testing"

func TestValidateGossSpec(t *testing.T) {
	valid := "file:\n  /etc/hosts:\n    exists: true\n    mode: \"0644\"\n"
	if err := validateGossSpec(valid); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	invalid := "file:\n  /etc/hosts:\n    exists: \"not-a-boolean\"\n"
	if err := validateGossSpec(invalid); err == nil {
		t.Fatal("invalid spec must be rejected")
	}
	broken := "file: [unclosed"
	if err := validateGossSpec(broken); err == nil {
		t.Fatal("broken yaml must be rejected")
	}
}
