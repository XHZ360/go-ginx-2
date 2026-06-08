package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/winservice"
)

func TestLoadServerConfigDefaultsToDeploymentRoot(t *testing.T) {
	deploymentRoot := t.TempDir()
	stateDir := t.TempDir()
	setServerExecutable(t, deploymentRoot)
	t.Chdir(stateDir)

	cfg, err := loadServerConfig("")
	if err != nil {
		t.Fatalf("load managed server config: %v", err)
	}

	wantDataDir := filepath.Join(deploymentRoot, "data")
	if cfg.DataDir != wantDataDir || cfg.SQLitePath != filepath.Join(deploymentRoot, "data", "go-ginx.db") {
		t.Fatalf("expected deployment-root data paths, got data_dir=%q sqlite_path=%q", cfg.DataDir, cfg.SQLitePath)
	}
	for _, path := range []string{
		cfg.ControlTLSCAFile,
		cfg.ControlTLSCertFile,
		cfg.ControlTLSKeyFile,
		cfg.AdminJWTSecretFile,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated control tls file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "data")); !os.IsNotExist(err) {
		t.Fatalf("expected no cwd-relative managed data dir, got err=%v", err)
	}
}

func TestLoadServerConfigResolvesRelativeConfigAndPathsFromDeploymentRoot(t *testing.T) {
	deploymentRoot := t.TempDir()
	stateDir := t.TempDir()
	setServerExecutable(t, deploymentRoot)
	t.Chdir(stateDir)
	configPath := filepath.Join(deploymentRoot, "config", "server.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultServer()
	cfg.ControlTLSCAFile = "data/certs/custom-ca.crt"
	cfg.ControlTLSCertFile = "data/certs/custom.crt"
	cfg.ControlTLSKeyFile = "data/certs/custom.key"
	content, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadServerConfig(filepath.Join("config", "server.json"))
	if err != nil {
		t.Fatalf("load server config: %v", err)
	}

	if loaded.SQLitePath != filepath.Join(deploymentRoot, "data", "go-ginx.db") {
		t.Fatalf("expected deployment-root sqlite path, got %q", loaded.SQLitePath)
	}
	if loaded.ControlTLSCertFile != filepath.Join(deploymentRoot, "data", "certs", "custom.crt") {
		t.Fatalf("expected deployment-root control cert path, got %q", loaded.ControlTLSCertFile)
	}
	if loaded.AdminJWTSecretFile != filepath.Join(deploymentRoot, "data", "admin-jwt.key") {
		t.Fatalf("expected deployment-root admin jwt secret path, got %q", loaded.AdminJWTSecretFile)
	}
}

func TestRunServiceCommandUsesServerDefaults(t *testing.T) {
	oldRun := runWindowsServiceCommand
	var gotArgs []string
	var gotOptions winservice.Options
	runWindowsServiceCommand = func(args []string, options winservice.Options) error {
		gotArgs = append([]string(nil), args...)
		gotOptions = options
		return nil
	}
	t.Cleanup(func() { runWindowsServiceCommand = oldRun })

	if err := runServiceCommand([]string{"status"}); err != nil {
		t.Fatalf("run service command: %v", err)
	}

	if len(gotArgs) != 1 || gotArgs[0] != "status" {
		t.Fatalf("unexpected args: %+v", gotArgs)
	}
	if gotOptions.Definition.DefaultName != "goginx-server" || gotOptions.Definition.DisplayName != "go-ginx server" {
		t.Fatalf("unexpected service definition: %+v", gotOptions.Definition)
	}
	if gotOptions.Runner == nil || gotOptions.ValidateInstall == nil || gotOptions.ExecutablePath == nil || gotOptions.Stdout == nil {
		t.Fatalf("expected service options to be populated: %+v", gotOptions)
	}
}

func TestRunServerReturnsConfigError(t *testing.T) {
	err := runServer(context.Background(), filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected config error")
	}
}

func setServerExecutable(t *testing.T, deploymentRoot string) {
	t.Helper()
	previous := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(deploymentRoot, "bin", "goginx-server"), nil
	}
	t.Cleanup(func() {
		executablePath = previous
	})
}
