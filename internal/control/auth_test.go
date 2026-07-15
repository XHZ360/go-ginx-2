package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

func TestAuthenticatorAcceptsEnabledClientWithMatchingCredential(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	auth := Authenticator{
		Store:             newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		AllowedProtocols:  []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS},
		HeartbeatInterval: 10 * time.Second,
		Now:               func() time.Time { return now },
	}

	result, err := auth.Authenticate(context.Background(), AuthRequest{
		ClientID:   "client-1",
		Credential: "secret",
		Timestamp:  now,
		Protocols:  []domain.Protocol{domain.ProtocolTCPTLS, domain.ProtocolQUIC},
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if result.SelectedProtocol != domain.ProtocolQUIC {
		t.Fatalf("expected QUIC priority, got %s", result.SelectedProtocol)
	}
	if result.HeartbeatInterval != 10*time.Second {
		t.Fatalf("unexpected heartbeat interval %s", result.HeartbeatInterval)
	}
}

func TestAuthenticatorRejectsDisabledUser(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	auth := Authenticator{Store: newAuthStore(domain.UserDisabled, domain.ClientOffline, domain.HashCredential("secret")), Now: func() time.Time { return now }}

	_, err := auth.Authenticate(context.Background(), AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expected authentication failure, got %v", err)
	}
}

func TestAuthenticatorRejectsUnsupportedProtocol(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	auth := Authenticator{Store: newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")), AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Now: func() time.Time { return now }}

	_, err := auth.Authenticate(context.Background(), AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
	if !errors.Is(err, ErrProtocolUnavailable) {
		t.Fatalf("expected protocol unavailable, got %v", err)
	}
}

type authStore struct {
	user            domain.User
	client          domain.Client
	clients         []domain.Client
	proxies         []domain.Proxy
	clientStatusLog *[]domain.ClientStatus
}

func newAuthStore(userStatus domain.UserStatus, clientStatus domain.ClientStatus, credentialHash string) authStore {
	return authStore{
		user:   domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: userStatus},
		client: domain.Client{ID: "client-1", UserID: "user-1", Name: "home", Status: clientStatus, CredentialHash: credentialHash, Version: 7},
	}
}

// newConsumerAuthStore creates an auth store that supports both provider and consumer client lookups.
func newConsumerAuthStore(credentialHash string) authStore {
	return authStore{
		user: domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled},
		client: domain.Client{ID: "client-provider", UserID: "user-1", Name: "provider", Kind: domain.ClientKindProvider, Status: domain.ClientOffline, CredentialHash: credentialHash, Version: 7},
		clients: []domain.Client{
			{ID: "client-provider", UserID: "user-1", Name: "provider", Kind: domain.ClientKindProvider, Status: domain.ClientOffline, CredentialHash: credentialHash, Version: 7},
			{ID: "client-consumer", UserID: "user-1", Name: "consumer", Kind: domain.ClientKindConsumer, Status: domain.ClientOffline, CredentialHash: credentialHash, Version: 7},
		},
	}
}

func (s authStore) Users() store.UserRepository { return authUserRepository{s.user} }

func (s authStore) Clients() store.ClientRepository {
	return authClientRepository{client: s.client, clients: s.clients, statusLog: s.clientStatusLog}
}

func (s authStore) ClientEnrollments() store.ClientEnrollmentRepository {
	return authClientEnrollmentRepository{}
}

func (s authStore) Domains() store.DomainRepository { return authDomainRepository{} }

func (s authStore) DomainEntries() store.DomainEntryRepository { return authDomainEntryRepository{} }

func (s authStore) Proxies() store.ProxyRepository { return authProxyRepository{s.proxies} }

func (s authStore) Certificates() store.CertificateRepository { return authCertificateRepository{} }

func (s authStore) ProviderCredentials() store.ProviderCredentialRepository {
	return authProviderCredentialRepository{}
}

func (s authStore) Stats() store.StatsRepository { return authStatsRepository{} }

func (s authStore) AuditEvents() store.AuditRepository { return nil }

func (s authStore) Close() error { return nil }

type authUserRepository struct{ user domain.User }

func (r authUserRepository) Create(context.Context, domain.User) error { return nil }

func (r authUserRepository) ByID(_ context.Context, id string) (domain.User, error) {
	if id != r.user.ID {
		return domain.User{}, store.ErrNotFound
	}
	return r.user, nil
}

func (r authUserRepository) ByUsername(context.Context, string) (domain.User, error) {
	return r.user, nil
}

func (r authUserRepository) List(context.Context) ([]domain.User, error) {
	return []domain.User{r.user}, nil
}

func (r authUserRepository) SetStatus(context.Context, string, domain.UserStatus) error { return nil }

func (r authUserRepository) SetPassword(context.Context, string, string) error { return nil }

func (r authUserRepository) Delete(context.Context, string) error { return nil }

type authClientRepository struct {
	client    domain.Client
	clients   []domain.Client
	statusLog *[]domain.ClientStatus
}

func (r authClientRepository) Create(context.Context, domain.Client) error { return nil }

func (r authClientRepository) ByID(_ context.Context, id string) (domain.Client, error) {
	for _, c := range r.clients {
		if c.ID == id {
			return c, nil
		}
	}
	if id != r.client.ID {
		return domain.Client{}, store.ErrNotFound
	}
	return r.client, nil
}

func (r authClientRepository) List(context.Context) ([]domain.Client, error) {
	if len(r.clients) > 0 {
		return r.clients, nil
	}
	return []domain.Client{r.client}, nil
}

func (r authClientRepository) SetStatus(_ context.Context, _ string, status domain.ClientStatus) error {
	if r.statusLog != nil {
		*r.statusLog = append(*r.statusLog, status)
	}
	return nil
}

func (r authClientRepository) RotateCredential(context.Context, string, string) error { return nil }
func (r authClientRepository) Delete(context.Context, string) error                   { return nil }

type authClientEnrollmentRepository struct{}

func (authClientEnrollmentRepository) Create(context.Context, domain.ClientEnrollment) error {
	return nil
}

func (authClientEnrollmentRepository) ByID(context.Context, string) (domain.ClientEnrollment, error) {
	return domain.ClientEnrollment{}, store.ErrNotFound
}

func (authClientEnrollmentRepository) LatestReviewableByClientID(context.Context, string, time.Time) (domain.ClientEnrollment, error) {
	return domain.ClientEnrollment{}, store.ErrNotFound
}

func (authClientEnrollmentRepository) LatestUnusedByClientID(context.Context, string) (domain.ClientEnrollment, error) {
	return domain.ClientEnrollment{}, store.ErrNotFound
}

func (authClientEnrollmentRepository) MarkUsed(context.Context, string, time.Time) error { return nil }

type authProxyRepository struct{ proxies []domain.Proxy }

func (r authProxyRepository) Create(context.Context, domain.Proxy) error { return nil }

func (r authProxyRepository) ByID(context.Context, string) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) List(context.Context) ([]domain.Proxy, error) {
	return r.proxies, nil
}

