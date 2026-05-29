package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestRunCreatesResources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin.db")

	if err := run([]string{"create-user", "-db", dbPath, "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client", "-db", dbPath, "-id", "client-1", "-user", "user-1", "-name", "home", "-credential", "secret"}); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := run([]string{"create-tcp-proxy", "-db", dbPath, "-id", "proxy-1", "-user", "user-1", "-client", "client-1", "-name", "ssh", "-port", "10022", "-target-host", "127.0.0.1", "-target-port", "22"}); err != nil {
		t.Fatalf("create tcp proxy: %v", err)
	}
	if err := run([]string{"create-udp-proxy", "-db", dbPath, "-id", "udp-1", "-user", "user-1", "-client", "client-1", "-name", "dns", "-port", "10053", "-target-host", "127.0.0.1", "-target-port", "53"}); err != nil {
		t.Fatalf("create udp proxy: %v", err)
	}
	if err := run([]string{"create-https-proxy", "-db", dbPath, "-id", "https-1", "-user", "user-1", "-client", "client-1", "-name", "secure", "-host", "secure.example.com", "-target-host", "127.0.0.1", "-target-port", "8443"}); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	found, err := db.Proxies().ByTCPEntryPort(context.Background(), 10022)
	if err != nil {
		t.Fatalf("lookup tcp proxy: %v", err)
	}
	if found.ID != "proxy-1" {
		t.Fatalf("unexpected proxy: %+v", found)
	}
	foundUDP, err := db.Proxies().ByUDPEntryPort(context.Background(), 10053)
	if err != nil {
		t.Fatalf("lookup udp proxy: %v", err)
	}
	if foundUDP.ID != "udp-1" {
		t.Fatalf("unexpected udp proxy: %+v", foundUDP)
	}
	foundHTTPS, err := db.Proxies().ByHTTPSHost(context.Background(), "secure.example.com")
	if err != nil {
		t.Fatalf("lookup https proxy: %v", err)
	}
	if foundHTTPS.ID != "https-1" {
		t.Fatalf("unexpected https proxy: %+v", foundHTTPS)
	}
}

func TestRunInitializesAdminUser(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin.db")
	if err := run([]string{"init-admin", "-db", dbPath, "-id", "admin-1", "-username", "admin", "-password", "secret"}); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Users().SetStatus(context.Background(), "admin-1", domain.UserDisabled); err != nil {
		t.Fatalf("disable admin before reinit: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db before reinit: %v", err)
	}

	if err := run([]string{"init-admin", "-db", dbPath, "-username", "admin", "-password", "updated-secret"}); err != nil {
		t.Fatalf("reinit admin: %v", err)
	}
	db, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	user, err := db.Users().ByID(context.Background(), "admin-1")
	if err != nil {
		t.Fatalf("lookup admin: %v", err)
	}
	if user.Role != "admin" || user.Status != "enabled" || !domain.CheckPasswordHash("updated-secret", user.PasswordHash) {
		t.Fatalf("unexpected admin user: %+v", user)
	}
}

func TestRunInitializesAdminUserDefaultDBFromDeploymentRoot(t *testing.T) {
	deploymentRoot := t.TempDir()
	stateDir := t.TempDir()
	previousExecutablePath := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(deploymentRoot, "bin", "goginx-admin"), nil
	}
	t.Cleanup(func() {
		executablePath = previousExecutablePath
	})
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(stateDir); err != nil {
		t.Fatalf("chdir state dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(workingDir)
	})

	if err := run([]string{"init-admin", "-id", "admin-1", "-username", "admin", "-password", "secret"}); err != nil {
		t.Fatalf("init admin with default db: %v", err)
	}

	dbPath := filepath.Join(deploymentRoot, "data", "go-ginx.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected default deployment db %s: %v", dbPath, err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "data", "go-ginx.db")); !os.IsNotExist(err) {
		t.Fatalf("expected no cwd-relative db, got err=%v", err)
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	user, err := db.Users().ByID(context.Background(), "admin-1")
	if err != nil {
		t.Fatalf("lookup admin: %v", err)
	}
	if user.Username != "admin" || user.Role != domain.RoleAdmin {
		t.Fatalf("unexpected admin user: %+v", user)
	}
}

