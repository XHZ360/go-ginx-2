package control

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
)

func TestQUICHandshakeRegistersSession(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestListener(t, Authenticator{
		Store:             newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		AllowedProtocols:  []domain.Protocol{domain.ProtocolQUIC},
		HeartbeatInterval: 10 * time.Second,
		Now:               func() time.Time { return now },
	})

	client, response, err := DialAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), nil, AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if !response.Accepted || response.SessionID != "session-1" || response.SelectedProtocol != domain.ProtocolQUIC {
		t.Fatalf("unexpected auth response: %+v", response)
	}
	latest, ok := sessions.Latest("client-1")
	if !ok || latest.ID != response.SessionID || latest.UserID != "user-1" {
		t.Fatalf("expected registered latest session, got %+v", latest)
	}
}

func TestQUICHandshakeRejectsWrongCredential(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestListener(t, Authenticator{
		Store: newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		Now:   func() time.Time { return now },
	})

	client, response, err := DialAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), nil, AuthRequest{ClientID: "client-1", Credential: "wrong", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	if client != nil {
		t.Fatal("rejected authentication should not return client connection")
	}
	if response.Accepted || response.Reason == "" {
		t.Fatalf("expected rejected auth response, got %+v", response)
	}
	if _, ok := sessions.Latest("client-1"); ok {
		t.Fatal("rejected client must not register a session")
	}
}

func TestQUICHeartbeatUpdatesSession(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestListener(t, Authenticator{
		Store: newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		Now:   func() time.Time { return now },
	})

	client, response, err := DialAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), nil, AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.SendHeartbeat(Heartbeat{SessionID: response.SessionID, ClientID: "client-1", ObservedAt: now, ConfigVersion: 9, ActiveProxies: 2, ActiveStreams: 3, UploadBytes: 128, DownloadBytes: 256}); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		latest, ok := sessions.Latest("client-1")
		if ok && latest.ConfigVersion == 9 && latest.Stats.ActiveStreams == 3 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("heartbeat did not update session before deadline")
}

func TestQUICDialRejectsUntrustedServerCertificate(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, _ := startTestListener(t, Authenticator{
		Store: newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		Now:   func() time.Time { return now },
	})

	_, _, err := DialAndAuthenticate(context.Background(), listener.Addr().String(), &tls.Config{ServerName: "localhost", NextProtos: []string{ControlALPN}, MinVersion: tls.VersionTLS13}, nil, AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err == nil {
		t.Fatal("expected certificate verification error")
	}
}

func startTestListener(t *testing.T, authenticator Authenticator) (*Listener, *session.Manager) {
	t.Helper()
	sessions := session.NewManager()
	listener, err := ListenAddr("127.0.0.1:0", Server{
		Authenticator: authenticator,
		Sessions:      sessions,
		TLSConfig:     testServerTLSConfig(t),
		NewSessionID:  func() (string, error) { return "session-1", nil },
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
	})
	go func() { _ = listener.Serve(ctx) }()
	return listener, sessions
}

func testServerTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	cert, _ := testCertificate(t)
	return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{ControlALPN}, MinVersion: tls.VersionTLS13}
}

func testClientTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	_, pool := testCertificate(t)
	return &tls.Config{RootCAs: pool, ServerName: "localhost", NextProtos: []string{ControlALPN}, MinVersion: tls.VersionTLS13}
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	testTLSOnce.Do(func() {
		testTLSCert, testTLSPool, testTLSErr = generateTestCertificate()
	})
	if testTLSErr != nil {
		t.Fatal(testTLSErr)
	}
	return testTLSCert, testTLSPool.Clone()
}

var (
	testTLSOnce sync.Once
	testTLSCert tls.Certificate
	testTLSPool *x509.CertPool
	testTLSErr  error
)

func generateTestCertificate() (tls.Certificate, *x509.CertPool, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "go-ginx-test-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, BasicConstraintsValid: true, IsCA: true}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	serverTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})) {
		return tls.Certificate{}, nil, errors.New("append CA cert")
	}
	return cert, pool, nil
}
