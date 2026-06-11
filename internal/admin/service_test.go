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
	"strings"
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
	if record.ClientID != result.Client.ID || record.TokenHash != enrollment.HashToken(result.Token) || record.Token != result.Token {
		t.Fatalf("unexpected enrollment record: %+v", record)
	}
	reviewed, err := service.ReviewClientJoinToken(ctx, result.Client.ID, "admin-1")
	if err != nil {
		t.Fatalf("review join token: %v", err)
	}
	reviewedAgain, err := service.ReviewClientJoinToken(ctx, result.Client.ID, "admin-1")
	if err != nil {
		t.Fatalf("review join token again: %v", err)
	}
	if reviewed.Token != result.Token || reviewedAgain.Token != result.Token || reviewed.ExpiresAt.IsZero() {
		t.Fatalf("unexpected reviewed token: first=%+v second=%+v", reviewed, reviewedAgain)
	}
	if _, err := db.Clients().ByID(ctx, result.Client.ID); err != nil {
		t.Fatalf("client was not created: %v", err)
	}
}

func TestServiceReviewClientJoinTokenResetsUnavailableTokenFromDefaults(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, []byte("default-ca-pem"), 0o644); err != nil {
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
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	usedAt := time.Now().UTC()
	if err := db.ClientEnrollments().Create(ctx, domain.ClientEnrollment{
		ID:         "join-used",
		ClientID:   client.ID,
		SecretHash: "secret-hash",
		TokenHash:  "token-hash-used",
		Token:      "goginx_join_used",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
		UsedAt:     &usedAt,
	}); err != nil {
		t.Fatalf("create enrollment: %v", err)
	}

	reviewed, err := service.ReviewClientJoinToken(ctx, client.ID, "admin-1")
	if err != nil {
		t.Fatalf("review unavailable token: %v", err)
	}
	payload, err := enrollment.DecodeToken(reviewed.Token)
	if err != nil {
		t.Fatalf("decode reset token: %v", err)
	}
	if payload.ClientID != client.ID || payload.EnrollmentURL != service.DefaultJoin.EnrollmentURL || payload.CAPEM != "default-ca-pem" || !reviewed.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected reset token payload=%+v result=%+v", payload, reviewed)
	}
	reloadedClient, err := db.Clients().ByID(ctx, client.ID)
	if err != nil {
		t.Fatalf("reload client: %v", err)
	}
	if reloadedClient.CredentialHash != domain.HashCredential(payload.Credential) {
		t.Fatalf("expected client credential to be rotated for reset token")
	}
}

func TestServiceReviewClientJoinTokenResetsExpiredToken(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "old-secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	createdAt := time.Now().UTC().Add(-2 * time.Hour)
	expiresAt := time.Now().UTC().Add(-time.Hour)
	expiredPayload := enrollment.TokenPayload{
		EnrollmentID:     "join-expired",
		Secret:           "expired-secret",
		EnrollmentURL:    "http://127.0.0.1:8080/api/client/enroll",
		ServerAddress:    "127.0.0.1:8443",
		ServerTLSAddress: "127.0.0.1:9443",
		ServerName:       "go-ginx-control.test",
		CAPEM:            "ca-pem",
		ClientID:         client.ID,
		Credential:       "old-secret",
		AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS},
		Reconnect:        config.DefaultClient().Reconnect,
		ExpiresAt:        expiresAt,
	}
	expiredToken, err := enrollment.EncodeToken(expiredPayload)
	if err != nil {
		t.Fatalf("encode expired token: %v", err)
	}
	if err := db.ClientEnrollments().Create(ctx, domain.ClientEnrollment{ID: expiredPayload.EnrollmentID, ClientID: client.ID, SecretHash: enrollment.HashSecret(expiredPayload.Secret), TokenHash: enrollment.HashToken(expiredToken), Token: expiredToken, ExpiresAt: expiresAt, CreatedAt: createdAt, UpdatedAt: createdAt}); err != nil {
		t.Fatalf("create expired enrollment: %v", err)
	}

	reviewed, err := service.ReviewClientJoinToken(ctx, client.ID, "admin-1")
	if err != nil {
		t.Fatalf("review expired token: %v", err)
	}
	if reviewed.Token == expiredToken || !reviewed.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("expected reset token with future expiry, got %+v", reviewed)
	}
	resetPayload, err := enrollment.DecodeToken(reviewed.Token)
	if err != nil {
		t.Fatalf("decode reset token: %v", err)
	}
	if resetPayload.ClientID != client.ID || resetPayload.Credential == "old-secret" || resetPayload.ServerAddress != expiredPayload.ServerAddress || resetPayload.ServerTLSAddress != expiredPayload.ServerTLSAddress {
		t.Fatalf("unexpected reset token payload: %+v", resetPayload)
	}
	reloadedClient, err := db.Clients().ByID(ctx, client.ID)
	if err != nil {
		t.Fatalf("reload client: %v", err)
	}
	if reloadedClient.CredentialHash != domain.HashCredential(resetPayload.Credential) {
		t.Fatalf("expected client credential to be rotated for reset token")
	}
}

