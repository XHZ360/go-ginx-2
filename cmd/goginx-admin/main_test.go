package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestRunCreatesResources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin.db")

	if err := run([]string{"create-user", "-db", dbPath, "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client", "-db", dbPath, "-id", "client-1", "-user", "user-1", "-name", "home", "-credential", "secret"}); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := run([]string{"create-tcp-proxy", "-db", dbPath, "-id", "proxy-1", "-user", "user-1", "-client", "client-1", "-name", "ssh", "-port", "10022", "-target-host", "127.0.0.1", "-target-port", "22"}); err != nil {
		t.Fatalf("create tcp proxy: %v", err)
	}
	if err := run([]string{"create-udp-proxy", "-db", dbPath, "-id", "udp-1", "-user", "user-1", "-client", "client-1", "-name", "dns", "-port", "10053", "-target-host", "127.0.0.1", "-target-port", "53"}); err != nil {
		t.Fatalf("create udp proxy: %v", err)
	}
	if err := run([]string{"create-https-proxy", "-db", dbPath, "-id", "https-1", "-user", "user-1", "-client", "client-1", "-name", "secure", "-host", "secure.example.com", "-target-host", "127.0.0.1", "-target-port", "8443"}); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	found, err := db.Proxies().ByTCPEntryPort(context.Background(), 10022)
	if err != nil {
		t.Fatalf("lookup tcp proxy: %v", err)
	}
	if found.ID != "proxy-1" {
		t.Fatalf("unexpected proxy: %+v", found)
	}
	foundUDP, err := db.Proxies().ByUDPEntryPort(context.Background(), 10053)
	if err != nil {
		t.Fatalf("lookup udp proxy: %v", err)
	}
	if foundUDP.ID != "udp-1" {
		t.Fatalf("unexpected udp proxy: %+v", foundUDP)
	}
	foundHTTPS, err := db.Proxies().ByHTTPSHost(context.Background(), "secure.example.com")
	if err != nil {
		t.Fatalf("lookup https proxy: %v", err)
	}
	if foundHTTPS.ID != "https-1" {
		t.Fatalf("unexpected https proxy: %+v", foundHTTPS)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"unknown"}); err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestRunManagesCertificates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin.db")
	t.Setenv("CF_DNS_API_TOKEN", "token")
	oldIssuer := newACMEIssuer
	oldProvider := newDNSProvider
	newACMEIssuer = func() certmanager.Issuer { return adminMainFakeIssuer{} }
	newDNSProvider = func(string) (certmanager.DNSChallengeProvider, error) { return adminMainFakeDNSProvider{}, nil }
	t.Cleanup(func() {
		newACMEIssuer = oldIssuer
		newDNSProvider = oldProvider
	})

	if err := run([]string{"create-user", "-db", dbPath, "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client", "-db", dbPath, "-id", "client-1", "-user", "user-1", "-name", "home", "-credential", "secret"}); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := run([]string{"create-https-proxy", "-db", dbPath, "-id", "https-1", "-user", "user-1", "-client", "client-1", "-name", "secure", "-host", "app.example.com", "-target-host", "127.0.0.1", "-target-port", "8080"}); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	certDir := t.TempDir()
	if err := run([]string{"issue-managed-certificate", "-db", dbPath, "-proxy", "https-1", "-certificate-dir", certDir, "-acme-account-email", "ops@example.com", "-acme-terms-accepted"}); err != nil {
		t.Fatalf("issue managed certificate: %v", err)
	}
	if err := run([]string{"renew-managed-certificate", "-db", dbPath, "-proxy", "https-1", "-certificate-dir", certDir, "-acme-account-email", "ops@example.com", "-acme-terms-accepted"}); err != nil {
		t.Fatalf("renew managed certificate: %v", err)
	}
	if err := run([]string{"managed-certificate-status", "-db", dbPath, "-proxy", "https-1", "-certificate-dir", certDir, "-acme-account-email", "ops@example.com", "-acme-terms-accepted"}); err != nil {
		t.Fatalf("certificate status: %v", err)
	}

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	certificate, err := db.Certificates().ByProxyID(context.Background(), "https-1")
	if err != nil {
		t.Fatalf("lookup certificate: %v", err)
	}
	if certificate.CertFile == "" || certificate.KeyFile == "" || certificate.Status == "" {
		t.Fatalf("unexpected certificate metadata: %+v", certificate)
	}
}

func TestRunBuildsDeployBundle(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := run([]string{"build-deploy-bundle", "-output", outputDir, "-goos", runtime.GOOS, "-goarch", runtime.GOARCH, "-install-root", "/opt/go-ginx"}); err != nil {
		t.Fatalf("build deploy bundle: %v", err)
	}
	for _, path := range []string{
		filepath.Join(outputDir, "bin", bundleBinaryName("goginx-server")),
		filepath.Join(outputDir, "bin", bundleBinaryName("goginx-client")),
		filepath.Join(outputDir, "bin", bundleBinaryName("goginx-admin")),
		filepath.Join(outputDir, "config", "server.json"),
		filepath.Join(outputDir, "config", "client.json"),
		filepath.Join(outputDir, "config", "admin-credentials.json.example"),
		filepath.Join(outputDir, "config", "goginx-server.env.example"),
		filepath.Join(outputDir, "config", "goginx-client.env.example"),
		filepath.Join(outputDir, "systemd", "goginx-server.service"),
		filepath.Join(outputDir, "systemd", "goginx-client.service"),
		filepath.Join(outputDir, "data", "certs", "managed"),
		filepath.Join(outputDir, "logs"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func bundleBinaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

type adminMainFakeDNSProvider struct{}

func (adminMainFakeDNSProvider) Present(context.Context, string, string) error { return nil }
func (adminMainFakeDNSProvider) CleanUp(context.Context, string, string) error { return nil }

type adminMainFakeIssuer struct{}

func (adminMainFakeIssuer) Issue(context.Context, certmanager.IssueRequest) (certmanager.IssuedCertificate, error) {
	certPEM, keyPEM, notAfter := adminMainTestCertificatePEM("app.example.com", time.Now().Add(time.Hour))
	return certmanager.IssuedCertificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

func adminMainTestCertificatePEM(host string, notAfter time.Time) ([]byte, []byte, time.Time) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), notAfter
}
