package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	nethttp "net/http"
	"net/http/cookiejar"
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

	"github.com/simp-frp/go-ginx-2/internal/deploy"
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

func TestExternalProcessesProxyHTTPS(t *testing.T) {
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
	originAddress, originPool := startTLSEchoOrigin(t, smokeCtx, "secure.example.com")
	originHost, originPort := splitAddress(t, originAddress)
	controlPort := reservePort(t)
	httpEntryPort := reservePort(t)
	httpsEntryPort := reservePort(t)
	certFile, keyFile, caFile := writeTLSFiles(t, workDir)
	dbPath := filepath.Join(workDir, "go-ginx.db")
	seedSQLite(t, dbPath, domain.Proxy{ID: "https-1", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: originHost, TargetPort: originPort})
	serverConfig := writeJSON(t, filepath.Join(workDir, "server.json"), map[string]any{
		"admin_listen":          "127.0.0.1:0",
		"control_quic_listen":   net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"control_tls_cert_file": certFile,
		"control_tls_key_file":  keyFile,
		"tcp_entry_host":        "127.0.0.1",
		"http_entry_listen":     net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)),
		"https_entry_listen":    net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsEntryPort)),
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
	waitForTCPAccept(t, smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsEntryPort)))
	client := startProcess(t, root, clientBin, "-config", clientConfig)

	entryAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(httpsEntryPort))
	if err := waitForTLSEcho(smokeCtx, entryAddress, "secure.example.com", originPool, "ping\n"); err != nil {
		t.Fatalf("external process HTTPS smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
}

func TestExternalProcessesProxyUDP(t *testing.T) {
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
	echoAddress := startUDPEchoOrigin(t, smokeCtx)
	echoHost, echoPort := splitAddress(t, echoAddress)
	controlPort := reservePort(t)
	udpEntryPort := reserveUDPPort(t)
	httpEntryPort := reservePort(t)
	certFile, keyFile, caFile := writeTLSFiles(t, workDir)
	dbPath := filepath.Join(workDir, "go-ginx.db")
	seedSQLite(t, dbPath, domain.Proxy{ID: "udp-1", UserID: "user-1", ClientID: "client-1", Name: "dns", Type: domain.ProxyUDP, Status: domain.ProxyEnabled, EntryPort: udpEntryPort, TargetHost: echoHost, TargetPort: echoPort})
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
	client := startProcess(t, root, clientBin, "-config", clientConfig)
	entryAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(udpEntryPort))
	if err := waitForUDPEcho(smokeCtx, entryAddress, "ping"); err != nil {
		t.Fatalf("external process UDP smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
}

func TestExternalProcessesAdminAPIUI(t *testing.T) {
	root := repositoryRoot(t)
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	frontendDir := writeAdminFrontendFixture(t)
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverBin := buildCommand(t, root, binDir, "goginx-server", "./cmd/goginx-server")
	clientBin := buildCommand(t, root, binDir, "goginx-client", "./cmd/goginx-client")

	smokeCtx, cancelSmoke := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancelSmoke)
	controlPort := reservePort(t)
	httpEntryPort := reservePort(t)
	adminPort := reservePort(t)
	certFile, keyFile, caFile := writeTLSFiles(t, workDir)
	adminCredsFile := writeAdminCredentialsFile(t, workDir, "admin", "secret")
	dbPath := filepath.Join(workDir, "go-ginx.db")
	seedSQLite(t, dbPath, domain.Proxy{ID: "http-1", UserID: "user-1", ClientID: "client-1", Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080})
	serverConfig := writeJSON(t, filepath.Join(workDir, "server.json"), map[string]any{
		"admin_listen":           net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort)),
		"admin_credentials_file": adminCredsFile,
		"admin_frontend_dir":     frontendDir,
		"control_quic_listen":    net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)),
		"control_tls_cert_file":  certFile,
		"control_tls_key_file":   keyFile,
		"tcp_entry_host":         "127.0.0.1",
		"http_entry_listen":      net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)),
		"sqlite_path":            dbPath,
		"data_dir":               filepath.Join(workDir, "data"),
		"certificate_dir":        filepath.Join(workDir, "certs"),
		"heartbeat_timeout":      int64(time.Second),
		"log_retention_days":     1,
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
	waitForTCPAccept(t, smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort)))
	client := startProcess(t, root, clientBin, "-config", clientConfig)
	if err := waitForAdminFrontendRoute(smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort))); err != nil {
		t.Fatalf("admin frontend route smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
	if err := waitForMissingAdminAsset(smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort))); err != nil {
		t.Fatalf("admin missing asset smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
	if err := waitForAdminSession(smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort)), "admin", "secret"); err != nil {
		t.Fatalf("admin session smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
	if err := waitForAdminGraphQL(smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort)), "admin", "secret"); err != nil {
		t.Fatalf("admin graphql smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
}

func TestDeployBundleRuntimeRestartRecovery(t *testing.T) {
	root := repositoryRoot(t)
	workDir := t.TempDir()
	bundleDir := filepath.Join(workDir, "bundle")
	if err := deploy.BuildBundle(context.Background(), deploy.BundleOptions{RepoRoot: root, OutputDir: bundleDir, GoOS: runtime.GOOS, GoArch: runtime.GOARCH, InstallRoot: "/opt/go-ginx"}); err != nil {
		t.Fatalf("build deploy bundle: %v", err)
	}

	smokeCtx, cancelSmoke := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancelSmoke)
	echoAddress := startEchoOrigin(t, smokeCtx)
	echoHost, echoPort := splitAddress(t, echoAddress)
	controlTLSPort := reservePort(t)
	tcpEntryPort := reservePort(t)
	httpEntryPort := reservePort(t)
	certDir := filepath.Join(bundleDir, "data", "certs")
	certFile, keyFile, caFile := writeTLSFiles(t, certDir)
	dbPath := filepath.Join(bundleDir, "data", "go-ginx.db")
	seedSQLite(t, dbPath, domain.Proxy{ID: "tcp-1", UserID: "user-1", ClientID: "client-1", Name: "echo", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: tcpEntryPort, TargetHost: echoHost, TargetPort: echoPort})
	serverConfig := writeJSON(t, filepath.Join(bundleDir, "config", "server.json"), map[string]any{
		"admin_listen":          "127.0.0.1:0",
		"control_quic_listen":   "127.0.0.1:0",
		"control_tls_listen":    net.JoinHostPort("127.0.0.1", strconv.Itoa(controlTLSPort)),
		"control_tls_cert_file": filepath.ToSlash(filepath.Join("data", "certs", filepath.Base(certFile))),
		"control_tls_key_file":  filepath.ToSlash(filepath.Join("data", "certs", filepath.Base(keyFile))),
		"tcp_entry_host":        "127.0.0.1",
		"http_entry_listen":     net.JoinHostPort("127.0.0.1", strconv.Itoa(httpEntryPort)),
		"sqlite_path":           filepath.ToSlash(filepath.Join("data", filepath.Base(dbPath))),
		"data_dir":              "data",
		"certificate_dir":       filepath.ToSlash(filepath.Join("data", "certs")),
		"heartbeat_timeout":     int64(time.Second),
		"log_retention_days":    1,
	})
	clientConfig := writeJSON(t, filepath.Join(bundleDir, "config", "client.json"), map[string]any{
		"server_address":     "127.0.0.1:1",
		"server_tls_address": net.JoinHostPort("127.0.0.1", strconv.Itoa(controlTLSPort)),
		"server_name":        "localhost",
		"server_ca_file":     filepath.ToSlash(filepath.Join("data", "certs", filepath.Base(caFile))),
		"client_id":          "client-1",
		"credential":         "secret",
		"allowed_protocols":  []string{string(domain.ProtocolTCPTLS)},
		"reconnect": map[string]any{
			"initial_delay": int64(10 * time.Millisecond),
			"max_delay":     int64(20 * time.Millisecond),
		},
	})

	serverBin := filepath.Join(bundleDir, "bin", bundleBinaryName("goginx-server"))
	clientBin := filepath.Join(bundleDir, "bin", bundleBinaryName("goginx-client"))
	server := startProcess(t, bundleDir, serverBin, "-config", filepath.ToSlash(filepath.Join("config", filepath.Base(serverConfig))))
	waitForTCPAccept(t, smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpEntryPort)))
	client := startProcess(t, bundleDir, clientBin, "-config", filepath.ToSlash(filepath.Join("config", filepath.Base(clientConfig))))

	entryAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpEntryPort))
	if err := waitForEcho(smokeCtx, entryAddress, "ping\n"); err != nil {
		t.Fatalf("bundle startup smoke failed: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
	}
	server.cancel()
	select {
	case <-server.done:
	case <-time.After(3 * time.Second):
		t.Fatalf("server process did not stop in time\nserver output:\n%s", server.Output())
	}

	restartedServer := startProcess(t, bundleDir, serverBin, "-config", filepath.ToSlash(filepath.Join("config", filepath.Base(serverConfig))))
	waitForTCPAccept(t, smokeCtx, net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpEntryPort)))
	if err := waitForEcho(smokeCtx, entryAddress, "pong\n"); err != nil {
		t.Fatalf("bundle restart recovery failed: %v\nserver output:\n%s\nrestarted server output:\n%s\nclient output:\n%s", err, server.Output(), restartedServer.Output(), client.Output())
	}
}