func TestServiceReviewClientJoinTokenMigratesLegacyAdminEnrollmentURL(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db, DefaultJoin: config.JoinServiceDefaults{
		EnrollmentURL:            "http://server.example.com:8081/api/client/enroll",
		LegacyAdminEnrollmentURL: "http://server.example.com:8080/api/client/enroll",
		ServerAddress:            "server.example.com:8443",
		ServerTLSAddress:         "server.example.com:9443",
		ServerName:               "go-ginx-control.test",
	}}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	client, err := service.CreateClient(ctx, CreateClientInput{ID: "client-1", UserID: user.ID, Name: "home", Credential: "old-secret", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	createdAt := time.Now().UTC().Add(-10 * time.Minute)
	expiresAt := time.Now().UTC().Add(time.Hour)
	legacyPayload := enrollment.TokenPayload{
		EnrollmentID:     "join-legacy",
		Secret:           "legacy-secret",
		EnrollmentURL:    service.DefaultJoin.LegacyAdminEnrollmentURL,
		ServerAddress:    service.DefaultJoin.ServerAddress,
		ServerTLSAddress: service.DefaultJoin.ServerTLSAddress,
		ServerName:       service.DefaultJoin.ServerName,
		CAPEM:            "ca-pem",
		ClientID:         client.ID,
		Credential:       "old-secret",
		AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS},
		Reconnect:        config.DefaultClient().Reconnect,
		ExpiresAt:        expiresAt,
	}
	legacyToken, err := enrollment.EncodeToken(legacyPayload)
	if err != nil {
		t.Fatalf("encode legacy token: %v", err)
	}
	if err := db.ClientEnrollments().Create(ctx, domain.ClientEnrollment{ID: legacyPayload.EnrollmentID, ClientID: client.ID, SecretHash: enrollment.HashSecret(legacyPayload.Secret), TokenHash: enrollment.HashToken(legacyToken), Token: legacyToken, ExpiresAt: expiresAt, CreatedAt: createdAt, UpdatedAt: createdAt}); err != nil {
		t.Fatalf("create legacy enrollment: %v", err)
	}

	reviewed, err := service.ReviewClientJoinToken(ctx, client.ID, "admin-1")
	if err != nil {
		t.Fatalf("review legacy token: %v", err)
	}
	if reviewed.Token == legacyToken {
		t.Fatal("expected legacy admin enrollment token to be reset")
	}
	payload, err := enrollment.DecodeToken(reviewed.Token)
	if err != nil {
		t.Fatalf("decode migrated token: %v", err)
	}
	if payload.EnrollmentURL != service.DefaultJoin.EnrollmentURL || payload.ClientID != client.ID || payload.Credential == "old-secret" {
		t.Fatalf("unexpected migrated token payload: %+v", payload)
	}
}

