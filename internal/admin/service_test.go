package admin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServiceCreatesMilestoneOneResources(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}

	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Role != domain.RoleUser || user.Status != domain.UserEnabled {
		t.Fatalf("unexpected user: %+v", user)
	}

	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if client.CredentialHash == "secret" || client.CredentialHash == "" {
		t.Fatalf("credential was not hashed: %+v", client)
	}

	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "web", Type: domain.ProxyHTTP, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	found, err := db.Proxies().ByHTTPHost(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("lookup proxy: %v", err)
	}
	if found.ID != proxy.ID || found.Status != domain.ProxyEnabled {
		t.Fatalf("unexpected proxy: %+v", found)
	}
	udpProxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "udp-1", UserID: user.ID, ClientID: client.ID, Name: "dns", Type: domain.ProxyUDP, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 53, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create udp proxy: %v", err)
	}
	foundUDP, err := db.Proxies().ByUDPEntryPort(ctx, 10053)
	if err != nil {
		t.Fatalf("lookup udp proxy: %v", err)
	}
	if foundUDP.ID != udpProxy.ID || foundUDP.Status != domain.ProxyEnabled {
		t.Fatalf("unexpected udp proxy: %+v", foundUDP)
	}
}

func TestServiceCreatesClientJoinToken(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, []byte("ca-pem"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := service.CreateClientJoin(ctx, CreateClientJoinInput{ID: "client-join-1", UserID: user.ID, Name: "home", ActorID: "admin-1", EnrollmentURL: "http://127.0.0.1:8080/api/client/enroll", ServerAddress: "127.0.0.1:8443", ServerTLSAddress: "127.0.0.1:9443", ServerName: "go-ginx-control.test", ServerCAFile: caFile, TTL: time.Hour})
	if err != nil {
		t.Fatalf("create client join: %v", err)
	}
	if result.Client.ID != "client-join-1" || result.Token == "" {
		t.Fatalf("unexpected client join result: %+v", result)
	}
	payload, err := enrollment.DecodeToken(result.Token)
	if err != nil {
		t.Fatalf("decode join token: %v", err)
	}
	if payload.ClientID != result.Client.ID || payload.Credential == "" || payload.ServerName != "go-ginx-control.test" {
		t.Fatalf("unexpected join token payload: %+v", payload)
	}
	record, err := db.ClientEnrollments().ByID(ctx, payload.EnrollmentID)
	if err != nil {
		t.Fatalf("load enrollment record: %v", err)
	}
	if record.ClientID != result.Client.ID || record.TokenHash != enrollment.HashToken(result.Token) {
		t.Fatalf("unexpected enrollment record: %+v", record)
	}
	if _, err := db.Clients().ByID(ctx, result.Client.ID); err != nil {
		t.Fatalf("client was not created: %v", err)
	}
}

func TestServiceCreatesClientJoinTokenFromDefaultJoin(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, []byte("ca-pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{Store: db, DefaultJoin: config.JoinServiceDefaults{
		EnrollmentURL:    "http://server.example.com:8080/api/client/enroll",
		ServerAddress:    "server.example.com:8443",
		ServerTLSAddress: "server.example.com:9443",
		ServerName:       "go-ginx-control.test",
		ServerCAFile:     caFile,
	}}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	result, err := service.CreateClientJoin(ctx, CreateClientJoinInput{ID: "client-join-1", UserID: user.ID, Name: "home", ActorID: "admin-1", TTL: time.Hour})
	if err != nil {
		t.Fatalf("create client join: %v", err)
	}
	payload, err := enrollment.DecodeToken(result.Token)
	if err != nil {
		t.Fatalf("decode join token: %v", err)
	}
	if payload.EnrollmentURL != service.DefaultJoin.EnrollmentURL || payload.ServerAddress != service.DefaultJoin.ServerAddress || payload.ServerTLSAddress != service.DefaultJoin.ServerTLSAddress || payload.ServerName != service.DefaultJoin.ServerName {
		t.Fatalf("join token did not use defaults: %+v", payload)
	}
}

func TestServiceRejectsInvalidMilestoneOneInputs(t *testing.T) {
	ctx := context.Background()
	service := Service{Store: openTestStore(t)}

	if _, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: "user-1", Name: "home"}); err == nil {
		t.Fatal("expected missing credential error")
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: "user-1", ClientID: "client-1", Name: "tcp", Type: domain.ProxyTCP, TargetHost: "127.0.0.1", TargetPort: 22}); err == nil {
		t.Fatal("expected missing TCP entry port error")
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: "user-1", ClientID: "client-1", Name: "udp", Type: domain.ProxyUDP, TargetHost: "127.0.0.1", TargetPort: 53}); err == nil {
		t.Fatal("expected missing UDP entry port error")
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: "user-1", ClientID: "client-1", Name: "http", Type: domain.ProxyHTTP, TargetHost: "127.0.0.1", TargetPort: 8080}); err == nil {
		t.Fatal("expected missing HTTP entry host error")
	}
}

