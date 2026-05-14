package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func BuildBundle(ctx context.Context, options BundleOptions) error {
	if err := options.validate(); err != nil {
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
	server.AdminCredentialsFile = ""
	server.ControlTLSCertFile = "data/certs/control.crt"
	server.ControlTLSKeyFile = "data/certs/control.key"
	server.SQLitePath = "data/go-ginx.db"
	server.DataDir = "data"
	server.CertificateDir = "data/certs"
	return server
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
	client.ServerName = "server.example.com"
	client.ServerCAFile = "data/certs/ca.crt"
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