func TestServiceReviewClientJoinTokenPreservesExplicitExternalEnrollmentURL(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, []byte("ca-pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := Service{Store: db, DefaultJoin: config.JoinServiceDefaults{
		EnrollmentURL:            "http://server.example.com:8081/api/client/enroll",
		LegacyAdminEnrollmentURL: "http://server.example.com:8080/api/client/enroll",
		ServerAddress:            "server.example.com:8443",
		ServerTLSAddress:         "server.example.com:9443",
		ServerName:               "go-ginx-control.test",
		ServerCAFile:             caFile,
	}}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	result, err := service.CreateClientJoin(ctx, CreateClientJoinInput{ID: "client-1", UserID: user.ID, Name: "home", ActorID: "admin-1", EnrollmentURL: "https://join.example.com/api/client/enroll", TTL: time.Hour})
	if err != nil {
		t.Fatalf("create explicit join token: %v", err)
	}

	reviewed, err := service.ReviewClientJoinToken(ctx, result.Client.ID, "admin-1")
	if err != nil {
		t.Fatalf("review explicit token: %v", err)
	}
	if reviewed.Token != result.Token {
		t.Fatal("expected non-legacy explicit enrollment URL token to be preserved")
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
	if err := db.ClientEnrollments().Create(ctx, domain.ClientEnrollment{ID: "join-1", ClientID: client.ID, SecretHash: "secret-hash", TokenHash: "token-hash", Token: "goginx_join_token", ExpiresAt: time.Now().UTC().Add(time.Hour)}); err != nil {
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

func TestServiceManagesProviderCredentialsSecretSafe(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	secretStore := &adminMemorySecretStore{values: make(map[string]string)}
	originClient := &adminOriginCAClient{}
	service := Service{Store: db, Certificates: certmanager.Service{ProviderSecretStore: secretStore, OriginCAClient: originClient, OriginCASettings: domain.OriginCAProviderSettings{Enabled: true}}}

	if _, err := service.CreateProviderCredential(ctx, ProviderCredentialInput{ID: "service-key", Name: "legacy", Token: "v1.0-legacy-service-key", ActorID: "admin-1"}); err == nil {
		t.Fatal("expected legacy service key to be rejected")
	}
	created, err := service.CreateProviderCredential(ctx, ProviderCredentialInput{ID: "cred-1", Name: "Production Origin CA", Scope: "Zone SSL:Edit", Token: "cf-token-1", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create provider credential: %v", err)
	}
	if created.ProviderType != domain.CertificateProviderCloudflareOriginCA || created.Status != domain.ProviderCredentialPending || created.SecretRef == "" || created.TokenFingerprint != certmanager.TokenFingerprint("cf-token-1") {
		t.Fatalf("unexpected created credential metadata: %+v", created)
	}
	if secretStore.values[created.SecretRef] != "cf-token-1" {
		t.Fatalf("expected token material in secret store, got %+v", secretStore.values)
	}
	if credentialContainsToken(created, "cf-token-1") {
		t.Fatalf("credential metadata leaked token: %+v", created)
	}

	verified, err := service.VerifyProviderCredential(ctx, created.ID, "admin-1")
	if err != nil {
		t.Fatalf("verify provider credential: %v", err)
	}
	if verified.Status != domain.ProviderCredentialVerified || verified.LastVerifiedAt == nil || len(originClient.verifiedTokens) != 1 || originClient.verifiedTokens[0] != "cf-token-1" {
		t.Fatalf("unexpected verified credential=%+v verifiedTokens=%+v", verified, originClient.verifiedTokens)
	}

	updated, err := service.UpdateProviderCredential(ctx, UpdateProviderCredentialInput{ID: created.ID, Name: "Rotated Origin CA", Scope: "Zone SSL:Read,Edit", Token: "cf-token-2", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("update provider credential: %v", err)
	}
	if updated.Name != "Rotated Origin CA" || updated.Status != domain.ProviderCredentialPending || updated.LastVerifiedAt != nil || updated.TokenFingerprint != certmanager.TokenFingerprint("cf-token-2") {
		t.Fatalf("unexpected updated credential: %+v", updated)
	}
	if secretStore.values[updated.SecretRef] != "cf-token-2" || credentialContainsToken(updated, "cf-token-2") {
		t.Fatalf("updated credential was not secret-safe: credential=%+v secrets=%+v", updated, secretStore.values)
	}

	disabled, err := service.DisableProviderCredential(ctx, created.ID, "admin-1")
	if err != nil {
		t.Fatalf("disable provider credential: %v", err)
	}
	if disabled.Status != domain.ProviderCredentialDisabled {
		t.Fatalf("unexpected disabled credential: %+v", disabled)
	}
	if err := service.DeleteProviderCredential(ctx, created.ID, "admin-1"); err != nil {
		t.Fatalf("delete provider credential: %v", err)
	}
	if _, ok := secretStore.values[created.SecretRef]; ok {
		t.Fatalf("expected deleted secret ref %q", created.SecretRef)
	}
	if _, err := db.ProviderCredentials().ByID(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected credential to be deleted, got %v", err)
	}

	events, err := db.AuditEvents().ListRecent(ctx, 20)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	actions := make(map[string]bool, len(events))
	for _, event := range events {
		actions[event.Action] = true
		if strings.Contains(event.ErrorSummary, "cf-token") || strings.Contains(event.ResourceID, "cf-token") {
			t.Fatalf("audit event leaked token: %+v", event)
		}
	}
	for _, action := range []string{"create_provider_credential", "verify_provider_credential", "update_provider_credential", "disable_provider_credential", "delete_provider_credential"} {
		if !actions[action] {
			t.Fatalf("expected audit action %q in %+v", action, events)
		}
	}
}

func TestServiceRejectsDeletingReferencedProviderCredential(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	secretStore := &adminMemorySecretStore{values: make(map[string]string)}
	service := Service{Store: db, Certificates: certmanager.Service{ProviderSecretStore: secretStore}}
	user, client := createAdminTestOwnership(ctx, t, service)
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-https", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	credential, err := service.CreateProviderCredential(ctx, ProviderCredentialInput{ID: "cred-1", Name: "Production Origin CA", Token: "cf-token", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create provider credential: %v", err)
	}
	if err := db.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-1", ProxyID: proxy.ID, Host: proxy.EntryHost, Status: domain.CertificateValid, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CredentialID: credential.ID, ProviderStatus: domain.CertificateProviderStatusActive}); err != nil {
		t.Fatalf("create managed certificate: %v", err)
	}

	err = service.DeleteProviderCredential(ctx, credential.ID, "admin-1")
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeConflict {
		t.Fatalf("expected referenced credential conflict, got %v", err)
	}
	if secretStore.values[credential.SecretRef] != "cf-token" {
		t.Fatalf("referenced credential secret should be retained, got %+v", secretStore.values)
	}
	if _, err := db.ProviderCredentials().ByID(ctx, credential.ID); err != nil {
		t.Fatalf("referenced credential metadata should be retained: %v", err)
	}
	if _, err := service.UpdateProviderCredential(ctx, UpdateProviderCredentialInput{Name: "missing id", ActorID: "admin-1"}); !errors.As(err, &contractError) || contractError.Code != contracterr.CodeValidationFailed {
		t.Fatalf("expected missing id validation error, got %v", err)
	}
}

func TestServiceProviderCredentialRequiresConfiguredSecretStore(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}

	var contractError *contracterr.Error
	if _, err := service.CreateProviderCredential(ctx, ProviderCredentialInput{ID: "cred-1", Name: "Production Origin CA", Token: "cf-token", ActorID: "admin-1"}); !errors.As(err, &contractError) || contractError.Code != contracterr.CodeUnsupported {
		t.Fatalf("expected unsupported credential storage error, got %v", err)
	}

	if err := db.ProviderCredentials().Create(ctx, domain.ProviderCredential{ID: "cred-1", Name: "Production Origin CA", ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: "Zone SSL:Edit", TokenFingerprint: "fingerprint", SecretRef: "cred-1.secret", Status: domain.ProviderCredentialPending}); err != nil {
		t.Fatalf("seed provider credential: %v", err)
	}
	if _, err := service.UpdateProviderCredential(ctx, UpdateProviderCredentialInput{ID: "cred-1", Name: "Rotated Origin CA", Token: "cf-token-2", ActorID: "admin-1"}); !errors.As(err, &contractError) || contractError.Code != contracterr.CodeUnsupported {
		t.Fatalf("expected unsupported update credential storage error, got %v", err)
	}
	if _, err := service.VerifyProviderCredential(ctx, "cred-1", "admin-1"); !errors.As(err, &contractError) || contractError.Code != contracterr.CodeUnsupported {
		t.Fatalf("expected unsupported verify credential storage error, got %v", err)
	}
}

