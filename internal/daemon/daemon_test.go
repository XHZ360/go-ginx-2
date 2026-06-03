package daemon

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	nethttp "net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestStartServerWiresRuntimeListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, _ := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "server.db")
	tcpPort := reservePort(t)
	udpPort := reserveUDPPort(t)
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "tcp-1", UserID: "user-1", ClientID: "client-1", Name: "ssh", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: tcpPort, TargetHost: "127.0.0.1", TargetPort: 22}, {ID: "udp-1", UserID: "user-1", ClientID: "client-1", Name: "dns", Type: domain.ProxyUDP, Status: domain.ProxyEnabled, EntryPort: udpPort, TargetHost: "127.0.0.1", TargetPort: 53}, {ID: "https-1", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443}})

	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", HTTPSEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	if runtime.ControlListener == nil || runtime.ControlListener.Addr() == nil {
		t.Fatal("expected control listener")
	}
	if runtime.ControlTLSListener == nil || runtime.ControlTLSListener.Addr() == nil {
		t.Fatal("expected control tcp+tls listener")
	}
	if runtime.EnrollmentServer == nil || runtime.EnrollmentServer.Addr() == nil {
		t.Fatal("expected client enrollment server")
	}
	if len(runtime.TCPListeners) != 1 || runtime.TCPListeners[0].Addr() == nil {
		t.Fatalf("expected one TCP listener, got %d", len(runtime.TCPListeners))
	}
	if len(runtime.UDPListeners) != 1 || runtime.UDPListeners[0].Addr() == nil {
		t.Fatalf("expected one UDP listener, got %d", len(runtime.UDPListeners))
	}
	if runtime.HTTPServer == nil || runtime.HTTPServer.Addr() == nil {
		t.Fatal("expected HTTP server")
	}
	if runtime.HTTPSListener == nil || runtime.HTTPSListener.Addr() == nil {
		t.Fatal("expected HTTPS listener")
	}
	runtime.Stats.RecordTCPStart("tcp-1")
	runtime.Stats.RecordTCPEnd("tcp-1", 12, 34, false)
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db.Close()
	loaded, err := db.Stats().List(ctx)
	if err != nil {
		t.Fatalf("list stats: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ProxyID != "tcp-1" || loaded[0].TCPConnections != 1 || loaded[0].TCPDownloadBytes != 34 {
		t.Fatalf("expected persisted tcp stats, got %+v", loaded)
	}
}

func TestStartServerServesConfiguredAdminFrontend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, _ := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "admin-frontend.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	frontendDir := writeDaemonAdminFrontendFixture(t)
	adminCredentialsFile := writeDaemonAdminCredentials(t)
	adminPort := reservePort(t)
	enrollmentPort := reservePort(t)
	controlQUICPort := reserveUDPPort(t)
	httpEntryPort := reservePort(t)

	runtime, err := StartServer(ctx, config.Server{AdminListen: net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort)), AdminCredentialsFile: adminCredentialsFile, AdminFrontendDir: frontendDir, ClientEnrollmentListen: net.JoinHostPort("127.0.0.1", strconv.Itoa(enrollmentPort)), ControlQUICListen: net.JoinHostPort("127.0.0.1", strconv.Itoa(controlQUICPort)), ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)), SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server with admin frontend: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if runtime.AdminServer == nil {
		t.Fatal("expected admin server to start")
	}

	response, err := nethttp.Get("http://" + runtime.AdminServer.Addr().String() + "/dashboard")
	if err != nil {
		t.Fatalf("get admin frontend route: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read admin frontend route body: %v", err)
	}
	if response.StatusCode != nethttp.StatusOK || !strings.Contains(string(body), "daemon admin frontend") {
		t.Fatalf("unexpected admin frontend response status=%d body=%q", response.StatusCode, string(body))
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := &nethttp.Client{Jar: jar}
	loginResponse, err := postDaemonJSON(client, "http://"+runtime.AdminServer.Addr().String()+"/api/admin/login", map[string]string{"username": "admin", "password": "secret"})
	if err != nil {
		t.Fatalf("login admin api: %v", err)
	}
	defer loginResponse.Body.Close()
	if loginResponse.StatusCode != nethttp.StatusOK {
		body, _ := io.ReadAll(loginResponse.Body)
		t.Fatalf("unexpected login status %d body=%s", loginResponse.StatusCode, string(body))
	}

	graphqlResponse, err := postDaemonJSON(client, "http://"+runtime.AdminServer.Addr().String()+"/api/admin/graphql", map[string]any{"query": `query { dashboard { enabledProxyCount } }`})
	if err != nil {
		t.Fatalf("query admin graphql: %v", err)
	}
	defer graphqlResponse.Body.Close()
	if graphqlResponse.StatusCode != nethttp.StatusOK {
		body, _ := io.ReadAll(graphqlResponse.Body)
		t.Fatalf("unexpected graphql status %d body=%s", graphqlResponse.StatusCode, string(body))
	}
}

