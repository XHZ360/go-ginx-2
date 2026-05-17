package config

import (
	"crypto/tls"
	"crypto/x509"
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
	content := `{"admin_listen":"127.0.0.1:8080","admin_credentials_file":"admins.json","admin_frontend_dir":"web/admin","control_quic_listen":"127.0.0.1:8443","control_tls_listen":"127.0.0.1:9443","control_tls_cert_file":"control.crt","control_tls_key_file":"control.key","tcp_entry_host":"127.0.0.1","http_entry_listen":"127.0.0.1:8081","sqlite_path":"data/go-ginx.db","data_dir":"data","certificate_dir":"data/certs","heartbeat_timeout":1000000000,"log_retention_days":7}`
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

func TestManagedServerCreatesRuntimeDirectoriesAndControlTLS(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultServer()
	cfg.AdminEnabled = true
	cfg.AdminListen = "127.0.0.1:0"
	cfg.ControlQUICListen = "127.0.0.1:0"
	cfg.ControlTLSListen = "127.0.0.1:0"
	cfg.HTTPEntryListen = "127.0.0.1:0"
	cfg.DataDir = filepath.Join(root, "data")
	cfg.CertificateDir = filepath.Join(root, "data", "certs")
	cfg.SQLitePath = filepath.Join(root, "data", "go-ginx.db")
	cfg.ControlTLSCAFile = filepath.Join(root, "data", "certs", "control-ca.crt")
	cfg.ControlTLSCertFile = filepath.Join(root, "data", "certs", "control.crt")
	cfg.ControlTLSKeyFile = filepath.Join(root, "data", "certs", "control.key")
	cfg.ControlTLSServerName = "go-ginx-control.test"

	if err := PrepareManagedServer(&cfg); err != nil {
		t.Fatalf("prepare managed server: %v", err)
	}
	for _, path := range []string{cfg.DataDir, cfg.CertificateDir, cfg.ControlTLSCAFile, cfg.ControlTLSCertFile, cfg.ControlTLSKeyFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected managed path %s: %v", path, err)
		}
	}
	certInfo, err := os.Stat(cfg.ControlTLSCertFile)
	if err != nil {
		t.Fatal(err)
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
	t.Setenv("GOGINX_CONTROL_QUIC_LISTEN", "127.0.0.1:18443")
	t.Setenv("GOGINX_CONTROL_TLS_LISTEN", "127.0.0.1:19443")
	t.Setenv("GOGINX_HTTP_ENTRY_LISTEN", "127.0.0.1:18081")
	t.Setenv("GOGINX_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("GOGINX_SQLITE_PATH", filepath.Join(root, "data", "go-ginx.db"))
	t.Setenv("GOGINX_CERTIFICATE_DIR", filepath.Join(root, "data", "certs"))
	t.Setenv("GOGINX_CONTROL_TLS_CA_FILE", filepath.Join(root, "data", "certs", "control-ca.crt"))
	t.Setenv("GOGINX_CONTROL_TLS_CERT_FILE", filepath.Join(root, "data", "certs", "control.crt"))
	t.Setenv("GOGINX_CONTROL_TLS_KEY_FILE", filepath.Join(root, "data", "certs", "control.key"))

	cfg, err := LoadManagedServer()
	if err != nil {
		t.Fatalf("load managed server: %v", err)
	}
	if !cfg.AdminEnabled || cfg.AdminListen != "127.0.0.1:18080" || cfg.ControlQUICListen != "127.0.0.1:18443" || cfg.ControlTLSListen != "127.0.0.1:19443" {
		t.Fatalf("environment overrides were not applied: %+v", cfg)
	}
	if _, err := os.Stat(cfg.ControlTLSCAFile); err != nil {
		t.Fatalf("expected generated ca file: %v", err)
	}
}
