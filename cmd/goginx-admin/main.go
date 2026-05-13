package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
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

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: goginx-admin <create-user|create-client|create-tcp-proxy|create-udp-proxy|create-http-proxy|create-https-proxy|issue-managed-certificate|renew-managed-certificate|managed-certificate-status> [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dbPath := flags.String("db", "data/go-ginx.db", "SQLite database path")
	actorID := flags.String("actor", "system", "audit actor user ID")

	switch command {
	case "create-user":
		id := flags.String("id", "", "user ID")
		username := flags.String("username", "", "username")
		role := flags.String("role", string(domain.RoleUser), "role: admin or user")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
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
	default:
		return fmt.Errorf("unknown command %q", command)
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
	dbPath := flags.Lookup("db").Value.String()
	actorID := flags.Lookup("actor").Value.String()
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
	dbPath := flags.Lookup("db").Value.String()
	actorID := flags.Lookup("actor").Value.String()
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
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return admin.Service{}, nil, err
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