func (r authProxyRepository) ByClientID(_ context.Context, clientID string) ([]domain.Proxy, error) {
	proxies := make([]domain.Proxy, 0)
	for _, proxy := range r.proxies {
		if proxy.ClientID == clientID {
			proxies = append(proxies, proxy)
		}
	}
	return proxies, nil
}

func (r authProxyRepository) ByUserID(_ context.Context, userID string) ([]domain.Proxy, error) {
	proxies := make([]domain.Proxy, 0)
	for _, proxy := range r.proxies {
		if proxy.UserID == userID {
			proxies = append(proxies, proxy)
		}
	}
	return proxies, nil
}

func (r authProxyRepository) ByDomainID(_ context.Context, domainID string) ([]domain.Proxy, error) {
	proxies := make([]domain.Proxy, 0)
	for _, proxy := range r.proxies {
		if proxy.DomainID == domainID {
			proxies = append(proxies, proxy)
		}
	}
	return proxies, nil
}

func (r authProxyRepository) EnabledWebByDomainID(ctx context.Context, domainID string) ([]domain.Proxy, error) {
	proxies, err := r.ByDomainID(ctx, domainID)
	if err != nil {
		return nil, err
	}
	enabled := make([]domain.Proxy, 0)
	for _, proxy := range proxies {
		if proxy.Status == domain.ProxyEnabled && proxy.Type.IsWeb() {
			enabled = append(enabled, proxy)
		}
	}
	return enabled, nil
}

func (r authProxyRepository) ByDomainAndPath(ctx context.Context, domainID string, path string) (domain.Proxy, error) {
	proxies, err := r.EnabledWebByDomainID(ctx, domainID)
	if err != nil {
		return domain.Proxy{}, err
	}
	selected, ok := domain.SelectWebProxy(proxies, path)
	if !ok {
		return domain.Proxy{}, store.ErrNotFound
	}
	return selected, nil
}

func (r authProxyRepository) EnabledByType(_ context.Context, proxyType domain.ProxyType) ([]domain.Proxy, error) {
	proxies := make([]domain.Proxy, 0)
	for _, proxy := range r.proxies {
		if proxy.Type == proxyType && proxy.Status == domain.ProxyEnabled {
			proxies = append(proxies, proxy)
		}
	}
	return proxies, nil
}

func (r authProxyRepository) ByTCPEntryPort(context.Context, int) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByUDPEntryPort(context.Context, int) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByTCPEntry(context.Context, string, int, bool) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByUDPEntry(context.Context, string, int, bool) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByHTTPRoute(context.Context, string, int, string, bool) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByHTTPSRoute(context.Context, string, int, string, bool) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByHTTPHost(context.Context, string) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByHTTPSHost(context.Context, string) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByCertificateID(context.Context, string) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) SetStatus(context.Context, string, domain.ProxyStatus) error { return nil }

func (r authProxyRepository) Update(context.Context, domain.Proxy) error { return nil }

func (r authProxyRepository) Delete(context.Context, string) error { return nil }

type authDomainRepository struct{}

