package deploy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildBundleCreatesExpectedLayout(t *testing.T) {
	root := testBundleRepoRoot(t, false)
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := BuildBundle(context.Background(), BundleOptions{RepoRoot: root, OutputDir: outputDir, GoOS: runtime.GOOS, GoArch: runtime.GOARCH, InstallRoot: "/opt/go-ginx"}); err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	for _, path := range []string{
		filepath.Join(outputDir, "bin", binaryName("goginx-server", runtime.GOOS)),
		filepath.Join(outputDir, "bin", binaryName("goginx-client", runtime.GOOS)),
		filepath.Join(outputDir, "bin", binaryName("goginx-admin", runtime.GOOS)),
		filepath.Join(outputDir, "config", "server.json"),
		filepath.Join(outputDir, "config", "client.json"),
		filepath.Join(outputDir, "config", "admin-credentials.json.example"),
		filepath.Join(outputDir, "config", "goginx-server.env.example"),
		filepath.Join(outputDir, "config", "goginx-client.env.example"),
		filepath.Join(outputDir, "systemd", "goginx-server.service"),
		filepath.Join(outputDir, "systemd", "goginx-client.service"),
		filepath.Join(outputDir, "data", "certs", "managed"),
		filepath.Join(outputDir, "logs"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	serverConfig := readBundledServerConfig(t, filepath.Join(outputDir, "config", "server.json"))
	if serverConfig.AdminFrontendDir != "" {
		t.Fatalf("expected empty admin_frontend_dir when frontend dist is missing, got %q", serverConfig.AdminFrontendDir)
	}
	if _, err := os.Stat(filepath.Join(outputDir, bundledAdminFrontendDir)); !os.IsNotExist(err) {
		t.Fatalf("expected bundled admin frontend directory to be absent when dist is missing, got err=%v", err)
	}
}

func TestBuildBundleCopiesAdminFrontendAssetsWhenPresent(t *testing.T) {
	root := testBundleRepoRoot(t, true)
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := BuildBundle(context.Background(), BundleOptions{RepoRoot: root, OutputDir: outputDir, GoOS: runtime.GOOS, GoArch: runtime.GOARCH, InstallRoot: "/opt/go-ginx"}); err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(outputDir, bundledAdminFrontendDir, "index.html"))
	if err != nil {
		t.Fatalf("read bundled admin frontend index: %v", err)
	}
	if string(content) != "<html>admin</html>" {
		t.Fatalf("unexpected bundled admin frontend content %q", string(content))
	}
	assetContent, err := os.ReadFile(filepath.Join(outputDir, bundledAdminFrontendDir, "assets", "app.js"))
	if err != nil {
		t.Fatalf("read bundled admin frontend asset: %v", err)
	}
	if string(assetContent) != "console.log('admin');" {
		t.Fatalf("unexpected bundled admin frontend asset %q", string(assetContent))
	}
	serverConfig := readBundledServerConfig(t, filepath.Join(outputDir, "config", "server.json"))
	if serverConfig.AdminFrontendDir != bundledAdminFrontendDir {
		t.Fatalf("expected admin_frontend_dir %q, got %q", bundledAdminFrontendDir, serverConfig.AdminFrontendDir)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(workingDir, "..", ".."))
}

func testBundleRepoRoot(t *testing.T, includeFrontendDist bool) string {
	t.Helper()
	root := repoRoot(t)
	tempRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(tempRoot, "cmd", "goginx-server"))
	mustMkdirAll(t, filepath.Join(tempRoot, "cmd", "goginx-client"))
	mustMkdirAll(t, filepath.Join(tempRoot, "cmd", "goginx-admin"))
	mustMkdirAll(t, filepath.Join(tempRoot, "deploy", "systemd"))
	copyFileForTest(t, filepath.Join(root, "go.mod"), filepath.Join(tempRoot, "go.mod"))
	copyFileForTest(t, filepath.Join(root, "go.sum"), filepath.Join(tempRoot, "go.sum"))
	copyFileForTest(t, filepath.Join(root, "cmd", "goginx-server", "main.go"), filepath.Join(tempRoot, "cmd", "goginx-server", "main.go"))
	copyFileForTest(t, filepath.Join(root, "cmd", "goginx-client", "main.go"), filepath.Join(tempRoot, "cmd", "goginx-client", "main.go"))
	copyFileForTest(t, filepath.Join(root, "cmd", "goginx-admin", "main.go"), filepath.Join(tempRoot, "cmd", "goginx-admin", "main.go"))
	copyFileForTest(t, filepath.Join(root, "deploy", "systemd", "goginx-server.service"), filepath.Join(tempRoot, "deploy", "systemd", "goginx-server.service"))
	copyFileForTest(t, filepath.Join(root, "deploy", "systemd", "goginx-client.service"), filepath.Join(tempRoot, "deploy", "systemd", "goginx-client.service"))
	if err := os.Symlink(filepath.Join(root, "internal"), filepath.Join(tempRoot, "internal")); err != nil {
		t.Skipf("symlink internal package tree: %v", err)
	}
	if includeFrontendDist {
		mustMkdirAll(t, filepath.Join(tempRoot, "admin-ui", "dist", "assets"))
		mustWriteFile(t, filepath.Join(tempRoot, "admin-ui", "dist", "index.html"), []byte("<html>admin</html>"), 0o644)
		mustWriteFile(t, filepath.Join(tempRoot, "admin-ui", "dist", "assets", "app.js"), []byte("console.log('admin');"), 0o644)
	}
	return tempRoot
}

func readBundledServerConfig(t *testing.T, path string) bundledServerConfig {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}
	var cfg bundledServerConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("decode server config: %v", err)
	}
	return cfg
}

type bundledServerConfig struct {
	AdminFrontendDir string `json:"admin_frontend_dir"`
}

func copyFileForTest(t *testing.T, sourcePath string, destPath string) {
	t.Helper()
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat %s: %v", sourcePath, err)
	}
	mustWriteFile(t, destPath, content, info.Mode().Perm())
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, content []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, content, perm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
