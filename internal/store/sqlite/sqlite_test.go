package sqlite

import (
	"context"
	"database/sql"
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

func TestProxyEntryBindHostAndRouteQueries(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	first := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryBindHost: "127.0.0.1", EntryHost: "app.example.com", EntryPort: 18080, TargetHost: "127.0.0.1", TargetPort: 8080}
	second := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "web-alt", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryBindHost: "127.0.0.1", EntryHost: "app.example.com", EntryPort: 18081, TargetHost: "127.0.0.1", TargetPort: 8081}
	duplicate := domain.Proxy{ID: "p3", UserID: "u1", ClientID: "c1", Name: "web-dup", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryBindHost: "127.0.0.1", EntryHost: "APP.EXAMPLE.COM", EntryPort: 18080, TargetHost: "127.0.0.1", TargetPort: 8082}
	legacy := domain.Proxy{ID: "p4", UserID: "u1", ClientID: "c1", Name: "legacy", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "legacy.example.com", TargetHost: "127.0.0.1", TargetPort: 8083}

	if err := db.Proxies().Create(ctx, first); err != nil {
		t.Fatalf("create first proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, second); err != nil {
		t.Fatalf("create second proxy on different port: %v", err)
	}
	if err := db.Proxies().Create(ctx, duplicate); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected duplicate route to fail, got %v", err)
	}
	if err := db.Proxies().Create(ctx, legacy); err != nil {
		t.Fatalf("create legacy proxy: %v", err)
	}
	found, err := db.Proxies().ByHTTPRoute(ctx, "127.0.0.1", 18081, "app.example.com", false)
	if err != nil {
		t.Fatalf("lookup exact route: %v", err)
	}
	if found.ID != second.ID || found.EntryBindHost != "127.0.0.1" {
		t.Fatalf("unexpected exact route: %+v", found)
	}
	found, err = db.Proxies().ByHTTPRoute(ctx, "127.0.0.1", 18080, "legacy.example.com", true)
	if err != nil {
		t.Fatalf("lookup fallback route: %v", err)
	}
	if found.ID != legacy.ID {
		t.Fatalf("unexpected fallback route: %+v", found)
	}
}

