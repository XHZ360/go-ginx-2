package httpproxy

import (
	"bufio"
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

	"github.com/quic-go/quic-go"
	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestHTTPEntryProxiesThroughQUICClientStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	type originCase struct {
		requestOrigin *string
		wantPresent   bool
		wantOrigin    string
	}
	var targetAuthority string
	testCases := map[string]originCase{
		"/hello":          {requestOrigin: new("https://app.example.com"), wantPresent: true},
		"/missing-origin": {wantPresent: false},
		"/null-origin":    {requestOrigin: new("null"), wantPresent: true, wantOrigin: "null"},
		"/empty-origin":   {requestOrigin: new(""), wantPresent: true, wantOrigin: ""},
		"/ftp-origin":     {requestOrigin: new("ftp://app.example.com"), wantPresent: true, wantOrigin: "ftp://app.example.com"},
		"/bad-origin":     {requestOrigin: new("://bad"), wantPresent: true, wantOrigin: "://bad"},
	}
	origin := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		testCase, ok := testCases[r.URL.Path]
		if !ok {
			t.Fatalf("unexpected origin request path=%s", r.URL.Path)
		}
		if string(body) != "request-body" || r.Header.Get("X-Test") != "yes" {
			t.Fatalf("unexpected origin request path=%s body=%q header=%s", r.URL.Path, string(body), r.Header.Get("X-Test"))
		}
		if r.Host != targetAuthority {
			t.Fatalf("expected target host %q, got %q", targetAuthority, r.Host)
		}
		wantOrigin := testCase.wantOrigin
		if r.URL.Path == "/hello" {
			wantOrigin = "http://" + targetAuthority
		}
		origins, present := r.Header["Origin"]
		if present != testCase.wantPresent {
			t.Fatalf("unexpected origin presence path=%s present=%t want=%t values=%v", r.URL.Path, present, testCase.wantPresent, origins)
		}
		if testCase.wantPresent && (len(origins) != 1 || origins[0] != wantOrigin) {
			t.Fatalf("unexpected origin path=%s got=%v want=%q", r.URL.Path, origins, wantOrigin)
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
	targetAuthority = originURL.Host
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

	for path, testCase := range testCases {
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, "http://"+entry.Addr().String()+path, strings.NewReader("request-body"))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "app.example.com"
		request.Header.Set("X-Test", "yes")
		if testCase.requestOrigin != nil {
			request.Header["Origin"] = []string{*testCase.requestOrigin}
		}
		responseFromProxy, err := nethttp.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("proxy request %s: %v", path, err)
		}
		defer responseFromProxy.Body.Close()
		responseBody, err := io.ReadAll(responseFromProxy.Body)
		if err != nil {
			t.Fatal(err)
		}
		if responseFromProxy.StatusCode != nethttp.StatusCreated || responseFromProxy.Header.Get("X-Origin") != "ok" || string(responseBody) != "origin-response" {
			t.Fatalf("unexpected proxy response path=%s status=%d header=%s body=%q", path, responseFromProxy.StatusCode, responseFromProxy.Header.Get("X-Origin"), string(responseBody))
		}
	}
	snapshot := memoryStats.Snapshot("proxy-1")
	if snapshot.HTTPRequests != int64(len(testCases)) || snapshot.HTTPStatusCodes[nethttp.StatusCreated] != int64(len(testCases)) || snapshot.HTTPUploadBytes != int64(len("request-body")*len(testCases)) || snapshot.HTTPDownloadBytes != int64(len("origin-response")*len(testCases)) || snapshot.HTTPErrors != 0 {
		t.Fatalf("unexpected HTTP stats: %+v", snapshot)
	}
}

