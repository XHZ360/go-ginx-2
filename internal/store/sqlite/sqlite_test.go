package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
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
	byProxyIDs, err := db.Certificates().ListByProxyIDs(ctx, []string{proxy.ID, "missing", proxy.ID})
	if err != nil {
		t.Fatalf("list certificates by proxy ids: %v", err)
	}
	if len(byProxyIDs) != 1 || byProxyIDs[0].ID != certificate.ID {
		t.Fatalf("unexpected certificates by proxy ids: %+v", byProxyIDs)
	}
}

func TestCertificateRepositoryListsProviderAwareLifecycleCandidates(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	proxies := []domain.Proxy{
		{ID: "acme-due", UserID: "u1", ClientID: "c1", Name: "acme due", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "acme-due.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
		{ID: "acme-later", UserID: "u1", ClientID: "c1", Name: "acme later", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "acme-later.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
		{ID: "origin-due", UserID: "u1", ClientID: "c1", Name: "origin due", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "origin-due.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
		{ID: "origin-backoff", UserID: "u1", ClientID: "c1", Name: "origin backoff", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "origin-backoff.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
	}
	for _, proxy := range proxies {
		if err := db.Proxies().Create(ctx, proxy); err != nil {
			t.Fatalf("create proxy %s: %v", proxy.ID, err)
		}
	}
	acmeDue := now.Add(30 * time.Minute)
	acmeLater := now.Add(2 * time.Hour)
	originDue := now.Add(12 * time.Hour)
	originBackoff := now.Add(12 * time.Hour)
	nextAttempt := now.Add(time.Hour)
	certificates := []domain.ManagedCertificate{
		{ID: "cert-acme-due", ProxyID: "acme-due", Host: "acme-due.example.com", Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable, ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: "cloudflare", CertFile: "active.crt", KeyFile: "active.key", NotAfter: &acmeDue},
		{ID: "cert-acme-later", ProxyID: "acme-later", Host: "acme-later.example.com", Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable, ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: "cloudflare", CertFile: "active.crt", KeyFile: "active.key", NotAfter: &acmeLater},
		{ID: "cert-origin-due", ProxyID: "origin-due", Host: "origin-due.example.com", Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CertFile: "active.crt", KeyFile: "active.key", NotAfter: &originDue},
		{ID: "cert-origin-backoff", ProxyID: "origin-backoff", Host: "origin-backoff.example.com", Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CertFile: "active.crt", KeyFile: "active.key", NotAfter: &originBackoff, NextAttemptAt: &nextAttempt},
	}
	for _, certificate := range certificates {
		if err := db.Certificates().Create(ctx, certificate); err != nil {
			t.Fatalf("create certificate %s: %v", certificate.ID, err)
		}
	}
	acmeBefore := now.Add(time.Hour)
	originBefore := now.Add(24 * time.Hour)

	candidates, err := db.Certificates().ListLifecycleCandidates(ctx, store.CertificateLifecycleCandidateQuery{Now: now, ACMEBefore: &acmeBefore, OriginCABefore: &originBefore})
	if err != nil {
		t.Fatalf("list lifecycle candidates: %v", err)
	}
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	if strings.Join(ids, ",") != "cert-acme-due,cert-origin-due" {
		t.Fatalf("unexpected lifecycle candidates: %+v", ids)
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
	scoped, err := db.ProviderCredentials().ListByProviderType(ctx, domain.CertificateProviderCloudflareOriginCA, []domain.ProviderCredentialStatus{domain.ProviderCredentialPending})
	if err != nil {
		t.Fatalf("list provider credentials by provider type: %v", err)
	}
	if len(scoped) != 1 || scoped[0].ID != credential.ID {
		t.Fatalf("unexpected provider-scoped credentials: %+v", scoped)
	}
	verifiedScoped, err := db.ProviderCredentials().ListByProviderType(ctx, domain.CertificateProviderCloudflareOriginCA, []domain.ProviderCredentialStatus{domain.ProviderCredentialVerified})
	if err != nil {
		t.Fatalf("list verified provider credentials by provider type: %v", err)
	}
	if len(verifiedScoped) != 0 {
		t.Fatalf("unexpected verified provider-scoped credentials: %+v", verifiedScoped)
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
	if !indexExists(t, ctx, db.db, "managed_certificates_lifecycle_provider_idx") {
		t.Fatal("expected managed certificate lifecycle provider index")
	}
	if !indexExists(t, ctx, db.db, "provider_credentials_provider_status_idx") {
		t.Fatal("expected provider credential provider/status index")
	}
}

func TestCertificateBecomesIndependentResource(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)
	proxy := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	bound := domain.ManagedCertificate{ID: "cert-bound", ProxyID: proxy.ID, Host: "app.example.com", Status: domain.CertificatePending, Provider: "cloudflare"}
	if err := db.Certificates().Create(ctx, bound); err != nil {
		t.Fatalf("create bound certificate: %v", err)
	}

	// (a) 未绑定证书（proxy_id="")可以创建，且多张未绑定证书不会因 proxy_id='' 冲突。
	unbound := domain.ManagedCertificate{ID: "cert-unbound", ProxyID: "", Host: "free.example.com", Status: domain.CertificatePending, Provider: "cloudflare"}
	if err := db.Certificates().Create(ctx, unbound); err != nil {
		t.Fatalf("create unbound certificate: %v", err)
	}
	secondUnbound := domain.ManagedCertificate{ID: "cert-unbound-2", ProxyID: "", Host: "free2.example.com", Status: domain.CertificatePending, Provider: "cloudflare"}
	if err := db.Certificates().Create(ctx, secondUnbound); err != nil {
		t.Fatalf("create second unbound certificate: %v", err)
	}

	// (b) ByID 与 Delete 正常工作。
	loaded, err := db.Certificates().ByID(ctx, unbound.ID)
	if err != nil {
		t.Fatalf("lookup certificate by id: %v", err)
	}
	if loaded.ID != unbound.ID || loaded.ProxyID != "" || loaded.Host != "free.example.com" {
		t.Fatalf("unexpected certificate by id: %+v", loaded)
	}
	if err := db.Certificates().Delete(ctx, secondUnbound.ID); err != nil {
		t.Fatalf("delete certificate: %v", err)
	}
	if _, err := db.Certificates().ByID(ctx, secondUnbound.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected deleted certificate not found, got %v", err)
	}
	if err := db.Certificates().Delete(ctx, "missing-cert"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected delete of missing certificate to report not found, got %v", err)
	}

	// (c) 删除代理后，其证书作为独立资源存活（不再级联删除）。
	if err := db.Proxies().Delete(ctx, proxy.ID); err != nil {
		t.Fatalf("delete proxy: %v", err)
	}
	if _, err := db.Proxies().ByID(ctx, proxy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected proxy deleted, got %v", err)
	}
	survivor, err := db.Certificates().ByID(ctx, bound.ID)
	if err != nil {
		t.Fatalf("certificate should survive proxy deletion: %v", err)
	}
	if survivor.ID != bound.ID {
		t.Fatalf("unexpected surviving certificate: %+v", survivor)
	}
}

func TestProxyCertificateBindingRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedUserAndClient(t, ctx, db)

	cert := domain.ManagedCertificate{ID: "cert-1", ProxyID: "", Host: "app.example.com", Status: domain.CertificatePending, Provider: "cloudflare"}
	if err := db.Certificates().Create(ctx, cert); err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	proxy := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, CertificateID: cert.ID}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy bound to certificate: %v", err)
	}

	loaded, err := db.Proxies().ByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("lookup proxy: %v", err)
	}
	if loaded.CertificateID != cert.ID {
		t.Fatalf("expected proxy bound to %s, got %+v", cert.ID, loaded)
	}
	byCert, err := db.Proxies().ByCertificateID(ctx, cert.ID)
	if err != nil {
		t.Fatalf("lookup proxy by certificate id: %v", err)
	}
	if byCert.ID != proxy.ID {
		t.Fatalf("unexpected proxy by certificate id: %+v", byCert)
	}
	if _, err := db.Proxies().ByCertificateID(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected no proxy for unknown certificate, got %v", err)
	}

	// 一证一代理：第二个代理绑定同一证书应被部分唯一索引拒绝。
	other := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "secure-2", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "alt.example.com", TargetHost: "127.0.0.1", TargetPort: 8081, CertificateID: cert.ID}
	if err := db.Proxies().Create(ctx, other); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected one cert -> one proxy to be enforced, got %v", err)
	}
}

