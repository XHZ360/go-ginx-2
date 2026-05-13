package certmanager

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
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServiceIssuesManagedCertificate(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	certPEM, keyPEM := testCertificatePEM(t, "app.example.com", time.Now().Add(time.Hour))
	service := Service{Store: db, Issuer: fakeIssuer{certPEM: certPEM, keyPEM: keyPEM}, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}

	certificate, err := service.Issue(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	if certificate.Status != domain.CertificateValid || certificate.CertFile == "" || certificate.KeyFile == "" || certificate.NotAfter == nil {
		t.Fatalf("unexpected certificate: %+v", certificate)
	}
	if _, err := service.Status(ctx, "proxy-1"); err != nil {
		t.Fatalf("status: %v", err)
	}
}

func TestServiceRecordsIssueFailure(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	service := Service{Store: db, Issuer: fakeIssuer{err: errors.New("dns failed")}, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}

	if _, err := service.Issue(ctx, "proxy-1"); err == nil {
		t.Fatal("expected issue failure")
	}
	certificate, err := db.Certificates().ByProxyID(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("lookup certificate: %v", err)
	}
	if certificate.Status != domain.CertificateIssueFailed || certificate.LastError != "dns failed" {
		t.Fatalf("unexpected failure metadata: %+v", certificate)
	}
}

func TestServiceRenewsAndRetainsPreviousFiles(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	firstCert, firstKey := testCertificatePEM(t, "app.example.com", time.Now().Add(time.Hour))
	secondCert, secondKey := testCertificatePEM(t, "app.example.com", time.Now().Add(2*time.Hour))
	issuer := &sequenceIssuer{certs: []IssuedCertificate{{CertPEM: firstCert, KeyPEM: firstKey}, {CertPEM: secondCert, KeyPEM: secondKey}}}
	service := Service{Store: db, Issuer: issuer, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}

	issued, err := service.Issue(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	renewed, err := service.Renew(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("renew certificate: %v", err)
	}
	if renewed.PreviousCertFile == "" || renewed.PreviousKeyFile == "" || renewed.CertFile != issued.CertFile || renewed.KeyFile != issued.KeyFile {
		t.Fatalf("expected renewal to retain previous files: issued=%+v renewed=%+v", issued, renewed)
	}
}

type fakeIssuer struct {
	certPEM []byte
	keyPEM  []byte
	err     error
}

func (issuer fakeIssuer) Issue(context.Context, IssueRequest) (IssuedCertificate, error) {
	if issuer.err != nil {
		return IssuedCertificate{}, issuer.err
	}
	return IssuedCertificate{CertPEM: issuer.certPEM, KeyPEM: issuer.keyPEM}, nil
}

type sequenceIssuer struct {
	certs []IssuedCertificate
	next  int
}

func (issuer *sequenceIssuer) Issue(context.Context, IssueRequest) (IssuedCertificate, error) {
	if issuer.next >= len(issuer.certs) {
		return IssuedCertificate{}, errors.New("no certificate")
	}
	cert := issuer.certs[issuer.next]
	issuer.next++
	return cert, nil
}

type fakeDNSProvider struct{}

func (fakeDNSProvider) Present(context.Context, string, string) error { return nil }
func (fakeDNSProvider) CleanUp(context.Context, string, string) error { return nil }

func openTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedHTTPSProxy(t *testing.T, ctx context.Context, db *sqlite.Store) {
	t.Helper()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	proxy := domain.Proxy{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
}

func testCertificatePEM(t *testing.T, host string, notAfter time.Time) ([]byte, []byte) {
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