func TestOpenMigratesLegacyProxyEntryBindHostBeforeIndexes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := raw.ExecContext(ctx, `
create table users (
    id text primary key,
    username text not null unique,
    role text not null,
    status text not null,
    created_at timestamp not null,
    updated_at timestamp not null
);
create table clients (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    name text not null,
    status text not null,
    credential_hash text not null,
    version integer not null default 1,
    last_online_at timestamp,
    last_offline_at timestamp,
    created_at timestamp not null,
    updated_at timestamp not null
);
create table proxies (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    client_id text not null references clients(id) on delete cascade,
    name text not null,
    type text not null,
    status text not null,
    entry_host text not null default '',
    entry_port integer not null default 0,
    target_host text not null,
    target_port integer not null,
    description text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);
create unique index proxies_tcp_entry_port_unique on proxies(entry_port) where type = 'tcp' and entry_port > 0;
create unique index proxies_udp_entry_port_unique on proxies(entry_port) where type = 'udp' and entry_port > 0;
create unique index proxies_http_entry_host_unique on proxies(lower(entry_host)) where type = 'http' and entry_host <> '';
create unique index proxies_https_entry_host_unique on proxies(lower(entry_host)) where type = 'https' and entry_host <> '';
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `insert into users (id, username, role, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?)`, "u1", "alice", domain.RoleUser, domain.UserEnabled, now, now); err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `insert into clients (id, user_id, name, status, credential_hash, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?)`, "c1", "u1", "home", domain.ClientOffline, domain.HashCredential("secret"), now, now); err != nil {
		t.Fatalf("insert legacy client: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `insert into proxies (id, user_id, client_id, name, type, status, entry_port, target_host, target_port, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "p1", "u1", "c1", "ssh", domain.ProxyTCP, domain.ProxyEnabled, 10022, "127.0.0.1", 22, now, now); err != nil {
		t.Fatalf("insert legacy proxy: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("open migrated db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	loaded, err := db.Proxies().ByID(ctx, "p1")
	if err != nil {
		t.Fatalf("load migrated proxy: %v", err)
	}
	if loaded.EntryBindHost != "" || loaded.CertFile != "" || loaded.KeyFile != "" {
		t.Fatalf("unexpected migrated proxy fields: %+v", loaded)
	}
	if indexExists(t, ctx, db.db, "proxies_tcp_entry_port_unique") {
		t.Fatal("legacy tcp entry port index should be dropped")
	}
	if !indexExists(t, ctx, db.db, "proxies_tcp_entry_unique") {
		t.Fatal("new tcp entry index should exist")
	}
	second := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "ssh-alt", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryBindHost: "127.0.0.1", EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 2222}
	if err := db.Proxies().Create(ctx, second); err != nil {
		t.Fatalf("create same-port proxy on different bind host after migration: %v", err)
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
	checkedAt := time.Now().UTC().Truncate(time.Second)
	syncedAt := checkedAt.Add(time.Minute)
	if err := db.Certificates().UpdateSuccess(ctx, certificate.ID, store.CertificateSuccess{CertFile: "active.crt", KeyFile: "active.key", NotAfter: notAfter, ServingStatus: domain.CertificateServingExpiringSoon, ProviderStatus: domain.CertificateProviderStatusActive, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CredentialID: "cred-1", CloudflareID: "cf-cert-1", Hostnames: []string{"app.example.com", "api.example.com"}, RequestType: "origin-ecc", RequestedValidity: 5475, Fingerprint: "ABCDEF", LastCheckedAt: checkedAt, LastAttemptedAt: checkedAt, LastSyncedAt: &syncedAt}); err != nil {
		t.Fatalf("update success: %v", err)
	}

	found, err := db.Certificates().ByHost(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("lookup certificate by host: %v", err)
	}
	if found.ProxyID != proxy.ID || found.Status != domain.CertificateExpiringSoon || found.ServingStatus != domain.CertificateServingExpiringSoon || found.OperationStatus != domain.CertificateOperationIdle || found.CertFile != "active.crt" || found.KeyFile != "active.key" || found.NotAfter == nil || !found.NotAfter.Equal(notAfter) || found.Fingerprint != "abcdef" || found.LastCheckedAt == nil || !found.LastCheckedAt.Equal(checkedAt) {
		t.Fatalf("unexpected certificate: %+v", found)
	}
	if found.ProviderType != domain.CertificateProviderCloudflareOriginCA || found.ProviderName != "cloudflare" || found.ProviderStatus != domain.CertificateProviderStatusActive || found.CredentialID != "cred-1" || found.CloudflareCertificateID != "cf-cert-1" || found.RequestType != "origin-ecc" || found.RequestedValidity != 5475 || found.LastSyncedAt == nil || !found.LastSyncedAt.Equal(syncedAt) {
		t.Fatalf("unexpected provider metadata: %+v", found)
	}
	if len(found.Hostnames) != 2 || found.Hostnames[0] != "app.example.com" || found.Hostnames[1] != "api.example.com" {
		t.Fatalf("unexpected origin ca hostnames: %+v", found.Hostnames)
	}
	renewable, err := db.Certificates().ListRenewable(ctx, notAfter.Add(time.Hour), time.Now().UTC())
	if err != nil {
		t.Fatalf("list renewable: %v", err)
	}
	if len(renewable) != 1 || renewable[0].ID != certificate.ID {
		t.Fatalf("unexpected renewable certificates: %+v", renewable)
	}
}

func TestProviderCredentialRepositoryPersistsMetadataOnly(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	verifiedAt := time.Now().UTC().Truncate(time.Second)
	credential := domain.ProviderCredential{ID: "cred-1", Name: "Cloudflare Origin", ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: "Zone SSL:Edit", TokenFingerprint: "abcdef", SecretRef: "cred-1.secret", Status: domain.ProviderCredentialPending}

	if err := db.ProviderCredentials().Create(ctx, credential); err != nil {
		t.Fatalf("create provider credential: %v", err)
	}
	found, err := db.ProviderCredentials().ByID(ctx, credential.ID)
	if err != nil {
		t.Fatalf("lookup provider credential: %v", err)
	}
	if found.Name != credential.Name || found.ProviderType != credential.ProviderType || found.Scope != credential.Scope || found.TokenFingerprint != "abcdef" || found.SecretRef != "cred-1.secret" || found.Status != domain.ProviderCredentialPending {
		t.Fatalf("unexpected provider credential: %+v", found)
	}
	if err := db.ProviderCredentials().SetStatus(ctx, credential.ID, domain.ProviderCredentialVerified, &verifiedAt, ""); err != nil {
		t.Fatalf("set provider credential status: %v", err)
	}
	found, err = db.ProviderCredentials().ByID(ctx, credential.ID)
	if err != nil {
		t.Fatalf("lookup verified provider credential: %v", err)
	}
	if found.Status != domain.ProviderCredentialVerified || found.LastVerifiedAt == nil || !found.LastVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("unexpected verified provider credential: %+v", found)
	}
	found.Name = "Rotated Origin"
	found.TokenFingerprint = "123456"
	found.Status = domain.ProviderCredentialPending
	found.LastVerifiedAt = nil
	found.LastError = "verification pending"
	if err := db.ProviderCredentials().Update(ctx, found); err != nil {
		t.Fatalf("update provider credential: %v", err)
	}
	listed, err := db.ProviderCredentials().List(ctx)
	if err != nil {
		t.Fatalf("list provider credentials: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "Rotated Origin" || listed[0].TokenFingerprint != "123456" || listed[0].LastError != "verification pending" {
		t.Fatalf("unexpected provider credential list: %+v", listed)
	}
	if err := db.ProviderCredentials().Delete(ctx, credential.ID); err != nil {
		t.Fatalf("delete provider credential: %v", err)
	}
	if _, err := db.ProviderCredentials().ByID(ctx, credential.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected deleted provider credential not found, got %v", err)
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

	nextAttempt := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Second)
	if err := db.Certificates().UpdateFailure(ctx, certificate.ID, store.CertificateFailure{Status: domain.CertificateRenewalFailed, ServingStatus: domain.CertificateServingUsable, OperationStatus: domain.CertificateOperationRenewalFailed, LastError: "dns failed", NextAttemptAt: &nextAttempt, FailureCount: 2}); err != nil {
		t.Fatalf("update failure: %v", err)
	}
	found, err := db.Certificates().ByProxyID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("lookup certificate by proxy: %v", err)
	}
	if found.Status != domain.CertificateRenewalFailed || found.ServingStatus != domain.CertificateServingUsable || found.OperationStatus != domain.CertificateOperationRenewalFailed || found.LastError != "dns failed" || found.CertFile != "active.crt" || found.KeyFile != "active.key" || found.FailureCount != 2 || found.NextAttemptAt == nil || !found.NextAttemptAt.Equal(nextAttempt) {
		t.Fatalf("unexpected failure state: %+v", found)
	}
	checkedAt := time.Now().UTC().Truncate(time.Second)
	notAfter := checkedAt.Add(time.Hour)
	if err := db.Certificates().UpdateHealth(ctx, certificate.ID, store.CertificateHealth{ServingStatus: domain.CertificateServingUsable, NotAfter: &notAfter, Fingerprint: "ABCDEF", CheckedAt: checkedAt}); err != nil {
		t.Fatalf("update health: %v", err)
	}
	found, err = db.Certificates().ByProxyID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("lookup after health update: %v", err)
	}
	if found.Status != domain.CertificateRenewalFailed || found.LastError != "dns failed" || found.ServingStatus != domain.CertificateServingUsable || found.Fingerprint != "abcdef" {
		t.Fatalf("health update should preserve lifecycle failure state: %+v", found)
	}
	syncedAt := checkedAt.Add(time.Minute)
	if err := db.Certificates().UpdateProviderSync(ctx, certificate.ID, store.CertificateProviderSync{ProviderStatus: domain.CertificateProviderStatusActive, SyncedAt: syncedAt}); err != nil {
		t.Fatalf("update provider sync: %v", err)
	}
	found, err = db.Certificates().ByProxyID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("lookup after provider sync: %v", err)
	}
	if found.ProviderStatus != domain.CertificateProviderStatusActive || found.LastError != "dns failed" || found.LastSyncedAt == nil || !found.LastSyncedAt.Equal(syncedAt) {
		t.Fatalf("provider sync should preserve lifecycle failure error: %+v", found)
	}
}