func TestExternalProcessesConfiglessServerAndClientJoin(t *testing.T) {
	root := repositoryRoot(t)
	workDir := t.TempDir()
	deployDir := filepath.Join(workDir, "deploy")
	stateDir := filepath.Join(workDir, "state")
	binDir := filepath.Join(deployDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverBin := buildCommand(t, root, binDir, "goginx-server", "./cmd/goginx-server")
	clientBin := buildCommand(t, root, binDir, "goginx-client", "./cmd/goginx-client")
	adminBin := buildCommand(t, root, binDir, "goginx-admin", "./cmd/goginx-admin")

	smokeCtx, cancelSmoke := context.WithTimeout(context.Background(), 25*time.Second)
	t.Cleanup(cancelSmoke)
	adminAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(reservePort(t)))
	controlQUICAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(reserveUDPPort(t)))
	controlTLSAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(reservePort(t)))
	httpEntryAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(reservePort(t)))
	writeAdminFrontendFixtureAt(t, filepath.Join(deployDir, "admin-ui"))
	deployDataDir := filepath.Join(deployDir, "data")
	deployCertDir := filepath.Join(deployDataDir, "certs")
	server := startProcessEnv(t, stateDir, map[string]string{
		"GOGINX_ADMIN_LISTEN":          adminAddress,
		"GOGINX_CONTROL_QUIC_LISTEN":   controlQUICAddress,
		"GOGINX_CONTROL_TLS_LISTEN":    controlTLSAddress,
		"GOGINX_CONTROL_TLS_CA_FILE":   filepath.Join(deployCertDir, "control-ca.crt"),
		"GOGINX_CONTROL_TLS_CERT_FILE": filepath.Join(deployCertDir, "control.crt"),
		"GOGINX_CONTROL_TLS_KEY_FILE":  filepath.Join(deployCertDir, "control.key"),
		"GOGINX_HTTP_ENTRY_LISTEN":     httpEntryAddress,
		"GOGINX_SQLITE_PATH":           filepath.Join(deployDataDir, "go-ginx.db"),
		"GOGINX_DATA_DIR":              deployDataDir,
		"GOGINX_CERTIFICATE_DIR":       deployCertDir,
	}, serverBin)
	waitForTCPAccept(t, smokeCtx, adminAddress)
	waitForFile(t, smokeCtx, filepath.Join(deployCertDir, "control-ca.crt"))
	waitForFile(t, smokeCtx, filepath.Join(deployDataDir, "go-ginx.db"))
	if err := waitForAdminFrontendRoute(smokeCtx, adminAddress); err != nil {
		t.Fatalf("configless admin frontend failed: %v\nserver output:\n%s", err, server.Output())
	}

	runCommand(t, smokeCtx, stateDir, adminBin, "init-admin", "-id", "admin-1", "-username", "admin", "-password", "secret")
	if err := waitForAdminDashboard(smokeCtx, adminAddress, "admin", "secret", 0); err != nil {
		t.Fatalf("configless admin login failed: %v\nserver output:\n%s", err, server.Output())
	}
	token := strings.TrimSpace(runCommand(t, smokeCtx, stateDir, adminBin, "create-client-join", "-id", "client-1", "-user", "admin-1", "-name", "home", "-server-ca-file", filepath.Join(deployCertDir, "control-ca.crt"), "-server-name", "go-ginx-control.local", "-server-address", controlQUICAddress, "-server-tls-address", controlTLSAddress, "-enrollment-url", "http://"+adminAddress+"/api/client/enroll"))
	if token == "" {
		t.Fatal("expected join token")
	}
	runCommand(t, smokeCtx, stateDir, clientBin, "join", token)
	waitForFile(t, smokeCtx, filepath.Join(deployDir, "data", "client-state.json"))
	client := startProcess(t, stateDir, clientBin)
	if err := waitForAdminDashboard(smokeCtx, adminAddress, "admin", "secret", 1); err != nil {
		t.Fatalf("joined configless client did not come online: %v\nserver output:\n%s\nclient output:\n%s", err, server.Output(), client.Output())
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

func runCommand(t *testing.T, ctx context.Context, dir string, binary string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %s: %v\n%s", binary, strings.Join(args, " "), err, string(combined))
	}
	return string(combined)
}