func TestRunClientAuthenticatesAndSendsHeartbeat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "client.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
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

func TestRunClientReconnectsAfterInitialDialFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "client-reconnect.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	quicPort := reserveUDPPort(t)
	serverAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(quicPort))

	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(ctx, config.Client{ServerAddress: serverAddress, ServerName: "localhost", ServerCAFile: caFile, ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.Reconnect{InitialDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond}})
	}()
	select {
	case err := <-clientDone:
		t.Fatalf("client exited before server startup: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: serverAddress, ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-clientDone:
			t.Fatalf("client exited before reconnect completed: %v", err)
		default:
		}
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
	t.Fatal("client did not reconnect after server startup")
}

func TestRunClientAuthenticationRejectedStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "client-auth-reject.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(ctx, config.Client{ServerAddress: runtime.ControlListener.Addr().String(), ServerName: "localhost", ServerCAFile: caFile, ClientID: "client-1", Credential: "wrong", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.Reconnect{InitialDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond}})
	}()
	select {
	case err := <-clientDone:
		if err == nil {
			t.Fatal("expected authentication rejection error")
		}
	case <-time.After(time.Second):
		t.Fatal("client did not stop after authentication rejection")
	}
	if _, ok := runtime.Sessions.Latest("client-1"); ok {
		t.Fatal("rejected client must not register a session")
	}
}

func TestRunClientReconnectsAfterServerRestart(t *testing.T) {
	clientCtx, cancelClient := context.WithCancel(context.Background())
	t.Cleanup(cancelClient)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "client-restart.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	tlsPort := reservePort(t)
	serverTLSAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(tlsPort))

	serverCtx1, cancelServer1 := context.WithCancel(context.Background())
	runtime1, err := StartServer(serverCtx1, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSListen: serverTLSAddress, ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start first server: %v", err)
	}
	defer func() {
		cancelServer1()
		_ = runtime1.Close()
	}()

	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(clientCtx, config.Client{ServerAddress: "127.0.0.1:1", ServerTLSAddress: serverTLSAddress, ServerName: "localhost", ServerCAFile: caFile, ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolTCPTLS}, Reconnect: config.Reconnect{InitialDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond}})
	}()
	waitForActiveProxySession(t, runtime1)

	cancelServer1()
	if err := runtime1.Close(); err != nil {
		t.Fatalf("close first server: %v", err)
	}

	runtime2, cancelServer2 := startServerWithRetry(t, serverTLSAddress, serverCert, serverKey, dbPath)
	defer cancelServer2()
	t.Cleanup(func() { _ = runtime2.Close() })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-clientDone:
			t.Fatalf("client exited before reconnect after restart: %v", err)
		default:
		}
		latest, ok := runtime2.Sessions.Latest("client-1")
		if ok && latest.Stats.ActiveProxies == 1 {
			cancelClient()
			if err := <-clientDone; err != nil {
				t.Fatalf("run client: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelClient()
	t.Fatal("client did not reconnect after server restart")
}

func TestRunClientFallsBackToTCPTLSControl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "client-tls.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(ctx, config.Client{ServerAddress: "127.0.0.1:1", ServerTLSAddress: runtime.ControlTLSListener.Addr().String(), ServerName: "localhost", ServerCAFile: caFile, ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS}, Reconnect: config.Reconnect{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		latest, ok := runtime.Sessions.Latest("client-1")
		if ok && latest.Protocol == domain.ProtocolTCPTLS && latest.Stats.ActiveProxies == 1 {
			cancel()
			if err := <-clientDone; err != nil {
				t.Fatalf("run client: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatalf("client heartbeat did not update tcp+tls session")
}

func TestRunClientFallsBackToTCPTLSProxyTraffic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	target := startEchoTarget(t)
	targetPort := portNumber(t, target.Addr())
	entryPort := reservePort(t)
	dbPath := filepath.Join(t.TempDir(), "client-tls-proxy.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "tcp-1", UserID: "user-1", ClientID: "client-1", Name: "echo", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: entryPort, TargetHost: "127.0.0.1", TargetPort: targetPort}})
	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(ctx, config.Client{ServerAddress: "127.0.0.1:1", ServerTLSAddress: runtime.ControlTLSListener.Addr().String(), ServerName: "localhost", ServerCAFile: caFile, ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS}, Reconnect: config.Reconnect{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}})
	}()
	waitForTCPTLSSession(t, runtime)

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(entryPort)), time.Second)
	if err != nil {
		t.Fatalf("dial tcp entry: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write tcp entry: %v", err)
	}
	payload := make([]byte, 4)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatalf("read tcp entry: %v", err)
	}
	if string(payload) != "ping" {
		t.Fatalf("unexpected tcp fallback payload %q", string(payload))
	}
	cancel()
	if err := <-clientDone; err != nil {
		t.Fatalf("run client: %v", err)
	}
}

