package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/deploypath"
)

const (
	DefaultClientStatePath  = "data/client-state.json"
	DefaultClientConfigPath = "config/client.json"
	DefaultClientCAFile     = "data/certs/server-ca.crt"
	DefaultServerConfigPath = "config/server.json"
)

type JoinServiceDefaultsOptions struct {
	Root             string
	ServerConfigPath string
}

type JoinServiceDefaultsResult struct {
	Defaults   JoinServiceDefaults
	Server     Server
	ConfigPath string
	Source     string
}

func LoadManagedServer() (Server, error) {
	return LoadManagedServerAtRoot("")
}

func LoadManagedServerAtRoot(root string) (Server, error) {
	cfg := DefaultServer()
	cfg.AdminEnabled = true
	applyManagedServerEnv(&cfg)
	ResolveServerPaths(&cfg, root)
	if err := PrepareManagedServer(&cfg); err != nil {
		return Server{}, err
	}
	return cfg, cfg.Validate()
}

func LoadJoinServiceDefaults(options JoinServiceDefaultsOptions) (JoinServiceDefaultsResult, error) {
	cfg := DefaultServer()
	cfg.AdminEnabled = true
	source := "managed_defaults"
	configPath := ""
	if options.ServerConfigPath != "" {
		resolvedPath := resolveServerConfigPath(options.Root, options.ServerConfigPath)
		loaded, err := LoadServer(resolvedPath)
		if err != nil {
			return JoinServiceDefaultsResult{}, err
		}
		cfg = loaded
		configPath = resolvedPath
		source = "server_config"
	} else if options.Root != "" {
		resolvedPath := deploypath.Resolve(options.Root, DefaultServerConfigPath)
		if fileExists(resolvedPath) {
			loaded, err := LoadServer(resolvedPath)
			if err != nil {
				return JoinServiceDefaultsResult{}, err
			}
			cfg = loaded
			configPath = resolvedPath
			source = "deployment_server_config"
		}
	}
	if options.ServerConfigPath == "" {
		applyManagedServerEnv(&cfg)
	}
	ResolveServerPaths(&cfg, options.Root)
	defaults, err := ConfirmJoinServiceDefaults(cfg)
	if err != nil {
		return JoinServiceDefaultsResult{}, err
	}
	return JoinServiceDefaultsResult{Defaults: defaults, Server: cfg, ConfigPath: configPath, Source: source}, nil
}

func resolveServerConfigPath(root string, path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return deploypath.Resolve(root, path)
}

func ResolveServerPaths(cfg *Server, root string) {
	if cfg == nil {
		return
	}
	cfg.AdminCredentialsFile = deploypath.Resolve(root, cfg.AdminCredentialsFile)
	cfg.AdminFrontendDir = deploypath.Resolve(root, cfg.AdminFrontendDir)
	cfg.ControlTLSCAFile = deploypath.Resolve(root, cfg.ControlTLSCAFile)
	cfg.ControlTLSCertFile = deploypath.Resolve(root, cfg.ControlTLSCertFile)
	cfg.ControlTLSKeyFile = deploypath.Resolve(root, cfg.ControlTLSKeyFile)
	cfg.SQLitePath = deploypath.Resolve(root, cfg.SQLitePath)
	cfg.DataDir = deploypath.Resolve(root, cfg.DataDir)
	cfg.CertificateDir = deploypath.Resolve(root, cfg.CertificateDir)
}

func ResolveClientPaths(cfg *Client, root string) {
	if cfg == nil {
		return
	}
	cfg.ServerCAFile = deploypath.Resolve(root, cfg.ServerCAFile)
}

func applyManagedServerEnv(cfg *Server) {
	envString("GOGINX_ADMIN_LISTEN", &cfg.AdminListen)
	envString("GOGINX_CLIENT_ENROLLMENT_LISTEN", &cfg.ClientEnrollmentListen)
	envString("GOGINX_CONTROL_QUIC_LISTEN", &cfg.ControlQUICListen)
	envString("GOGINX_CONTROL_TLS_LISTEN", &cfg.ControlTLSListen)
	envString("GOGINX_CONTROL_TLS_SERVER_NAME", &cfg.ControlTLSServerName)
	envString("GOGINX_CONTROL_TLS_CA_FILE", &cfg.ControlTLSCAFile)
	envString("GOGINX_CONTROL_TLS_CERT_FILE", &cfg.ControlTLSCertFile)
	envString("GOGINX_CONTROL_TLS_KEY_FILE", &cfg.ControlTLSKeyFile)
	envString("GOGINX_JOIN_SERVICE_HOST", &cfg.JoinServiceHost)
	envString("GOGINX_TCP_ENTRY_HOST", &cfg.TCPEntryHost)
	envString("GOGINX_HTTP_ENTRY_LISTEN", &cfg.HTTPEntryListen)
	envString("GOGINX_HTTPS_ENTRY_LISTEN", &cfg.HTTPSEntryListen)
	envString("GOGINX_SQLITE_PATH", &cfg.SQLitePath)
	envString("GOGINX_DATA_DIR", &cfg.DataDir)
	envString("GOGINX_CERTIFICATE_DIR", &cfg.CertificateDir)
}

func envString(name string, target *string) {
	if value := os.Getenv(name); value != "" {
		*target = value
	}
}

func PrepareManagedServer(cfg *Server) error {
	if cfg == nil {
		return errors.New("server config is required")
	}
	for _, dir := range []string{cfg.DataDir, cfg.CertificateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create managed directory %s: %w", dir, err)
		}
	}
	return EnsureControlTLS(cfg)
}

func LoadManagedClient() (Client, error) {
	return LoadClient(DefaultClientStatePath)
}

func SaveManagedClient(cfg Client, path string) error {
	if path == "" {
		path = DefaultClientStatePath
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o600)
}

func WriteClientCA(caPEM []byte, path string) error {
	if path == "" {
		path = DefaultClientCAFile
	}
	if len(caPEM) == 0 {
		return errors.New("ca pem is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, caPEM, 0o644)
}

func EnsureControlTLS(cfg *Server) error {
	if cfg == nil {
		return errors.New("server config is required")
	}
	if fileExists(cfg.ControlTLSCAFile) && fileExists(cfg.ControlTLSCertFile) && fileExists(cfg.ControlTLSKeyFile) {
		return nil
	}
	caCertPEM, serverCertPEM, serverKeyPEM, err := generateControlTLS(cfg.ControlTLSServerName)
	if err != nil {
		return err
	}
	files := []struct {
		path    string
		content []byte
		mode    os.FileMode
	}{
		{path: cfg.ControlTLSCAFile, content: caCertPEM, mode: 0o644},
		{path: cfg.ControlTLSCertFile, content: serverCertPEM, mode: 0o644},
		{path: cfg.ControlTLSKeyFile, content: serverKeyPEM, mode: 0o600},
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file.path, file.content, file.mode); err != nil {
			return err
		}
	}
	return nil
}

func generateControlTLS(serverName string) ([]byte, []byte, []byte, error) {
	now := time.Now().UTC()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          serialNumber(),
		Subject:               pkix.Name{CommonName: "go-ginx control local CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject:      pkix.Name{CommonName: serverName},
		DNSNames:     []string{serverName, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(2, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}),
		nil
}

func serialNumber() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	value, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return value
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
