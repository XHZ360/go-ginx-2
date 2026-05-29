package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/deploy"
	"github.com/simp-frp/go-ginx-2/internal/deploypath"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

var (
	newACMEIssuer  = func() certmanager.Issuer { return certmanager.ACMEIssuer{} }
	newDNSProvider = func(tokenEnv string) (certmanager.DNSChallengeProvider, error) {
		provider, err := certmanager.NewCloudflareDNSProviderFromEnv(tokenEnv)
		if err != nil {
			return nil, err
		}
		return provider, nil
	}
)

var executablePath = os.Executable

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: goginx-admin <init-admin|create-user|create-client|create-client-join|create-tcp-proxy|create-udp-proxy|create-http-proxy|create-https-proxy|issue-managed-certificate|renew-managed-certificate|managed-certificate-status|build-deploy-bundle> [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dbPath := flags.String("db", defaultAdminDBPath(), "SQLite database path")
	actorID := flags.String("actor", "system", "audit actor user ID")

	switch command {
	case "init-admin":
		username := flags.String("username", "", "administrator username")
		password := flags.String("password", "", "administrator password")
		id := flags.String("id", "", "administrator user ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		*dbPath = deploymentRelativePath(*dbPath)
		if strings.TrimSpace(*password) == "" {
			return fmt.Errorf("administrator password is required")
		}
		service, closeStore, err := openService(*dbPath)
		if err != nil {
			return err
		}
		defer closeStore()
		user, err := service.CreateUser(context.Background(), admin.CreateUserInput{ID: *id, Username: *username, Password: *password, Role: domain.RoleAdmin, ActorID: *actorID})
		if err != nil {
			if !errors.Is(err, store.ErrAlreadyExists) {
				return err
			}
			adminID, err := existingAdminID(context.Background(), service, *id, *username)
			if err != nil {
				return err
			}
			if err := service.SetUserPassword(context.Background(), adminID, *password, *actorID); err != nil {
				return err
			}
			if err := service.EnableUser(context.Background(), adminID, *actorID); err != nil {
				return err
			}
			fmt.Println(adminID)
			return nil
		}
		fmt.Println(user.ID)
		return nil
	case "create-user":
		id := flags.String("id", "", "user ID")
		username := flags.String("username", "", "username")
		role := flags.String("role", string(domain.RoleUser), "role: admin or user")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		*dbPath = deploymentRelativePath(*dbPath)
		service, closeStore, err := openService(*dbPath)
		if err != nil {
			return err
		}
		defer closeStore()
		user, err := service.CreateUser(context.Background(), admin.CreateUserInput{ID: *id, Username: *username, Role: domain.Role(*role), ActorID: *actorID})
		if err != nil {
			return err
		}
		fmt.Println(user.ID)
		return nil
	case "create-client":
		id := flags.String("id", "", "client ID")
		userID := flags.String("user", "", "owner user ID")
		name := flags.String("name", "", "client display name")
		credential := flags.String("credential", "", "client credential")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		*dbPath = deploymentRelativePath(*dbPath)
		service, closeStore, err := openService(*dbPath)
		if err != nil {
			return err
		}
		defer closeStore()
		client, err := service.CreateClient(context.Background(), admin.CreateClientInput{ID: *id, UserID: *userID, Name: *name, Credential: *credential, ActorID: *actorID})
		if err != nil {
			return err
		}
		fmt.Println(client.ID)
		return nil
	case "create-client-join":
		return createClientJoin(flags, args[1:])
	case "create-tcp-proxy":
		return createProxy(flags, args[1:], domain.ProxyTCP)
	case "create-udp-proxy":
		return createProxy(flags, args[1:], domain.ProxyUDP)
	case "create-http-proxy":
		return createProxy(flags, args[1:], domain.ProxyHTTP)
	case "create-https-proxy":
		return createProxy(flags, args[1:], domain.ProxyHTTPS)
	case "issue-managed-certificate":
		return manageCertificate(flags, args[1:], "issue")
	case "renew-managed-certificate":
		return manageCertificate(flags, args[1:], "renew")
	case "managed-certificate-status":
		return manageCertificate(flags, args[1:], "status")
	case "build-deploy-bundle":
		return buildDeployBundle(flags, args[1:])
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func buildDeployBundle(flags *flag.FlagSet, args []string) error {
	goos := flags.String("goos", runtime.GOOS, "target GOOS")
	goarch := flags.String("goarch", runtime.GOARCH, "target GOARCH")
	outputDir := flags.String("output", "", "bundle output directory")
	installRoot := flags.String("install-root", "/opt/go-ginx", "install root rendered into service templates")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*outputDir) == "" {
		*outputDir = filepath.Join("dist", *goos+"-"+*goarch+"-bundle")
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot, err = findRepoRoot(repoRoot)
	if err != nil {
		return err
	}
	return deploy.BuildBundle(context.Background(), deploy.BundleOptions{RepoRoot: repoRoot, OutputDir: *outputDir, GoOS: *goos, GoArch: *goarch, InstallRoot: *installRoot})
}

func findRepoRoot(start string) (string, error) {
	current := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found from %s", start)
		}
		current = parent
	}
}

func createProxy(flags *flag.FlagSet, args []string, proxyType domain.ProxyType) error {
	id := flags.String("id", "", "proxy ID")
	userID := flags.String("user", "", "owner user ID")
	clientID := flags.String("client", "", "client ID")
	name := flags.String("name", "", "proxy name")
	entryHost := flags.String("host", "", "HTTP or HTTPS entry host")
	entryPort := flags.Int("port", 0, "TCP entry port")
	targetHost := flags.String("target-host", "", "local target host")
	targetPort := flags.Int("target-port", 0, "local target port")
	certFile := flags.String("cert-file", "", "HTTPS termination certificate file")
	keyFile := flags.String("key-file", "", "HTTPS termination private key file")
	description := flags.String("description", "", "proxy description")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dbPath := deploymentRelativePath(flags.Lookup("db").Value.String())
	actorID := flags.Lookup("actor").Value.String()
	*certFile = deploymentRelativePath(*certFile)
	*keyFile = deploymentRelativePath(*keyFile)
	service, closeStore, err := openService(dbPath)
	if err != nil {
		return err
	}
	defer closeStore()
	proxy, err := service.CreateProxy(context.Background(), admin.CreateProxyInput{ID: *id, UserID: *userID, ClientID: *clientID, Name: *name, Type: proxyType, EntryHost: *entryHost, EntryPort: *entryPort, TargetHost: *targetHost, TargetPort: *targetPort, CertFile: *certFile, KeyFile: *keyFile, Description: *description, ActorID: actorID})
	if err != nil {
		return err
	}
	fmt.Println(proxy.ID)
	return nil
}

func existingAdminID(ctx context.Context, service admin.Service, id string, username string) (string, error) {
	if service.Store == nil {
		return "", fmt.Errorf("store is required")
	}
	var user domain.User
	var err error
	if strings.TrimSpace(id) != "" {
		user, err = service.Store.Users().ByID(ctx, id)
	}
	if strings.TrimSpace(id) == "" || errors.Is(err, store.ErrNotFound) {
		user, err = service.Store.Users().ByUsername(ctx, username)
	}
	if err != nil {
		return "", err
	}
	if user.Role != domain.RoleAdmin {
		return "", fmt.Errorf("existing user %q is not an administrator", user.ID)
	}
	return user.ID, nil
}

func createClientJoin(flags *flag.FlagSet, args []string) error {
	joinDefaults, err := defaultJoinServiceDefaults()
	if err != nil {
		return err
	}
	id := flags.String("id", "", "client ID")
	userID := flags.String("user", "", "owner user ID")
	name := flags.String("name", "", "client display name")
	enrollmentURL := flags.String("enrollment-url", joinDefaults.EnrollmentURL, "client enrollment URL")
	serverAddress := flags.String("server-address", joinDefaults.ServerAddress, "control QUIC server address")
	serverTLSAddress := flags.String("server-tls-address", joinDefaults.ServerTLSAddress, "control TCP+TLS server address")
	serverName := flags.String("server-name", joinDefaults.ServerName, "control TLS server name")
	serverCAFile := flags.String("server-ca-file", joinDefaults.ServerCAFile, "control TLS CA file")
	ttl := flags.Duration("ttl", time.Hour, "join token lifetime")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dbPath := deploymentRelativePath(flags.Lookup("db").Value.String())
	actorID := flags.Lookup("actor").Value.String()
	*serverCAFile = deploymentRelativePath(*serverCAFile)
	service, closeStore, err := openService(dbPath)
	if err != nil {
		return err
	}
	defer closeStore()
	result, err := service.CreateClientJoin(context.Background(), admin.CreateClientJoinInput{
		ID:               *id,
		UserID:           *userID,
		Name:             *name,
		ActorID:          actorID,
		EnrollmentURL:    *enrollmentURL,
		ServerAddress:    *serverAddress,
		ServerTLSAddress: *serverTLSAddress,
		ServerName:       *serverName,
		ServerCAFile:     *serverCAFile,
		TTL:              *ttl,
	})
	if err != nil {
		return err
	}
	fmt.Println(result.Token)
	return nil
}

func defaultJoinServiceDefaults() (config.JoinServiceDefaults, error) {
	cfg := config.DefaultServer()
	cfg.JoinServiceHost = "127.0.0.1"
	defaults, err := config.ConfirmJoinServiceDefaults(cfg)
	if err != nil {
		return config.JoinServiceDefaults{}, err
	}
	defaults.ServerCAFile = deploymentRelativePath(defaults.ServerCAFile)
	return defaults, nil
}

func manageCertificate(flags *flag.FlagSet, args []string, action string) error {
	proxyID := flags.String("proxy", "", "HTTPS proxy ID")
	certificateDir := flags.String("certificate-dir", "data/certs", "managed certificate directory")
	acmeDirectoryURL := flags.String("acme-directory-url", "https://acme-v02.api.letsencrypt.org/directory", "ACME directory URL")
	acmeAccountEmail := flags.String("acme-account-email", "", "ACME account email")
	acmeTermsAccepted := flags.Bool("acme-terms-accepted", false, "accept ACME terms of service")
	acmeTokenEnv := flags.String("acme-cloudflare-token-env", "CF_DNS_API_TOKEN", "Cloudflare API token environment variable")
	acmeRenewalWindow := flags.Duration("acme-renewal-window", 30*24*time.Hour, "managed certificate renewal window")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dbPath := deploymentRelativePath(flags.Lookup("db").Value.String())
	actorID := flags.Lookup("actor").Value.String()
	*certificateDir = deploymentRelativePath(*certificateDir)
	service, closeStore, err := openService(dbPath, certificateServiceConfig{CertificateDir: *certificateDir, DirectoryURL: *acmeDirectoryURL, AccountEmail: *acmeAccountEmail, TermsAccepted: *acmeTermsAccepted, TokenEnv: *acmeTokenEnv, RenewalWindow: *acmeRenewalWindow})
	if err != nil {
		return err
	}
	defer closeStore()
	switch action {
	case "issue":
		certificate, err := service.IssueManagedCertificate(context.Background(), admin.CertificateInput{ProxyID: *proxyID, ActorID: actorID})
		if err != nil {
			return err
		}
		fmt.Println(certificate.ID)
	case "renew":
		certificate, err := service.RenewManagedCertificate(context.Background(), admin.CertificateInput{ProxyID: *proxyID, ActorID: actorID})
		if err != nil {
			return err
		}
		fmt.Println(certificate.ID)
	case "status":
		status, err := service.ManagedCertificateStatus(context.Background(), *proxyID)
		if err != nil {
			return err
		}
		expires := ""
		if status.Certificate.NotAfter != nil {
			expires = status.Certificate.NotAfter.Format(time.RFC3339)
		}
		fmt.Printf("%s %s %s\n", status.Certificate.ID, status.Certificate.Status, expires)
	}
	return nil
}

type certificateServiceConfig struct {
	CertificateDir string
	DirectoryURL   string
	AccountEmail   string
	TermsAccepted  bool
	TokenEnv       string
	RenewalWindow  time.Duration
}

func openService(dbPath string, certificateCfg ...certificateServiceConfig) (admin.Service, func(), error) {
	if err := ensureDatabaseParentDir(dbPath); err != nil {
		return admin.Service{}, nil, err
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return admin.Service{}, nil, fmt.Errorf("open sqlite database %s: %w", dbPath, err)
	}
	service := admin.Service{Store: db}
	if len(certificateCfg) > 0 {
		provider, err := newDNSProvider(certificateCfg[0].TokenEnv)
		if err != nil {
			_ = db.Close()
			return admin.Service{}, nil, err
		}
		service.Certificates = certmanager.Service{Store: db, Issuer: newACMEIssuer(), DNSProvider: provider, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateCfg[0].CertificateDir}, Settings: domain.ACMEProviderSettings{DirectoryURL: certificateCfg[0].DirectoryURL, AccountEmail: certificateCfg[0].AccountEmail, TermsAccepted: certificateCfg[0].TermsAccepted, RenewalWindow: certificateCfg[0].RenewalWindow, DNSProvider: "cloudflare", DNSProviderTokenEnv: certificateCfg[0].TokenEnv}}
	}
	return service, func() { _ = db.Close() }, nil
}

func defaultAdminDBPath() string {
	root, err := deploymentRoot()
	if err != nil {
		return config.DefaultServer().SQLitePath
	}
	return deploypath.Resolve(root, config.DefaultServer().SQLitePath)
}

func deploymentRoot() (string, error) {
	return deploypath.Root(executablePath)
}

func deploymentRelativePath(path string) string {
	root, err := deploymentRoot()
	if err != nil {
		return path
	}
	return deploypath.Resolve(root, path)
}

func ensureDatabaseParentDir(dbPath string) error {
	if strings.TrimSpace(dbPath) == "" || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") {
		return nil
	}
	parent := filepath.Dir(dbPath)
	if parent == "." || parent == "" {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create sqlite database directory %s: %w", parent, err)
	}
	return nil
}
