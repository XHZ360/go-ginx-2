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

func TestByClientIDReturnsOnlyClientOwnedProxies(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	otherUser := domain.User{ID: "u2", Username: "bob", Role: domain.RoleUser, Status: domain.UserEnabled}
	otherClient := domain.Client{ID: "c2", UserID: otherUser.ID, Name: "office", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	if err := db.Users().Create(ctx, otherUser); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	if err := db.Clients().Create(ctx, otherClient); err != nil {
		t.Fatalf("create other client: %v", err)
	}

	clientProxy := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	otherProxy := domain.Proxy{ID: "p2", UserID: "u2", ClientID: "c2", Name: "ssh", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22}
	if err := db.Proxies().Create(ctx, clientProxy); err != nil {
		t.Fatalf("create client proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, otherProxy); err != nil {
		t.Fatalf("create other proxy: %v", err)
	}

	found, err := db.Proxies().ByClientID(ctx, "c1")
	if err != nil {
		t.Fatalf("lookup by client: %v", err)
	}
	if len(found) != 1 || found[0].ID != clientProxy.ID {
		t.Fatalf("expected only client proxy, got %+v", found)
	}
}

func TestEnabledByTypeReturnsOnlyEnabledMatchingProxies(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	enabledTCP := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "ssh", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22}
	disabledTCP := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "disabled", Type: domain.ProxyTCP, Status: domain.ProxyDisabled, EntryPort: 10023, TargetHost: "127.0.0.1", TargetPort: 23}
	enabledHTTP := domain.Proxy{ID: "p3", UserID: "u1", ClientID: "c1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	for _, proxy := range []domain.Proxy{enabledTCP, disabledTCP, enabledHTTP} {
		if err := db.Proxies().Create(ctx, proxy); err != nil {
			t.Fatalf("create proxy %s: %v", proxy.ID, err)
		}
	}

	found, err := db.Proxies().EnabledByType(ctx, domain.ProxyTCP)
	if err != nil {
		t.Fatalf("enabled by type: %v", err)
	}
	if len(found) != 1 || found[0].ID != enabledTCP.ID {
		t.Fatalf("expected enabled TCP proxy only, got %+v", found)
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