func TestHTTPEntryWebSocketUpgradeAcrossControlProtocols(t *testing.T) {
	tests := map[string]domain.Protocol{
		"quic":    domain.ProtocolQUIC,
		"tcp_tls": domain.ProtocolTCPTLS,
	}
	for name, protocol := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			origin := startRawWebSocketOrigin(t, func(conn net.Conn, reader *bufio.Reader) {
				_, _ = conn.Write([]byte("server-first"))
				payload := make([]byte, 4)
				if _, err := io.ReadFull(reader, payload); err != nil {
					return
				}
				if string(payload) == "ping" {
					_, _ = conn.Write([]byte("pong"))
				}
			})
			originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			originPort, err := strconv.Atoi(originPortText)
			if err != nil {
				t.Fatal(err)
			}
			entry, memoryStats := startHTTPProxyRuntime(t, ctx, protocol, originHost, originPort)

			conn, reader, response := openRawWebSocket(t, entry.Addr().String(), "app.example.com", false)
			defer conn.Close()
			if response.StatusCode != nethttp.StatusSwitchingProtocols || response.Header.Get("Sec-WebSocket-Accept") != "target-accept" || response.Header.Get("Sec-WebSocket-Protocol") != "chat" || response.Header.Get("Sec-WebSocket-Extensions") != "permessage-deflate" {
				t.Fatalf("unexpected upgrade response status=%d headers=%v", response.StatusCode, response.Header)
			}
			serverFirst := make([]byte, len("server-first"))
			if _, err := io.ReadFull(reader, serverFirst); err != nil {
				t.Fatalf("read buffered target payload: %v", err)
			}
			if string(serverFirst) != "server-first" {
				t.Fatalf("unexpected buffered target payload %q", string(serverFirst))
			}
			if _, err := conn.Write([]byte("ping")); err != nil {
				t.Fatalf("write tunneled payload: %v", err)
			}
			payload := make([]byte, 4)
			if _, err := io.ReadFull(reader, payload); err != nil {
				t.Fatalf("read tunneled payload: %v", err)
			}
			if string(payload) != "pong" {
				t.Fatalf("unexpected tunneled payload %q", string(payload))
			}
			_ = conn.Close()
			waitForHTTPStats(t, memoryStats, func(snapshot stats.ProxyStats) bool {
				return snapshot.HTTPStatusCodes[nethttp.StatusSwitchingProtocols] == 1 && snapshot.HTTPErrors == 0 && snapshot.HTTPUploadBytes >= 4 && snapshot.HTTPDownloadBytes >= int64(len("server-first")+len("pong"))
			})
		})
	}
}

func TestHTTPEntryWebSocketBufferedClientPayload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	origin := startRawWebSocketOrigin(t, func(conn net.Conn, reader *bufio.Reader) {
		payload := make([]byte, len("client-first"))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return
		}
		if string(payload) == "client-first" {
			_, _ = conn.Write([]byte("target-seen"))
		}
	})
	originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	originPort, err := strconv.Atoi(originPortText)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := startHTTPProxyRuntime(t, ctx, domain.ProtocolQUIC, originHost, originPort)

	conn, reader, response := openRawWebSocket(t, entry.Addr().String(), "app.example.com", true)
	defer conn.Close()
	if response.StatusCode != nethttp.StatusSwitchingProtocols {
		t.Fatalf("unexpected status %d", response.StatusCode)
	}
	payload := make([]byte, len("target-seen"))
	if _, err := io.ReadFull(reader, payload); err != nil {
		t.Fatalf("read target acknowledgement: %v", err)
	}
	if string(payload) != "target-seen" {
		t.Fatalf("unexpected target acknowledgement %q", string(payload))
	}
}

