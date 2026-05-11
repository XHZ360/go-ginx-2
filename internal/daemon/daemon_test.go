package daemon

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestStartServerWiresRuntimeListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, _ := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "server.db")
	tcpPort := reservePort(t)
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "tcp-1", UserID: "user-1", ClientID: "client-1", Name: "ssh", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: tcpPort, TargetHost: "127.0.0.1", TargetPort: 22}})

	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	if runtime.ControlListener == nil || runtime.ControlListener.Addr() == nil {
		t.Fatal("expected control listener")
	}
	if len(runtime.TCPListeners) != 1 || runtime.TCPListeners[0].Addr() == nil {
		t.Fatalf("expected one TCP listener, got %d", len(runtime.TCPListeners))
	}
	if runtime.HTTPServer == nil || runtime.HTTPServer.Addr() == nil {
		t.Fatal("expected HTTP server")
	}
}

func TestRunClientAuthenticatesAndSendsHeartbeat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "client.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(ctx, config.Client{ServerAddress: runtime.ControlListener.Addr().String(), ServerName: "localhost", ServerCAFile: caFile, ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.Reconnect{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		latest, ok := runtime.Sessions.Latest("client-1")
		if ok && latest.Stats.ActiveProxies == 1 {
			cancel()
			if err := <-clientDone; err != nil {
				t.Fatalf("run client: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatalf("client heartbeat did not update session")
}

func seedDatabase(t *testing.T, path string, proxies []domain.Proxy) {
	t.Helper()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
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

func reservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func writeTestTLSFiles(t *testing.T) (string, string, string) {
	t.Helper()
	certPEM, keyPEM, caPEM := generateTestCertificate(t)
	dir := t.TempDir()
	certFile := filepath.Join(dir, "control.crt")
	keyFile := filepath.Join(dir, "control.key")
	caFile := filepath.Join(dir, "ca.crt")
	writeFile(t, certFile, certPEM)
	writeFile(t, keyFile, keyPEM)
	writeFile(t, caFile, caPEM)
	return certFile, keyFile, caFile
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func generateTestCertificate(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "go-ginx-test-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, BasicConstraintsValid: true, IsCA: true}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}