func TestRunClientFallsBackToTCPTLSHTTPProxyTraffic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, caFile := writeTestTLSFiles(t)
	target := startHTTPTextTarget(t, "hello over tcp tls")
	targetPort := portNumber(t, target.Addr())
	dbPath := filepath.Join(t.TempDir(), "client-tls-http.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: targetPort}})
	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(ctx, config.Client{ServerAddress: "127.0.0.1:1", ServerTLSAddress: runtime.ControlTLSListener.Addr().String(), ServerName: "localhost", ServerCAFile: caFile, ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS}, Reconnect: config.Reconnect{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}})
	}()
	waitForTCPTLSSession(t, runtime)

	request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://"+runtime.HTTPServer.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Host = "app.example.com"
	response, err := nethttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != nethttp.StatusOK || string(payload) != "hello over tcp tls" {
		t.Fatalf("unexpected response status=%d body=%q", response.StatusCode, string(payload))
	}
	cancel()
	if err := <-clientDone; err != nil {
		t.Fatalf("run client: %v", err)
	}
}

func TestStartServerRenewsManagedCertificates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverCert, serverKey, _ := writeTestTLSFiles(t)
	dbPath := filepath.Join(t.TempDir(), "acme-renew.db")
	seedDatabase(t, dbPath, []domain.Proxy{{ID: "https-1", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}})
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	firstCert, firstKey := daemonTestCertificatePEM(t, "app.example.com", time.Now().Add(30*time.Minute))
	certificateDir := t.TempDir()
	stored, err := httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}.Store("app.example.com", firstCert, firstKey)
	if err != nil {
		t.Fatalf("store initial certificate: %v", err)
	}
	notAfter := time.Now().UTC().Add(10 * time.Minute)
	if err := db.Certificates().Create(context.Background(), domain.ManagedCertificate{ID: "cert-1", ProxyID: "https-1", Host: "app.example.com", Status: domain.CertificateValid, Provider: "cloudflare", CertFile: stored.CertFile, KeyFile: stored.KeyFile, NotAfter: &notAfter}); err != nil {
		t.Fatalf("create certificate metadata: %v", err)
	}
	_ = db.Close()
	oldIssuer := newDaemonACMEIssuer
	oldProvider := newDaemonDNSProvider
	newDaemonACMEIssuer = func() certmanager.Issuer { return daemonFakeIssuer{} }
	newDaemonDNSProvider = func(string) (certmanager.DNSChallengeProvider, error) { return daemonFakeDNSProvider{}, nil }
	t.Cleanup(func() {
		newDaemonACMEIssuer = oldIssuer
		newDaemonDNSProvider = oldProvider
	})

	runtime, err := StartServer(ctx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: certificateDir, ACMEEnabled: true, ACMEDirectoryURL: "https://acme.example.test/directory", ACMEAccountEmail: "ops@example.com", ACMETermsAccepted: true, ACMERenewalWindow: time.Hour, ACMECloudflareTokenEnv: "CF_DNS_API_TOKEN", HeartbeatTimeout: time.Second, LogRetentionDays: 1})
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		certificate, err := runtime.Store.Certificates().ByProxyID(ctx, "https-1")
		if err == nil && certificate.PreviousCertFile != "" && certificate.LastRenewedAt != nil {
			cancel()
			_ = runtime.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = runtime.Close()
	t.Fatal("managed certificate was not renewed")
}

