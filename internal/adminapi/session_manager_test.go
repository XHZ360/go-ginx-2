package adminapi

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSessionManagerSignsAndVerifiesJWT(t *testing.T) {
	now := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	manager, err := newSessionManager(testAdminJWTSecret(), 8*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new session manager: %v", err)
	}
	session, err := manager.Create("admin")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID == "" || !strings.Contains(session.ID, ".") || session.CSRFToken == "" {
		t.Fatalf("expected jwt session with csrf token: %+v", session)
	}
	authenticated, ok := manager.Get(session.ID)
	if !ok {
		t.Fatal("expected jwt to verify")
	}
	if authenticated.Username != "admin" || authenticated.CSRFToken != session.CSRFToken {
		t.Fatalf("unexpected authenticated session: %+v", authenticated)
	}

	now = now.Add(7 * time.Hour)
	if _, ok := manager.Get(session.ID); !ok {
		t.Fatal("expected jwt to remain valid before absolute expiry")
	}
	now = now.Add(time.Hour + time.Second)
	if _, ok := manager.Get(session.ID); ok {
		t.Fatal("expected jwt to expire after absolute lifetime")
	}
}

func TestSessionManagerRejectsInvalidJWTs(t *testing.T) {
	now := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	manager, err := newSessionManager(testAdminJWTSecret(), 8*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new session manager: %v", err)
	}
	validClaims := adminJWTClaims{Type: adminJWTType, Version: adminJWTVersion, Subject: "admin", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), CSRFToken: "csrf"}
	validToken, err := manager.sign(validClaims)
	if err != nil {
		t.Fatalf("sign valid token: %v", err)
	}
	otherManager, err := newSessionManager([]byte("fedcba9876543210fedcba9876543210"), 8*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new other manager: %v", err)
	}
	wrongSignatureToken, err := otherManager.sign(validClaims)
	if err != nil {
		t.Fatalf("sign wrong token: %v", err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "malformed", token: "not-a-jwt"},
		{name: "wrong signature", token: wrongSignatureToken},
		{name: "tampered signature", token: validToken + "x"},
		{name: "none algorithm", token: tokenWithHeader(t, manager, adminJWTHeader{Algorithm: "none", Type: "JWT"}, validClaims)},
		{name: "wrong type", token: tokenWithClaims(t, manager, adminJWTClaims{Type: "client", Version: adminJWTVersion, Subject: "admin", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), CSRFToken: "csrf"})},
		{name: "missing subject", token: tokenWithClaims(t, manager, adminJWTClaims{Type: adminJWTType, Version: adminJWTVersion, IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), CSRFToken: "csrf"})},
		{name: "missing csrf", token: tokenWithClaims(t, manager, adminJWTClaims{Type: adminJWTType, Version: adminJWTVersion, Subject: "admin", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix()})},
		{name: "expired", token: tokenWithClaims(t, manager, adminJWTClaims{Type: adminJWTType, Version: adminJWTVersion, Subject: "admin", IssuedAt: now.Add(-2 * time.Hour).Unix(), ExpiresAt: now.Add(-time.Hour).Unix(), CSRFToken: "csrf"})},
		{name: "future issued at", token: tokenWithClaims(t, manager, adminJWTClaims{Type: adminJWTType, Version: adminJWTVersion, Subject: "admin", IssuedAt: now.Add(2 * time.Minute).Unix(), ExpiresAt: now.Add(time.Hour).Unix(), CSRFToken: "csrf"})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := manager.Get(test.token); ok {
				t.Fatalf("expected token %q to be rejected", test.name)
			}
		})
	}
}

func TestSessionManagerRejectsShortSecret(t *testing.T) {
	if _, err := newSessionManager([]byte("short"), 0, time.Now); err == nil {
		t.Fatal("expected short secret error")
	}
}

func tokenWithClaims(t *testing.T, manager *sessionManager, claims adminJWTClaims) string {
	t.Helper()
	token, err := manager.sign(claims)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func tokenWithHeader(t *testing.T, manager *sessionManager, header adminJWTHeader, claims adminJWTClaims) string {
	t.Helper()
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	return unsigned + "." + manager.signature(unsigned)
}
