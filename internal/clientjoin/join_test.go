package clientjoin

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestJoinRedeemsTokenAndReturnsClientConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client/enroll" || r.Method != http.MethodPost {
			t.Fatalf("unexpected enrollment request %s %s", r.Method, r.URL.Path)
		}
		var request enrollment.RedeemRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Token == "" {
			t.Fatal("expected token")
		}
		_ = json.NewEncoder(w).Encode(enrollment.RedeemResponse{ServerAddress: "127.0.0.1:8443", ServerTLSAddress: "127.0.0.1:9443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect})
	}))
	defer server.Close()
	token, err := enrollment.EncodeToken(enrollment.TokenPayload{EnrollmentID: "join-1", Secret: "join-secret", EnrollmentURL: server.URL + "/api/client/enroll", ServerAddress: "127.0.0.1:8443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}

	cfg, caPEM, err := Join(context.Background(), token, nil)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if cfg.ClientID != "client-1" || cfg.Credential != "secret" || cfg.ServerName != "go-ginx-control.test" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if string(caPEM) != "ca-pem" {
		t.Fatalf("unexpected ca pem %q", string(caPEM))
	}
}

func TestJoinReturnsHostFromServerConfigDefaults(t *testing.T) {
	root := t.TempDir()
	serverCfg := config.DefaultServer()
	serverCfg.JoinServiceHost = "join.example.com"
	serverCfg.ControlQUICListen = ":18443"
	serverCfg.ControlTLSListen = ":19443"
	serverCfg.AdminListen = ":18080"
	serverCfg.ControlTLSServerName = "go-ginx-control.test"
	writeClientJoinServerConfig(t, root, serverCfg)
	defaults, err := config.LoadJoinServiceDefaults(config.JoinServiceDefaultsOptions{Root: root})
	if err != nil {
		t.Fatalf("load join defaults: %v", err)
	}

	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "join.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Users().Create(ctx, domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, domain.Client{ID: "client-1", UserID: "user-1", Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("client-secret")}); err != nil {
		t.Fatalf("create client: %v", err)
	}

	enrollmentService := enrollment.Service{Store: db}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client/enroll" || r.Method != http.MethodPost {
			t.Fatalf("unexpected enrollment request %s %s", r.Method, r.URL.Path)
		}
		var request enrollment.RedeemRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		response, err := enrollmentService.Redeem(r.Context(), request.Token)
		if err != nil {
			http.Error(w, err.Error(), enrollment.HTTPStatusForError(err))
			return
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	payload := enrollment.TokenPayload{
		EnrollmentID:     "join-1",
		Secret:           "join-secret",
		EnrollmentURL:    server.URL + "/api/client/enroll",
		ServerAddress:    defaults.Defaults.ServerAddress,
		ServerTLSAddress: defaults.Defaults.ServerTLSAddress,
		ServerName:       defaults.Defaults.ServerName,
		CAPEM:            "ca-pem",
		ClientID:         "client-1",
		Credential:       "client-secret",
		AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS},
		Reconnect:        config.DefaultClient().Reconnect,
		ExpiresAt:        time.Now().UTC().Add(time.Hour),
	}
	token, err := enrollment.EncodeToken(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ClientEnrollments().Create(ctx, domain.ClientEnrollment{ID: payload.EnrollmentID, ClientID: payload.ClientID, SecretHash: enrollment.HashSecret(payload.Secret), TokenHash: enrollment.HashToken(token), Token: token, ExpiresAt: payload.ExpiresAt}); err != nil {
		t.Fatalf("create enrollment: %v", err)
	}

	cfg, _, err := Join(ctx, token, nil)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	host, _, err := net.SplitHostPort(cfg.ServerAddress)
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	tlsHost, _, err := net.SplitHostPort(cfg.ServerTLSAddress)
	if err != nil {
		t.Fatalf("split tls server address: %v", err)
	}
	if host != serverCfg.JoinServiceHost || tlsHost != serverCfg.JoinServiceHost {
		t.Fatalf("join response host did not match server config: server=%q tls=%q config=%q cfg=%+v", host, tlsHost, serverCfg.JoinServiceHost, cfg)
	}
	if cfg.ServerAddress != "join.example.com:18443" || cfg.ServerTLSAddress != "join.example.com:19443" || cfg.ServerName != serverCfg.ControlTLSServerName {
		t.Fatalf("unexpected joined client config from server defaults: %+v", cfg)
	}
}

func writeClientJoinServerConfig(t *testing.T, root string, cfg config.Server) {
	t.Helper()
	configPath := filepath.Join(root, config.DefaultServerConfigPath)
	content, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
