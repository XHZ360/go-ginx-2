package httpproxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestHTTPEntryProxiesThroughQUICClientStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	origin := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.URL.Path != "/hello" || string(body) != "request-body" || r.Header.Get("X-Test") != "yes" {
			t.Fatalf("unexpected origin request path=%s body=%q header=%s", r.URL.Path, string(body), r.Header.Get("X-Test"))
		}
		w.Header().Set("X-Origin", "ok")
		w.WriteHeader(nethttp.StatusCreated)
		_, _ = w.Write([]byte("origin-response"))
	}))
	t.Cleanup(origin.Close)
	originURL, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	originHost, originPortText, err := net.SplitHostPort(originURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	originPort, err := strconv.Atoi(originPortText)
	if err != nil {
		t.Fatal(err)
	}

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedHTTPProxy(t, ctx, db, originHost, originPort)

	sessions := session.NewManager()
	controlListener, err := control.ListenAddr("127.0.0.1:0", control.Server{
		Authenticator: control.Authenticator{Store: db, Now: func() time.Time { return time.Now().UTC() }},
		Sessions:      sessions,
		TLSConfig:     testServerTLSConfig(t),
		NewSessionID:  func() (string, error) { return "session-1", nil },
	})
	if err != nil {
		t.Fatalf("control listen: %v", err)
	}
	t.Cleanup(func() { _ = controlListener.Close() })
	go func() { _ = controlListener.Serve(ctx) }()

	client, response, err := control.DialAndAuthenticate(ctx, controlListener.Addr().String(), testClientTLSConfig(t), nil, control.AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: time.Now().UTC(), Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	if !response.Accepted {
		t.Fatalf("expected accepted auth response: %+v", response)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	go func() { _ = client.ServeProxyStreams(ctx) }()
	memoryStats := stats.NewMemory()

	entry, err := Listen(Entry{Store: db, Sessions: sessions, ListenAddress: "127.0.0.1:0", NewRequest: func() (string, error) { return "req-1", nil }, Stats: memoryStats})
	if err != nil {
		t.Fatalf("http listen: %v", err)
	}
	t.Cleanup(func() { _ = entry.Close() })
	go func() { _ = entry.Serve(ctx) }()

	request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, "http://"+entry.Addr().String()+"/hello", strings.NewReader("request-body"))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "app.example.com"
	request.Header.Set("X-Test", "yes")
	responseFromProxy, err := nethttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer responseFromProxy.Body.Close()
	responseBody, err := io.ReadAll(responseFromProxy.Body)
	if err != nil {
		t.Fatal(err)
	}
	if responseFromProxy.StatusCode != nethttp.StatusCreated || responseFromProxy.Header.Get("X-Origin") != "ok" || string(responseBody) != "origin-response" {
		t.Fatalf("unexpected proxy response status=%d header=%s body=%q", responseFromProxy.StatusCode, responseFromProxy.Header.Get("X-Origin"), string(responseBody))
	}
	snapshot := memoryStats.Snapshot("proxy-1")
	if snapshot.HTTPRequests != 1 || snapshot.HTTPStatusCodes[nethttp.StatusCreated] != 1 || snapshot.HTTPUploadBytes != int64(len("request-body")) || snapshot.HTTPDownloadBytes != int64(len("origin-response")) || snapshot.HTTPErrors != 0 {
		t.Fatalf("unexpected HTTP stats: %+v", snapshot)
	}
}

func seedHTTPProxy(t *testing.T, ctx context.Context, db *sqlite.Store, targetHost string, targetPort int) {
	t.Helper()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	proxy := domain.Proxy{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: targetHost, TargetPort: targetPort}
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

func testServerTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	cert, _ := testCertificate(t)
	return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{control.ControlALPN}, MinVersion: tls.VersionTLS13}
}

func testClientTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	_, pool := testCertificate(t)
	return &tls.Config{RootCAs: pool, ServerName: "localhost", NextProtos: []string{control.ControlALPN}, MinVersion: tls.VersionTLS13}
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