func TestServiceVerifyProviderCredentialReturnsContractErrors(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	secretStore := &adminMemorySecretStore{values: map[string]string{"cred-1.secret": "cf-token"}}
	originClient := &adminOriginCAClient{verifyErr: errors.New("cloudflare origin ca request failed: status 401")}
	service := Service{Store: db, Certificates: certmanager.Service{ProviderSecretStore: secretStore, OriginCAClient: originClient}}
	if err := db.ProviderCredentials().Create(ctx, domain.ProviderCredential{ID: "cred-1", Name: "Production Origin CA", ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: "Zone SSL:Edit", TokenFingerprint: "fingerprint", SecretRef: "cred-1.secret", Status: domain.ProviderCredentialPending}); err != nil {
		t.Fatalf("seed provider credential: %v", err)
	}

	var contractError *contracterr.Error
	if _, err := service.VerifyProviderCredential(ctx, "cred-1", "admin-1"); !errors.As(err, &contractError) || contractError.Code != contracterr.CodeConflict {
		t.Fatalf("expected conflict verification error, got %v", err)
	}
	credential, err := db.ProviderCredentials().ByID(ctx, "cred-1")
	if err != nil {
		t.Fatalf("read provider credential: %v", err)
	}
	if credential.Status != domain.ProviderCredentialVerificationFailed || credential.LastError != "cloudflare origin ca request failed: status 401" {
		t.Fatalf("expected verification failure metadata, got %+v", credential)
	}

	if err := db.ProviderCredentials().SetStatus(ctx, "cred-1", domain.ProviderCredentialDisabled, nil, ""); err != nil {
		t.Fatalf("disable provider credential: %v", err)
	}
	originClient.verifyErr = nil
	if _, err := service.VerifyProviderCredential(ctx, "cred-1", "admin-1"); !errors.As(err, &contractError) || contractError.Code != contracterr.CodeConflict || contractError.Message != "provider credential is disabled" {
		t.Fatalf("expected disabled credential conflict, got %v", err)
	}
}

