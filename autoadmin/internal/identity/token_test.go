package identity

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenRoundTrip(t *testing.T) {
	manager := NewTokenManager("test-secret", time.Hour)
	rawToken, err := manager.Issue(42, "operator", []string{"assets:hosts:view"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	claims, err := manager.Parse(rawToken)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.UserID != 42 || claims.Username != "operator" {
		t.Fatalf("unexpected identity claims: %+v", claims)
	}
	if len(claims.Perms) != 1 || claims.Perms[0] != "assets:hosts:view" {
		t.Fatalf("unexpected permissions: %v", claims.Perms)
	}

	var payload map[string]any
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if _, exists := payload["iat"]; exists {
		t.Fatal("Django-compatible token must not include iat")
	}
}

func TestTokenRejectsExpiredAndWrongAlgorithm(t *testing.T) {
	manager := NewTokenManager("test-secret", -time.Minute)
	expired, err := manager.Issue(1, "expired", nil)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, err := manager.Parse(expired); err == nil {
		t.Fatal("expected expired token to fail")
	}

	claims := Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	wrongAlgorithm, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("sign wrong-algorithm token: %v", err)
	}
	if _, err := manager.Parse(wrongAlgorithm); err == nil {
		t.Fatal("expected non-HS256 token to fail")
	}
}