func TestOpenBackfillsProxyCertificateBindingFromLegacyProxyID(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-bind.db")
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
create unique index managed_certificates_proxy_unique on managed_certificates(proxy_id);
create unique index managed_certificates_host_unique on managed_certificates(lower(host));
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

	// 回填：旧 managed cert 的 proxy_id 反向引用迁移为代理侧 certificate_id 权威绑定。
	proxy, err := db.Proxies().ByID(ctx, "p1")
	if err != nil {
		t.Fatalf("lookup migrated proxy: %v", err)
	}
	if proxy.CertificateID != "cert-1" {
		t.Fatalf("expected proxy backfilled with certificate_id=cert-1, got %+v", proxy)
	}
	bound, err := db.Proxies().ByCertificateID(ctx, "cert-1")
	if err != nil {
		t.Fatalf("lookup proxy by certificate id: %v", err)
	}
	if bound.ID != "p1" {
		t.Fatalf("unexpected proxy bound to cert-1: %+v", bound)
	}

	// 重建后的旧级联外键已移除：删除代理不再删除证书，证书作为独立资源存活。
	if err := db.Proxies().Delete(ctx, "p1"); err != nil {
		t.Fatalf("delete migrated proxy: %v", err)
	}
	survivor, err := db.Certificates().ByID(ctx, "cert-1")
	if err != nil {
		t.Fatalf("certificate should survive proxy deletion after rebuild: %v", err)
	}
	if survivor.ID != "cert-1" {
		t.Fatalf("unexpected surviving certificate: %+v", survivor)
	}
	if indexExists(t, ctx, db.db, "managed_certificates_proxy_unique") {
		t.Fatal("legacy managed_certificates_proxy_unique index should be dropped")
	}
	if !indexExists(t, ctx, db.db, "managed_certificates_host_unique") {
		t.Fatal("managed_certificates_host_unique index should be preserved")
	}
	if !indexExists(t, ctx, db.db, "proxies_certificate_id_unique") {
		t.Fatal("proxies_certificate_id_unique index should exist after migration")
	}
	// 幂等性：再次打开同一数据库不应失败（重建无操作）。
	if err := db.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen migrated store should be idempotent: %v", err)
	}
	defer reopened.Close()
}

