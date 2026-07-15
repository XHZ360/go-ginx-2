package e2e_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestExternalProcessesHTTPPathRouting(t *testing.T) {
	root := repositoryRoot(t)
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverBin := buildCommand(t, root, binDir, "goginx-server", "./cmd/goginx-server")
	clientBin := buildCommand(t, root, binDir, "goginx-client", "./cmd/goginx-client")

	smokeCtx, cancelSmoke := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancelSmoke)

	defaultOrigin := startPathOrigin(t, "default")
	apiOrigin := startPathOrigin(t, "api")
	defaultURL, err := url.Parse(defaultOrigin.URL)
	if err != nil {
		t.Fatal(err)
	}
	apiURL, err := url.Parse(apiOrigin.URL)
	if err != nil {
		t.Fatal(err)
	}
	defaultHost, defaultPort := splitAddress(t, defaultURL.Host)
	apiHost, apiPort := splitAddress(t, apiURL.Host)

	controlPort := reservePort(t)
	httpEntryPort := reservePort(t)
	certFile, keyFile, caFile := writeTLSFiles(t, workDir)
	dbPath := filepath.Join(workDir, "go-ginx.db")
	seedSQLiteWithRoutes(t, dbPath,
		domain.Proxy{
			ID:         "http-route-1",
			UserID:     "user-1",
			ClientID:   "client-1",
			Name:       "web",
			Type:       domain.ProxyHTTP,
			Status:     domain.ProxyEnabled,
			EntryHost:  "app.example.com",
			TargetHost: defaultHost,
			TargetPort: defaultPort,
		},
		domain.ProxyRoute{
			ID:                 "route-api",
			ProxyID:            "http-route-1",
			ClientID:           "client-1",
			PathPrefix:         "/api",
			StripPrefix:        true,
			UpstreamPathPrefix: "/",
			TargetHost:         apiHost,
			TargetPort:         apiPort,
			Status:             domain.ProxyRouteEnabled,
		},
	)
	serverConfig := writeJSON(t, filepath.Join(workDir, "server.json"), map[string]any{
		"admin_listen":             "127.0.0.1:0",
		"client_enrollment_listen": "127.0.0.1:0",
		"control_quic_listen":      net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"control_tls_cert_file":    certFile,
		"control_tls_key_file":     keyFile,
		"tcp_entry_host":           "127.0.0.1",
		"http_entry_listen":        net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)),
		"https_entry_listen":       "127.0.0.1:0",
		"sqlite_path":              dbPath,
		"data_dir":                 filepath.Join(workDir, "data"),
		"certificate_dir":          filepath.Join(workDir, "certs"),
		"heartbeat_timeout":        int64(time.Second),
		"log_retention_days":       1,
	})
	clientConfig := writeJSON(t, filepath.Join(workDir, "client.json"), map[string]any{
		"server_address":    net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"server_name":       "localhost",
		"server_ca_file":    caFile,
		"client_id":         "client-1",
		"credential":        "secret",
		"allowed_protocols": []string{string(domain.ProtocolQUIC)},
		"reconnect": map[string]any{
			"initial_delay": int64(10 * time.Millisecond),
			"max_delay":     int64(10 * time.Millisecond),
		},
	})

	server := startProcess(t, root, serverBin, "-config", serverConfig)
	waitForTCPAccept(t, smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)))
	client := startProcess(t, root, clientBin, "-config", clientConfig)

	defaultResponse, err := waitForHTTP(smokeCtx, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort))+"/hello", "app.example.com", "request-body")
	if err != nil {
		t.Fatalf("default path failed: %v\nserver:\n%s\nclient:\n%s", err, server.Output(), client.Output())
	}
	defer defaultResponse.Body.Close()
	defaultBody, _ := io.ReadAll(defaultResponse.Body)
	if string(defaultBody) != "default:/hello" {
		t.Fatalf("unexpected default body %q", string(defaultBody))
	}

	apiResponse, err := waitForHTTPStatus(smokeCtx, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort))+"/api/users", "app.example.com", "request-body", nethttp.StatusCreated)
	if err != nil {
		t.Fatalf("api path failed: %v\nserver:\n%s\nclient:\n%s", err, server.Output(), client.Output())
	}
	defer apiResponse.Body.Close()
	apiBody, _ := io.ReadAll(apiResponse.Body)
	if string(apiBody) != "api:/users" {
		t.Fatalf("unexpected api body %q", string(apiBody))
	}
}

