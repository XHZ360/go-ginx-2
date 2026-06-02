package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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

func TestClientSetStatusMaintainsOnlineOfflineTimestamps(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	if err := db.Clients().SetStatus(ctx, "c1", domain.ClientOnline); err != nil {
		t.Fatalf("set online: %v", err)
	}
	online, err := db.Clients().ByID(ctx, "c1")
	if err != nil {
		t.Fatalf("lookup online client: %v", err)
	}
	if online.Status != domain.ClientOnline || online.LastOnlineAt == nil || online.LastOfflineAt != nil {
		t.Fatalf("unexpected online client timestamps: %+v", online)
	}

	if err := db.Clients().SetStatus(ctx, "c1", domain.ClientOffline); err != nil {
		t.Fatalf("set offline: %v", err)
	}
	offline, err := db.Clients().ByID(ctx, "c1")
	if err != nil {
		t.Fatalf("lookup offline client: %v", err)
	}
	if offline.Status != domain.ClientOffline || offline.LastOnlineAt == nil || offline.LastOfflineAt == nil {
		t.Fatalf("unexpected offline client timestamps: %+v", offline)
	}
}

func TestClientEnrollmentRepositoryStoresReviewableToken(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	first := domain.ClientEnrollment{ID: "join-1", ClientID: "c1", SecretHash: "secret-hash-1", TokenHash: "token-hash-1", Token: "goginx_join_first", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	second := domain.ClientEnrollment{ID: "join-2", ClientID: "c1", SecretHash: "secret-hash-2", TokenHash: "token-hash-2", Token: "goginx_join_second", ExpiresAt: time.Now().UTC().Add(2 * time.Hour), CreatedAt: time.Now().UTC().Add(time.Second)}
	if err := db.ClientEnrollments().Create(ctx, first); err != nil {
		t.Fatalf("create first enrollment: %v", err)
	}
	if err := db.ClientEnrollments().Create(ctx, second); err != nil {
		t.Fatalf("create second enrollment: %v", err)
	}
	if _, err := db.db.ExecContext(ctx, `insert into client_enrollments (id, client_id, secret_hash, token_hash, token, expires_at, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?)`, "join-empty", "c1", "secret-hash-empty", "token-hash-empty", "", time.Now().UTC().Add(3*time.Hour), time.Now().UTC().Add(2*time.Second), time.Now().UTC().Add(2*time.Second)); err != nil {
		t.Fatalf("insert legacy empty-token enrollment: %v", err)
	}

	found, err := db.ClientEnrollments().ByID(ctx, "join-1")
	if err != nil {
		t.Fatalf("lookup enrollment: %v", err)
	}
	if found.Token != first.Token {
		t.Fatalf("expected token text to round trip, got %+v", found)
	}
	latest, err := db.ClientEnrollments().LatestReviewableByClientID(ctx, "c1", time.Now().UTC())
	if err != nil {
		t.Fatalf("lookup latest enrollment: %v", err)
	}
	if latest.ID != "join-2" || latest.Token != second.Token {
		t.Fatalf("expected latest token, got %+v", latest)
	}
	if err := db.ClientEnrollments().MarkUsed(ctx, "join-2", time.Now().UTC()); err != nil {
		t.Fatalf("mark latest token used: %v", err)
	}
	latest, err = db.ClientEnrollments().LatestReviewableByClientID(ctx, "c1", time.Now().UTC())
	if err != nil {
		t.Fatalf("lookup previous reviewable enrollment: %v", err)
	}
	if latest.ID != "join-1" {
		t.Fatalf("expected used and empty-token enrollments to be skipped, got %+v", latest)
	}
	latestUnused, err := db.ClientEnrollments().LatestUnusedByClientID(ctx, "c1")
	if err != nil {
		t.Fatalf("lookup latest unused enrollment: %v", err)
	}
	if latestUnused.ID != "join-1" {
		t.Fatalf("expected latest unused token to skip used and empty-token enrollments, got %+v", latestUnused)
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

func TestDuplicateUDPEntryPortIsRejected(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	first := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "first", Type: domain.ProxyUDP, Status: domain.ProxyEnabled, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 53}
	second := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "second", Type: domain.ProxyUDP, Status: domain.ProxyEnabled, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 5353}

	if err := db.Proxies().Create(ctx, first); err != nil {
		t.Fatalf("create first proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, second); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected already exists, got %v", err)
	}
	found, err := db.Proxies().ByUDPEntryPort(ctx, 10053)
	if err != nil {
		t.Fatalf("lookup udp proxy: %v", err)
	}
	if found.ID != first.ID {
		t.Fatalf("unexpected proxy: %+v", found)
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

func TestDuplicateHTTPSHostIsRejectedCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	first := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "first", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "App.Example.com", TargetHost: "127.0.0.1", TargetPort: 8443}
	second := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "second", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 9443}

	if err := db.Proxies().Create(ctx, first); err != nil {
		t.Fatalf("create first proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, second); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected already exists, got %v", err)
	}
	found, err := db.Proxies().ByHTTPSHost(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("lookup https proxy: %v", err)
	}
	if found.ID != first.ID {
		t.Fatalf("unexpected https proxy: %+v", found)
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

func TestStatsRepositoryPersistsProxyStats(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)
	proxy := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	snapshot := store.ProxyStats{ProxyID: proxy.ID, TCPConnections: 2, TCPUploadBytes: 10, TCPDownloadBytes: 20, TCPErrors: 1, UDPPackets: 4, UDPUploadBytes: 50, UDPDownloadBytes: 60, UDPErrors: 1, HTTPRequests: 3, HTTPUploadBytes: 30, HTTPDownloadBytes: 40, HTTPErrors: 1, HTTPStatusCodes: map[int]int64{200: 2, 502: 1}}
	if err := db.Stats().Save(ctx, []store.ProxyStats{snapshot}); err != nil {
		t.Fatalf("save stats: %v", err)
	}

	loaded, err := db.Stats().List(ctx)
	if err != nil {
		t.Fatalf("list stats: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected one stats row, got %+v", loaded)
	}
	found := loaded[0]
	if found.ProxyID != proxy.ID || found.TCPConnections != 2 || found.UDPPackets != 4 || found.UDPUploadBytes != 50 || found.HTTPRequests != 3 || found.HTTPStatusCodes[200] != 2 || found.HTTPStatusCodes[502] != 1 {
		t.Fatalf("unexpected stats: %+v", found)
	}

	snapshot.HTTPRequests = 4
	snapshot.HTTPStatusCodes[201] = 1
	if err := db.Stats().Save(ctx, []store.ProxyStats{snapshot}); err != nil {
		t.Fatalf("update stats: %v", err)
	}
	loaded, err = db.Stats().List(ctx)
	if err != nil {
		t.Fatalf("list updated stats: %v", err)
	}
	if loaded[0].HTTPRequests != 4 || loaded[0].HTTPStatusCodes[201] != 1 {
		t.Fatalf("unexpected updated stats: %+v", loaded[0])
	}
}

func TestCertificateRepositoryPersistsLifecycleMetadata(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)
	proxy := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	notAfter := time.Now().UTC().Add(20 * 24 * time.Hour).Truncate(time.Second)
	certificate := domain.ManagedCertificate{ID: "cert-1", ProxyID: proxy.ID, Host: "App.Example.com", Status: domain.CertificatePending, Provider: "cloudflare"}
	if err := db.Certificates().Create(ctx, certificate); err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	if err := db.Certificates().UpdateSuccess(ctx, certificate.ID, store.CertificateSuccess{CertFile: "active.crt", KeyFile: "active.key", NotAfter: notAfter}); err != nil {
		t.Fatalf("update success: %v", err)
	}

	found, err := db.Certificates().ByHost(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("lookup certificate by host: %v", err)
	}
	if found.ProxyID != proxy.ID || found.Status != domain.CertificateValid || found.CertFile != "active.crt" || found.KeyFile != "active.key" || found.NotAfter == nil || !found.NotAfter.Equal(notAfter) {
		t.Fatalf("unexpected certificate: %+v", found)
	}
	renewable, err := db.Certificates().ListRenewable(ctx, notAfter.Add(time.Hour))
	if err != nil {
		t.Fatalf("list renewable: %v", err)
	}
	if len(renewable) != 1 || renewable[0].ID != certificate.ID {
		t.Fatalf("unexpected renewable certificates: %+v", renewable)
	}
}

func TestCertificateRepositoryRecordsFailureWithoutReplacingFiles(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)
	proxy := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	certificate := domain.ManagedCertificate{ID: "cert-1", ProxyID: proxy.ID, Host: "app.example.com", Status: domain.CertificateValid, Provider: "cloudflare", CertFile: "active.crt", KeyFile: "active.key"}
	if err := db.Certificates().Create(ctx, certificate); err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	if err := db.Certificates().UpdateFailure(ctx, certificate.ID, store.CertificateFailure{Status: domain.CertificateRenewalFailed, LastError: "dns failed"}); err != nil {
		t.Fatalf("update failure: %v", err)
	}
	found, err := db.Certificates().ByProxyID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("lookup certificate by proxy: %v", err)
	}
	if found.Status != domain.CertificateRenewalFailed || found.LastError != "dns failed" || found.CertFile != "active.crt" || found.KeyFile != "active.key" {
		t.Fatalf("unexpected failure state: %+v", found)
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