func TestServiceAuditsOriginCALifecycleActions(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	secretStore := &adminMemorySecretStore{values: make(map[string]string)}
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	createCount := 0
	originClient := &adminOriginCAClient{
		create: func(ctx context.Context, token string, request certmanager.OriginCACreateRequest) (certmanager.OriginCACertificate, error) {
			createCount++
			id := "cf-cert-1"
			if createCount == 2 {
				id = "cf-cert-2"
			}
			return certmanager.OriginCACertificate{ID: id, CertificatePEM: adminOriginCATestCertificateFromCSR(t, request.CSR, request.Hostnames, now.Add(time.Duration(createCount)*365*24*time.Hour)), Hostnames: request.Hostnames, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity, Status: "active"}, nil
		},
		get: func(ctx context.Context, token string, certificateID string) (certmanager.OriginCACertificate, error) {
			return certmanager.OriginCACertificate{ID: certificateID, Status: "active"}, nil
		},
		revoke: func(ctx context.Context, token string, certificateID string) error {
			return nil
		},
	}
	service := Service{Store: db, Certificates: certmanager.Service{ProviderSecretStore: secretStore, OriginCAClient: originClient, Storage: certmanagerTestStorage(t), OriginCASettings: domain.OriginCAProviderSettings{Enabled: true, DefaultRequestType: certmanager.OriginCARequestTypeECC, RequestedValidity: 365}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return now }}}
	user, client := createAdminTestOwnership(ctx, t, service)
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-https", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create https proxy: %v", err)
	}
	credential, err := service.CreateProviderCredential(ctx, ProviderCredentialInput{ID: "cred-1", Name: "Production Origin CA", Token: "cf-token", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create provider credential: %v", err)
	}

	issued, err := service.IssueManagedCertificate(ctx, CertificateInput{ProxyID: proxy.ID, ProviderType: domain.CertificateProviderCloudflareOriginCA, CredentialID: credential.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("issue origin ca certificate: %v", err)
	}
	if issued.CloudflareCertificateID != "cf-cert-1" {
		t.Fatalf("unexpected issued origin ca certificate: %+v", issued)
	}
	rotated, err := service.RotateOriginCACertificate(ctx, CertificateInput{ProxyID: proxy.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("rotate origin ca certificate: %v", err)
	}
	if rotated.CloudflareCertificateID != "cf-cert-2" || rotated.PreviousCloudflareCertificateID != "cf-cert-1" {
		t.Fatalf("unexpected rotated origin ca certificate: %+v", rotated)
	}
	if _, err := service.SyncOriginCACertificate(ctx, CertificateInput{ProxyID: proxy.ID, ActorID: "admin-1"}); err != nil {
		t.Fatalf("sync origin ca certificate: %v", err)
	}
	if _, err := service.RevokeOriginCACertificate(ctx, RevokeOriginCACertificateInput{ProxyID: proxy.ID, Host: "app.example.com", CloudflareCertificateID: "cf-cert-1", ActorID: "admin-1"}); err != nil {
		t.Fatalf("revoke previous origin ca certificate: %v", err)
	}

	events, err := db.AuditEvents().ListRecent(ctx, 30)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	actions := make(map[string]bool, len(events))
	for _, event := range events {
		actions[event.Action] = true
	}
	for _, action := range []string{"issue_cloudflare_origin_certificate", "rotate_cloudflare_origin_certificate", "sync_cloudflare_origin_certificate", "revoke_cloudflare_origin_certificate"} {
		if !actions[action] {
			t.Fatalf("expected audit action %q in %+v", action, events)
		}
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

func TestServiceDeletesUserWithoutDependentResources(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db}
	user, err := service.CreateUser(ctx, CreateUserInput{ID: "user-1", Username: "alice", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := service.DeleteUser(ctx, user.ID, "admin-1"); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := db.Users().ByID(ctx, user.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected deleted user not found, got %v", err)
	}
}

func TestServiceDeleteUserRejectsDependentResources(t *testing.T) {
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
	err = service.DeleteUser(ctx, user.ID, "admin-1")
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeConflict {
		t.Fatalf("expected conflict deleting user with dependencies, got %v", err)
	}
	if _, err := db.Users().ByID(ctx, user.ID); err != nil {
		t.Fatalf("user should remain after rejected delete: %v", err)
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
		{Network: domain.ListenerNetworkTCP, Port: 10081, Source: "client_enrollment_listen", ResourceID: "client_enrollment_listen"},
		{Network: domain.ListenerNetworkUDP, Port: 10053, Source: "control_quic_listen", ResourceID: "control_quic_listen"},
	}}
	user, client := createAdminTestOwnership(ctx, t, service)

	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-static-conflict", UserID: user.ID, ClientID: client.ID, Name: "ssh", Type: domain.ProxyTCP, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected tcp static listener conflict, got %v", err)
	}
	if _, err := service.CreateProxy(ctx, CreateProxyInput{ID: "tcp-enrollment-conflict", UserID: user.ID, ClientID: client.ID, Name: "join-port", Type: domain.ProxyTCP, EntryPort: 10081, TargetHost: "127.0.0.1", TargetPort: 8081, ActorID: "admin-1"}); !errors.Is(err, domain.ErrEntryConflict) {
		t.Fatalf("expected enrollment static listener conflict, got %v", err)
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

func TestCreateProxyReconcileFailureRollsBackCreatedProxy(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	reconciler := &fakeProxyListenerReconciler{err: errors.New("bind failed")}
	service := Service{Store: db, ListenerReconciler: reconciler}
	user, client := createAdminTestOwnership(ctx, t, service)

	_, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "ssh", Type: domain.ProxyTCP, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22, ActorID: "admin-1"})
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeEntryConflict {
		t.Fatalf("expected entry conflict reconcile error, got %v", err)
	}
	if reconciler.calls == 0 {
		t.Fatal("expected reconciler to be called")
	}
	if _, err := db.Proxies().ByID(ctx, "proxy-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected proxy rollback, got %v", err)
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

type fakeProxyListenerReconciler struct {
	calls int
	err   error
}

func (reconciler *fakeProxyListenerReconciler) ReconcileProxyListeners(context.Context) error {
	reconciler.calls++
	return reconciler.err
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

type adminMemorySecretStore struct {
	values map[string]string
}

func (store *adminMemorySecretStore) Write(_ context.Context, credentialID string, material string) (string, error) {
	ref := credentialID + ".secret"
	store.values[ref] = material
	return ref, nil
}

func (store *adminMemorySecretStore) Read(_ context.Context, secretRef string) (string, error) {
	value, ok := store.values[secretRef]
	if !ok {
		return "", errors.New("secret material is missing")
	}
	return value, nil
}

func (store *adminMemorySecretStore) Delete(_ context.Context, secretRef string) error {
	delete(store.values, secretRef)
	return nil
}

type adminOriginCAClient struct {
	verifiedTokens []string
	verifyErr      error
	create         func(context.Context, string, certmanager.OriginCACreateRequest) (certmanager.OriginCACertificate, error)
	get            func(context.Context, string, string) (certmanager.OriginCACertificate, error)
	revoke         func(context.Context, string, string) error
}

func (client *adminOriginCAClient) Create(ctx context.Context, token string, request certmanager.OriginCACreateRequest) (certmanager.OriginCACertificate, error) {
	if client.create != nil {
		return client.create(ctx, token, request)
	}
	return certmanager.OriginCACertificate{}, errors.New("unexpected origin ca create")
}

func (client *adminOriginCAClient) Get(ctx context.Context, token string, certificateID string) (certmanager.OriginCACertificate, error) {
	if client.get != nil {
		return client.get(ctx, token, certificateID)
	}
	return certmanager.OriginCACertificate{}, errors.New("unexpected origin ca get")
}

func (client *adminOriginCAClient) List(context.Context, string) ([]certmanager.OriginCACertificate, error) {
	return nil, errors.New("unexpected origin ca list")
}

func (client *adminOriginCAClient) Revoke(ctx context.Context, token string, certificateID string) error {
	if client.revoke != nil {
		return client.revoke(ctx, token, certificateID)
	}
	return errors.New("unexpected origin ca revoke")
}

func (client *adminOriginCAClient) VerifyToken(_ context.Context, token string) error {
	client.verifiedTokens = append(client.verifiedTokens, token)
	return client.verifyErr
}

func credentialContainsToken(credential domain.ProviderCredential, token string) bool {
	return strings.Contains(credential.ID, token) ||
		strings.Contains(credential.Name, token) ||
		strings.Contains(credential.Scope, token) ||
		strings.Contains(credential.TokenFingerprint, token) ||
		strings.Contains(credential.SecretRef, token) ||
		strings.Contains(credential.LastError, token)
}

func adminOriginCATestCertificateFromCSR(t *testing.T, csrPEM string, hostnames []string, notAfter time.Time) []byte {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		t.Fatal("decode csr")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse csr: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("check csr signature: %v", err)
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: hostnames[0]}, DNSNames: hostnames, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, csr.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create origin ca test certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