func TestExternalProcessesHTTPSAccessActivation(t *testing.T) {
	root := repositoryRoot(t)
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverBin := buildCommand(t, root, binDir, "goginx-server", "./cmd/goginx-server")
	clientBin := buildCommand(t, root, binDir, "goginx-client", "./cmd/goginx-client")

	smokeCtx, cancelSmoke := context.WithTimeout(context.Background(), 40*time.Second)
	t.Cleanup(cancelSmoke)

	var (
		mu             sync.Mutex
		seenAuthCookie bool
		originHits     int
	)
	origin := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		mu.Lock()
		originHits++
		if strings.Contains(r.Header.Get("Cookie"), "__Host-goginx-access-") {
			seenAuthCookie = true
		}
		mu.Unlock()
		w.Header().Set("X-Origin", "ok")
		w.WriteHeader(nethttp.StatusCreated)
		_, _ = w.Write([]byte("origin-response"))
	}))
	t.Cleanup(origin.Close)
	originURL, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	originHost, originPort := splitAddress(t, originURL.Host)

	controlPort := reservePort(t)
	httpEntryPort := reservePort(t)
	httpsEntryPort := reservePort(t)
	certFile, keyFile, caFile := writeTLSFiles(t, workDir)
	entryCertPEM, entryKeyPEM, entryCAPEM := generateCertificateFor(t, "secure.example.com")
	entryCertDir := filepath.Join(workDir, "certs")
	if err := os.MkdirAll(entryCertDir, 0o700); err != nil {
		t.Fatal(err)
	}
	entryCertFile := filepath.Join(entryCertDir, "secure.crt")
	entryKeyFile := filepath.Join(entryCertDir, "secure.key")
	writeFile(t, entryCertFile, entryCertPEM)
	writeFile(t, entryKeyFile, entryKeyPEM)
	dbPath := filepath.Join(workDir, "go-ginx.db")
	seedSQLite(t, dbPath, domain.Proxy{
		ID:         "https-auth-1",
		UserID:     "user-1",
		ClientID:   "client-1",
		Name:       "secure",
		Type:       domain.ProxyHTTPS,
		Status:     domain.ProxyEnabled,
		EntryHost:  "secure.example.com",
		TargetHost: originHost,
		TargetPort: originPort,
		CertFile:   entryCertFile,
		KeyFile:    entryKeyFile,
	})

	serverConfig := writeJSON(t, filepath.Join(workDir, "server.json"), map[string]any{
		"admin_listen":             "127.0.0.1:0",
		"client_enrollment_listen": "127.0.0.1:0",
		"control_quic_listen":      net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"control_tls_cert_file":    certFile,
		"control_tls_key_file":     keyFile,
		"tcp_entry_host":           "127.0.0.1",
		"http_entry_listen":        net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)),
		"https_entry_listen":       net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsEntryPort)),
		"sqlite_path":              dbPath,
		"data_dir":                 filepath.Join(workDir, "data"),
		"certificate_dir":          filepath.Join(workDir, "certs"),
		"heartbeat_timeout":        int64(time.Second),
		"log_retention_days":       1,
	})
	clientConfig := writeJSON(t, filepath.Join(workDir, "client.json"), map[string]any{
		"server_address":    net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"server_name":       "localhost",
		"server_ca_file":    caFile,
		"client_id":         "client-1",
		"credential":        "secret",
		"allowed_protocols": []string{string(domain.ProtocolQUIC)},
		"reconnect": map[string]any{
			"initial_delay": int64(10 * time.Millisecond),
			"max_delay":     int64(10 * time.Millisecond),
		},
	})

	server := startProcess(t, root, serverBin, "-config", serverConfig)
	waitForTCPAccept(t, smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsEntryPort)))
	client := startProcess(t, root, clientBin, "-config", clientConfig)

	// Wait until proxy is ready without auth.
	readyURL := "https://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsEntryPort)) + "/hello"
	if _, err := waitForHTTPSHTTP(smokeCtx, readyURL, "secure.example.com", entryCAPEM, "request-body"); err != nil {
		t.Fatalf("https proxy not ready: %v\nserver:\n%s\nclient:\n%s", err, server.Output(), client.Output())
	}

	tokenValue := "activation-token-e2e-001"
	enableAccessAuth(t, dbPath, "https-auth-1", tokenValue)

	tlsClient := newHTTPSClient(t, "secure.example.com", entryCAPEM, true)
	unauthorized, err := tlsClient.Get(readyURL)
	if err != nil {
		t.Fatalf("unauthorized request failed: %v", err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401 before activation, got %d", unauthorized.StatusCode)
	}

	activateURL := "https://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsEntryPort)) + "/.well-known/goginx/activate/" + tokenValue
	getResponse, err := tlsClient.Get(activateURL)
	if err != nil {
		t.Fatalf("activation GET failed: %v", err)
	}
	getBody, _ := io.ReadAll(getResponse.Body)
	getResponse.Body.Close()
	if getResponse.StatusCode != nethttp.StatusOK || !strings.Contains(string(getBody), "Activate access") {
		t.Fatalf("unexpected activation GET status=%d body=%q", getResponse.StatusCode, string(getBody))
	}

	postResponse, err := tlsClient.Post(activateURL, "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("activation POST failed: %v", err)
	}
	postResponse.Body.Close()
	if postResponse.StatusCode != nethttp.StatusSeeOther {
		t.Fatalf("expected 303 after activation, got %d", postResponse.StatusCode)
	}

	// Token should be single-use.
	replay, err := tlsClient.Post(activateURL, "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("activation replay failed: %v", err)
	}
	replay.Body.Close()
	if replay.StatusCode != nethttp.StatusNotFound {
		t.Fatalf("expected replay 404, got %d", replay.StatusCode)
	}

	// Authenticated business request should succeed and not leak cookie upstream.
	beforeHits := originHits
	authResponse, err := tlsClient.Post(readyURL, "text/plain", strings.NewReader("request-body"))
	if err != nil {
		t.Fatalf("authenticated request failed: %v", err)
	}
	authBody, _ := io.ReadAll(authResponse.Body)
	authResponse.Body.Close()
	if authResponse.StatusCode != nethttp.StatusCreated || string(authBody) != "origin-response" {
		t.Fatalf("unexpected authenticated response status=%d body=%q", authResponse.StatusCode, string(authBody))
	}
	mu.Lock()
	if originHits <= beforeHits {
		mu.Unlock()
		t.Fatal("origin was not hit after activation")
	}
	if seenAuthCookie {
		mu.Unlock()
		t.Fatal("origin received go-ginx access cookie")
	}
	mu.Unlock()

	// Revoke all access and ensure cookie no longer works.
	revokeAllAccess(t, dbPath, "https-auth-1")
	revoked, err := tlsClient.Post(readyURL, "text/plain", strings.NewReader("request-body"))
	if err != nil {
		t.Fatalf("revoked request failed: %v", err)
	}
	revoked.Body.Close()
	if revoked.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401 after revoke, got %d", revoked.StatusCode)
	}
}