type runningProcess struct {
	cancel context.CancelFunc
	done   chan error
	stdout safeBuffer
	stderr safeBuffer
}

func startProcess(t *testing.T, root string, binary string, args ...string) *runningProcess {
	return startProcessEnv(t, root, nil, binary, args...)
}

func startProcessEnv(t *testing.T, root string, env map[string]string, binary string, args ...string) *runningProcess {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = root
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
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
			select {
			case <-process.done:
			case <-time.After(3 * time.Second):
			}
		}
	})
	return process
}

func bundleBinaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
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

func waitForFile(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	for ctx.Err() == nil {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("file %s was not created: %v", path, ctx.Err())
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

func waitForUDPEcho(ctx context.Context, address string, payload string) error {
	var lastErr error
	for ctx.Err() == nil {
		conn, err := net.DialTimeout("udp", address, 200*time.Millisecond)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		_, writeErr := conn.Write([]byte(payload))
		if writeErr == nil {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			buffer := make([]byte, 64*1024)
			n, readErr := conn.Read(buffer)
			if readErr == nil && string(buffer[:n]) == payload {
				_ = conn.Close()
				return nil
			}
			if readErr != nil {
				lastErr = readErr
			} else {
				lastErr = fmt.Errorf("unexpected echo %q", string(buffer[:n]))
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

func waitForTLSEcho(ctx context.Context, address string, serverName string, roots *x509.CertPool, payload string) error {
	var lastErr error
	for ctx.Err() == nil {
		dialer := &net.Dialer{Timeout: 200 * time.Millisecond}
		conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{RootCAs: roots, ServerName: serverName, MinVersion: tls.VersionTLS12})
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
				lastErr = fmt.Errorf("unexpected tls echo %q", line)
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

func waitForAdminFrontendRoute(ctx context.Context, address string) error {
	var lastErr error
	for ctx.Err() == nil {
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://"+address+"/dashboard", nil)
		if err != nil {
			return err
		}
		response, err := (&nethttp.Client{Timeout: 500 * time.Millisecond}).Do(request)
		if err == nil && response.StatusCode == nethttp.StatusOK {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil && strings.Contains(string(body), "admin frontend shell") {
				return nil
			}
			lastErr = readErr
		} else if response != nil {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected status %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return ctx.Err()
}

func waitForMissingAdminAsset(ctx context.Context, address string) error {
	var lastErr error
	for ctx.Err() == nil {
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://"+address+"/assets/missing.js", nil)
		if err != nil {
			return err
		}
		response, err := (&nethttp.Client{Timeout: 500 * time.Millisecond}).Do(request)
		if err == nil && response.StatusCode == nethttp.StatusNotFound {
			_ = response.Body.Close()
			return nil
		} else if response != nil {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected status %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return ctx.Err()
}

func waitForAdminSession(ctx context.Context, address string, username string, password string) error {
	var lastErr error
	for ctx.Err() == nil {
		client, err := loginAdminAPI(address, username, password)
		if err == nil {
			request, requestErr := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, "http://"+address+"/api/admin/session", nil)
			if requestErr != nil {
				return requestErr
			}
			response, doErr := client.Do(request)
			if doErr == nil && response.StatusCode == nethttp.StatusOK {
				body, readErr := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if readErr == nil && strings.Contains(string(body), `"authenticated":true`) {
					return nil
				}
				lastErr = readErr
			} else if response != nil {
				_ = response.Body.Close()
				lastErr = fmt.Errorf("unexpected status %d", response.StatusCode)
			} else {
				lastErr = doErr
			}
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return ctx.Err()
}

func waitForAdminGraphQL(ctx context.Context, address string, username string, password string) error {
	var lastErr error
	for ctx.Err() == nil {
		client, err := loginAdminAPI(address, username, password)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		payload := bytes.NewBufferString(`{"query":"query { dashboardSummary { onlineClientCount enabledProxyCount } }"}`)
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, "http://"+address+"/api/admin/graphql", payload)
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err == nil && response.StatusCode == nethttp.StatusOK {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil && strings.Contains(string(body), "dashboardSummary") {
				return nil
			}
			lastErr = readErr
		} else if response != nil {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected status %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return ctx.Err()
}

func waitForAdminDashboard(ctx context.Context, address string, username string, password string, onlineClients int) error {
	var lastErr error
	for ctx.Err() == nil {
		client, err := loginAdminAPI(address, username, password)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		payload := bytes.NewBufferString(`{"query":"query { dashboard { onlineClientCount enabledProxyCount } }"}`)
		request, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, "http://"+address+"/api/admin/graphql", payload)
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != nethttp.StatusOK {
			lastErr = fmt.Errorf("unexpected status %d body=%s", response.StatusCode, string(body))
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if readErr != nil {
			lastErr = readErr
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var decoded struct {
			Data struct {
				Dashboard struct {
					OnlineClientCount int `json:"onlineClientCount"`
				} `json:"dashboard"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if decoded.Data.Dashboard.OnlineClientCount == onlineClients {
			return nil
		}
		lastErr = fmt.Errorf("online clients = %d, want %d", decoded.Data.Dashboard.OnlineClientCount, onlineClients)
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return ctx.Err()
}

func loginAdminAPI(address string, username string, password string) (*nethttp.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &nethttp.Client{Timeout: 500 * time.Millisecond, Jar: jar}
	body := bytes.NewBufferString(`{"username":"` + username + `","password":"` + password + `"}`)
	request, err := nethttp.NewRequest(nethttp.MethodPost, "http://"+address+"/api/admin/login", body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("unexpected login status %d body=%s", response.StatusCode, string(body))
	}
	return client, nil
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

func startUDPEchoOrigin(t *testing.T, ctx context.Context) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	go func() {
		buffer := make([]byte, 64*1024)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = conn.WriteTo(buffer[:n], addr)
		}
	}()
	return conn.LocalAddr().String()
}

func startTLSEchoOrigin(t *testing.T, ctx context.Context, serverName string) (string, *x509.CertPool) {
	t.Helper()
	certPEM, keyPEM, caPEM := generateCertificateFor(t, serverName)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append origin CA")
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
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
	return listener.Addr().String(), pool
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

func writeAdminCredentialsFile(t *testing.T, dir string, username string, password string) string {
	t.Helper()
	hash, err := domain.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "admin-creds.json")
	writeFile(t, path, []byte(`{"administrators":[{"username":"`+username+`","password_hash":"`+hash+`"}]}`))
	return path
}

func writeAdminFrontendFixture(t *testing.T) string {
	t.Helper()
	return writeAdminFrontendFixtureAt(t, t.TempDir())
}

func writeAdminFrontendFixtureAt(t *testing.T, dir string) string {
	t.Helper()
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "index.html"), []byte("<!doctype html><html><head><title>Admin UI</title></head><body><div id=app>admin frontend shell</div></body></html>"))
	writeFile(t, filepath.Join(assetsDir, "app.js"), []byte("window.__adminFrontend = true;"))
	return dir
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func generateCertificate(t *testing.T) ([]byte, []byte, []byte) {
	return generateCertificateFor(t, "localhost")
}

func generateCertificateFor(t *testing.T, serverName string) ([]byte, []byte, []byte) {
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
	serverTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: serverName}, DNSNames: []string{serverName}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
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

func requireDefaultConfiglessPorts(t *testing.T) {
	t.Helper()
	for _, address := range []string{"127.0.0.1:8080", "127.0.0.1:8081", "127.0.0.1:9443"} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			t.Skipf("default configless TCP port unavailable %s: %v", address, err)
		}
		_ = listener.Close()
	}
	packet, err := net.ListenPacket("udp", "127.0.0.1:8443")
	if err != nil {
		t.Skipf("default configless UDP port unavailable 127.0.0.1:8443: %v", err)
	}
	_ = packet.Close()
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
