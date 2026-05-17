package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/config"
)

type BundleOptions struct {
	RepoRoot    string
	OutputDir   string
	GoOS        string
	GoArch      string
	InstallRoot string
}

const bundledAdminFrontendDir = "admin-ui"

func BuildBundle(ctx context.Context, options BundleOptions) error {
	if err := options.validate(); err != nil {
		return err
	}
	adminFrontendDist, err := requireAdminFrontendDist(options)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(options.OutputDir); err != nil {
		return err
	}
	for _, dir := range []string{
		options.OutputDir,
		filepath.Join(options.OutputDir, "bin"),
		filepath.Join(options.OutputDir, "config"),
		filepath.Join(options.OutputDir, "data"),
		filepath.Join(options.OutputDir, "data", "certs"),
		filepath.Join(options.OutputDir, "data", "certs", "managed"),
		filepath.Join(options.OutputDir, "logs"),
		filepath.Join(options.OutputDir, "systemd"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	for _, target := range []struct {
		name        string
		packagePath string
	}{
		{name: binaryName("goginx-server", options.GoOS), packagePath: "./cmd/goginx-server"},
		{name: binaryName("goginx-client", options.GoOS), packagePath: "./cmd/goginx-client"},
		{name: binaryName("goginx-admin", options.GoOS), packagePath: "./cmd/goginx-admin"},
	} {
		if err := buildBinary(ctx, options, target.packagePath, filepath.Join(options.OutputDir, "bin", target.name)); err != nil {
			return err
		}
	}
	if err := copyAdminFrontendAssets(adminFrontendDist, filepath.Join(options.OutputDir, bundledAdminFrontendDir)); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(options.OutputDir, "config", "server.json"), defaultServerBundleConfig()); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(options.OutputDir, "config", "client.json"), defaultClientBundleConfig()); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(options.OutputDir, "config", "admin-credentials.json.example"), []byte(adminCredentialsExample()), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(options.OutputDir, "config", "goginx-server.env.example"), []byte(serverEnvExample()), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(options.OutputDir, "config", "goginx-client.env.example"), []byte(clientEnvExample()), 0o644); err != nil {
		return err
	}
	for _, serviceName := range []string{"goginx-server.service", "goginx-client.service"} {
		content, err := renderSystemdTemplate(options, serviceName)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(options.OutputDir, "systemd", serviceName), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (options BundleOptions) validate() error {
	if strings.TrimSpace(options.RepoRoot) == "" {
		return errors.New("repo root is required")
	}
	if strings.TrimSpace(options.OutputDir) == "" {
		return errors.New("output directory is required")
	}
	if strings.TrimSpace(options.GoOS) == "" {
		options.GoOS = runtime.GOOS
	}
	if strings.TrimSpace(options.GoArch) == "" {
		return errors.New("goarch is required")
	}
	if strings.TrimSpace(options.InstallRoot) == "" {
		return errors.New("install root is required")
	}
	if _, err := os.Stat(filepath.Join(options.RepoRoot, "go.mod")); err != nil {
		return fmt.Errorf("repo root must contain go.mod: %w", err)
	}
	return nil
}

func buildBinary(ctx context.Context, options BundleOptions, packagePath string, output string) error {
	command := exec.CommandContext(ctx, "go", "build", "-o", output, packagePath)
	command.Dir = options.RepoRoot
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+options.GoOS, "GOARCH="+options.GoArch)
	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build %s: %w: %s", packagePath, err, strings.TrimSpace(string(combined)))
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o644)
}

func defaultServerBundleConfig() config.Server {
	server := config.DefaultServer()
	server.AdminEnabled = true
	server.AdminCredentialsFile = ""
	server.AdminFrontendDir = ""
	server.ControlTLSCertFile = "data/certs/control.crt"
	server.ControlTLSKeyFile = "data/certs/control.key"
	server.SQLitePath = "data/go-ginx.db"
	server.DataDir = "data"
	server.CertificateDir = "data/certs"
	return server
}

func requireAdminFrontendDist(options BundleOptions) (string, error) {
	sourceDir := filepath.Join(options.RepoRoot, "admin-ui", "dist")
	info, err := os.Stat(sourceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("admin frontend build output is required at %s; run npm ci and npm run build in %s before build-deploy-bundle", sourceDir, filepath.Join(options.RepoRoot, "admin-ui"))
		}
		return "", fmt.Errorf("stat admin frontend dist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("admin frontend dist path is not a directory: %s", sourceDir)
	}
	indexPath := filepath.Join(sourceDir, "index.html")
	indexInfo, err := os.Stat(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("admin frontend index is required at %s; run npm run build in %s before build-deploy-bundle", indexPath, filepath.Join(options.RepoRoot, "admin-ui"))
		}
		return "", fmt.Errorf("stat admin frontend index: %w", err)
	}
	if indexInfo.IsDir() {
		return "", fmt.Errorf("admin frontend index path is a directory: %s", indexPath)
	}
	return sourceDir, nil
}

func copyAdminFrontendAssets(sourceDir string, destDir string) error {
	return copyDir(sourceDir, destDir)
}

func copyDir(sourceDir string, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDir, entry.Name())
		destPath := filepath.Join(destDir, entry.Name())
		if entry.IsDir() {
			if err := copyDir(sourcePath, destPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(sourcePath, destPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(sourcePath string, destPath string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		return err
	}
	return nil
}

func adminCredentialsExample() string {
	return strings.Join([]string{
		"{",
		"  \"administrators\": [",
		"    {",
		"      \"username\": \"admin\",",
		"      \"password_hash\": \"replace-with-bcrypt-hash\"",
		"    }",
		"  ]",
		"}",
		"",
	}, "\n")
}

func defaultClientBundleConfig() config.Client {
	client := config.DefaultClient()
	client.ServerAddress = "server.example.com:8443"
	client.ServerTLSAddress = "server.example.com:9443"
	client.ServerName = config.DefaultServer().ControlTLSServerName
	client.ServerCAFile = config.DefaultClientCAFile
	client.ClientID = "client-1"
	client.Credential = "change-me"
	return client
}

func serverEnvExample() string {
	return strings.Join([]string{
		"# Optional ACME and service overrides for goginx-server",
		"CF_DNS_API_TOKEN=",
		"",
	}, "\n")
}

func clientEnvExample() string {
	return strings.Join([]string{
		"# Optional environment overrides for goginx-client service wrapper",
		"",
	}, "\n")
}

func renderSystemdTemplate(options BundleOptions, serviceName string) ([]byte, error) {
	templatePath := filepath.Join(options.RepoRoot, "deploy", "systemd", serviceName)
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}
	rendered := strings.ReplaceAll(string(content), "{{INSTALL_ROOT}}", filepath.ToSlash(options.InstallRoot))
	return []byte(rendered), nil
}

func binaryName(name string, goos string) string {
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}