func startPathOrigin(t *testing.T, label string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusCreated)
		_, _ = fmt.Fprintf(w, "%s:%s", label, r.URL.Path)
	}))
	t.Cleanup(server.Close)
	return server
}

func seedSQLiteWithRoutes(t *testing.T, dbPath string, proxy domain.Proxy, routes ...domain.ProxyRoute) {
	t.Helper()
	db, err := sqlite.Open(dbPath)
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
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	parent, err := db.Proxies().ByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("reload parent proxy: %v", err)
	}
	for _, route := range routes {
		status := domain.ProxyEnabled
		if route.Status == domain.ProxyRouteDisabled {
			status = domain.ProxyDisabled
		}
		webProxy := domain.Proxy{
			ID:                 route.ID,
			UserID:             parent.UserID,
			ClientID:           route.ClientID,
			Name:               parent.Name + " " + route.PathPrefix,
			Type:               domain.ProxyWeb,
			Status:             status,
			DomainID:           parent.DomainID,
			PathPrefix:         route.PathPrefix,
			StripPrefix:        route.StripPrefix,
			UpstreamPathPrefix: route.UpstreamPathPrefix,
			TargetHost:         route.TargetHost,
			TargetPort:         route.TargetPort,
		}
		if err := db.Proxies().Create(ctx, webProxy); err != nil {
			t.Fatalf("create path proxy %s: %v", route.ID, err)
		}
	}
}

func enableAccessAuth(t *testing.T, dbPath string, proxyID string, tokenValue string) {
	t.Helper()
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	proxy, err := db.Proxies().ByID(ctx, proxyID)
	if err != nil {
		t.Fatal(err)
	}
	token := domain.ProxyActivationToken{
		ID:         "activation-e2e-1",
		ProxyID:    proxyID,
		AuthVersion: proxy.AccessAuthVersion + 1,
		TokenHash:  hashValue(tokenValue),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
		CreatedBy:  "test",
	}
	if err := db.ProxyAccess().EnableAuthAndCreateActivation(ctx, proxyID, token.AuthVersion, token); err != nil {
		t.Fatalf("enable access auth: %v", err)
	}
}

func revokeAllAccess(t *testing.T, dbPath string, proxyID string) {
	t.Helper()
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	proxy, err := db.Proxies().ByID(ctx, proxyID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ProxyAccess().RevokeAllAccess(ctx, proxyID, proxy.AccessAuthVersion+1); err != nil {
		t.Fatalf("revoke access: %v", err)
	}
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func newHTTPSClient(t *testing.T, serverName string, caPEM []byte, withJar bool) *nethttp.Client {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append CA")
	}
	transport := &nethttp.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: serverName, MinVersion: tls.VersionTLS12}}
	client := &nethttp.Client{
		Timeout:   2 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *nethttp.Request, via []*nethttp.Request) error {
			return nethttp.ErrUseLastResponse
		},
	}
	if withJar {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		client.Jar = jar
	}
	return client
}

func waitForHTTPStatus(ctx context.Context, rawURL string, host string, body string, wantStatus int) (*nethttp.Response, error) {
	var lastErr error
	client := &nethttp.Client{Timeout: 500 * time.Millisecond}
	for ctx.Err() == nil {
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, rawURL, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Host = host
		request.Header.Set("X-Smoke", "yes")
		response, err := client.Do(request)
		if err == nil && response.StatusCode == wantStatus {
			return response, nil
		}
		if response != nil {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected status %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ctx.Err()
}
