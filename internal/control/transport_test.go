package control

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
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
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

func TestQUICHandshakePersistsClientOnlineAndOffline(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	statusLog := make([]domain.ClientStatus, 0)
	authStore := newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret"))
	authStore.clientStatusLog = &statusLog
	listener, _ := startTestListener(t, Authenticator{
		Store:             authStore,
		AllowedProtocols:  []domain.Protocol{domain.ProtocolQUIC},
		HeartbeatInterval: 10 * time.Second,
		Now:               func() time.Time { return now },
	})

	client, response, err := DialAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), nil, AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	if !response.Accepted {
		t.Fatalf("unexpected auth response: %+v", response)
	}
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read proxy snapshot: %v", err)
	}
	waitForClientStatusLog(t, &statusLog, []domain.ClientStatus{domain.ClientOnline})

	if err := client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	waitForClientStatusLog(t, &statusLog, []domain.ClientStatus{domain.ClientOnline, domain.ClientOffline})
}

func TestQUICHandshakeSendsProxySnapshot(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	authStore := newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret"))
	authStore.proxies = []domain.Proxy{
		{ID: "p1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
		{ID: "p2", UserID: "user-2", ClientID: "other-client", Name: "other", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22},
	}
	listener, _ := startTestListener(t, Authenticator{Store: authStore, Now: func() time.Time { return now }})

	client, response, err := DialAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), nil, AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if !response.Accepted {
		t.Fatalf("expected accepted auth response: %+v", response)
	}

	snapshot, err := client.ReadProxySnapshot()
	if err != nil {
		t.Fatalf("read proxy snapshot: %v", err)
	}
	if snapshot.Version != 7 || len(snapshot.Proxies) != 1 || snapshot.Proxies[0].ID != "p1" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
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
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read proxy snapshot: %v", err)
	}

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