func TestHTTPEntryWebSocketCloseDirections(t *testing.T) {
	t.Run("target close tears down public side", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		origin := startRawWebSocketOrigin(t, nil)
		originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		originPort, err := strconv.Atoi(originPortText)
		if err != nil {
			t.Fatal(err)
		}
		entry, _ := startHTTPProxyRuntime(t, ctx, domain.ProtocolQUIC, originHost, originPort)
		conn, reader, response := openRawWebSocket(t, entry.Addr().String(), "app.example.com", false)
		defer conn.Close()
		if response.StatusCode != nethttp.StatusSwitchingProtocols {
			t.Fatalf("unexpected status %d", response.StatusCode)
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var one [1]byte
		if _, err := reader.Read(one[:]); err == nil {
			t.Fatal("expected public side read to fail after target close")
		}
	})

	t.Run("public close tears down target side", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		targetClosed := make(chan struct{})
		origin := startRawWebSocketOrigin(t, func(conn net.Conn, reader *bufio.Reader) {
			var one [1]byte
			_, _ = reader.Read(one[:])
			close(targetClosed)
		})
		originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		originPort, err := strconv.Atoi(originPortText)
		if err != nil {
			t.Fatal(err)
		}
		entry, _ := startHTTPProxyRuntime(t, ctx, domain.ProtocolQUIC, originHost, originPort)
		conn, _, response := openRawWebSocket(t, entry.Addr().String(), "app.example.com", false)
		if response.StatusCode != nethttp.StatusSwitchingProtocols {
			t.Fatalf("unexpected status %d", response.StatusCode)
		}
		_ = conn.Close()
		select {
		case <-targetClosed:
		case <-time.After(2 * time.Second):
			t.Fatal("target side was not torn down after public close")
		}
	})

	t.Run("close frame round trip before tcp close", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		origin := startRawWebSocketOrigin(t, func(conn net.Conn, reader *bufio.Reader) {
			payload := make([]byte, 2)
			if _, err := io.ReadFull(reader, payload); err == nil && string(payload) == "\x88\x00" {
				_, _ = conn.Write([]byte("\x88\x00"))
			}
		})
		originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		originPort, err := strconv.Atoi(originPortText)
		if err != nil {
			t.Fatal(err)
		}
		entry, _ := startHTTPProxyRuntime(t, ctx, domain.ProtocolQUIC, originHost, originPort)
		conn, reader, response := openRawWebSocket(t, entry.Addr().String(), "app.example.com", false)
		defer conn.Close()
		if response.StatusCode != nethttp.StatusSwitchingProtocols {
			t.Fatalf("unexpected status %d", response.StatusCode)
		}
		if _, err := conn.Write([]byte("\x88\x00")); err != nil {
			t.Fatalf("write close frame: %v", err)
		}
		payload := make([]byte, 2)
		if _, err := io.ReadFull(reader, payload); err != nil {
			t.Fatalf("read close frame response: %v", err)
		}
		if string(payload) != "\x88\x00" {
			t.Fatalf("unexpected close frame response %q", string(payload))
		}
	})
}

func TestHTTPEntryWebSocketHijackFailureClosesStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	targetRead := make(chan error, 1)
	origin := startRawWebSocketOrigin(t, func(conn net.Conn, reader *bufio.Reader) {
		var one [1]byte
		_, err := reader.Read(one[:])
		targetRead <- err
	})
	originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	originPort, err := strconv.Atoi(originPortText)
	if err != nil {
		t.Fatal(err)
	}
	entry, memoryStats := startHTTPProxyRuntime(t, ctx, domain.ProtocolQUIC, originHost, originPort)

	request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://app.example.com/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "app.example.com"
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Connection", "keep-alive, Upgrade")
	request.Header.Set("Sec-WebSocket-Key", "key")
	request.Header.Set("Sec-WebSocket-Version", "13")

	entry.ServeHTTP(httptest.NewRecorder(), request)

	select {
	case err := <-targetRead:
		if err == nil {
			t.Fatal("expected target stream read to fail after hijack failure")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("target stream was not closed after hijack failure")
	}
	waitForHTTPStats(t, memoryStats, func(snapshot stats.ProxyStats) bool {
		return snapshot.HTTPStatusCodes[nethttp.StatusSwitchingProtocols] == 1 && snapshot.HTTPErrors == 0
	})
}