type daemonFakeDNSProvider struct{}

func (daemonFakeDNSProvider) Present(context.Context, string, string) error { return nil }
func (daemonFakeDNSProvider) CleanUp(context.Context, string, string) error { return nil }

type daemonFakeIssuer struct{}

func (daemonFakeIssuer) Issue(context.Context, certmanager.IssueRequest) (certmanager.IssuedCertificate, error) {
	notAfter := time.Now().Add(2 * time.Hour)
	certPEM, keyPEM := daemonTestCertificatePEMBytes("app.example.com", notAfter)
	return certmanager.IssuedCertificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
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

func waitForTCPTLSSession(t *testing.T, runtime *ServerRuntime) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		latest, ok := runtime.Sessions.Latest("client-1")
		if ok && latest.Protocol == domain.ProtocolTCPTLS && latest.StreamOpener != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("tcp+tls session did not become ready")
}

func waitForActiveProxySession(t *testing.T, runtime *ServerRuntime) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		latest, ok := runtime.Sessions.Latest("client-1")
		if ok && latest.Stats.ActiveProxies == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("client session did not become active")
}

func startServerWithRetry(t *testing.T, serverTLSAddress string, serverCert string, serverKey string, dbPath string) (*ServerRuntime, context.CancelFunc) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		serverCtx, cancelServer := context.WithCancel(context.Background())
		runtime, err := StartServer(serverCtx, config.Server{AdminListen: "127.0.0.1:0", ClientEnrollmentListen: "127.0.0.1:0", ControlQUICListen: "127.0.0.1:0", ControlTLSListen: serverTLSAddress, ControlTLSCertFile: serverCert, ControlTLSKeyFile: serverKey, TCPEntryHost: "127.0.0.1", HTTPEntryListen: "127.0.0.1:0", SQLitePath: dbPath, DataDir: t.TempDir(), CertificateDir: t.TempDir(), HeartbeatTimeout: time.Second, LogRetentionDays: 1})
		if err == nil {
			return runtime, cancelServer
		}
		cancelServer()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("second server did not restart before deadline")
	return nil, nil
}

func startEchoTarget(t *testing.T) net.Listener {
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

func startHTTPTextTarget(t *testing.T, body string) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen http target: %v", err)
	}
	server := &nethttp.Server{Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_, _ = w.Write([]byte(body))
	})}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(listener) }()
	return listener
}

func portNumber(t *testing.T, addr net.Addr) int {
	t.Helper()
	_, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return port
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

func reserveUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, portText, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func daemonTestCertificatePEM(t *testing.T, host string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	certPEM, keyPEM := daemonTestCertificatePEMBytes(host, notAfter)
	return certPEM, keyPEM
}

func daemonTestCertificatePEMBytes(host string, notAfter time.Time) ([]byte, []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
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

func writeDaemonAdminFrontendFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><html><body>daemon admin frontend</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeDaemonAdminCredentials(t *testing.T) string {
	t.Helper()
	hash, err := domain.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "admins.json")
	content := []byte(`{"administrators":[{"username":"admin","password_hash":"` + hash + `"}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func postDaemonJSON(client *nethttp.Client, url string, payload any) (*nethttp.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := nethttp.NewRequest(nethttp.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return client.Do(request)
}