func TestServicePropagatesDuplicateUser(t *testing.T) {
	ctx := context.Background()
	service := Service{Store: openTestStore(t)}
	input := CreateUserInput{ID: "user-1", Username: "alice"}
	if _, err := service.CreateUser(ctx, input); err != nil {
		t.Fatalf("create first user: %v", err)
	}
	if _, err := service.CreateUser(ctx, input); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestServiceDeleteClientRemovesDisabledClientResources(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "web", Type: domain.ProxyHTTP, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if err := service.DisableProxy(ctx, proxy.ID, "admin-1"); err != nil {
		t.Fatalf("disable proxy: %v", err)
	}
	if err := db.ClientEnrollments().Create(ctx, domain.ClientEnrollment{ID: "join-1", ClientID: client.ID, SecretHash: "secret-hash", TokenHash: "token-hash", ExpiresAt: time.Now().UTC().Add(time.Hour)}); err != nil {
		t.Fatalf("create enrollment: %v", err)
	}

	if err := service.DeleteClient(ctx, client.ID, "admin-1"); err != nil {
		t.Fatalf("delete client: %v", err)
	}
	if _, err := db.Clients().ByID(ctx, client.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected deleted client not found, got %v", err)
	}
	if _, err := db.Proxies().ByID(ctx, proxy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected disabled proxy cascade delete, got %v", err)
	}
	if _, err := db.ClientEnrollments().ByID(ctx, "join-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected enrollment cascade delete, got %v", err)
	}
}

func TestServiceDeleteClientRejectsEnabledProxy(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "web", Type: domain.ProxyHTTP, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"}); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	err = service.DeleteClient(ctx, client.ID, "admin-1")
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeConflict {
		t.Fatalf("expected conflict deleting client with enabled proxy, got %v", err)
	}
	if _, err := db.Clients().ByID(ctx, client.ID); err != nil {
		t.Fatalf("client should remain after rejected delete: %v", err)
	}
}

func TestServiceIssuesRenewsAndReportsManagedCertificate(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db, Certificates: certmanager.Service{Issuer: adminFakeIssuer{}, DNSProvider: adminFakeDNSProvider{}, Storage: certmanagerTestStorage(t), Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return time.Now().UTC() }}}

	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	certificate, err := service.IssueManagedCertificate(ctx, CertificateInput{ProxyID: proxy.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	if certificate.Status != domain.CertificateValid {
		t.Fatalf("unexpected issued certificate: %+v", certificate)
	}
	renewed, err := service.RenewManagedCertificate(ctx, CertificateInput{ProxyID: proxy.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("renew certificate: %v", err)
	}
	if renewed.Status != domain.CertificateValid {
		t.Fatalf("unexpected renewed certificate: %+v", renewed)
	}
	status, err := service.ManagedCertificateStatus(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("status certificate: %v", err)
	}
	if status.Certificate.ID != renewed.ID {
		t.Fatalf("unexpected certificate status: %+v", status.Certificate)
	}
}

func TestServiceDisablesUserAndSetsPassword(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", Password: "secret-1", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.PasswordHash == "" || !domain.CheckPasswordHash("secret-1", user.PasswordHash) {
		t.Fatalf("expected password hash on created user: %+v", user)
	}
	if err := service.DisableUser(ctx, user.ID, "admin-1"); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	disabled, err := db.Users().ByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("reload disabled user: %v", err)
	}
	if disabled.Status != domain.UserDisabled {
		t.Fatalf("expected disabled user, got %+v", disabled)
	}
	if err := service.SetUserPassword(ctx, user.ID, "secret-2", "admin-1"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	updated, err := db.Users().ByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("reload password user: %v", err)
	}
	if !domain.CheckPasswordHash("secret-2", updated.PasswordHash) {
		t.Fatalf("expected updated password hash: %+v", updated)
	}
}

