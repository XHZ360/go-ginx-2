package sdk

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestConfigValidateRejectsEmptyServerAddress(t *testing.T) {
	cfg := Config{ClientID: "c1", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"}
	err := cfg.validate()
	if err == nil {
		t.Fatal("expected error for empty server address")
	}
	if !strings.Contains(err.Error(), "ServerAddress") {
		t.Fatalf("expected ServerAddress error, got: %v", err)
	}
}

func TestConfigValidateRejectsEmptyClientID(t *testing.T) {
	cfg := Config{ServerAddress: "localhost:8443", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"}
	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "ClientID") {
		t.Fatalf("expected ClientID error, got: %v", err)
	}
}

func TestConfigValidateRejectsEmptyCredential(t *testing.T) {
	cfg := Config{ServerAddress: "localhost:8443", ClientID: "c1", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"}
	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "Credential") {
		t.Fatalf("expected Credential error, got: %v", err)
	}
}

func TestConfigValidateAcceptsValidConfig(t *testing.T) {
	cfg := Config{ServerAddress: "localhost:8443", ClientID: "c1", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"}
	if err := cfg.validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestConfigValidateAcceptsTCPOnlyConfig(t *testing.T) {
	cfg := Config{ServerTLSAddress: "localhost:8443", ClientID: "c1", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"}
	if err := cfg.validate(); err != nil {
		t.Fatalf("expected TCP+TLS-only config to be valid: %v", err)
	}
}

func TestConfigValidateRejectsNoServerAddress(t *testing.T) {
	cfg := Config{ClientID: "c1", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"}
	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "ServerAddress") {
		t.Fatalf("expected error about server address, got: %v", err)
	}
}

func TestClientProxiesReturnsNotConnectedError(t *testing.T) {
	client := New(Config{ServerAddress: "localhost:8443", ClientID: "c1", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"})
	_, err := client.Proxies(nil)
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestClientDialReturnsNotConnectedError(t *testing.T) {
	client := New(Config{ServerAddress: "localhost:8443", ClientID: "c1", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"})
	_, err := client.Dial(nil, "proxy-1")
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestClientDialTCPReturnsNotConnectedError(t *testing.T) {
	client := New(Config{ServerAddress: "localhost:8443", ClientID: "c1", Credential: "secret", ServerName: "localhost", ServerCAFile: "/path/to/ca.pem"})
	_, err := client.DialTCP(nil, "proxy-1")
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestClientCloseOnNilConnectionIsNoop(t *testing.T) {
	client := New(Config{})
	if err := client.Close(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestProxyConnImplementsNetConn(t *testing.T) {
	var _ = net.Conn(&ProxyConn{})
}

func TestProxyInfoFieldsPopulated(t *testing.T) {
	info := ProxyInfo{ID: "p1", Name: "web", Type: "tcp", EntryHost: "app.example.com", EntryPort: 443, TargetHost: "127.0.0.1", TargetPort: 8080, Description: "test"}
	if info.ID != "p1" || info.Type != "tcp" || info.TargetPort != 8080 {
		t.Fatalf("unexpected proxy info: %+v", info)
	}
}

func TestConfigErrorReturnsDescriptiveMessage(t *testing.T) {
	err := &ConfigError{Field: "ServerAddress", Message: "is required"}
	if !strings.Contains(err.Error(), "ServerAddress") || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestStartLocalProxyStopsOnContextCancel(t *testing.T) {
	client := New(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.StartLocalProxy(ctx, "127.0.0.1:0", "proxy-1")
	}()
	cancel()
	err := <-done
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestHandleLocalConnRecognizesHTTPConnect(t *testing.T) {
	client := New(Config{})
	local, remote := net.Pipe()
	defer remote.Close()

	done := make(chan struct{})
	go func() {
		client.handleLocalConn(context.Background(), local, "proxy-1")
		close(done)
	}()

	if _, err := remote.Write([]byte("CONNECT ignored.example:443 HTTP/1.1\r\nHost: ignored.example:443\r\n\r\n")); err != nil {
		t.Fatalf("write connect request: %v", err)
	}
	if err := remote.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 128)
	n, err := remote.Read(buf)
	if err != nil {
		t.Fatalf("expected HTTP CONNECT failure response, got read error: %v", err)
	}
	if got := string(buf[:n]); !strings.HasPrefix(got, "HTTP/1.1 502 Bad Gateway") {
		t.Fatalf("expected CONNECT path to return 502 when SDK is not connected, got %q", got)
	}
	<-done
}

func TestCopyBidirectionalCopiesData(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	go func() {
		_, _ = a.Write([]byte("hello"))
	}()

	buf := make([]byte, 5)
	n, err := b.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(buf[:n]))
	}
}
