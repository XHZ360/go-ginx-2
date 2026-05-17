package enrollment

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServiceRedeemsJoinTokenOnce(t *testing.T) {
	ctx := context.Background()
	db := enrollmentTestStore(t)
	now := time.Now().UTC()
	token := enrollmentTestToken(t, db, now.Add(time.Hour))

	service := Service{Store: db, Now: func() time.Time { return now }}
	response, err := service.Redeem(ctx, token)
	if err != nil {
		t.Fatalf("redeem token: %v", err)
	}
	if response.ClientID != "client-1" || response.Credential != "secret" || response.ServerName != "go-ginx-control.test" {
		t.Fatalf("unexpected redeem response: %+v", response)
	}

	if _, err := service.Redeem(ctx, token); err == nil {
		t.Fatal("expected duplicate redeem to fail")
	} else if HTTPStatusForError(err) != 409 {
		t.Fatalf("expected duplicate redeem conflict, got %v", err)
	}
}

func TestServiceRejectsExpiredJoinToken(t *testing.T) {
	ctx := context.Background()
	db := enrollmentTestStore(t)
	now := time.Now().UTC()
	token := enrollmentTestToken(t, db, now.Add(-time.Minute))

	service := Service{Store: db, Now: func() time.Time { return now }}
	if _, err := service.Redeem(ctx, token); err == nil {
		t.Fatal("expected expired token to fail")
	} else if !isStoreNotFound(err) {
		t.Fatalf("expected not found style error for expired token, got %v", err)
	}
}

func enrollmentTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "enrollment.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := db.Users().Create(ctx, domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}); err != nil {
		t.Fatal(err)
	}
	if err := db.Clients().Create(ctx, domain.Client{ID: "client-1", UserID: "user-1", Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}); err != nil {
		t.Fatal(err)
	}
	return db
}

func enrollmentTestToken(t *testing.T, db *sqlite.Store, expiresAt time.Time) string {
	t.Helper()
	payload := TokenPayload{
		EnrollmentID:     "join-1",
		Secret:           "join-secret",
		EnrollmentURL:    "http://127.0.0.1:8080/api/client/enroll",
		ServerAddress:    "127.0.0.1:8443",
		ServerTLSAddress: "127.0.0.1:9443",
		ServerName:       "go-ginx-control.test",
		CAPEM:            "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
		ClientID:         "client-1",
		Credential:       "secret",
		AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS},
		Reconnect:        config.DefaultClient().Reconnect,
		ExpiresAt:        expiresAt,
	}
	token, err := EncodeToken(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ClientEnrollments().Create(context.Background(), domain.ClientEnrollment{ID: payload.EnrollmentID, ClientID: payload.ClientID, SecretHash: HashSecret(payload.Secret), TokenHash: HashToken(token), ExpiresAt: expiresAt}); err != nil {
		t.Fatal(err)
	}
	return token
}

func isStoreNotFound(err error) bool {
	return errors.Is(err, store.ErrNotFound) && HTTPStatusForError(err) == 401
}
