package config

import (
	"os"
	"path/filepath"
	"testing"
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