func TestRunCreatesClientJoinToken(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin.db")
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, []byte("ca-pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"create-user", "-db", dbPath, "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client-join", "-db", dbPath, "-id", "client-1", "-user", "user-1", "-name", "home", "-server-ca-file", caFile, "-server-name", "go-ginx-control.test", "-server-address", "127.0.0.1:8443", "-enrollment-url", "http://127.0.0.1:8080/api/client/enroll"}); err != nil {
		t.Fatalf("create client join: %v", err)
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Clients().ByID(context.Background(), "client-1"); err != nil {
		t.Fatalf("lookup client: %v", err)
	}
	events, err := db.AuditEvents().ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Action == "create_client_join" && event.ResourceID == "client-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected create_client_join audit event, got %+v", events)
	}
}

func TestRunCreateClientJoinDefaultsCAFileFromDeploymentRoot(t *testing.T) {
	deploymentRoot := t.TempDir()
	stateDir := t.TempDir()
	previousExecutablePath := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(deploymentRoot, "bin", "goginx-admin"), nil
	}
	t.Cleanup(func() {
		executablePath = previousExecutablePath
	})
	t.Chdir(stateDir)
	if err := os.MkdirAll(filepath.Join(deploymentRoot, "data", "certs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deploymentRoot, "data", "certs", "control-ca.crt"), []byte("ca-pem"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(deploymentRoot, "data", "go-ginx.db")
	if err := run([]string{"create-user", "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client-join", "-id", "client-1", "-user", "user-1", "-name", "home", "-server-name", "go-ginx-control.test", "-server-address", "127.0.0.1:8443", "-enrollment-url", "http://127.0.0.1:8080/api/client/enroll"}); err != nil {
		t.Fatalf("create client join: %v", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected deployment-root db: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "data", "go-ginx.db")); !os.IsNotExist(err) {
		t.Fatalf("expected no cwd-relative db, got err=%v", err)
	}
}

func TestRunManageCertificateDefaultsDirectoryFromDeploymentRoot(t *testing.T) {
	deploymentRoot := t.TempDir()
	stateDir := t.TempDir()
	previousExecutablePath := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(deploymentRoot, "bin", "goginx-admin"), nil
	}
	t.Cleanup(func() {
		executablePath = previousExecutablePath
	})
	t.Chdir(stateDir)
	t.Setenv("CF_DNS_API_TOKEN", "token")
	oldIssuer := newACMEIssuer
	oldProvider := newDNSProvider
	newACMEIssuer = func() certmanager.Issuer { return adminMainFakeIssuer{} }
	newDNSProvider = func(string) (certmanager.DNSChallengeProvider, error) { return adminMainFakeDNSProvider{}, nil }
	t.Cleanup(func() {
		newACMEIssuer = oldIssuer
		newDNSProvider = oldProvider
	})

	if err := run([]string{"create-user", "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client", "-id", "client-1", "-user", "user-1", "-name", "home", "-credential", "secret"}); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := run([]string{"create-https-proxy", "-id", "https-1", "-user", "user-1", "-client", "client-1", "-name", "secure", "-host", "app.example.com", "-target-host", "127.0.0.1", "-target-port", "8080"}); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	if err := run([]string{"issue-managed-certificate", "-proxy", "https-1", "-acme-account-email", "ops@example.com", "-acme-terms-accepted"}); err != nil {
		t.Fatalf("issue managed certificate: %v", err)
	}

	wantCert := filepath.Join(deploymentRoot, "data", "certs", "managed", "app.example.com", "active.crt")
	if _, err := os.Stat(wantCert); err != nil {
		t.Fatalf("expected deployment-root managed certificate %s: %v", wantCert, err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "data", "certs", "managed", "app.example.com", "active.crt")); !os.IsNotExist(err) {
		t.Fatalf("expected no cwd-relative managed certificate, got err=%v", err)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"unknown"}); err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestRunManagesCertificates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin.db")
	t.Setenv("CF_DNS_API_TOKEN", "token")
	oldIssuer := newACMEIssuer
	oldProvider := newDNSProvider
	newACMEIssuer = func() certmanager.Issuer { return adminMainFakeIssuer{} }
	newDNSProvider = func(string) (certmanager.DNSChallengeProvider, error) { return adminMainFakeDNSProvider{}, nil }
	t.Cleanup(func() {
		newACMEIssuer = oldIssuer
		newDNSProvider = oldProvider
	})

	if err := run([]string{"create-user", "-db", dbPath, "-id", "user-1", "-username", "alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := run([]string{"create-client", "-db", dbPath, "-id", "client-1", "-user", "user-1", "-name", "home", "-credential", "secret"}); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := run([]string{"create-https-proxy", "-db", dbPath, "-id", "https-1", "-user", "user-1", "-client", "client-1", "-name", "secure", "-host", "app.example.com", "-target-host", "127.0.0.1", "-target-port", "8080"}); err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	certDir := t.TempDir()
	if err := run([]string{"issue-managed-certificate", "-db", dbPath, "-proxy", "https-1", "-certificate-dir", certDir, "-acme-account-email", "ops@example.com", "-acme-terms-accepted"}); err != nil {
		t.Fatalf("issue managed certificate: %v", err)
	}
	if err := run([]string{"renew-managed-certificate", "-db", dbPath, "-proxy", "https-1", "-certificate-dir", certDir, "-acme-account-email", "ops@example.com", "-acme-terms-accepted"}); err != nil {
		t.Fatalf("renew managed certificate: %v", err)
	}
	if err := run([]string{"managed-certificate-status", "-db", dbPath, "-proxy", "https-1", "-certificate-dir", certDir, "-acme-account-email", "ops@example.com", "-acme-terms-accepted"}); err != nil {
		t.Fatalf("certificate status: %v", err)
	}

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	certificate, err := db.Certificates().ByProxyID(context.Background(), "https-1")
	if err != nil {
		t.Fatalf("lookup certificate: %v", err)
	}
	if certificate.CertFile == "" || certificate.KeyFile == "" || certificate.Status == "" {
		t.Fatalf("unexpected certificate metadata: %+v", certificate)
	}
}

func TestRunBuildsDeployBundle(t *testing.T) {
	repoRoot := adminMainTestBundleRepoRoot(t, true)
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	defer func() {
		_ = os.Chdir(workingDir)
	}()
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := run([]string{"build-deploy-bundle", "-output", outputDir, "-goos", "linux", "-goarch", runtime.GOARCH, "-install-root", "/opt/go-ginx"}); err != nil {
		t.Fatalf("build deploy bundle: %v", err)
	}
	for _, path := range []string{
		filepath.Join(outputDir, "bin", "goginx-server"),
		filepath.Join(outputDir, "bin", "goginx-client"),
		filepath.Join(outputDir, "bin", "goginx-admin"),
		filepath.Join(outputDir, "config", "server.json"),
		filepath.Join(outputDir, "config", "client.json"),
		filepath.Join(outputDir, "config", "admin-credentials.json.example"),
		filepath.Join(outputDir, "config", "goginx-server.env.example"),
		filepath.Join(outputDir, "config", "goginx-client.env.example"),
		filepath.Join(outputDir, "systemd", "goginx-server.service"),
		filepath.Join(outputDir, "systemd", "goginx-client.service"),
		filepath.Join(outputDir, "data", "certs", "managed"),
		filepath.Join(outputDir, "logs"),
		filepath.Join(outputDir, "admin-ui", "index.html"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	serverConfig := readBundleServerConfig(t, filepath.Join(outputDir, "config", "server.json"))
	if !serverConfig.AdminEnabled {
		t.Fatal("expected bundled server config to enable admin")
	}
	if serverConfig.ControlTLSCAFile != "data/certs/control-ca.crt" || serverConfig.ControlTLSCertFile != "data/certs/control.crt" || serverConfig.ControlTLSKeyFile != "data/certs/control.key" {
		t.Fatalf("unexpected server control TLS paths: %+v", serverConfig)
	}
	if serverConfig.AdminFrontendDir != "" {
		t.Fatalf("expected empty admin_frontend_dir for default admin-ui directory, got %q", serverConfig.AdminFrontendDir)
	}
	clientConfig := readBundleClientConfig(t, filepath.Join(outputDir, "config", "client.json"))
	if clientConfig.ServerName != "go-ginx-control.local" || clientConfig.ServerCAFile != "data/certs/server-ca.crt" {
		t.Fatalf("unexpected client trust config: %+v", clientConfig)
	}
}

func TestRunBuildsWindowsDeployBundle(t *testing.T) {
	repoRoot := adminMainTestBundleRepoRoot(t, true)
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	defer func() {
		_ = os.Chdir(workingDir)
	}()
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := run([]string{"build-deploy-bundle", "-output", outputDir, "-goos", "windows", "-goarch", "amd64"}); err != nil {
		t.Fatalf("build windows deploy bundle: %v", err)
	}
	for _, path := range []string{
		filepath.Join(outputDir, "bin", "goginx-server.exe"),
		filepath.Join(outputDir, "bin", "goginx-client.exe"),
		filepath.Join(outputDir, "bin", "goginx-admin.exe"),
		filepath.Join(outputDir, "config", "server.json"),
		filepath.Join(outputDir, "config", "client.json"),
		filepath.Join(outputDir, "data", "certs", "managed"),
		filepath.Join(outputDir, "logs"),
		filepath.Join(outputDir, "admin-ui", "index.html"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "systemd")); !os.IsNotExist(err) {
		t.Fatalf("expected no systemd directory for windows bundle, got err=%v", err)
	}
}

func TestRunBuildDeployBundleRequiresAdminFrontendAssets(t *testing.T) {
	repoRoot := adminMainTestBundleRepoRoot(t, false)
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	defer func() {
		_ = os.Chdir(workingDir)
	}()
	outputDir := filepath.Join(t.TempDir(), "bundle")
	err = run([]string{"build-deploy-bundle", "-output", outputDir, "-goos", runtime.GOOS, "-goarch", runtime.GOARCH, "-install-root", "/opt/go-ginx"})
	if err == nil {
		t.Fatal("expected build deploy bundle to require admin frontend assets")
	}
	if !strings.Contains(err.Error(), "admin frontend build output is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBuildsDeployBundleWithAdminFrontendAssets(t *testing.T) {
	repoRoot := adminMainTestBundleRepoRoot(t, true)
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	defer func() {
		_ = os.Chdir(workingDir)
	}()
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := run([]string{"build-deploy-bundle", "-output", outputDir, "-goos", runtime.GOOS, "-goarch", runtime.GOARCH, "-install-root", "/opt/go-ginx"}); err != nil {
		t.Fatalf("build deploy bundle: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(outputDir, "admin-ui", "index.html"))
	if err != nil {
		t.Fatalf("read bundled admin frontend index: %v", err)
	}
	if string(content) != "<html>admin</html>" {
		t.Fatalf("unexpected bundled admin frontend content %q", string(content))
	}
	assetContent, err := os.ReadFile(filepath.Join(outputDir, "admin-ui", "assets", "app.js"))
	if err != nil {
		t.Fatalf("read bundled admin frontend asset: %v", err)
	}
	if string(assetContent) != "console.log('admin');" {
		t.Fatalf("unexpected bundled admin frontend asset %q", string(assetContent))
	}
	serverConfig := readBundleServerConfig(t, filepath.Join(outputDir, "config", "server.json"))
	if !serverConfig.AdminEnabled {
		t.Fatal("expected bundled server config to enable admin")
	}
	if serverConfig.ControlTLSCAFile != "data/certs/control-ca.crt" || serverConfig.ControlTLSCertFile != "data/certs/control.crt" || serverConfig.ControlTLSKeyFile != "data/certs/control.key" {
		t.Fatalf("unexpected server control TLS paths: %+v", serverConfig)
	}
	if serverConfig.AdminFrontendDir != "" {
		t.Fatalf("expected admin_frontend_dir to remain optional, got %q", serverConfig.AdminFrontendDir)
	}
	clientConfig := readBundleClientConfig(t, filepath.Join(outputDir, "config", "client.json"))
	if clientConfig.ServerName != "go-ginx-control.local" || clientConfig.ServerCAFile != "data/certs/server-ca.crt" {
		t.Fatalf("unexpected client trust config: %+v", clientConfig)
	}
}

func adminMainTestBundleRepoRoot(t *testing.T, includeFrontendDist bool) string {
	t.Helper()
	root := adminMainRepoRoot(t)
	tempRoot := t.TempDir()
	adminMainMustMkdirAll(t, filepath.Join(tempRoot, "cmd", "goginx-server"))
	adminMainMustMkdirAll(t, filepath.Join(tempRoot, "cmd", "goginx-client"))
	adminMainMustMkdirAll(t, filepath.Join(tempRoot, "cmd", "goginx-admin"))
	adminMainMustMkdirAll(t, filepath.Join(tempRoot, "deploy", "systemd"))
	adminMainCopyFile(t, filepath.Join(root, "go.mod"), filepath.Join(tempRoot, "go.mod"))
	adminMainCopyFile(t, filepath.Join(root, "go.sum"), filepath.Join(tempRoot, "go.sum"))
	adminMainCopyFile(t, filepath.Join(root, "cmd", "goginx-server", "main.go"), filepath.Join(tempRoot, "cmd", "goginx-server", "main.go"))
	adminMainCopyFile(t, filepath.Join(root, "cmd", "goginx-client", "main.go"), filepath.Join(tempRoot, "cmd", "goginx-client", "main.go"))
	adminMainCopyFile(t, filepath.Join(root, "cmd", "goginx-admin", "main.go"), filepath.Join(tempRoot, "cmd", "goginx-admin", "main.go"))
	adminMainCopyFile(t, filepath.Join(root, "deploy", "systemd", "goginx-server.service"), filepath.Join(tempRoot, "deploy", "systemd", "goginx-server.service"))
	adminMainCopyFile(t, filepath.Join(root, "deploy", "systemd", "goginx-client.service"), filepath.Join(tempRoot, "deploy", "systemd", "goginx-client.service"))
	if err := os.Symlink(filepath.Join(root, "internal"), filepath.Join(tempRoot, "internal")); err != nil {
		t.Skipf("symlink internal package tree: %v", err)
	}
	if includeFrontendDist {
		adminMainMustMkdirAll(t, filepath.Join(tempRoot, "admin-ui", "dist", "assets"))
		adminMainMustWriteFile(t, filepath.Join(tempRoot, "admin-ui", "dist", "index.html"), []byte("<html>admin</html>"), 0o644)
		adminMainMustWriteFile(t, filepath.Join(tempRoot, "admin-ui", "dist", "assets", "app.js"), []byte("console.log('admin');"), 0o644)
	}
	return tempRoot
}

func adminMainRepoRoot(t *testing.T) string {
	t.Helper()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(workingDir, "..", ".."))
}

func readBundleServerConfig(t *testing.T, path string) bundleServerConfig {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}
	var cfg bundleServerConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("decode server config: %v", err)
	}
	return cfg
}

type bundleServerConfig struct {
	AdminEnabled       bool   `json:"admin_enabled"`
	AdminFrontendDir   string `json:"admin_frontend_dir"`
	ControlTLSCAFile   string `json:"control_tls_ca_file"`
	ControlTLSCertFile string `json:"control_tls_cert_file"`
	ControlTLSKeyFile  string `json:"control_tls_key_file"`
}

func readBundleClientConfig(t *testing.T, path string) bundleClientConfig {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read client config: %v", err)
	}
	var cfg bundleClientConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("decode client config: %v", err)
	}
	return cfg
}

type bundleClientConfig struct {
	ServerName   string `json:"server_name"`
	ServerCAFile string `json:"server_ca_file"`
}

func adminMainCopyFile(t *testing.T, sourcePath string, destPath string) {
	t.Helper()
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat %s: %v", sourcePath, err)
	}
	adminMainMustWriteFile(t, destPath, content, info.Mode().Perm())
}

func adminMainMustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func adminMainMustWriteFile(t *testing.T, path string, content []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, content, perm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type adminMainFakeDNSProvider struct{}

func (adminMainFakeDNSProvider) Present(context.Context, string, string) error { return nil }
func (adminMainFakeDNSProvider) CleanUp(context.Context, string, string) error { return nil }

type adminMainFakeIssuer struct{}

func (adminMainFakeIssuer) Issue(context.Context, certmanager.IssueRequest) (certmanager.IssuedCertificate, error) {
	certPEM, keyPEM, notAfter := adminMainTestCertificatePEM("app.example.com", time.Now().Add(time.Hour))
	return certmanager.IssuedCertificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

func adminMainTestCertificatePEM(host string, notAfter time.Time) ([]byte, []byte, time.Time) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), notAfter
}
