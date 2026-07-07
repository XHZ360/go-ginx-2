package sdk

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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestSDKDialMissingProxyReturnsProxyNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	db := openSDKTestStore(t)
	echoTarget := startSDKEchoTarget(t)
	seedSDKControlStore(t, ctx, db, echoTarget)
	serverTLS, caFile := sdkTestCertificate(t)
	listener := startSDKTestTLSListener(t, db, serverTLS)

	client := New(Config{
		ServerTLSAddress: listener.Addr().String(),
		ServerName:       "localhost",
		ServerCAFile:     caFile,
		ClientID:         "client-consumer",
		Credential:       "secret",
		AllowedProtocols: []string{"tcp_tls"},
	})
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	conn, err := client.Dial(ctx, "missing-proxy")
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected missing proxy dial to fail")
	}
	if !errors.Is(err, ErrProxyNotFound) {
		t.Fatalf("expected ErrProxyNotFound, got %v", err)
	}
}

func TestConnectRejectsInvalidCredentialWithoutPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	db := openSDKTestStore(t)
	echoTarget := startSDKEchoTarget(t)
	seedSDKControlStore(t, ctx, db, echoTarget)
	serverTLS, caFile := sdkTestCertificate(t)
	listener := startSDKTestTLSListener(t, db, serverTLS)

	client := New(Config{
		ServerTLSAddress: listener.Addr().String(),
		ServerName:       "localhost",
		ServerCAFile:     caFile,
		ClientID:         "client-consumer",
		Credential:       "wrong-secret",
		AllowedProtocols: []string{"tcp_tls"},
	})

	var err error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Connect must return an auth error instead of panicking: %v", recovered)
			}
		}()
		err = client.Connect(ctx)
	}()
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "wrong-secret") {
		t.Fatalf("authentication error leaked credential: %v", err)
	}
}

func TestSDKTCPTLSConnectProxiesAndDialEcho(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := openSDKTestStore(t)
	echoTarget := startSDKEchoTarget(t)
	seedSDKControlStore(t, ctx, db, echoTarget)
	serverTLS, caFile := sdkTestCertificate(t)
	listener := startSDKTestTLSListener(t, db, serverTLS)

	providerConn, providerResp, err := control.DialTLSAndAuthenticate(ctx, listener.Addr().String(), sdkClientTLSConfig(t, caFile), control.AuthRequest{
		ClientID:   "client-provider",
		Credential: "secret",
		Timestamp:  time.Now().UTC(),
		Protocols:  []domain.Protocol{domain.ProtocolTCPTLS},
	})
	if err != nil {
		t.Fatalf("provider dial: %v", err)
	}
	t.Cleanup(func() { _ = providerConn.Close() })
	if !providerResp.Accepted {
		t.Fatalf("provider auth rejected: %+v", providerResp)
	}
	if _, err := providerConn.ReadProxySnapshot(); err != nil {
		t.Fatalf("provider read snapshot: %v", err)
	}
	providerDone := make(chan error, 1)
	go func() { providerDone <- providerConn.ServeProxyStreams(ctx) }()

	client := New(Config{
		ServerTLSAddress: listener.Addr().String(),
		ServerName:       "localhost",
		ServerCAFile:     caFile,
		ClientID:         "client-consumer",
		Credential:       "secret",
		AllowedProtocols: []string{"tcp_tls"},
	})
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	proxies, err := client.Proxies(ctx)
	if err != nil {
		t.Fatalf("sdk proxies: %v", err)
	}
	if len(proxies) != 1 || proxies[0].ID != "proxy-1" {
		t.Fatalf("unexpected proxies: %+v", proxies)
	}

	conn, err := client.Dial(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("sdk dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write proxy conn: %v", err)
	}
	payload := make([]byte, 4)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatalf("read proxy conn: %v", err)
	}
	if string(payload) != "ping" {
		t.Fatalf("expected echo payload %q, got %q", "ping", string(payload))
	}

	cancel()
	if err := <-providerDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("provider serve proxy streams: %v", err)
	}
}

func TestLocalDirectTCPForwardsWithoutWaitingForEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, providerDone := startConnectedSDKClient(t, ctx)
	defer client.Close()

	local, remote := net.Pipe()
	defer remote.Close()
	done := make(chan struct{})
	go func() {
		client.handleLocalConn(ctx, local, "proxy-1")
		close(done)
	}()

	writeDone := make(chan error, 1)
	go func() {
		_, err := remote.Write([]byte("ping"))
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("write local payload: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("local direct write blocked")
	}

	payload := make([]byte, 4)
	if err := remote.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, err := io.ReadFull(remote, payload); err != nil {
		t.Fatalf("read local echo: %v", err)
	}
	if string(payload) != "ping" {
		t.Fatalf("expected echo payload %q, got %q", "ping", string(payload))
	}

	_ = remote.Close()
	<-done
	cancel()
	if err := <-providerDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("provider serve proxy streams: %v", err)
	}
}

func openSDKTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sdk-test.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func startConnectedSDKClient(t *testing.T, ctx context.Context) (*Client, <-chan error) {
	t.Helper()
	db := openSDKTestStore(t)
	echoTarget := startSDKEchoTarget(t)
	seedSDKControlStore(t, ctx, db, echoTarget)
	serverTLS, caFile := sdkTestCertificate(t)
	listener := startSDKTestTLSListener(t, db, serverTLS)

	providerConn, providerResp, err := control.DialTLSAndAuthenticate(ctx, listener.Addr().String(), sdkClientTLSConfig(t, caFile), control.AuthRequest{
		ClientID:   "client-provider",
		Credential: "secret",
		Timestamp:  time.Now().UTC(),
		Protocols:  []domain.Protocol{domain.ProtocolTCPTLS},
	})
	if err != nil {
		t.Fatalf("provider dial: %v", err)
	}
	t.Cleanup(func() { _ = providerConn.Close() })
	if !providerResp.Accepted {
		t.Fatalf("provider auth rejected: %+v", providerResp)
	}
	if _, err := providerConn.ReadProxySnapshot(); err != nil {
		t.Fatalf("provider read snapshot: %v", err)
	}
	providerDone := make(chan error, 1)
	go func() { providerDone <- providerConn.ServeProxyStreams(ctx) }()

	client := New(Config{
		ServerTLSAddress: listener.Addr().String(),
		ServerName:       "localhost",
		ServerCAFile:     caFile,
		ClientID:         "client-consumer",
		Credential:       "secret",
		AllowedProtocols: []string{"tcp_tls"},
	})
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("sdk connect: %v", err)
	}
	return client, providerDone
}

func seedSDKControlStore(t *testing.T, ctx context.Context, db *sqlite.Store, target net.Listener) {
	t.Helper()
	targetHost, targetPortText, err := net.SplitHostPort(target.Addr().String())
	if err != nil {
		t.Fatalf("split target addr: %v", err)
	}
	targetPort, err := strconv.Atoi(targetPortText)
	if err != nil {
		t.Fatalf("parse target port: %v", err)
	}

	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	provider := domain.Client{ID: "client-provider", UserID: user.ID, Name: "provider", Kind: domain.ClientKindProvider, Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret"), Version: 7}
	if err := db.Clients().Create(ctx, provider); err != nil {
		t.Fatalf("create provider client: %v", err)
	}
	consumer := domain.Client{ID: "client-consumer", UserID: user.ID, Name: "consumer", Kind: domain.ClientKindConsumer, Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret"), Version: 7}
	if err := db.Clients().Create(ctx, consumer); err != nil {
		t.Fatalf("create consumer client: %v", err)
	}
	proxy := domain.Proxy{ID: "proxy-1", UserID: user.ID, ClientID: provider.ID, Name: "echo", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: targetHost, TargetPort: targetPort}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
}

func startSDKTestTLSListener(t *testing.T, db *sqlite.Store, tlsConfig *tls.Config) *control.TLSListener {
	t.Helper()
	now := time.Now().UTC()
	listener, err := control.ListenTLSAddr("127.0.0.1:0", control.Server{
		Authenticator: control.Authenticator{Store: db, Now: func() time.Time { return now }},
		Sessions:      session.NewManager(),
		TLSConfig:     tlsConfig,
		NewSessionID:  sdkSessionIDGenerator(),
	})
	if err != nil {
		t.Fatalf("listen control tls: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
	})
	go func() { _ = listener.Serve(ctx) }()
	return listener
}

func sdkSessionIDGenerator() func() (string, error) {
	next := 0
	return func() (string, error) {
		next++
		return "sdk-test-session-" + strconv.Itoa(next), nil
	}
}

func startSDKEchoTarget(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo target: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(conn)
		}
	}()
	return listener
}

func sdkTestCertificate(t *testing.T) (*tls.Config, string) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "go-ginx-sdk-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse server key pair: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{control.ControlALPN}, MinVersion: tls.VersionTLS13}, caFile
}

func sdkClientTLSConfig(t *testing.T, caFile string) *tls.Config {
	t.Helper()
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatalf("read CA file: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append CA certificate")
	}
	return &tls.Config{RootCAs: pool, ServerName: "localhost", NextProtos: []string{control.ControlALPN}, MinVersion: tls.VersionTLS13}
}