func TestHTTPEntryConcurrentWebSocketsOverTCPTLSMux(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	origin := startRawWebSocketOrigin(t, func(conn net.Conn, reader *bufio.Reader) {
		_, _ = io.Copy(conn, reader)
	})
	originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	originPort, err := strconv.Atoi(originPortText)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := startHTTPProxyRuntime(t, ctx, domain.ProtocolTCPTLS, originHost, originPort)

	const clients = 4
	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := range clients {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			conn, reader, response := openRawWebSocket(t, entry.Addr().String(), "app.example.com", false)
			defer conn.Close()
			if response.StatusCode != nethttp.StatusSwitchingProtocols {
				errs <- errors.New("unexpected upgrade response")
				return
			}
			payload := []byte("client-" + strconv.Itoa(index))
			if _, err := conn.Write(payload); err != nil {
				errs <- err
				return
			}
			echo := make([]byte, len(payload))
			if _, err := io.ReadFull(reader, echo); err != nil {
				errs <- err
				return
			}
			if string(echo) != string(payload) {
				errs <- errors.New("unexpected echo payload")
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestHTTPEntryFlushesEventStreamChunks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	origin := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		flusher, ok := w.(nethttp.Flusher)
		if !ok {
			t.Fatal("origin response writer does not support flush")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(nethttp.StatusOK)
		for _, event := range []string{"data: one\n\n", "data: two\n\n", "data: three\n\n"} {
			if _, err := w.Write([]byte(event)); err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}
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
	entry, _ := startHTTPProxyRuntime(t, ctx, domain.ProtocolQUIC, originHost, originPort)
	request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://"+entry.Addr().String()+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "app.example.com"
	request.Header.Set("Accept", "text/event-stream")
	response, err := nethttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusOK {
		t.Fatalf("unexpected status %d", response.StatusCode)
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		t.Fatalf("expected event-stream content type, got %q", response.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "data: one\n\ndata: two\n\ndata: three\n\n" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestHTTPEntryUpgradeLikeNonWebSocketUsesHTTPPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	seen := make(chan struct{}, 1)
	origin := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Header.Get("Upgrade") != "h2c" {
			t.Fatalf("expected h2c upgrade-like request, got %q", r.Header.Get("Upgrade"))
		}
		seen <- struct{}{}
		_, _ = w.Write([]byte("ordinary-response"))
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
	entry, _ := startHTTPProxyRuntime(t, ctx, domain.ProtocolQUIC, originHost, originPort)
	request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://"+entry.Addr().String()+"/upgrade-like", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "app.example.com"
	request.Header.Set("Upgrade", "h2c")
	request.Header.Set("Connection", "upgrade")
	response, err := nethttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != nethttp.StatusOK || string(body) != "ordinary-response" {
		t.Fatalf("unexpected response status=%d body=%q", response.StatusCode, string(body))
	}
	select {
	case <-seen:
	case <-time.After(2 * time.Second):
		t.Fatal("origin did not receive upgrade-like request")
	}
}

func TestHTTPEntryReleasesQUICStreamsAfterShortRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	origin := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "text/plain")
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

	const streamLimit = 4
	entry, _ := startHTTPProxyRuntimeWithQUICConfig(t, ctx, domain.ProtocolQUIC, originHost, originPort, &quic.Config{MaxIncomingStreams: streamLimit})
	client := &nethttp.Client{Timeout: 2 * time.Second}

	for i := range streamLimit * 3 {
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://"+entry.Addr().String()+"/ok-"+strconv.Itoa(i), nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "app.example.com"
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("short proxy request %d: %v", i, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read short proxy response %d: %v", i, err)
		}
		if response.StatusCode != nethttp.StatusOK || string(body) != "origin-response" {
			t.Fatalf("unexpected short proxy response %d status=%d body=%q", i, response.StatusCode, string(body))
		}
	}
}

//go:fix inline
func stringPtr(value string) *string {
	return new(value)
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

func startHTTPProxyRuntime(t *testing.T, ctx context.Context, protocol domain.Protocol, targetHost string, targetPort int) (*Server, *stats.Memory) {
	return startHTTPProxyRuntimeWithQUICConfig(t, ctx, protocol, targetHost, targetPort, nil)
}

func startHTTPProxyRuntimeWithQUICConfig(t *testing.T, ctx context.Context, protocol domain.Protocol, targetHost string, targetPort int, clientQUICConfig *quic.Config) (*Server, *stats.Memory) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedHTTPProxy(t, ctx, db, targetHost, targetPort)
	sessions := session.NewManager()
	var client *control.ClientConn
	switch protocol {
	case domain.ProtocolQUIC:
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
		var response control.AuthResponse
		client, response, err = control.DialAndAuthenticate(ctx, controlListener.Addr().String(), testClientTLSConfig(t), clientQUICConfig, control.AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: time.Now().UTC(), Protocols: []domain.Protocol{domain.ProtocolQUIC}})
		if err != nil {
			t.Fatalf("dial authenticate: %v", err)
		}
		if !response.Accepted {
			t.Fatalf("expected accepted auth response: %+v", response)
		}
	case domain.ProtocolTCPTLS:
		controlListener, err := control.ListenTLSAddr("127.0.0.1:0", control.Server{
			Authenticator: control.Authenticator{Store: db, Now: func() time.Time { return time.Now().UTC() }},
			Sessions:      sessions,
			TLSConfig:     testServerTLSConfig(t),
			NewSessionID:  func() (string, error) { return "session-1", nil },
		})
		if err != nil {
			t.Fatalf("control tls listen: %v", err)
		}
		t.Cleanup(func() { _ = controlListener.Close() })
		go func() { _ = controlListener.Serve(ctx) }()
		var response control.AuthResponse
		client, response, err = control.DialTLSAndAuthenticate(ctx, controlListener.Addr().String(), testClientTLSConfig(t), control.AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: time.Now().UTC(), Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
		if err != nil {
			t.Fatalf("dial tls authenticate: %v", err)
		}
		if !response.Accepted {
			t.Fatalf("expected accepted auth response: %+v", response)
		}
	default:
		t.Fatalf("unsupported protocol %s", protocol)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	go func() { _ = client.ServeProxyStreams(ctx) }()
	waitForStreamOpener(t, sessions)

	memoryStats := stats.NewMemory()
	var requestMu sync.Mutex
	requestCount := 0
	entry, err := Listen(Entry{Store: db, Sessions: sessions, ListenAddress: "127.0.0.1:0", NewRequest: func() (string, error) {
		requestMu.Lock()
		defer requestMu.Unlock()
		requestCount++
		return "req-" + strconv.Itoa(requestCount), nil
	}, Stats: memoryStats})
	if err != nil {
		t.Fatalf("http listen: %v", err)
	}
	t.Cleanup(func() { _ = entry.Close() })
	go func() { _ = entry.Serve(ctx) }()
	return entry, memoryStats
}

func waitForStreamOpener(t *testing.T, sessions *session.Manager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		latest, ok := sessions.Latest("client-1")
		if ok && latest.StreamOpener != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("session stream opener did not become ready")
}

func startRawWebSocketOrigin(t *testing.T, afterUpgrade func(net.Conn, *bufio.Reader)) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen websocket origin: %v", err)
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
				reader := bufio.NewReader(conn)
				request, err := nethttp.ReadRequest(reader)
				if err != nil {
					return
				}
				_ = request.Body.Close()
				_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: target-accept\r\nSec-WebSocket-Protocol: chat\r\nSec-WebSocket-Extensions: permessage-deflate\r\n\r\n"))
				if afterUpgrade != nil {
					afterUpgrade(conn, reader)
				}
			}(conn)
		}
	}()
	return listener
}

func openRawWebSocket(t *testing.T, addr string, host string, pipelineClientPayload bool) (net.Conn, *bufio.Reader, *nethttp.Response) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial websocket entry: %v", err)
	}
	requestText := "GET /ws HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: keep-alive, Upgrade\r\n" +
		"Sec-WebSocket-Key: key\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Protocol: chat\r\n\r\n"
	if pipelineClientPayload {
		requestText += "client-first"
	}
	if _, err := conn.Write([]byte(requestText)); err != nil {
		_ = conn.Close()
		t.Fatalf("write websocket request: %v", err)
	}
	reader := bufio.NewReader(conn)
	response, err := nethttp.ReadResponse(reader, &nethttp.Request{Method: nethttp.MethodGet})
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read websocket response: %v", err)
	}
	return conn, reader, response
}

func waitForHTTPStats(t *testing.T, memory *stats.Memory, matches func(stats.ProxyStats) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if matches(memory.Snapshot("proxy-1")) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("HTTP stats did not match before deadline: %+v", memory.Snapshot("proxy-1"))
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
