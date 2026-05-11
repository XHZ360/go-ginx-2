package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestExternalProcessesProxyTCP(t *testing.T) {
	root := repositoryRoot(t)
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverBin := buildCommand(t, root, binDir, "goginx-server", "./cmd/goginx-server")
	clientBin := buildCommand(t, root, binDir, "goginx-client", "./cmd/goginx-client")

	smokeCtx, cancelSmoke := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancelSmoke)
	echoAddress := startEchoOrigin(t, smokeCtx)
	echoHost, echoPort := splitAddress(t, echoAddress)
	controlPort := reservePort(t)
	tcpEntryPort := reservePort(t)
	httpEntryPort := reservePort(t)
	certFile, keyFile, caFile := writeTLSFiles(t, workDir)
	dbPath := filepath.Join(workDir, "go-ginx.db")
	seedSQLite(t, dbPath, domain.Proxy{ID: "tcp-1", UserID: "user-1", ClientID: "client-1", Name: "echo", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: tcpEntryPort, TargetHost: echoHost, TargetPort: echoPort})
	serverConfig := writeJSON(t, filepath.Join(workDir, "server.json"), map[string]any{
		"admin_listen":          "127.0.0.1:0",
		"control_quic_listen":   net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"control_tls_cert_file": certFile,
		"control_tls_key_file":  keyFile,
		"tcp_entry_host":        "127.0.0.1",
		"http_entry_listen":     net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)),
		"sqlite_path":           dbPath,
		"data_dir":              filepath.Join(workDir, "data"),
		"certificate_dir":       filepath.Join(workDir, "certs"),
		"heartbeat_timeout":     int64(time.Second),
		"log_retention_days":    1,
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
	waitForTCPAccept(t, smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpEntryPort)))
	client := startProcess(t, root, clientBin, "-config", clientConfig)

	entryAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpEntryPort))
	if err := waitForEcho(smokeCtx, entryAddress, "ping\n"); err != nil {
		t.Fatalf("external process TCP smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
}

func TestExternalProcessesProxyHTTP(t *testing.T) {
	root := repositoryRoot(t)
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverBin := buildCommand(t, root, binDir, "goginx-server", "./cmd/goginx-server")
	clientBin := buildCommand(t, root, binDir, "goginx-client", "./cmd/goginx-client")

	smokeCtx, cancelSmoke := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancelSmoke)
	origin := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read origin body: %v", err)
			return
		}
		if r.URL.Path != "/hello" || string(body) != "request-body" || r.Header.Get("X-Smoke") != "yes" {
			t.Errorf("unexpected origin request path=%s body=%q header=%s", r.URL.Path, string(body), r.Header.Get("X-Smoke"))
			return
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
	originHost, originPort := splitAddress(t, originURL.Host)
	controlPort := reservePort(t)
	httpEntryPort := reservePort(t)
	certFile, keyFile, caFile := writeTLSFiles(t, workDir)
	dbPath := filepath.Join(workDir, "go-ginx.db")
	seedSQLite(t, dbPath, domain.Proxy{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: originHost, TargetPort: originPort})
	serverConfig := writeJSON(t, filepath.Join(workDir, "server.json"), map[string]any{
		"admin_listen":          "127.0.0.1:0",
		"control_quic_listen":   net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"control_tls_cert_file": certFile,
		"control_tls_key_file":  keyFile,
		"tcp_entry_host":        "127.0.0.1",
		"http_entry_listen":     net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)),
		"sqlite_path":           dbPath,
		"data_dir":              filepath.Join(workDir, "data"),
		"certificate_dir":       filepath.Join(workDir, "certs"),
		"heartbeat_timeout":     int64(time.Second),
		"log_retention_days":    1,
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

	entryURL := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)) + "/hello"
	response, err := waitForHTTP(smokeCtx, entryURL, "app.example.com", "request-body")
	if err != nil {
		t.Fatalf("external process HTTP smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != nethttp.StatusCreated || response.Header.Get("X-Origin") != "ok" || string(responseBody) != "origin-response" {
		t.Fatalf("unexpected HTTP response status=%d header=%s body=%q", response.StatusCode, response.Header.Get("X-Origin"), string(responseBody))
	}
}

func buildCommand(t *testing.T, root string, binDir string, name string, packagePath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	output := filepath.Join(binDir, name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", output, packagePath)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, string(combined))
	}
	return output
}

type runningProcess struct {
	cancel context.CancelFunc
	done   chan error
	stdout safeBuffer
	stderr safeBuffer
}

func startProcess(t *testing.T, root string, binary string, args ...string) *runningProcess {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = root
	process := &runningProcess{cancel: cancel, done: make(chan error, 1)}
	cmd.Stdout = &process.stdout
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start %s: %v", binary, err)
	}
	go func() { process.done <- cmd.Wait() }()
	t.Cleanup(func() {
		process.cancel()
		select {
		case <-process.done:
		case <-time.After(3 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-process.done
		}
	})
	return process
}

func (process *runningProcess) Output() string {
	return "stdout:\n" + process.stdout.String() + "\nstderr:\n" + process.stderr.String()
}

type safeBuffer struct {
	buffer bytes.Buffer
	guard  sync.Mutex
}

func (buffer *safeBuffer) Write(p []byte) (int, error) {
	buffer.guard.Lock()
	defer buffer.guard.Unlock()
	return buffer.buffer.Write(p)
}

func (buffer *safeBuffer) String() string {
	buffer.guard.Lock()
	defer buffer.guard.Unlock()
	return buffer.buffer.String()
}

func waitForTCPAccept(t *testing.T, ctx context.Context, address string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for TCP listener: %v", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatalf("TCP listener %s did not accept connections", address)
}

func waitForEcho(ctx context.Context, address string, payload string) error {
	var lastErr error
	for ctx.Err() == nil {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		_, writeErr := conn.Write([]byte(payload))
		if writeErr == nil {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			line, readErr := bufio.NewReader(conn).ReadString('\n')
			if readErr == nil && line == payload {
				_ = conn.Close()
				return nil
			}
			if readErr != nil {
				lastErr = readErr
			} else {
				lastErr = fmt.Errorf("unexpected echo %q", line)
			}
		} else {
			lastErr = writeErr
		}
		_ = conn.Close()
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return ctx.Err()
}

func waitForHTTP(ctx context.Context, url string, host string, body string) (*nethttp.Response, error) {
	var lastErr error
	client := &nethttp.Client{Timeout: 500 * time.Millisecond}
	for ctx.Err() == nil {
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, url, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Host = host
		request.Header.Set("X-Smoke", "yes")
		response, err := client.Do(request)
		if err == nil && response.StatusCode == nethttp.StatusCreated {
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

func startEchoOrigin(t *testing.T, ctx context.Context) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go echoConnection(conn)
		}
	}()
	return listener.Addr().String()
}

func echoConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		if _, err := conn.Write(line); err != nil {
			return
		}
	}
}

func seedSQLite(t *testing.T, dbPath string, proxies ...domain.Proxy) {
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
	for _, proxy := range proxies {
		if err := db.Proxies().Create(ctx, proxy); err != nil {
			t.Fatalf("create proxy %s: %v", proxy.ID, err)
		}
	}
}

func writeJSON(t *testing.T, path string, value any) string {
	t.Helper()
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTLSFiles(t *testing.T, dir string) (string, string, string) {
	t.Helper()
	certPEM, keyPEM, caPEM := generateCertificate(t)
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

func generateCertificate(t *testing.T) ([]byte, []byte, []byte) {
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

func splitAddress(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd)
}
