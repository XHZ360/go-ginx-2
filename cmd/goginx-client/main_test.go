package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
)

func TestRunJoinWritesManagedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(enrollment.RedeemResponse{ServerAddress: "127.0.0.1:8443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect})
	}))
	defer server.Close()
	token, err := enrollment.EncodeToken(enrollment.TokenPayload{EnrollmentID: "join-1", Secret: "join-secret", EnrollmentURL: server.URL, ServerAddress: "127.0.0.1:8443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "client-state.json")
	caFile := filepath.Join(t.TempDir(), "server-ca.crt")
	if err := runJoin([]string{"-state", statePath, "-ca-file", caFile, token}); err != nil {
		t.Fatalf("run join: %v", err)
	}
	if content, err := os.ReadFile(caFile); err != nil || string(content) != "ca-pem" {
		t.Fatalf("unexpected ca file content=%q err=%v", string(content), err)
	}
	cfg, err := config.LoadClient(statePath)
	if err != nil {
		t.Fatalf("load client state: %v", err)
	}
	if cfg.ClientID != "client-1" || cfg.Credential != "secret" || cfg.ServerCAFile != caFile {
		t.Fatalf("unexpected managed client config: %+v", cfg)
	}
}

func TestRunJoinWritesManagedStateUnderDeploymentRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(enrollment.RedeemResponse{ServerAddress: "127.0.0.1:8443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect})
	}))
	defer server.Close()
	token, err := enrollment.EncodeToken(enrollment.TokenPayload{EnrollmentID: "join-1", Secret: "join-secret", EnrollmentURL: server.URL, ServerAddress: "127.0.0.1:8443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	deploymentRoot := t.TempDir()
	stateDir := t.TempDir()
	setClientExecutable(t, deploymentRoot)
	t.Chdir(stateDir)

	if err := runJoin([]string{token}); err != nil {
		t.Fatalf("run join: %v", err)
	}

	statePath := filepath.Join(deploymentRoot, "data", "client-state.json")
	caFile := filepath.Join(deploymentRoot, "data", "certs", "server-ca.crt")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected deployment-root client state: %v", err)
	}
	if content, err := os.ReadFile(caFile); err != nil || string(content) != "ca-pem" {
		t.Fatalf("unexpected ca file content=%q err=%v", string(content), err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "data", "client-state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no cwd-relative client state, got err=%v", err)
	}
	cfg, err := loadClientConfig("")
	if err != nil {
		t.Fatalf("load managed client config: %v", err)
	}
	if cfg.ClientID != "client-1" || cfg.ServerCAFile != caFile {
		t.Fatalf("unexpected managed client config: %+v", cfg)
	}
}

func TestLoadClientConfigExplainsMissingManagedState(t *testing.T) {
	deploymentRoot := t.TempDir()
	setClientExecutable(t, deploymentRoot)
	t.Chdir(t.TempDir())
	_, err := loadClientConfig("")
	if err == nil {
		t.Fatal("expected missing managed state error")
	}
	message := err.Error()
	for _, want := range []string{filepath.Join(deploymentRoot, "data", "client-state.json"), "goginx-client join <token>", "-config config/client.json"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in error %q", want, message)
		}
	}
}

func setClientExecutable(t *testing.T, deploymentRoot string) {
	t.Helper()
	previous := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(deploymentRoot, "bin", "goginx-client"), nil
	}
	t.Cleanup(func() {
		executablePath = previous
	})
}
