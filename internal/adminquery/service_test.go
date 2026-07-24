package adminquery

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
	"github.com/simp-frp/go-ginx-2/internal/systemclient"
)

func TestServiceHidesSystemUserAndMarksSystemObjects(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	if _, err := (systemclient.Service{Store: db}).Ensure(ctx); err != nil {
		t.Fatalf("ensure system client: %v", err)
	}
	proxy := domain.Proxy{ID: "local-proxy", UserID: systemclient.UserID, ClientID: systemclient.ClientID, Name: "local", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryBindHost: "127.0.0.1", EntryPort: 18080, TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Proxies().Create(systemclient.WithInternalMutation(ctx), proxy); err != nil {
		t.Fatalf("create local proxy: %v", err)
	}
	service := Service{Store: db}

	users, err := service.ListUsers(ctx, UserListInput{Page: PageInput{Page: 1, PageSize: 100}})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	for _, user := range users.Items {
		if user.ID == systemclient.UserID {
			t.Fatal("system owner must be hidden from normal user management")
		}
	}
	if _, err := service.UserDetail(ctx, systemclient.UserID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("system owner detail should be hidden, got %v", err)
	}

	client, err := service.ClientDetail(ctx, systemclient.ClientID)
	if err != nil || !client.IsSystem {
		t.Fatalf("system client should be marked, detail=%+v err=%v", client, err)
	}
	if len(client.ManagedProxies) != 1 || !client.ManagedProxies[0].IsSystem {
		t.Fatalf("system proxy summary should be marked: %+v", client.ManagedProxies)
	}
	proxyDetail, err := service.ProxyDetail(ctx, proxy.ID)
	if err != nil || !proxyDetail.IsSystem {
		t.Fatalf("system proxy should be marked, detail=%+v err=%v", proxyDetail, err)
	}
}

func TestServiceBuildsDashboardSummary(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	sessions := session.NewManager()
	registered, _, err := sessions.Register(session.RegisterInput{SessionID: "session-1", ClientID: "client-1", UserID: "user-1", Protocol: domain.ProtocolQUIC, ConfigVersion: 1})
	if err != nil {
		t.Fatalf("register session: %v", err)
	}
	_, err = sessions.Heartbeat(session.HeartbeatInput{SessionID: registered.ID, ConfigVersion: 1, Stats: session.HeartbeatStats{ActiveProxies: 1, ActiveStreams: 2, UploadBytes: 3, DownloadBytes: 4, ErrorSummary: "ok"}})
	if err != nil {
		t.Fatalf("heartbeat session: %v", err)
	}
	memory := stats.NewMemory()
	memory.RecordTCPStart("proxy-1")
	memory.RecordTCPEnd("proxy-1", 10, 20, true)
	memory.RecordUDP("proxy-2", 5, 6, true)
	memory.RecordHTTP("proxy-3", 200, 7, 8, true)

	service := Service{Store: db, Sessions: sessions, Stats: memory}
	summary, err := service.DashboardSummary(ctx)
	if err != nil {
		t.Fatalf("dashboard summary: %v", err)
	}
	if summary.OnlineClientCount != 1 || summary.EnabledProxyCount != 3 || summary.ActiveTCPConnectionCount != 0 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.CumulativeUploadBytes != 22 || summary.CumulativeDownloadBytes != 34 {
		t.Fatalf("unexpected summary bytes: %+v", summary)
	}
	if summary.CumulativeTCPErrorCount != 1 || summary.CumulativeUDPErrorCount != 1 || summary.CumulativeHTTPErrorCount != 1 {
		t.Fatalf("unexpected summary errors: %+v", summary)
	}
}

func TestServiceProjectsRuntimeSessionAsOnlineClientStatus(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	if err := db.Clients().SetStatus(ctx, "client-1", domain.ClientOffline); err != nil {
		t.Fatalf("set client offline: %v", err)
	}
	sessions := session.NewManager()
	registered, _, err := sessions.Register(session.RegisterInput{SessionID: "session-1", ClientID: "client-1", UserID: "user-1", Protocol: domain.ProtocolQUIC, ConfigVersion: 2})
	if err != nil {
		t.Fatalf("register session: %v", err)
	}
	if _, err := sessions.Heartbeat(session.HeartbeatInput{SessionID: registered.ID, ConfigVersion: 2, Stats: session.HeartbeatStats{ActiveProxies: 1, ActiveStreams: 2}}); err != nil {
		t.Fatalf("heartbeat session: %v", err)
	}
	service := Service{Store: db, Sessions: sessions}

	page, err := service.ListClients(ctx, ClientListInput{Filter: ClientFilter{Status: string(domain.ClientOnline)}})
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Status != domain.ClientOnline || !page.Items[0].Runtime.Online {
		t.Fatalf("expected runtime online list item, got %+v", page.Items)
	}

	detail, err := service.ClientDetail(ctx, "client-1")
	if err != nil {
		t.Fatalf("client detail: %v", err)
	}
	if detail.Status != domain.ClientOnline || !detail.Runtime.Online {
		t.Fatalf("expected runtime online detail, got %+v", detail)
	}
}

func TestServiceListsRecentAuditEvents(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	now := time.Now().UTC()
	if err := db.AuditEvents().Create(ctx, domain.AuditEvent{ID: "audit-1", ActorUserID: "admin-1", ResourceType: "proxy", ResourceID: "proxy-1", Action: "create_proxy", Result: "success", CreatedAt: now.Add(-time.Minute)}); err != nil {
		t.Fatalf("create audit 1: %v", err)
	}
	if err := db.AuditEvents().Create(ctx, domain.AuditEvent{ID: "audit-2", ActorUserID: "admin-1", ResourceType: "user", ResourceID: "user-1", Action: "disable_user", Result: "success", CreatedAt: now}); err != nil {
		t.Fatalf("create audit 2: %v", err)
	}
	service := Service{Store: db}
	events, err := service.ListRecentAuditEvents(ctx, AuditListInput{Page: PageInput{Page: 1, PageSize: 10}})
	if err != nil {
		t.Fatalf("list recent audit: %v", err)
	}
	if len(events.Items) != 2 || events.Items[0].ID != "audit-2" || events.Items[1].ID != "audit-1" {
		t.Fatalf("unexpected audit ordering: %+v", events)
	}
	if events.Items[0].ActorType != "admin" || events.Items[0].ActorID != "admin-1" {
		t.Fatalf("unexpected audit actor identity: %+v", events.Items[0])
	}
}

func TestServiceManagedCertificateSummaryDoesNotMutateFailureState(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	proxy := domain.Proxy{ID: "proxy-https", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	certificateDir := t.TempDir()
	notAfter := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	certPEM, keyPEM := queryTestCertificatePEM(t, proxy.EntryHost, notAfter)
	stored, err := httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}.Store(proxy.EntryHost, certPEM, keyPEM)
	if err != nil {
		t.Fatalf("store certificate material: %v", err)
	}
	lastCheckedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	certificate := domain.ManagedCertificate{ID: "cert-https", ProxyID: proxy.ID, Host: proxy.EntryHost, Status: domain.CertificateRenewalFailed, ServingStatus: domain.CertificateServingUsable, OperationStatus: domain.CertificateOperationRenewalFailed, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", ProviderStatus: domain.CertificateProviderStatusActive, CertFile: stored.CertFile, KeyFile: stored.KeyFile, NotAfter: &notAfter, LastCheckedAt: &lastCheckedAt, FailureCount: 1, Fingerprint: "oldfingerprint", LastError: "cloudflare rotation failed"}
	if err := db.Certificates().Create(ctx, certificate); err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	service := Service{Store: db, CertificateDir: certificateDir}

	page, err := service.ListManagedCertificates(ctx, CertificateListInput{Page: PageInput{Page: 1, PageSize: 10}})
	if err != nil {
		t.Fatalf("list managed certificates: %v", err)
	}
	var summary *ManagedCertificateSummary
	for index := range page.Items {
		if page.Items[index].ProxyID == proxy.ID {
			summary = &page.Items[index]
			break
		}
	}
	if summary == nil {
		t.Fatalf("expected https certificate summary in %+v", page.Items)
	}
	if summary.Status != domain.CertificateRenewalFailed || summary.ServingStatus != domain.CertificateServingUsable || summary.LastError != "cloudflare rotation failed" {
		t.Fatalf("summary should preserve lifecycle failure while showing active health: %+v", summary)
	}
	reloaded, err := db.Certificates().ByProxyID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("reload certificate: %v", err)
	}
	if reloaded.Status != domain.CertificateRenewalFailed || reloaded.LastError != "cloudflare rotation failed" || reloaded.Fingerprint != "oldfingerprint" || reloaded.LastCheckedAt == nil || !reloaded.LastCheckedAt.Equal(lastCheckedAt) {
		t.Fatalf("read path should not mutate persisted certificate state: %+v", reloaded)
	}
}

func TestServiceCertificateDetailUsesScopedCertificateLookup(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	proxy := seedQueryHTTPSCertificate(t, ctx, db)
	countingRepo := &queryCountingCertificateRepository{CertificateRepository: db.Certificates()}
	service := Service{Store: queryCountingStore{Store: db, certificates: countingRepo}}

	detail, err := service.ProxyDetail(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("proxy detail: %v", err)
	}
	if detail.Certificate == nil || detail.Certificate.ProxyID != proxy.ID {
		t.Fatalf("expected certificate summary on proxy detail: %+v", detail.Certificate)
	}
	if countingRepo.listCount != 0 || countingRepo.byProxyIDCount != 1 {
		t.Fatalf("expected scoped detail lookup, list=%d byProxyID=%d", countingRepo.listCount, countingRepo.byProxyIDCount)
	}
}

func TestServiceManagedCertificateListEnumeratesAllCertificateResources(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	proxy := seedQueryBoundHTTPSCertificate(t, ctx, db)
	// 额外种入一个未绑定证书资源，验证其出现在清单中。
	unbound := domain.ManagedCertificate{ID: "cert-unbound", Host: "unbound.example.com", Status: domain.CertificatePending, ServingStatus: domain.CertificateServingMissing, OperationStatus: domain.CertificateOperationIdle, ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: "cloudflare", ProviderStatus: domain.CertificateProviderStatusUnknown}
	if err := db.Certificates().Create(ctx, unbound); err != nil {
		t.Fatalf("create unbound certificate: %v", err)
	}
	countingRepo := &queryCountingCertificateRepository{CertificateRepository: db.Certificates()}
	service := Service{Store: queryCountingStore{Store: db, certificates: countingRepo}}

	page, err := service.ListManagedCertificates(ctx, CertificateListInput{Page: PageInput{Page: 1, PageSize: 10}})
	if err != nil {
		t.Fatalf("list managed certificates: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected bound and unbound certificates, got %+v", page.Items)
	}
	byID := make(map[string]ManagedCertificateSummary, len(page.Items))
	for _, item := range page.Items {
		byID[item.CertificateID] = item
	}
	bound, ok := byID["cert-https"]
	if !ok || bound.BoundProxyID != proxy.ID || !bound.Referenced || !bound.Servable || bound.DeletionRisk != CertificateDeletionRiskRequiresStrongConfirmation {
		t.Fatalf("unexpected bound certificate summary: %+v", bound)
	}
	free, ok := byID["cert-unbound"]
	if !ok || free.Referenced || free.BoundProxyID != "" || free.Servable || free.DeletionRisk != CertificateDeletionRiskLow {
		t.Fatalf("unexpected unbound certificate summary: %+v", free)
	}
	// 清单经由 List() 枚举全部证书资源，不再按 proxy_id 批量回查。
	if countingRepo.listCount != 1 || countingRepo.listByProxyIDsCount != 0 {
		t.Fatalf("expected single List() enumeration, list=%d listByProxyIDs=%d", countingRepo.listCount, countingRepo.listByProxyIDsCount)
	}
}

func openQueryTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "query.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedQueryTestData(t *testing.T, ctx context.Context, db *sqlite.Store) {
	t.Helper()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOnline, CredentialHash: domain.HashCredential("secret")}
	proxies := []domain.Proxy{
		{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "tcp", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22},
		{ID: "proxy-2", UserID: user.ID, ClientID: client.ID, Name: "udp", Type: domain.ProxyUDP, Status: domain.ProxyEnabled, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 53},
		{ID: "proxy-3", UserID: user.ID, ClientID: client.ID, Name: "http", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
	}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	for _, proxy := range proxies {
		if err := db.Proxies().Create(ctx, proxy); err != nil {
			t.Fatalf("create proxy %s: %v", proxy.ID, err)
		}
	}
}