func TestServiceEnforcesProxyLifecycleRules(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "web", Type: domain.ProxyHTTP, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if _, err := service.UpdateProxy(ctx, UpdateProxyInput{ID: proxy.ID, Type: domain.ProxyTCP, Name: proxy.Name, EntryPort: 10022, TargetHost: proxy.TargetHost, TargetPort: 22, ActorID: "admin-1"}); err == nil {
		t.Fatal("expected immutable proxy type error")
	}
	updated, err := service.UpdateProxy(ctx, UpdateProxyInput{ID: proxy.ID, Type: proxy.Type, Name: "web-updated", EntryHost: "api.example.com", TargetHost: "127.0.0.1", TargetPort: 8081, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("update proxy: %v", err)
	}
	if updated.Name != "web-updated" || updated.EntryHost != "api.example.com" || updated.TargetPort != 8081 {
		t.Fatalf("unexpected updated proxy: %+v", updated)
	}
	if err := service.DeleteProxy(ctx, proxy.ID, "admin-1"); err == nil {
		t.Fatal("expected delete-before-disable rejection")
	}
	if err := service.DisableProxy(ctx, proxy.ID, "admin-1"); err != nil {
		t.Fatalf("disable proxy: %v", err)
	}
	disabled, err := db.Proxies().ByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("reload disabled proxy: %v", err)
	}
	if disabled.Status != domain.ProxyDisabled {
		t.Fatalf("expected disabled proxy, got %+v", disabled)
	}
	if err := service.DeleteProxy(ctx, proxy.ID, "admin-1"); err != nil {
		t.Fatalf("delete proxy: %v", err)
	}
	if _, err := db.Proxies().ByID(ctx, proxy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected deleted proxy not found, got %v", err)
	}
}

func TestServiceListenerAdmissionRejectsStaticListenerConflicts(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db, StaticListenerClaims: []domain.ListenerClaim{
		{Network: domain.ListenerNetworkTCP, Port: 10022, Source: "admin_listen", ResourceID: "admin_listen"},
		{Network: domain.ListenerNetworkUDP, Port: 10053, Source: "control_quic_listen", ResourceID: "control_quic_listen"},
	}}
	user, client := createAdminTestOwnership(ctx, t, service)

	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-static-conflict", UserID: user.ID, ClientID: client.ID, Name: "ssh", Type: domain.ProxyTCP, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected tcp static listener conflict, got %v", err)
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "udp-static-conflict", UserID: user.ID, ClientID: client.ID, Name: "dns", Type: domain.ProxyUDP, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 53, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected udp static listener conflict, got %v", err)
	}
}

func TestServiceListenerAdmissionUsesEnabledProxyClaims(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, client := createAdminTestOwnership(ctx, t, service)

	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-active", UserID: user.ID, ClientID: client.ID, Name: "ssh", Type: domain.ProxyTCP, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22, ActorID: "admin-1"}); err != nil {
		t.Fatalf("create active tcp proxy: %v", err)
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "udp-active", UserID: user.ID, ClientID: client.ID, Name: "dns", Type: domain.ProxyUDP, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 53, ActorID: "admin-1"}); err != nil {
		t.Fatalf("create active udp proxy: %v", err)
	}

	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-conflict", UserID: user.ID, ClientID: client.ID, Name: "ssh-2", Type: domain.ProxyTCP, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 2222, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected tcp enabled-proxy conflict, got %v", err)
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "udp-conflict", UserID: user.ID, ClientID: client.ID, Name: "dns-2", Type: domain.ProxyUDP, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 5353, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected udp enabled-proxy conflict, got %v", err)
	}
}

