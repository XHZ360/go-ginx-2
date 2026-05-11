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