func (authDomainRepository) Create(context.Context, domain.Domain) error { return nil }
func (authDomainRepository) ByID(context.Context, string) (domain.Domain, error) {
	return domain.Domain{}, store.ErrNotFound
}
func (authDomainRepository) ByHost(context.Context, string) (domain.Domain, error) {
	return domain.Domain{}, store.ErrNotFound
}
func (authDomainRepository) ByCertificateID(context.Context, string) (domain.Domain, error) {
	return domain.Domain{}, store.ErrNotFound
}
func (authDomainRepository) List(context.Context) ([]domain.Domain, error) { return nil, nil }
func (authDomainRepository) ByUserID(context.Context, string) ([]domain.Domain, error) {
	return nil, nil
}
func (authDomainRepository) Update(context.Context, domain.Domain) error { return nil }
func (authDomainRepository) SetStatus(context.Context, string, domain.DomainStatus) error {
	return nil
}
func (authDomainRepository) Delete(context.Context, string) error { return nil }

type authDomainEntryRepository struct{}

func (authDomainEntryRepository) Create(context.Context, domain.DomainEntry) error { return nil }
func (authDomainEntryRepository) ByID(context.Context, string) (domain.DomainEntry, error) {
	return domain.DomainEntry{}, store.ErrNotFound
}
func (authDomainEntryRepository) ListByDomainID(context.Context, string) ([]domain.DomainEntry, error) {
	return nil, nil
}
func (authDomainEntryRepository) ListEnabled(context.Context) ([]domain.DomainEntry, error) {
	return nil, nil
}
func (authDomainEntryRepository) ByListener(context.Context, domain.DomainEntryProtocol, string, int, string, bool) (domain.Domain, domain.DomainEntry, error) {
	return domain.Domain{}, domain.DomainEntry{}, store.ErrNotFound
}
func (authDomainEntryRepository) Update(context.Context, domain.DomainEntry) error { return nil }
func (authDomainEntryRepository) Delete(context.Context, string) error             { return nil }
func (authDomainEntryRepository) DeleteByDomainID(context.Context, string) error   { return nil }

type authStatsRepository struct{}

type authCertificateRepository struct{}

func (authCertificateRepository) Create(context.Context, domain.ManagedCertificate) error {
	return nil
}

func (authCertificateRepository) ByProxyID(context.Context, string) (domain.ManagedCertificate, error) {
	return domain.ManagedCertificate{}, store.ErrNotFound
}

func (authCertificateRepository) ByHost(context.Context, string) (domain.ManagedCertificate, error) {
	return domain.ManagedCertificate{}, store.ErrNotFound
}

func (authCertificateRepository) ByID(context.Context, string) (domain.ManagedCertificate, error) {
	return domain.ManagedCertificate{}, store.ErrNotFound
}

func (authCertificateRepository) Delete(context.Context, string) error {
	return nil
}

func (authCertificateRepository) List(context.Context) ([]domain.ManagedCertificate, error) {
	return nil, nil
}

func (authCertificateRepository) ListByProxyIDs(context.Context, []string) ([]domain.ManagedCertificate, error) {
	return nil, nil
}

func (authCertificateRepository) ListRenewable(context.Context, time.Time, time.Time) ([]domain.ManagedCertificate, error) {
	return nil, nil
}

func (authCertificateRepository) ListLifecycleCandidates(context.Context, store.CertificateLifecycleCandidateQuery) ([]domain.ManagedCertificate, error) {
	return nil, nil
}

func (authCertificateRepository) UpdateSuccess(context.Context, string, store.CertificateSuccess) error {
	return nil
}

func (authCertificateRepository) UpdateFailure(context.Context, string, store.CertificateFailure) error {
	return nil
}

func (authCertificateRepository) UpdateHealth(context.Context, string, store.CertificateHealth) error {
	return nil
}

func (authCertificateRepository) UpdateProviderSync(context.Context, string, store.CertificateProviderSync) error {
	return nil
}

type authProviderCredentialRepository struct{}

func (authProviderCredentialRepository) Create(context.Context, domain.ProviderCredential) error {
	return nil
}

func (authProviderCredentialRepository) ByID(context.Context, string) (domain.ProviderCredential, error) {
	return domain.ProviderCredential{}, store.ErrNotFound
}

func (authProviderCredentialRepository) List(context.Context) ([]domain.ProviderCredential, error) {
	return nil, nil
}

func (authProviderCredentialRepository) ListByProviderType(context.Context, domain.CertificateProviderType, []domain.ProviderCredentialStatus) ([]domain.ProviderCredential, error) {
	return nil, nil
}

func (authProviderCredentialRepository) Update(context.Context, domain.ProviderCredential) error {
	return nil
}

func (authProviderCredentialRepository) SetStatus(context.Context, string, domain.ProviderCredentialStatus, *time.Time, string) error {
	return nil
}

func (authProviderCredentialRepository) Delete(context.Context, string) error {
	return nil
}

func (r authStatsRepository) Save(context.Context, []store.ProxyStats) error { return nil }

func (r authStatsRepository) List(context.Context) ([]store.ProxyStats, error) { return nil, nil }
