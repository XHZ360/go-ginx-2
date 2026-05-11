package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

func TestUserClientProxyRepositories(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)

	user := domain.User{ID: "u1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	client := domain.Client{ID: "c1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}

	proxy := domain.Proxy{ID: "p1", UserID: user.ID, ClientID: client.ID, Name: "ssh", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	found, err := db.Proxies().ByTCPEntryPort(ctx, 10022)
	if err != nil {
		t.Fatalf("lookup proxy: %v", err)
	}
	if found.ID != proxy.ID {
		t.Fatalf("expected proxy %s, got %s", proxy.ID, found.ID)
	}
}

func TestDuplicateTCPEntryPortIsRejected(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	first := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "first", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22}
	second := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "second", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 2222}

	if err := db.Proxies().Create(ctx, first); err != nil {
		t.Fatalf("create first proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, second); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected already exists, got %v", err)
	}
}

func TestDuplicateHTTPHostIsRejectedCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	first := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "first", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "App.Example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	second := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "second", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8081}

	if err := db.Proxies().Create(ctx, first); err != nil {
		t.Fatalf("create first proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, second); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected already exists, got %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return db
}

func seedUserAndClient(t *testing.T, ctx context.Context, db *Store) {
	t.Helper()
	user := domain.User{ID: "u1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "c1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
}