func TestClientKindDefaultsToProviderOnLegacyDB(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-kind.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := legacy.ExecContext(ctx, `
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
    version integer not null default 0,
    last_online_at timestamp,
    last_offline_at timestamp,
    created_at timestamp not null,
    updated_at timestamp not null
);
insert into users (id, username, role, status, created_at, updated_at) values ('u1', 'alice', 'user', 'enabled', ?, ?);
insert into clients (id, user_id, name, status, credential_hash, created_at, updated_at) values ('c1', 'u1', 'home', 'offline', 'hash', ?, ?);
`, now, now, now, now); err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("open migrated db: %v", err)
	}
	defer db.Close()

	loaded, err := db.Clients().ByID(ctx, "c1")
	if err != nil {
		t.Fatalf("load migrated client: %v", err)
	}
	if loaded.Kind != domain.ClientKindProvider {
		t.Fatalf("expected legacy client kind to be provider, got %q", loaded.Kind)
	}
}

func TestClientConsumerKindPersists(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)

	user := domain.User{ID: "u1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	provider := domain.Client{ID: "c-provider", UserID: "u1", Name: "provider", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("s1")}
	consumer := domain.Client{ID: "c-consumer", UserID: "u1", Name: "consumer", Kind: domain.ClientKindConsumer, Status: domain.ClientOffline, CredentialHash: domain.HashCredential("s2")}
	if err := db.Clients().Create(ctx, provider); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Clients().Create(ctx, consumer); err != nil {
		t.Fatalf("create consumer: %v", err)
	}

	loadedProvider, err := db.Clients().ByID(ctx, "c-provider")
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	if loadedProvider.Kind != domain.ClientKindProvider {
		t.Fatalf("expected provider kind, got %q", loadedProvider.Kind)
	}

	loadedConsumer, err := db.Clients().ByID(ctx, "c-consumer")
	if err != nil {
		t.Fatalf("load consumer: %v", err)
	}
	if loadedConsumer.Kind != domain.ClientKindConsumer {
		t.Fatalf("expected consumer kind, got %q", loadedConsumer.Kind)
	}

	listed, err := db.Clients().List(ctx)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(listed))
	}
}

func TestProxyByUserIDReturnsUserProxies(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)

	user1 := domain.User{ID: "u1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	user2 := domain.User{ID: "u2", Username: "bob", Role: domain.RoleUser, Status: domain.UserEnabled}
	if err := db.Users().Create(ctx, user1); err != nil {
		t.Fatalf("create user1: %v", err)
	}
	if err := db.Users().Create(ctx, user2); err != nil {
		t.Fatalf("create user2: %v", err)
	}
	client1 := domain.Client{ID: "c1", UserID: "u1", Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("s1")}
	client2 := domain.Client{ID: "c2", UserID: "u2", Name: "office", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("s2")}
	if err := db.Clients().Create(ctx, client1); err != nil {
		t.Fatalf("create client1: %v", err)
	}
	if err := db.Clients().Create(ctx, client2); err != nil {
		t.Fatalf("create client2: %v", err)
	}

	proxy1 := domain.Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "ssh", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22}
	proxy2 := domain.Proxy{ID: "p2", UserID: "u1", ClientID: "c1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	proxy3 := domain.Proxy{ID: "p3", UserID: "u2", ClientID: "c2", Name: "other", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10023, TargetHost: "127.0.0.1", TargetPort: 23}
	for _, p := range []domain.Proxy{proxy1, proxy2, proxy3} {
		if err := db.Proxies().Create(ctx, p); err != nil {
			t.Fatalf("create proxy %s: %v", p.ID, err)
		}
	}

	found, err := db.Proxies().ByUserID(ctx, "u1")
	if err != nil {
		t.Fatalf("by user id: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 proxies for u1, got %d", len(found))
	}
	if found[0].ID != "p1" || found[1].ID != "p2" {
		t.Fatalf("unexpected proxy order: %s, %s", found[0].ID, found[1].ID)
	}

	found2, err := db.Proxies().ByUserID(ctx, "u2")
	if err != nil {
		t.Fatalf("by user id u2: %v", err)
	}
	if len(found2) != 1 || found2[0].ID != "p3" {
		t.Fatalf("expected 1 proxy for u2, got %+v", found2)
	}

	foundEmpty, err := db.Proxies().ByUserID(ctx, "u-missing")
	if err != nil {
		t.Fatalf("by missing user: %v", err)
	}
	if len(foundEmpty) != 0 {
		t.Fatalf("expected 0 proxies for missing user, got %d", len(foundEmpty))
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
