package admin

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

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
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
	udpProxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "udp-1", UserID: user.ID, ClientID: client.ID, Name: "dns", Type: domain.ProxyUDP, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 53, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create udp proxy: %v", err)
	}
	foundUDP, err := db.Proxies().ByUDPEntryPort(ctx, 10053)
	if err != nil {
		t.Fatalf("lookup udp proxy: %v", err)
	}
	if foundUDP.ID != udpProxy.ID || foundUDP.Status != domain.ProxyEnabled {
		t.Fatalf("unexpected udp proxy: %+v", foundUDP)
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
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: "user-1", ClientID: "client-1", Name: "udp", Type: domain.ProxyUDP, TargetHost: "127.0.0.1", TargetPort: 53}); err == nil {
		t.Fatal("expected missing UDP entry port error")
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

func TestServiceIssuesRenewsAndReportsManagedCertificate(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db, Certificates: certmanager.Service{Issuer: adminFakeIssuer{}, DNSProvider: adminFakeDNSProvider{}, Storage: certmanagerTestStorage(t), Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return time.Now().UTC() }}}

	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	certificate, err := service.IssueManagedCertificate(ctx, CertificateInput{ProxyID: proxy.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	if certificate.Status != domain.CertificateValid {
		t.Fatalf("unexpected issued certificate: %+v", certificate)
	}
	renewed, err := service.RenewManagedCertificate(ctx, CertificateInput{ProxyID: proxy.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("renew certificate: %v", err)
	}
	if renewed.Status != domain.CertificateValid {
		t.Fatalf("unexpected renewed certificate: %+v", renewed)
	}
	status, err := service.ManagedCertificateStatus(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("status certificate: %v", err)
	}
	if status.Certificate.ID != renewed.ID {
		t.Fatalf("unexpected certificate status: %+v", status.Certificate)
	}
}

type adminFakeDNSProvider struct{}

func (adminFakeDNSProvider) Present(context.Context, string, string) error { return nil }
func (adminFakeDNSProvider) CleanUp(context.Context, string, string) error { return nil }

type adminFakeIssuer struct{}

func (adminFakeIssuer) Issue(context.Context, certmanager.IssueRequest) (certmanager.IssuedCertificate, error) {
	certPEM, keyPEM, notAfter := adminTestCertificatePEM("app.example.com", time.Now().Add(time.Hour))
	return certmanager.IssuedCertificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
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

func certmanagerTestStorage(t *testing.T) httpsproxy.ManagedCertificateStorage {
	t.Helper()
	return httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir(), Now: func() time.Time { return time.Now().UTC() }}
}

func adminTestCertificatePEM(host string, notAfter time.Time) ([]byte, []byte, time.Time) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, notAfter
}
