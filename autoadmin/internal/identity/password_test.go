package identity

import "testing"

func TestVerifyPassword(t *testing.T) {
	const encoded = "pbkdf2_sha256$870000$fixedtestsalt$DtqvdYT64Jb3nxddgUUvWXiWQV2fi+Hcd96bOt18LpI="

	if !VerifyPassword(encoded, "correct-password") {
		t.Fatal("expected Django PBKDF2-SHA256 password to verify")
	}
	if VerifyPassword(encoded, "wrong-password") {
		t.Fatal("expected incorrect password to fail")
	}
	if VerifyPassword("plaintext", "plaintext") {
		t.Fatal("plaintext fallback must not be accepted")
	}
}

func TestGeneratedAPITokenHashUsesDjangoEncoding(t *testing.T) {
	const token = "agent-shared-token"
	encoded, err := djangoHash(token)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(encoded, token) {
		t.Fatal("generated API token hash must be accepted by the Django password verifier")
	}
}
