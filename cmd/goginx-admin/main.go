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
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/admintui"
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

var runAdminTUI = func(ctx context.Context, opts admintui.RunOptions) error {
	return admintui.Run(ctx, opts)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: goginx-admin <tui|init-admin|create-user|create-client|create-client-join|client-join-command|create-tcp-proxy|create-udp-proxy|create-http-proxy|create-https-proxy|issue-managed-certificate|renew-managed-certificate|sync-origin-ca-certificate|revoke-origin-ca-certificate|managed-certificate-status|build-deploy-bundle> [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dbPath := flags.String("db", defaultAdminDBPath(), "SQLite database path")
	actorID := flags.String("actor", "system", "audit actor user ID")

	switch command {
	case "tui":
		return runTUI(flags, args[1:])
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
			adminID, err := existingAdminID(context.Background(), service.Store, *id, *username)
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
		consumer := flags.Bool("consumer", false, "create a consumer client for SDK use")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		*dbPath = deploymentRelativePath(*dbPath)
		service, closeStore, err := openService(*dbPath)
		if err != nil {
			return err
		}
		defer closeStore()
		kind := domain.ClientKindProvider
		if *consumer {
			kind = domain.ClientKindConsumer
		}
		client, err := service.CreateClient(context.Background(), admin.CreateClientInput{ID: *id, UserID: *userID, Name: *name, Kind: kind, Credential: *credential, ActorID: *actorID})
		if err != nil {
			return err
		}
		fmt.Println(client.ID)
		return nil
	case "create-client-join":
		return createClientJoin(flags, args[1:])
	case "client-join-command":
		return clientJoinCommand(flags, args[1:])
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
	case "sync-origin-ca-certificate":
		return manageCertificate(flags, args[1:], "sync")
	case "revoke-origin-ca-certificate":
		return manageCertificate(flags, args[1:], "revoke")
	case "managed-certificate-status":
		return manageCertificate(flags, args[1:], "status")
	case "build-deploy-bundle":
		return buildDeployBundle(flags, args[1:])
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func runTUI(flags *flag.FlagSet, args []string) error {
	serverConfigPath := flags.String("server-config", "", "server config path for join defaults")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dbPath := deploymentRelativePath(flags.Lookup("db").Value.String())
	actorID := flags.Lookup("actor").Value.String()
	service, closeStore, err := openService(dbPath)
	if err != nil {
		return err
	}
	defer closeStore()
	joinDefaultsResult, err := defaultJoinServiceDefaults(*serverConfigPath)
	if err != nil {
		return err
	}
	joinDefaults := joinDefaultsResult.Defaults
	service.Clients.DefaultJoin = joinDefaults
	backend := admintui.LocalBackend{
		Commands:          service,
		Queries:           adminquery.Service{Store: service.Store},
		JoinDefaultsValue: joinDefaults,
	}
	return runAdminTUI(context.Background(), admintui.RunOptions{
		Backend:                   backend,
		ActorID:                   actorID,
		ClientJoinCommandTemplate: adminClientJoinCommandTemplate(dbPath, explicitServerConfigPathForCommand(*serverConfigPath, joinDefaultsResult.ConfigPath)),
		RequireTTY:                true,
	})
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
	entryBindHost := flags.String("bind-host", "", "entry listener bind host")
	entryHost := flags.String("host", "", "HTTP Host or HTTPS SNI domain")
	entryPort := flags.Int("port", 0, "entry listener port")
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
	proxy, err := service.CreateProxy(context.Background(), admin.CreateProxyInput{ID: *id, UserID: *userID, ClientID: *clientID, Name: *name, Type: proxyType, EntryBindHost: *entryBindHost, EntryHost: *entryHost, EntryPort: *entryPort, TargetHost: *targetHost, TargetPort: *targetPort, CertFile: *certFile, KeyFile: *keyFile, Description: *description, ActorID: actorID})
	if err != nil {
		return err
	}
	fmt.Println(proxy.ID)
	return nil
}

func existingAdminID(ctx context.Context, db store.Store, id string, username string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("store is required")
	}
	var user domain.User
	var err error
	if strings.TrimSpace(id) != "" {
		user, err = db.Users().ByID(ctx, id)
	}
	if strings.TrimSpace(id) == "" || errors.Is(err, store.ErrNotFound) {
		user, err = db.Users().ByUsername(ctx, username)
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
	serverConfigPath := flags.String("server-config", "", "server config path for join defaults")
	id := flags.String("id", "", "client ID")
	userID := flags.String("user", "", "owner user ID")
	name := flags.String("name", "", "client display name")
	enrollmentURL := flags.String("enrollment-url", "", "client enrollment URL")
	serverAddress := flags.String("server-address", "", "control QUIC server address")
	serverTLSAddress := flags.String("server-tls-address", "", "control TCP+TLS server address")
	serverName := flags.String("server-name", "", "control TLS server name")
	serverCAFile := flags.String("server-ca-file", "", "control TLS CA file")
	ttl := flags.Duration("ttl", time.Hour, "join token lifetime")
	if err := flags.Parse(args); err != nil {
		return err
	}
	joinDefaultsResult, err := defaultJoinServiceDefaults(*serverConfigPath)
	if err != nil {
		return err
	}
	applyJoinDefaults(enrollmentURL, serverAddress, serverTLSAddress, serverName, serverCAFile, joinDefaultsResult.Defaults)
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

func clientJoinCommand(flags *flag.FlagSet, args []string) error {
	serverConfigPath := flags.String("server-config", "", "server config path for join defaults")
	clientID := flags.String("client", "", "client ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dbPath := deploymentRelativePath(flags.Lookup("db").Value.String())
	actorID := flags.Lookup("actor").Value.String()
	service, closeStore, err := openService(dbPath)
	if err != nil {
		return err
	}
	defer closeStore()
	joinDefaultsResult, err := defaultJoinServiceDefaults(*serverConfigPath)
	if err != nil {
		return err
	}
	service.Clients.DefaultJoin = joinDefaultsResult.Defaults
	result, err := service.ReviewClientJoinToken(context.Background(), *clientID, actorID)
	if err != nil {
		return err
	}
	fmt.Println(clientJoinRunCommand(result.Token))
	return nil
}

func clientJoinRunCommand(token string) string {
	return fmt.Sprintf("goginx-client join %s", token)
}

func adminClientJoinCommandTemplate(dbPath string, serverConfigPath string) string {
	parts := []string{shellQuote(adminExecutableCommandPath()), "client-join-command", "-db", shellQuote(dbPath)}
	if strings.TrimSpace(serverConfigPath) != "" {
		parts = append(parts, "-server-config", shellQuote(serverConfigPath))
	}
	parts = append(parts, "-client", "{client}")
	return strings.Join(parts, " ")
}

func adminExecutableCommandPath() string {
	executable, err := executablePath()
	if err == nil {
		absolute, absErr := filepath.Abs(executable)
		if absErr == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
				return resolved
			}
			return absolute
		}
	}
	root, err := deploymentRoot()
	if err == nil {
		return filepath.Join(root, deploypath.DefaultBinaryDir, adminBinaryName(runtime.GOOS))
	}
	return adminBinaryName(runtime.GOOS)
}

func adminBinaryName(goos string) string {
	if goos == "windows" {
		return "goginx-admin.exe"
	}
	return "goginx-admin"
}

func shellQuote(value string) string {
	if strings.TrimSpace(value) == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\r\n'\"") {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return value
}

func defaultJoinServiceDefaults(serverConfigPath string) (config.JoinServiceDefaultsResult, error) {
	root, err := deploymentRoot()
	if err != nil {
		root = ""
	}
	return config.LoadJoinServiceDefaults(config.JoinServiceDefaultsOptions{Root: root, ServerConfigPath: serverConfigPath})
}

func applyJoinDefaults(enrollmentURL *string, serverAddress *string, serverTLSAddress *string, serverName *string, serverCAFile *string, defaults config.JoinServiceDefaults) {
	if strings.TrimSpace(*enrollmentURL) == "" {
		*enrollmentURL = defaults.EnrollmentURL
	}
	if strings.TrimSpace(*serverAddress) == "" {
		*serverAddress = defaults.ServerAddress
	}
	if strings.TrimSpace(*serverTLSAddress) == "" {
		*serverTLSAddress = defaults.ServerTLSAddress
	}
	if strings.TrimSpace(*serverName) == "" {
		*serverName = defaults.ServerName
	}
	if strings.TrimSpace(*serverCAFile) == "" {
		*serverCAFile = defaults.ServerCAFile
	}
}

func explicitServerConfigPathForCommand(requestedPath string, resolvedPath string) string {
	if strings.TrimSpace(requestedPath) == "" {
		return ""
	}
	path := resolvedPath
	if strings.TrimSpace(path) == "" {
		path = requestedPath
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return path
}

func manageCertificate(flags *flag.FlagSet, args []string, action string) error {
	proxyID := flags.String("proxy", "", "HTTPS proxy ID")
	certificateDir := flags.String("certificate-dir", "data/certs", "managed certificate directory")
	acmeDirectoryURL := flags.String("acme-directory-url", "https://acme-v02.api.letsencrypt.org/directory", "ACME directory URL")
	acmeAccountEmail := flags.String("acme-account-email", "", "ACME account email")
	acmeTermsAccepted := flags.Bool("acme-terms-accepted", false, "accept ACME terms of service")
	acmeTokenEnv := flags.String("acme-cloudflare-token-env", "CF_DNS_API_TOKEN", "Cloudflare API token environment variable")
	acmeRenewalWindow := flags.Duration("acme-renewal-window", 30*24*time.Hour, "managed certificate renewal window")
	providerType := flags.String("provider", "", "certificate provider: acme_dns01 or cloudflare_origin_ca")
	credentialID := flags.String("credential", "", "provider credential ID")
	requestType := flags.String("origin-ca-request-type", "origin-ecc", "Cloudflare Origin CA request type")
	requestedValidity := flags.Int("origin-ca-requested-validity", 5475, "Cloudflare Origin CA requested validity days")
	originCASecretStore := flags.String("origin-ca-secret-store", "data/secrets/provider-credentials", "provider credential secret store directory")
	originCARotationWindow := flags.Duration("origin-ca-rotation-window", 30*24*time.Hour, "Cloudflare Origin CA rotation window")
	revokeHost := flags.String("host", "", "strong confirmation host for Origin CA revoke")
	revokeCloudflareID := flags.String("cloudflare-certificate-id", "", "strong confirmation Cloudflare certificate ID for Origin CA revoke")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dbPath := deploymentRelativePath(flags.Lookup("db").Value.String())
	actorID := flags.Lookup("actor").Value.String()
	*certificateDir = deploymentRelativePath(*certificateDir)
	*originCASecretStore = deploymentRelativePath(*originCASecretStore)
	acmeEnabled := action == "issue" || action == "renew"
	if action == "issue" && domain.CertificateProviderType(*providerType) == domain.CertificateProviderCloudflareOriginCA {
		acmeEnabled = false
	}
	service, closeStore, err := openService(dbPath, certificateServiceConfig{CertificateDir: *certificateDir, DirectoryURL: *acmeDirectoryURL, AccountEmail: *acmeAccountEmail, TermsAccepted: *acmeTermsAccepted, TokenEnv: *acmeTokenEnv, RenewalWindow: *acmeRenewalWindow, ACMEEnabled: acmeEnabled, OriginCAEnabled: true, OriginCASecretStore: *originCASecretStore, OriginCARequestType: *requestType, OriginCARequestedValidity: *requestedValidity, OriginCARotationWindow: *originCARotationWindow})
	if err != nil {
		return err
	}
	defer closeStore()
	switch action {
	case "issue":
		certificate, err := service.IssueManagedCertificate(context.Background(), admin.CertificateInput{ProxyID: *proxyID, ProviderType: domain.CertificateProviderType(*providerType), CredentialID: *credentialID, RequestType: *requestType, RequestedValidity: *requestedValidity, ActorID: actorID})
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
	case "sync":
		certificate, err := service.SyncOriginCACertificate(context.Background(), admin.CertificateInput{ProxyID: *proxyID, ActorID: actorID})
		if err != nil {
			return err
		}
		fmt.Println(certificate.ProviderStatus)
	case "revoke":
		certificate, err := service.RevokeOriginCACertificate(context.Background(), admin.RevokeOriginCACertificateInput{ProxyID: *proxyID, Host: *revokeHost, CloudflareCertificateID: *revokeCloudflareID, ActorID: actorID})
		if err != nil {
			return err
		}
		fmt.Println(certificate.ProviderStatus)
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
	CertificateDir            string
	DirectoryURL              string
	AccountEmail              string
	TermsAccepted             bool
	TokenEnv                  string
	RenewalWindow             time.Duration
	ACMEEnabled               bool
	OriginCAEnabled           bool
	OriginCASecretStore       string
	OriginCARequestType       string
	OriginCARequestedValidity int
	OriginCARotationWindow    time.Duration
}

func openService(dbPath string, certificateCfg ...certificateServiceConfig) (admin.Services, func(), error) {
	if err := ensureDatabaseParentDir(dbPath); err != nil {
		return admin.Services{}, nil, err
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return admin.Services{}, nil, fmt.Errorf("open sqlite database %s: %w", dbPath, err)
	}
	var certificates certmanager.Service
	if len(certificateCfg) > 0 {
		var provider certmanager.DNSChallengeProvider
		var issuer certmanager.Issuer
		if certificateCfg[0].ACMEEnabled {
			loadedProvider, err := newDNSProvider(certificateCfg[0].TokenEnv)
			if err != nil {
				_ = db.Close()
				return admin.Services{}, nil, err
			}
			provider = loadedProvider
			issuer = newACMEIssuer()
		}
		certificates = certmanager.Service{Store: db, Issuer: issuer, DNSProvider: provider, OriginCAClient: certmanager.CloudflareOriginCAClient{}, ProviderSecretStore: certmanager.FileSecretStore{Dir: certificateCfg[0].OriginCASecretStore}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateCfg[0].CertificateDir}, Settings: domain.ACMEProviderSettings{DirectoryURL: certificateCfg[0].DirectoryURL, AccountEmail: certificateCfg[0].AccountEmail, TermsAccepted: certificateCfg[0].TermsAccepted, RenewalWindow: certificateCfg[0].RenewalWindow, DNSProvider: "cloudflare", DNSProviderTokenEnv: certificateCfg[0].TokenEnv}, OriginCASettings: domain.OriginCAProviderSettings{Enabled: certificateCfg[0].OriginCAEnabled, SecretStorePath: certificateCfg[0].OriginCASecretStore, DefaultRequestType: certificateCfg[0].OriginCARequestType, RequestedValidity: certificateCfg[0].OriginCARequestedValidity, RotationWindow: certificateCfg[0].OriginCARotationWindow}}
	}
	return admin.NewServices(admin.Options{Store: db, Certificates: certificates}), func() { _ = db.Close() }, nil
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
