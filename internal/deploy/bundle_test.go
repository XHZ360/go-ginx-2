package deploy

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildBundleCreatesExpectedLayout(t *testing.T) {
	root := repoRoot(t)
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
}

func repoRoot(t *testing.T) string {
	t.Helper()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(workingDir, "..", ".."))
}