func TestServiceListenerAdmissionAllowsDisabledProxyEdits(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db, StaticListenerClaims: []domain.ListenerClaim{
		{Network: domain.ListenerNetworkTCP, Port: 10022, Source: "admin_listen", ResourceID: "admin_listen"},
		{Network: domain.ListenerNetworkUDP, Port: 10053, Source: "control_quic_listen", ResourceID: "control_quic_listen"},
	}}
	user, client := createAdminTestOwnership(ctx, t, service)

	if err := db.Proxies().Create(ctx, domain.Proxy{ID: "tcp-disabled", UserID: user.ID, ClientID: client.ID, Name: "disabled-tcp", Type: domain.ProxyTCP, Status: domain.ProxyDisabled, EntryPort: 10024, TargetHost: "127.0.0.1", TargetPort: 24}); err != nil {
		t.Fatalf("seed disabled tcp proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, domain.Proxy{ID: "udp-disabled", UserID: user.ID, ClientID: client.ID, Name: "disabled-udp", Type: domain.ProxyUDP, Status: domain.ProxyDisabled, EntryPort: 10054, TargetHost: "127.0.0.1", TargetPort: 54}); err != nil {
		t.Fatalf("seed disabled udp proxy: %v", err)
	}

	updatedTCP, err := service.UpdateProxy(ctx, UpdateProxyInput{ID: "tcp-disabled", Type: domain.ProxyTCP, Name: "disabled-tcp-updated", EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 24, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("update disabled tcp proxy: %v", err)
	}
	if updatedTCP.EntryPort != 10022 || updatedTCP.Status != domain.ProxyDisabled {
		t.Fatalf("unexpected disabled tcp update: %+v", updatedTCP)
	}
	updatedUDP, err := service.UpdateProxy(ctx, UpdateProxyInput{ID: "udp-disabled", Type: domain.ProxyUDP, Name: "disabled-udp-updated", EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 54, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("update disabled udp proxy: %v", err)
	}
	if updatedUDP.EntryPort != 10053 || updatedUDP.Status != domain.ProxyDisabled {
		t.Fatalf("unexpected disabled udp update: %+v", updatedUDP)
	}
}

func TestServiceListenerAdmissionCoversCreateUpdateAndEnable(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db, StaticListenerClaims: []domain.ListenerClaim{{Network: domain.ListenerNetworkTCP, Port: 10030, Source: "http_entry_listen", ResourceID: "http_entry_listen"}}}
	user, client := createAdminTestOwnership(ctx, t, service)

	created, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-create-success", UserID: user.ID, ClientID: client.ID, Name: "ssh", Type: domain.ProxyTCP, EntryPort: 10031, TargetHost: "127.0.0.1", TargetPort: 22, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy success: %v", err)
	}
	if created.EntryPort != 10031 {
		t.Fatalf("unexpected created proxy: %+v", created)
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-create-failure", UserID: user.ID, ClientID: client.ID, Name: "ssh-conflict", Type: domain.ProxyTCP, EntryPort: 10030, TargetHost: "127.0.0.1", TargetPort: 2200, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected create conflict, got %v", err)
	}

	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-update-peer", UserID: user.ID, ClientID: client.ID, Name: "ssh-peer", Type: domain.ProxyTCP, EntryPort: 10032, TargetHost: "127.0.0.1", TargetPort: 2222, ActorID: "admin-1"}); err != nil {
		t.Fatalf("create peer proxy: %v", err)
	}
	updated, err := service.UpdateProxy(ctx, UpdateProxyInput{ID: created.ID, Type: domain.ProxyTCP, Name: "ssh-renamed", EntryPort: 10031, TargetHost: "127.0.0.1", TargetPort: 23, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("update proxy self replacement: %v", err)
	}
	if updated.Name != "ssh-renamed" || updated.EntryPort != 10031 || updated.TargetPort != 23 {
		t.Fatalf("unexpected updated proxy: %+v", updated)
	}
	if _, err := service.UpdateProxy(ctx, UpdateProxyInput{ID: created.ID, Type: domain.ProxyTCP, Name: "ssh-conflict", EntryPort: 10032, TargetHost: "127.0.0.1", TargetPort: 23, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected update conflict, got %v", err)
	}

	if err := db.Proxies().Create(ctx, domain.Proxy{ID: "tcp-enable-success", UserID: user.ID, ClientID: client.ID, Name: "disabled-ok", Type: domain.ProxyTCP, Status: domain.ProxyDisabled, EntryPort: 10033, TargetHost: "127.0.0.1", TargetPort: 33}); err != nil {
		t.Fatalf("seed enable success proxy: %v", err)
	}
	if err := db.Proxies().Create(ctx, domain.Proxy{ID: "tcp-enable-failure", UserID: user.ID, ClientID: client.ID, Name: "disabled-conflict", Type: domain.ProxyTCP, Status: domain.ProxyDisabled, EntryPort: 10030, TargetHost: "127.0.0.1", TargetPort: 30}); err != nil {
		t.Fatalf("seed enable conflict proxy: %v", err)
	}
	if err := service.EnableProxy(ctx, "tcp-enable-success", "admin-1"); err != nil {
		t.Fatalf("enable proxy success: %v", err)
	}
	if err := service.EnableProxy(ctx, "tcp-enable-failure", "admin-1"); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected enable conflict, got %v", err)
	}
}

func createAdminTestOwnership(ctx context.Context, t *testing.T, service Service) (domain.User, domain.Client) {
	t.Helper()
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return user, client
}

type adminFakeDNSProvider struct{}

func (adminFakeDNSProvider) Present(context.Context, string, string) error { return nil }
func (adminFakeDNSProvider) CleanUp(context.Context, string, string) error { return nil }

type adminFakeIssuer struct{}

func (adminFakeIssuer) Issue(context.Context, certmanager.IssueRequest) (certmanager.IssuedCertificate, error) {
	certPEM, keyPEM, notAfter := adminTestCertificatePEM("app.example.com", time.Now().Add(time.Hour))
	return certmanager.IssuedCertificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

func openTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func certmanagerTestStorage(t *testing.T) httpsproxy.ManagedCertificateStorage {
	t.Helper()
	return httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir(), Now: func() time.Time { return time.Now().UTC() }}
}

func adminTestCertificatePEM(host string, notAfter time.Time) ([]byte, []byte, time.Time) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, notAfter
}