func seedQueryHTTPSCertificate(t *testing.T, ctx context.Context, db *sqlite.Store) domain.Proxy {
	t.Helper()
	proxy := domain.Proxy{ID: "proxy-https", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	notAfter := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	certPEM, keyPEM := queryTestCertificatePEM(t, proxy.EntryHost, notAfter)
	stored, err := httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}.Store(proxy.EntryHost, certPEM, keyPEM)
	if err != nil {
		t.Fatalf("store certificate material: %v", err)
	}
	certificate := domain.ManagedCertificate{ID: "cert-https", ProxyID: proxy.ID, Host: proxy.EntryHost, Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable, OperationStatus: domain.CertificateOperationIdle, ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: "cloudflare", ProviderStatus: domain.CertificateProviderStatusUnknown, CertFile: stored.CertFile, KeyFile: stored.KeyFile, NotAfter: &notAfter}
	if err := db.Certificates().Create(ctx, certificate); err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return proxy
}

// seedQueryBoundHTTPSCertificate 种入一个通过 certificate_id 权威绑定到代理的可服务证书。
func seedQueryBoundHTTPSCertificate(t *testing.T, ctx context.Context, db *sqlite.Store) domain.Proxy {
	t.Helper()
	proxy := domain.Proxy{ID: "proxy-https", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443, CertificateID: "cert-https"}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	notAfter := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	certPEM, keyPEM := queryTestCertificatePEM(t, proxy.EntryHost, notAfter)
	stored, err := httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}.Store(proxy.EntryHost, certPEM, keyPEM)
	if err != nil {
		t.Fatalf("store certificate material: %v", err)
	}
	certificate := domain.ManagedCertificate{ID: "cert-https", Host: proxy.EntryHost, Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable, OperationStatus: domain.CertificateOperationIdle, ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: "cloudflare", ProviderStatus: domain.CertificateProviderStatusUnknown, CertFile: stored.CertFile, KeyFile: stored.KeyFile, NotAfter: &notAfter}
	if err := db.Certificates().Create(ctx, certificate); err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return proxy
}

func queryTestCertificatePEM(t *testing.T, host string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

type queryCountingStore struct {
	*sqlite.Store
	certificates *queryCountingCertificateRepository
}

func (qs queryCountingStore) Certificates() store.CertificateRepository {
	if qs.certificates != nil {
		return qs.certificates
	}
	return qs.Store.Certificates()
}

type queryCountingCertificateRepository struct {
	store.CertificateRepository
	listCount           int
	byProxyIDCount      int
	listByProxyIDsCount int
}

func (repo *queryCountingCertificateRepository) List(ctx context.Context) ([]domain.ManagedCertificate, error) {
	repo.listCount++
	return repo.CertificateRepository.List(ctx)
}

func (repo *queryCountingCertificateRepository) ByProxyID(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	repo.byProxyIDCount++
	return repo.CertificateRepository.ByProxyID(ctx, proxyID)
}

func (repo *queryCountingCertificateRepository) ListByProxyIDs(ctx context.Context, proxyIDs []string) ([]domain.ManagedCertificate, error) {
	repo.listByProxyIDsCount++
	return repo.CertificateRepository.ListByProxyIDs(ctx, proxyIDs)
}
