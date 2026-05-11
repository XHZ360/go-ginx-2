package admin

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServiceCreatesMilestoneOneResources(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}

	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Role != domain.RoleUser || user.Status != domain.UserEnabled {
		t.Fatalf("unexpected user: %+v", user)
	}

	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if client.CredentialHash == "secret" || client.CredentialHash == "" {
		t.Fatalf("credential was not hashed: %+v", client)
	}

	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "web", Type: domain.ProxyHTTP, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	found, err := db.Proxies().ByHTTPHost(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("lookup proxy: %v", err)
	}
	if found.ID != proxy.ID || found.Status != domain.ProxyEnabled {
		t.Fatalf("unexpected proxy: %+v", found)
	}
}

func TestServiceRejectsInvalidMilestoneOneInputs(t *testing.T) {
	ctx := context.Background()
	service := Service{Store: openTestStore(t)}

	if _, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: "user-1", Name: "home"}); err == nil {
		t.Fatal("expected missing credential error")
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: "user-1", ClientID: "client-1", Name: "tcp", Type: domain.ProxyTCP, TargetHost: "127.0.0.1", TargetPort: 22}); err == nil {
		t.Fatal("expected missing TCP entry port error")
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: "user-1", ClientID: "client-1", Name: "http", Type: domain.ProxyHTTP, TargetHost: "127.0.0.1", TargetPort: 8080}); err == nil {
		t.Fatal("expected missing HTTP entry host error")
	}
}

func TestServicePropagatesDuplicateUser(t *testing.T) {
	ctx := context.Background()
	service := Service{Store: openTestStore(t)}
	input := CreateUserInput{ID: "user-1", Username: "alice"}
	if _, err := service.CreateUser(ctx, input); err != nil {
		t.Fatalf("create first user: %v", err)
	}
	if _, err := service.CreateUser(ctx, input); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func openTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
