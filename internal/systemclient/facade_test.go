package systemclient_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
	"github.com/simp-frp/go-ginx-2/internal/systemclient"
)

func TestEnsureCreatesSingleProtectedSystemIdentity(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := systemclient.Service{Store: db}
	first, err := service.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != systemclient.ClientID || second.ID != first.ID || first.UserID != systemclient.UserID || first.Kind != domain.ClientKindProvider {
		t.Fatalf("unexpected system client: %#v %#v", first, second)
	}
	clients, err := db.Clients().List(context.Background())
	if err != nil || len(clients) != 1 {
		t.Fatalf("expected one client, got %#v, %v", clients, err)
	}
	user, err := db.Users().ByID(context.Background(), systemclient.UserID)
	if err != nil || user.Status != domain.UserDisabled || user.PasswordHash != "" {
		t.Fatalf("unexpected system user: %#v, %v", user, err)
	}
}

func TestProtectionHelpersReturnForbidden(t *testing.T) {
	for _, err := range []error{
		systemclient.ProtectUserMutation(systemclient.UserID),
		systemclient.ProtectClientMutation(systemclient.ClientID),
		systemclient.ProtectProxyMutation(domain.Proxy{ClientID: systemclient.ClientID}),
	} {
		var contractError *contracterr.Error
		if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeForbidden {
			t.Fatalf("expected forbidden error, got %v", err)
		}
	}
}

func TestEnsureRejectsConflictingReservedIdentity(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "conflict.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := systemclient.Service{Store: db}
	if _, err := service.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Clients().SetStatus(systemclient.WithInternalMutation(ctx), systemclient.ClientID, domain.ClientDisabled); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Ensure(ctx); err == nil {
		t.Fatal("expected conflicting reserved client status to fail initialization")
	}
}

func TestSQLiteRepositoriesProtectSystemObjects(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "protected.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := (systemclient.Service{Store: db}).Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	proxy := domain.Proxy{ID: "local-1", UserID: systemclient.UserID, ClientID: systemclient.ClientID, Name: "local", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 18080, TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Proxies().Create(systemclient.WithInternalMutation(ctx), proxy); err != nil {
		t.Fatal(err)
	}

	operations := []func() error{
		func() error { return db.Users().SetStatus(ctx, systemclient.UserID, domain.UserEnabled) },
		func() error { return db.Users().SetPassword(ctx, systemclient.UserID, "hash") },
		func() error { return db.Users().Delete(ctx, systemclient.UserID) },
		func() error { return db.Clients().SetStatus(ctx, systemclient.ClientID, domain.ClientDisabled) },
		func() error { return db.Clients().RotateCredential(ctx, systemclient.ClientID, "hash") },
		func() error { return db.Clients().Delete(ctx, systemclient.ClientID) },
		func() error { return db.Proxies().SetStatus(ctx, proxy.ID, domain.ProxyDisabled) },
		func() error { return db.Proxies().Update(ctx, proxy) },
		func() error { return db.Proxies().Delete(ctx, proxy.ID) },
	}
	for index, operation := range operations {
		var contractError *contracterr.Error
		if err := operation(); !errors.As(err, &contractError) || contractError.Code != contracterr.CodeForbidden {
			t.Fatalf("operation %d: expected forbidden, got %v", index, err)
		}
	}

	if err := db.Proxies().SetStatus(systemclient.WithInternalMutation(ctx), proxy.ID, domain.ProxyDisabled); err != nil {
		t.Fatalf("internal proxy mutation should succeed: %v", err)
	}
}
