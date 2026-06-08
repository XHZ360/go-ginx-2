package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerValidateRequiresSQLitePath(t *testing.T) {
	cfg := DefaultServer()
	cfg.SQLitePath = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServerValidateRequiresRuntimeTLSFiles(t *testing.T) {
	cfg := DefaultServer()
	cfg.ControlTLSCertFile = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing cert file validation error")
	}
}

func TestServerValidateRequiresHTTPEntryListen(t *testing.T) {
	cfg := DefaultServer()
	cfg.HTTPEntryListen = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing HTTP entry listen validation error")
	}
}

func TestServerValidateRequiresClientEnrollmentListen(t *testing.T) {
	cfg := DefaultServer()
	cfg.ClientEnrollmentListen = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing client enrollment listen validation error")
	}
}

func TestDefaultServerUsesSeparatedEnrollmentAndWebEntryPorts(t *testing.T) {
	cfg := DefaultServer()

	if cfg.ClientEnrollmentListen != ":8081" || cfg.HTTPEntryListen != ":80" || cfg.HTTPSEntryListen != ":443" {
		t.Fatalf("unexpected default listener ports: %+v", cfg)
	}
	if cfg.AdminJWTSecretFile != "data/admin-jwt.key" {
		t.Fatalf("unexpected admin jwt secret file %q", cfg.AdminJWTSecretFile)
	}
	if cfg.LogRotation() != DefaultLogRotation() {
		t.Fatalf("unexpected default log rotation: %+v", cfg.LogRotation())
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate default server: %v", err)
	}
}

func TestDefaultClientIncludesLogRotationDefaults(t *testing.T) {
	cfg := DefaultClient()

	if cfg.LogRotation() != DefaultLogRotation() {
		t.Fatalf("unexpected default client log rotation: %+v", cfg.LogRotation())
	}
}

func TestServerValidateRequiresAdminJWTSecretWhenAdminEnabled(t *testing.T) {
	cfg := DefaultServer()
	cfg.AdminEnabled = true
	cfg.AdminJWTSecretFile = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing admin jwt secret validation error")
	}
}

func TestRuntimeListenerClaimsIncludeEnrollmentAndDefaultWebEntries(t *testing.T) {
	claims, err := DefaultServer().RuntimeListenerClaims(true)
	if err != nil {
		t.Fatalf("runtime listener claims: %v", err)
	}
	bySource := make(map[string]domainClaim)
	byEndpoint := make(map[domainClaim]string)
	for _, claim := range claims {
		endpoint := domainClaim{network: claim.Network, port: claim.Port}
		if previous, ok := byEndpoint[endpoint]; ok {
			t.Fatalf("duplicate listener claim endpoint %+v from %s and %s", endpoint, previous, claim.Source)
		}
		byEndpoint[endpoint] = claim.Source
		bySource[claim.Source] = endpoint
	}
	for source, expected := range map[string]domainClaim{
		"client_enrollment_listen": {network: "tcp", port: 8081},
		"http_entry_listen":        {network: "tcp", port: 80},
		"https_entry_listen":       {network: "tcp", port: 443},
	} {
		if bySource[source] != expected {
			t.Fatalf("unexpected claim for %s: got %+v want %+v from %+v", source, bySource[source], expected, claims)
		}
	}
}

type domainClaim struct {
	network string
	port    int
}

func TestServerValidateRequiresACMEFieldsWhenEnabled(t *testing.T) {
	cfg := DefaultServer()
	cfg.ACMEEnabled = true
	cfg.ACMEAccountEmail = ""
	cfg.ACMETermsAccepted = true

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing ACME account email validation error")
	}
}

func TestServerValidateAcceptsConfiguredACME(t *testing.T) {
	cfg := DefaultServer()
	cfg.ACMEEnabled = true
	cfg.ACMEAccountEmail = "ops@example.com"
	cfg.ACMETermsAccepted = true

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate ACME server config: %v", err)
	}
}

func TestLoadServerAcceptsAdminFrontendDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	content := `{"admin_listen":"127.0.0.1:8080","admin_credentials_file":"admins.json","admin_frontend_dir":"web/admin","client_enrollment_listen":"127.0.0.1:18081","control_quic_listen":"127.0.0.1:8443","control_tls_listen":"127.0.0.1:9443","control_tls_cert_file":"control.crt","control_tls_key_file":"control.key","join_service_host":"server.example.com","tcp_entry_host":"127.0.0.1","http_entry_listen":"127.0.0.1:8081","sqlite_path":"data/go-ginx.db","data_dir":"data","certificate_dir":"data/certs","heartbeat_timeout":1000000000,"log_max_size_mb":25,"log_max_backups":4,"log_retention_days":9,"log_compress":false}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadServer(path)
	if err != nil {
		t.Fatalf("load server with admin frontend dir: %v", err)
	}
	if cfg.AdminFrontendDir != "web/admin" {
		t.Fatalf("unexpected admin frontend dir %q", cfg.AdminFrontendDir)
	}
	if cfg.JoinServiceHost != "server.example.com" {
		t.Fatalf("unexpected join service host %q", cfg.JoinServiceHost)
	}
	if cfg.ClientEnrollmentListen != "127.0.0.1:18081" {
		t.Fatalf("unexpected client enrollment listen %q", cfg.ClientEnrollmentListen)
	}
	if cfg.LogRotation() != (LogRotation{MaxSizeMB: 25, MaxBackups: 4, RetentionDays: 9, Compress: false}) {
		t.Fatalf("unexpected log rotation config: %+v", cfg.LogRotation())
	}
}

func TestServerValidateRejectsInvalidLogRotation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Server)
	}{
		{name: "max size", mutate: func(cfg *Server) { cfg.LogMaxSizeMB = 0 }},
		{name: "max backups", mutate: func(cfg *Server) { cfg.LogMaxBackups = -1 }},
		{name: "retention", mutate: func(cfg *Server) { cfg.LogRetentionDays = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultServer()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected invalid log rotation validation error")
			}
		})
	}
}

func TestConfirmJoinServiceDefaultsUsesExplicitHost(t *testing.T) {
	cfg := DefaultServer()
	cfg.JoinServiceHost = "server.example.com"

	defaults, err := ConfirmJoinServiceDefaults(cfg)
	if err != nil {
		t.Fatalf("confirm join service defaults: %v", err)
	}
	if defaults.Host != "server.example.com" || defaults.Source != "join_service_host" {
		t.Fatalf("unexpected host defaults: %+v", defaults)
	}
	if defaults.ServerAddress != "server.example.com:8443" || defaults.ServerTLSAddress != "server.example.com:9443" {
		t.Fatalf("unexpected join addresses: %+v", defaults)
	}
	if defaults.EnrollmentURL != "http://server.example.com:8081/api/client/enroll" {
		t.Fatalf("unexpected enrollment url %q", defaults.EnrollmentURL)
	}
	if defaults.LegacyAdminEnrollmentURL != "http://server.example.com:8080/api/client/enroll" {
		t.Fatalf("unexpected legacy admin enrollment url %q", defaults.LegacyAdminEnrollmentURL)
	}
}

func TestConfirmJoinServiceDefaultsRejectsInvalidExplicitHost(t *testing.T) {
	cfg := DefaultServer()
	cfg.JoinServiceHost = "http://server.example.com:8443/path"

	if _, err := ConfirmJoinServiceDefaults(cfg); err == nil {
		t.Fatal("expected invalid join_service_host error")
	}
}

func TestConfirmJoinServiceDefaultsInfersConfiguredHost(t *testing.T) {
	cfg := DefaultServer()
	cfg.ControlQUICListen = "control.example.com:18443"
	cfg.ControlTLSListen = "control.example.com:19443"
	cfg.AdminListen = "127.0.0.1:18080"

	defaults, err := ConfirmJoinServiceDefaults(cfg)
	if err != nil {
		t.Fatalf("confirm join service defaults: %v", err)
	}
	if defaults.Host != "control.example.com" || defaults.Source != "control_quic_listen" {
		t.Fatalf("unexpected inferred defaults: %+v", defaults)
	}
	if defaults.ServerAddress != "control.example.com:18443" || defaults.ServerTLSAddress != "control.example.com:19443" {
		t.Fatalf("unexpected inferred addresses: %+v", defaults)
	}
}

func TestLoadJoinServiceDefaultsUsesExplicitServerConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, DefaultServerConfigPath)
	cfg := DefaultServer()
	cfg.JoinServiceHost = "server.example.com"
	cfg.ControlQUICListen = ":18443"
	cfg.ControlTLSListen = ":19443"
	cfg.AdminListen = ":18080"
	cfg.ClientEnrollmentListen = ":18081"
	cfg.ControlTLSServerName = "control.example.test"
	cfg.ControlTLSCAFile = "data/certs/custom-ca.crt"
	writeServerConfig(t, configPath, cfg)

	result, err := LoadJoinServiceDefaults(JoinServiceDefaultsOptions{Root: root, ServerConfigPath: DefaultServerConfigPath})
	if err != nil {
		t.Fatalf("load join defaults from explicit server config: %v", err)
	}
	if result.Source != "server_config" || result.ConfigPath != configPath {
		t.Fatalf("unexpected source: %+v", result)
	}
	if result.Defaults.ServerAddress != "server.example.com:18443" || result.Defaults.ServerTLSAddress != "server.example.com:19443" {
		t.Fatalf("unexpected join defaults: %+v", result.Defaults)
	}
	if result.Defaults.EnrollmentURL != "http://server.example.com:18081/api/client/enroll" || result.Defaults.ServerName != "control.example.test" {
		t.Fatalf("unexpected enrollment or server name: %+v", result.Defaults)
	}
	if result.Defaults.ServerCAFile != filepath.Join(root, "data", "certs", "custom-ca.crt") || result.Server.SQLitePath != filepath.Join(root, "data", "go-ginx.db") {
		t.Fatalf("expected deployment-root paths, got defaults=%+v server=%+v", result.Defaults, result.Server)
	}
}

func TestLoadJoinServiceDefaultsUsesDeploymentServerConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, DefaultServerConfigPath)
	cfg := DefaultServer()
	cfg.JoinServiceHost = "deploy.example.com"
	cfg.ControlQUICListen = ":28443"
	cfg.ControlTLSListen = ":29443"
	cfg.AdminListen = ":28080"
	cfg.ClientEnrollmentListen = ":28081"
	writeServerConfig(t, configPath, cfg)

	result, err := LoadJoinServiceDefaults(JoinServiceDefaultsOptions{Root: root})
	if err != nil {
		t.Fatalf("load join defaults from deployment server config: %v", err)
	}
	if result.Source != "deployment_server_config" || result.ConfigPath != configPath {
		t.Fatalf("unexpected source: %+v", result)
	}
	if result.Defaults.ServerAddress != "deploy.example.com:28443" || result.Defaults.ServerTLSAddress != "deploy.example.com:29443" || result.Defaults.EnrollmentURL != "http://deploy.example.com:28081/api/client/enroll" {
		t.Fatalf("unexpected deployment defaults: %+v", result.Defaults)
	}
}

func TestLoadJoinServiceDefaultsUsesEnvironmentOverrides(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOGINX_JOIN_SERVICE_HOST", "join.example.com")
	t.Setenv("GOGINX_ADMIN_LISTEN", ":38080")
	t.Setenv("GOGINX_CLIENT_ENROLLMENT_LISTEN", ":38081")
	t.Setenv("GOGINX_CONTROL_QUIC_LISTEN", ":38443")
	t.Setenv("GOGINX_CONTROL_TLS_LISTEN", ":39443")
	t.Setenv("GOGINX_CONTROL_TLS_SERVER_NAME", "join-control.example.com")
	t.Setenv("GOGINX_CONTROL_TLS_CA_FILE", "data/certs/env-ca.crt")
	t.Setenv("GOGINX_SQLITE_PATH", "data/custom.db")

	result, err := LoadJoinServiceDefaults(JoinServiceDefaultsOptions{Root: root})
	if err != nil {
		t.Fatalf("load join defaults from env: %v", err)
	}
	if result.Source != "managed_defaults" {
		t.Fatalf("unexpected source: %+v", result)
	}
	if result.Defaults.ServerAddress != "join.example.com:38443" || result.Defaults.ServerTLSAddress != "join.example.com:39443" || result.Defaults.EnrollmentURL != "http://join.example.com:38081/api/client/enroll" {
		t.Fatalf("unexpected env defaults: %+v", result.Defaults)
	}
	if result.Defaults.ServerName != "join-control.example.com" || result.Defaults.ServerCAFile != filepath.Join(root, "data", "certs", "env-ca.crt") || result.Server.SQLitePath != filepath.Join(root, "data", "custom.db") {
		t.Fatalf("expected env defaults and paths, got defaults=%+v server=%+v", result.Defaults, result.Server)
	}
}

func TestLoadJoinServiceDefaultsRejectsInvalidEnvironmentHost(t *testing.T) {
	t.Setenv("GOGINX_JOIN_SERVICE_HOST", "http://server.example.com:8443/path")

	if _, err := LoadJoinServiceDefaults(JoinServiceDefaultsOptions{Root: t.TempDir()}); err == nil {
		t.Fatal("expected invalid environment join service host error")
	}
}

func TestLoadJoinServiceDefaultsKeepsLocalFallback(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOGINX_CONTROL_QUIC_LISTEN", "127.0.0.1:18443")
	t.Setenv("GOGINX_CONTROL_TLS_LISTEN", "")
	t.Setenv("GOGINX_ADMIN_LISTEN", "127.0.0.1:18080")
	t.Setenv("GOGINX_CLIENT_ENROLLMENT_LISTEN", "127.0.0.1:18081")

	result, err := LoadJoinServiceDefaults(JoinServiceDefaultsOptions{Root: root})
	if err != nil {
		t.Fatalf("load local join defaults: %v", err)
	}
	if result.Defaults.Source != "control_quic_listen" || result.Defaults.ServerAddress != "127.0.0.1:18443" || result.Defaults.EnrollmentURL != "http://127.0.0.1:18081/api/client/enroll" {
		t.Fatalf("unexpected local defaults: %+v", result.Defaults)
	}
}

func TestClientValidateRequiresStrictServerIdentity(t *testing.T) {
	cfg := DefaultClient()
	cfg.ServerAddress = "127.0.0.1:8443"
	cfg.ClientID = "client-1"
	cfg.Credential = "secret"
	cfg.ServerCAFile = "ca.pem"

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing server_name validation error")
	}
}

func TestClientValidateRequiresServerCAFile(t *testing.T) {
	cfg := DefaultClient()
	cfg.ServerAddress = "127.0.0.1:8443"
	cfg.ServerName = "localhost"
	cfg.ClientID = "client-1"
	cfg.Credential = "secret"

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing server ca file validation error")
	}
}

func TestLoadClientRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	content := `{"server_address":"example.com:8443","server_name":"example.com","client_id":"client-1","credential":"secret","unknown":true}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadClient(path); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestLoadClientAcceptsLogRotationConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	content := `{"server_address":"example.com:8443","server_name":"example.com","server_ca_file":"ca.pem","client_id":"client-1","credential":"secret","allowed_protocols":["quic"],"reconnect":{"initial_delay":1000000000,"max_delay":30000000000},"log_max_size_mb":12,"log_max_backups":3,"log_retention_days":5,"log_compress":false}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadClient(path)
	if err != nil {
		t.Fatalf("load client with log rotation config: %v", err)
	}
	if cfg.LogRotation() != (LogRotation{MaxSizeMB: 12, MaxBackups: 3, RetentionDays: 5, Compress: false}) {
		t.Fatalf("unexpected log rotation config: %+v", cfg.LogRotation())
	}
}

func TestClientValidateRejectsInvalidLogRotation(t *testing.T) {
	cfg := DefaultClient()
	cfg.ServerAddress = "127.0.0.1:8443"
	cfg.ServerName = "localhost"
	cfg.ServerCAFile = "ca.pem"
	cfg.ClientID = "client-1"
	cfg.Credential = "secret"
	cfg.LogMaxSizeMB = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid log rotation validation error")
	}
}

func TestManagedServerCreatesRuntimeDirectoriesAndControlTLS(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultServer()
	cfg.AdminEnabled = true
	cfg.AdminListen = "127.0.0.1:0"
	cfg.ControlQUICListen = "127.0.0.1:0"
	cfg.ControlTLSListen = "127.0.0.1:0"
	cfg.ClientEnrollmentListen = "127.0.0.1:0"
	cfg.HTTPEntryListen = "127.0.0.1:0"
	cfg.HTTPSEntryListen = "127.0.0.1:0"
	cfg.DataDir = filepath.Join(root, "data")
	cfg.CertificateDir = filepath.Join(root, "data", "certs")
	cfg.SQLitePath = filepath.Join(root, "data", "go-ginx.db")
	cfg.ControlTLSCAFile = filepath.Join(root, "data", "certs", "control-ca.crt")
	cfg.ControlTLSCertFile = filepath.Join(root, "data", "certs", "control.crt")
	cfg.ControlTLSKeyFile = filepath.Join(root, "data", "certs", "control.key")
	cfg.ControlTLSServerName = "go-ginx-control.test"
	cfg.AdminJWTSecretFile = filepath.Join(root, "data", "admin-jwt.key")

	if err := PrepareManagedServer(&cfg); err != nil {
		t.Fatalf("prepare managed server: %v", err)
	}
	for _, path := range []string{cfg.DataDir, cfg.CertificateDir, cfg.ControlTLSCAFile, cfg.ControlTLSCertFile, cfg.ControlTLSKeyFile, cfg.AdminJWTSecretFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected managed path %s: %v", path, err)
		}
	}
	certInfo, err := os.Stat(cfg.ControlTLSCertFile)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := LoadAdminJWTSecret(cfg.AdminJWTSecretFile)
	if err != nil {
		t.Fatalf("load generated admin jwt secret: %v", err)
	}
	if err := PrepareManagedServer(&cfg); err != nil {
		t.Fatalf("prepare managed server again: %v", err)
	}
	reusedInfo, err := os.Stat(cfg.ControlTLSCertFile)
	if err != nil {
		t.Fatal(err)
	}
	if !reusedInfo.ModTime().Equal(certInfo.ModTime()) {
		t.Fatal("expected existing control certificate to be reused")
	}
	reusedSecret, err := LoadAdminJWTSecret(cfg.AdminJWTSecretFile)
	if err != nil {
		t.Fatalf("load reused admin jwt secret: %v", err)
	}
	if string(reusedSecret) != string(secret) {
		t.Fatal("expected existing admin jwt secret to be reused")
	}

	cert, err := tls.LoadX509KeyPair(cfg.ControlTLSCertFile, cfg.ControlTLSKeyFile)
	if err != nil {
		t.Fatalf("load generated control cert: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse generated control cert: %v", err)
	}
	caPEM, err := os.ReadFile(cfg.ControlTLSCAFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("expected generated CA PEM")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: cfg.ControlTLSServerName, Roots: pool, CurrentTime: time.Now().UTC()}); err != nil {
		t.Fatalf("verify generated control cert: %v", err)
	}
}

func TestManagedServerRejectsInvalidAdminJWTSecret(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultServer()
	cfg.DataDir = filepath.Join(root, "data")
	cfg.CertificateDir = filepath.Join(root, "data", "certs")
	cfg.AdminJWTSecretFile = filepath.Join(root, "data", "admin-jwt.key")
	if err := os.MkdirAll(filepath.Dir(cfg.AdminJWTSecretFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.AdminJWTSecretFile, []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := PrepareManagedServer(&cfg); err == nil {
		t.Fatal("expected invalid admin jwt secret error")
	}
}

func TestManagedClientStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client-state.json")
	cfg := DefaultClient()
	cfg.ServerAddress = "127.0.0.1:8443"
	cfg.ServerName = "go-ginx-control.test"
	cfg.ServerCAFile = "data/certs/server-ca.crt"
	cfg.ClientID = "client-1"
	cfg.Credential = "secret"

	if err := SaveManagedClient(cfg, path); err != nil {
		t.Fatalf("save managed client: %v", err)
	}
	loaded, err := LoadClient(path)
	if err != nil {
		t.Fatalf("load managed client state: %v", err)
	}
	if loaded.ClientID != cfg.ClientID || loaded.ServerName != cfg.ServerName {
		t.Fatalf("unexpected managed client state: %+v", loaded)
	}
}

func TestLoadManagedServerAppliesEnvironmentOverrides(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOGINX_ADMIN_LISTEN", "127.0.0.1:18080")
	t.Setenv("GOGINX_CLIENT_ENROLLMENT_LISTEN", "127.0.0.1:18082")
	t.Setenv("GOGINX_CONTROL_QUIC_LISTEN", "127.0.0.1:18443")
	t.Setenv("GOGINX_CONTROL_TLS_LISTEN", "127.0.0.1:19443")
	t.Setenv("GOGINX_HTTP_ENTRY_LISTEN", "127.0.0.1:18081")
	t.Setenv("GOGINX_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("GOGINX_SQLITE_PATH", filepath.Join(root, "data", "go-ginx.db"))
	t.Setenv("GOGINX_CERTIFICATE_DIR", filepath.Join(root, "data", "certs"))
	t.Setenv("GOGINX_ADMIN_JWT_SECRET_FILE", filepath.Join(root, "data", "admin-jwt.key"))
	t.Setenv("GOGINX_CONTROL_TLS_CA_FILE", filepath.Join(root, "data", "certs", "control-ca.crt"))
	t.Setenv("GOGINX_CONTROL_TLS_CERT_FILE", filepath.Join(root, "data", "certs", "control.crt"))
	t.Setenv("GOGINX_CONTROL_TLS_KEY_FILE", filepath.Join(root, "data", "certs", "control.key"))
	t.Setenv("GOGINX_JOIN_SERVICE_HOST", "join.example.com")

	cfg, err := LoadManagedServer()
	if err != nil {
		t.Fatalf("load managed server: %v", err)
	}
	if !cfg.AdminEnabled || cfg.AdminListen != "127.0.0.1:18080" || cfg.ControlQUICListen != "127.0.0.1:18443" || cfg.ControlTLSListen != "127.0.0.1:19443" {
		t.Fatalf("environment overrides were not applied: %+v", cfg)
	}
	if cfg.ClientEnrollmentListen != "127.0.0.1:18082" || cfg.HTTPEntryListen != "127.0.0.1:18081" {
		t.Fatalf("listener environment overrides were not applied: %+v", cfg)
	}
	if cfg.JoinServiceHost != "join.example.com" {
		t.Fatalf("join service environment override was not applied: %+v", cfg)
	}
	if cfg.AdminJWTSecretFile != filepath.Join(root, "data", "admin-jwt.key") {
		t.Fatalf("admin jwt secret environment override was not applied: %+v", cfg)
	}
	if _, err := os.Stat(cfg.ControlTLSCAFile); err != nil {
		t.Fatalf("expected generated ca file: %v", err)
	}
	if _, err := LoadAdminJWTSecret(cfg.AdminJWTSecretFile); err != nil {
		t.Fatalf("expected generated admin jwt secret: %v", err)
	}
}

func writeServerConfig(t *testing.T, path string, cfg Server) {
	t.Helper()
	content, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