func TestCertificateRepositoryMigratesLegacyLifecycleColumns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.ExecContext(ctx, `
create table users (
    id text primary key,
    username text not null unique,
    password_hash text not null default '',
    role text not null,
    status text not null,
    created_at timestamp not null,
    updated_at timestamp not null
);
create table clients (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    name text not null,
    status text not null,
    credential_hash text not null,
    version integer not null default 0,
    last_online_at timestamp,
    last_offline_at timestamp,
    created_at timestamp not null,
    updated_at timestamp not null
);
create table proxies (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    client_id text not null references clients(id) on delete cascade,
    name text not null,
    type text not null,
    status text not null,
    entry_bind_host text not null default '',
    entry_host text not null default '',
    entry_port integer not null default 0,
    target_host text not null,
    target_port integer not null,
    cert_file text not null default '',
    key_file text not null default '',
    description text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);
create table managed_certificates (
    id text primary key,
    proxy_id text not null references proxies(id) on delete cascade,
    host text not null,
    status text not null,
    provider text not null default '',
    cert_file text not null default '',
    key_file text not null default '',
    previous_cert_file text not null default '',
    previous_key_file text not null default '',
    not_after timestamp,
    last_issued_at timestamp,
    last_renewed_at timestamp,
    last_error text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);
insert into users (id, username, role, status, created_at, updated_at) values ('u1', 'alice', 'user', 'enabled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
insert into clients (id, user_id, name, status, credential_hash, created_at, updated_at) values ('c1', 'u1', 'home', 'offline', 'hash', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
insert into proxies (id, user_id, client_id, name, type, status, entry_host, target_host, target_port, created_at, updated_at) values ('p1', 'u1', 'c1', 'secure', 'https', 'enabled', 'app.example.com', '127.0.0.1', 8080, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
insert into managed_certificates (id, proxy_id, host, status, provider, cert_file, key_file, created_at, updated_at) values ('cert-1', 'p1', 'app.example.com', 'valid', 'cloudflare', 'active.crt', 'active.key', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`)
	if closeErr := legacy.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer db.Close()
	certificate, err := db.Certificates().ByProxyID(ctx, "p1")
	if err != nil {
		t.Fatalf("lookup migrated certificate: %v", err)
	}
	if certificate.ServingStatus != domain.CertificateServingUsable || certificate.OperationStatus != domain.CertificateOperationIdle || certificate.FailureCount != 0 || certificate.Fingerprint != "" {
		t.Fatalf("unexpected migrated lifecycle fields: %+v", certificate)
	}
	if certificate.ProviderType != domain.CertificateProviderACMEDNS01 || certificate.ProviderName != "cloudflare" || certificate.ProviderStatus != domain.CertificateProviderStatusUnknown {
		t.Fatalf("unexpected migrated provider fields: %+v", certificate)
	}
}

func indexExists(t *testing.T, ctx context.Context, db *sql.DB, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `select count(*) from sqlite_master where type = 'index' and name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("query index %s: %v", name, err)
	}
	return count > 0
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