func TestTCPTLSHandshakeRegistersSession(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestTLSListener(t, Authenticator{
		Store:             newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		AllowedProtocols:  []domain.Protocol{domain.ProtocolTCPTLS},
		HeartbeatInterval: 10 * time.Second,
		Now:               func() time.Time { return now },
	})

	client, response, err := DialTLSAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if !response.Accepted || response.SessionID != "session-1" || response.SelectedProtocol != domain.ProtocolTCPTLS {
		t.Fatalf("unexpected auth response: %+v", response)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		latest, ok := sessions.Latest("client-1")
		if ok && latest.ID == response.SessionID && latest.UserID == "user-1" && latest.Protocol == domain.ProtocolTCPTLS && latest.StreamOpener != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("tcp+tls session was not published after mux became ready")
}

func TestTCPTLSHandshakeSendsProxySnapshot(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	authStore := newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret"))
	authStore.proxies = []domain.Proxy{
		{ID: "p1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
		{ID: "p2", UserID: "user-2", ClientID: "other-client", Name: "other", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22},
	}
	listener, _ := startTestTLSListener(t, Authenticator{Store: authStore, AllowedProtocols: []domain.Protocol{domain.ProtocolTCPTLS}, Now: func() time.Time { return now }})

	client, response, err := DialTLSAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if !response.Accepted {
		t.Fatalf("expected accepted auth response: %+v", response)
	}

	snapshot, err := client.ReadProxySnapshot()
	if err != nil {
		t.Fatalf("read proxy snapshot: %v", err)
	}
	if snapshot.Version != 7 || len(snapshot.Proxies) != 1 || snapshot.Proxies[0].ID != "p1" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestTCPTLSHeartbeatUpdatesSession(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestTLSListener(t, Authenticator{
		Store:            newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		AllowedProtocols: []domain.Protocol{domain.ProtocolTCPTLS},
		Now:              func() time.Time { return now },
	})

	client, response, err := DialTLSAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read proxy snapshot: %v", err)
	}

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

func TestTCPTLSDialRejectsServerNameMismatch(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, _ := startTestTLSListener(t, Authenticator{
		Store:            newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		AllowedProtocols: []domain.Protocol{domain.ProtocolTCPTLS},
		Now:              func() time.Time { return now },
	})
	tlsConfig := testClientTLSConfig(t)
	tlsConfig.ServerName = "wrong.example.com"

	_, _, err := DialTLSAndAuthenticate(context.Background(), listener.Addr().String(), tlsConfig, AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
	if err == nil {
		t.Fatal("expected server name verification error")
	}
}

func TestQUICListenerRejectsTCPTLSOnlyOffer(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestListener(t, Authenticator{
		Store: newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		Now:   func() time.Time { return now },
	})

	client, response, err := DialAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), nil, AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	if client != nil {
		t.Fatal("rejected authentication should not return client connection")
	}
	if response.Accepted || response.Reason == "" {
		t.Fatalf("expected protocol rejection, got %+v", response)
	}
	if _, ok := sessions.Latest("client-1"); ok {
		t.Fatal("rejected protocol must not register a session")
	}
}

func TestTCPTLSListenerRejectsQUICOnlyOffer(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestTLSListener(t, Authenticator{
		Store: newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		Now:   func() time.Time { return now },
	})

	client, response, err := DialTLSAndAuthenticate(context.Background(), listener.Addr().String(), testClientTLSConfig(t), AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	if client != nil {
		t.Fatal("rejected authentication should not return client connection")
	}
	if response.Accepted || response.Reason == "" {
		t.Fatalf("expected protocol rejection, got %+v", response)
	}
	if _, ok := sessions.Latest("client-1"); ok {
		t.Fatal("rejected protocol must not register a session")
	}
}

func TestTCPTLSMuxServesProxyStream(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	listener, sessions := startTestTLSListener(t, Authenticator{
		Store: newAuthStore(domain.UserEnabled, domain.ClientOffline, domain.HashCredential("secret")),
		Now:   func() time.Time { return now },
	})
	target := startEchoTarget(t)
	_, targetPort, err := net.SplitHostPort(target.Addr().String())
	if err != nil {
		t.Fatalf("split target addr: %v", err)
	}
	parsedTargetPort, ok := parseTestPort(targetPort)
	if !ok {
		t.Fatalf("parse target port %q", targetPort)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client, response, err := DialTLSAndAuthenticate(ctx, listener.Addr().String(), testClientTLSConfig(t), AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: now, Protocols: []domain.Protocol{domain.ProtocolTCPTLS}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read proxy snapshot: %v", err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- client.ServeProxyStreams(ctx) }()

	var latest session.Session
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var ok bool
		latest, ok = sessions.Latest("client-1")
		if ok && latest.StreamOpener != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if latest.StreamOpener == nil {
		t.Fatalf("expected stream opener on tcp+tls session")
	}
	stream, err := latest.StreamOpener.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()
	if err := WriteMessage(stream, MessageOpenStream, OpenStream{ProxyID: "p1", ConnectionID: response.SessionID, TargetHost: "127.0.0.1", TargetPort: parsedTargetPort}); err != nil {
		t.Fatalf("write open stream: %v", err)
	}
	if _, err := stream.Write([]byte("ping")); err != nil {
		t.Fatalf("write stream payload: %v", err)
	}
	payload := make([]byte, 4)
	if _, err := io.ReadFull(stream, payload); err != nil {
		t.Fatalf("read stream payload: %v", err)
	}
	if string(payload) != "ping" {
		t.Fatalf("unexpected echo payload %q", string(payload))
	}
	cancel()
	if err := <-serveDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("serve proxy streams: %v", err)
	}
}

func TestHandleHTTPStreamWebSocketUpgradeForwardsHeadersAndTunnels(t *testing.T) {
	target := startWebSocketTarget(t, func(t *testing.T, request *http.Request, targetAuthority string) {
		if request.Proto != "HTTP/1.1" {
			t.Fatalf("expected HTTP/1.1 target request, got %s", request.Proto)
		}
		if request.Host != targetAuthority {
			t.Fatalf("expected target Host %q, got %q", targetAuthority, request.Host)
		}
		if got := request.Header.Get("Origin"); got != "http://"+targetAuthority {
			t.Fatalf("expected rewritten Origin, got %q", got)
		}
		if got := request.Header.Get("Upgrade"); got != "websocket" {
			t.Fatalf("expected Upgrade websocket, got %q", got)
		}
		if got := request.Header.Get("Connection"); got != "Upgrade" {
			t.Fatalf("expected normalized Connection Upgrade, got %q", got)
		}
		if got := request.Header.Get("Sec-WebSocket-Protocol"); got != "chat" {
			t.Fatalf("expected subprotocol header to pass through, got %q", got)
		}
	}, func(conn net.Conn, reader *bufio.Reader) {
		payload := make([]byte, 4)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return
		}
		if string(payload) == "ping" {
			_, _ = conn.Write([]byte("pong"))
		}
	})
	targetHost, targetPortText, err := net.SplitHostPort(target.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	targetPort, ok := parseTestPort(targetPortText)
	if !ok {
		t.Fatalf("parse target port %q", targetPortText)
	}

	client, stream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	go handleHTTPStream(stream, OpenStream{TargetHost: targetHost, TargetPort: targetPort})

	request := "GET /ws HTTP/1.1\r\n" +
		"Host: app.example.com\r\n" +
		"Origin: https://app.example.com\r\n" +
		"Upgrade: WebSocket\r\n" +
		"Connection: keep-alive, Upgrade\r\n" +
		"Sec-WebSocket-Key: key\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Protocol: chat\r\n\r\n"
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	reader := bufio.NewReader(client)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read upgrade response: %v", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols || response.Header.Get("Sec-WebSocket-Accept") != "target-accept" || response.Header.Get("Sec-WebSocket-Protocol") != "chat" || response.Header.Get("Sec-WebSocket-Extensions") != "permessage-deflate" {
		t.Fatalf("unexpected upgrade response status=%d headers=%v", response.StatusCode, response.Header)
	}
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write tunneled payload: %v", err)
	}
	payload := make([]byte, 4)
	if _, err := io.ReadFull(reader, payload); err != nil {
		t.Fatalf("read tunneled payload: %v", err)
	}
	if string(payload) != "pong" {
		t.Fatalf("unexpected tunneled payload %q", string(payload))
	}
}

func TestHandleHTTPStreamWebSocketNon101FallsBackToHTTPResponse(t *testing.T) {
	target := startHTTPResponseTarget(t, "HTTP/1.1 403 Forbidden\r\nContent-Length: 9\r\nX-Reason: denied\r\n\r\nforbidden")
	targetHost, targetPortText, err := net.SplitHostPort(target.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	targetPort, ok := parseTestPort(targetPortText)
	if !ok {
		t.Fatalf("parse target port %q", targetPortText)
	}

	client, stream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	go handleHTTPStream(stream, OpenStream{TargetHost: targetHost, TargetPort: targetPort})

	request := "GET /ws HTTP/1.1\r\nHost: app.example.com\r\nUpgrade: websocket\r\nConnection: upgrade\r\n\r\n"
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(client), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden || response.Header.Get("X-Reason") != "denied" || string(body) != "forbidden" {
		t.Fatalf("unexpected response status=%d headers=%v body=%q", response.StatusCode, response.Header, string(body))
	}
}

func TestHandleHTTPStreamWebSocketTargetUnreachableReturnsBadGateway(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	targetAddr := listener.Addr().String()
	_ = listener.Close()
	targetHost, targetPortText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatal(err)
	}
	targetPort, ok := parseTestPort(targetPortText)
	if !ok {
		t.Fatalf("parse target port %q", targetPortText)
	}

	client, stream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	go handleHTTPStream(stream, OpenStream{TargetHost: targetHost, TargetPort: targetPort})

	request := "GET /ws HTTP/1.1\r\nHost: app.example.com\r\nUpgrade: websocket\r\nConnection: upgrade\r\n\r\n"
	if _, err := client.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(client), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 response, got %d", response.StatusCode)
	}
}

func TestProxyStreamCloseCancelsReadAndClosesOnce(t *testing.T) {
	recorder := &cancelReadCloseRecorder{}
	stream := wrapProxyStream(recorder)

	if err := stream.Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second close stream: %v", err)
	}

	if recorder.cancelReads != 1 || recorder.cancelCode != 0 {
		t.Fatalf("expected one CancelRead(0), got count=%d code=%d", recorder.cancelReads, recorder.cancelCode)
	}
	if recorder.closes != 1 {
		t.Fatalf("expected one underlying close, got %d", recorder.closes)
	}
}

type cancelReadCloseRecorder struct {
	cancelReads int
	cancelCode  quic.StreamErrorCode
	closes      int
}

func (recorder *cancelReadCloseRecorder) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (recorder *cancelReadCloseRecorder) Write(p []byte) (int, error) {
	return len(p), nil
}

func (recorder *cancelReadCloseRecorder) Close() error {
	recorder.closes++
	return nil
}

func (recorder *cancelReadCloseRecorder) CancelRead(code quic.StreamErrorCode) {
	recorder.cancelReads++
	recorder.cancelCode = code
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

func startTestTLSListener(t *testing.T, authenticator Authenticator) (*TLSListener, *session.Manager) {
	t.Helper()
	sessions := session.NewManager()
	listener, err := ListenTLSAddr("127.0.0.1:0", Server{
		Authenticator: authenticator,
		Sessions:      sessions,
		TLSConfig:     testServerTLSConfig(t),
		NewSessionID:  func() (string, error) { return "session-1", nil },
	})
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
	})
	go func() { _ = listener.Serve(ctx) }()
	return listener, sessions
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

func startWebSocketTarget(t *testing.T, assertRequest func(*testing.T, *http.Request, string), afterUpgrade func(net.Conn, *bufio.Reader)) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen websocket target: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	targetAuthority := listener.Addr().String()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				request, err := http.ReadRequest(reader)
				if err != nil {
					return
				}
				if assertRequest != nil {
					assertRequest(t, request, targetAuthority)
				}
				_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: target-accept\r\nSec-WebSocket-Protocol: chat\r\nSec-WebSocket-Extensions: permessage-deflate\r\n\r\n"))
				if afterUpgrade != nil {
					afterUpgrade(conn, reader)
				}
			}(conn)
		}
	}()
	return listener
}

func startHTTPResponseTarget(t *testing.T, response string) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen response target: %v", err)
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
				_, _ = http.ReadRequest(bufio.NewReader(conn))
				_, _ = conn.Write([]byte(response))
			}(conn)
		}
	}()
	return listener
}

func waitForClientStatusLog(t *testing.T, got *[]domain.ClientStatus, want []domain.ClientStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if clientStatusLogMatches(*got, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("client status log did not reach %+v, got %+v", want, *got)
}

func clientStatusLogMatches(got []domain.ClientStatus, want []domain.ClientStatus) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func parseTestPort(port string) (int, bool) {
	value := 0
	for _, char := range port {
		if char < '0' || char > '9' {
			return 0, false
		}
		value = value*10 + int(char-'0')
	}
	return value, true
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
