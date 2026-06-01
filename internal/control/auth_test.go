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
	user    domain.User
	client  domain.Client
	proxies []domain.Proxy
}

func newAuthStore(userStatus domain.UserStatus, clientStatus domain.ClientStatus, credentialHash string) authStore {
	return authStore{
		user:   domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: userStatus},
		client: domain.Client{ID: "client-1", UserID: "user-1", Name: "home", Status: clientStatus, CredentialHash: credentialHash, Version: 7},
	}
}

func (s authStore) Users() store.UserRepository { return authUserRepository{s.user} }

func (s authStore) Clients() store.ClientRepository { return authClientRepository{s.client} }

func (s authStore) ClientEnrollments() store.ClientEnrollmentRepository {
	return authClientEnrollmentRepository{}
}

func (s authStore) Proxies() store.ProxyRepository { return authProxyRepository{s.proxies} }

func (s authStore) Certificates() store.CertificateRepository { return authCertificateRepository{} }

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

type authClientRepository struct{ client domain.Client }

func (r authClientRepository) Create(context.Context, domain.Client) error { return nil }

func (r authClientRepository) ByID(_ context.Context, id string) (domain.Client, error) {
	if id != r.client.ID {
		return domain.Client{}, store.ErrNotFound
	}
	return r.client, nil
}

func (r authClientRepository) List(context.Context) ([]domain.Client, error) {
	return []domain.Client{r.client}, nil
}

func (r authClientRepository) SetStatus(context.Context, string, domain.ClientStatus) error {
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

func (r authProxyRepository) ByHTTPHost(context.Context, string) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) ByHTTPSHost(context.Context, string) (domain.Proxy, error) {
	return domain.Proxy{}, store.ErrNotFound
}

func (r authProxyRepository) SetStatus(context.Context, string, domain.ProxyStatus) error { return nil }

func (r authProxyRepository) Update(context.Context, domain.Proxy) error { return nil }

func (r authProxyRepository) Delete(context.Context, string) error { return nil }

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

func (authCertificateRepository) List(context.Context) ([]domain.ManagedCertificate, error) {
	return nil, nil
}

func (authCertificateRepository) ListRenewable(context.Context, time.Time) ([]domain.ManagedCertificate, error) {
	return nil, nil
}

func (authCertificateRepository) UpdateSuccess(context.Context, string, store.CertificateSuccess) error {
	return nil
}

func (authCertificateRepository) UpdateFailure(context.Context, string, store.CertificateFailure) error {
	return nil
}

func (r authStatsRepository) Save(context.Context, []store.ProxyStats) error { return nil }

func (r authStatsRepository) List(context.Context) ([]store.ProxyStats, error) { return nil, nil }
