package certmanager

import (
	"bytes"
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

	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServiceIssuesManagedCertificate(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	certPEM, keyPEM := testCertificatePEM(t, "app.example.com", time.Now().Add(time.Hour))
	service := Service{Store: db, Issuer: fakeIssuer{certPEM: certPEM, keyPEM: keyPEM}, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}

	certificate, err := service.Issue(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	if certificate.Status != domain.CertificateValid || certificate.CertFile == "" || certificate.KeyFile == "" || certificate.NotAfter == nil {
		t.Fatalf("unexpected certificate: %+v", certificate)
	}
	if certificate.ServingStatus != domain.CertificateServingUsable || certificate.OperationStatus != domain.CertificateOperationIdle || certificate.Fingerprint == "" || certificate.FailureCount != 0 || certificate.NextAttemptAt != nil {
		t.Fatalf("unexpected lifecycle metadata: %+v", certificate)
	}
	if _, err := service.Status(ctx, "proxy-1"); err != nil {
		t.Fatalf("status: %v", err)
	}
}

func TestServiceIssueCertificateCreatesUnboundACMECertificate(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certPEM, keyPEM := testCertificatePEM(t, "free.example.com", time.Now().Add(time.Hour))
	service := Service{Store: db, Issuer: fakeIssuer{certPEM: certPEM, keyPEM: keyPEM}, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-unbound-1", nil }}

	certificate, err := service.IssueCertificate(ctx, CertificateIssueRequest{Host: "free.example.com", ProviderType: domain.CertificateProviderACMEDNS01})
	if err != nil {
		t.Fatalf("issue unbound certificate: %v", err)
	}
	if certificate.ID != "cert-unbound-1" || certificate.ProxyID != "" {
		t.Fatalf("expected unbound certificate resource, got %+v", certificate)
	}
	if certificate.Status != domain.CertificateValid || certificate.CertFile == "" || certificate.KeyFile == "" || certificate.NotAfter == nil {
		t.Fatalf("unexpected unbound certificate: %+v", certificate)
	}
	loaded, err := db.Certificates().ByID(ctx, certificate.ID)
	if err != nil {
		t.Fatalf("load unbound certificate: %v", err)
	}
	if loaded.ProxyID != "" {
		t.Fatalf("expected persisted certificate to remain unbound, got %+v", loaded)
	}
}

func TestServiceRecordsIssueFailure(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	service := Service{Store: db, Issuer: fakeIssuer{err: errors.New("dns failed")}, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}

	if _, err := service.Issue(ctx, "proxy-1"); err == nil {
		t.Fatal("expected issue failure")
	}
	certificate, err := db.Certificates().ByProxyID(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("lookup certificate: %v", err)
	}
	if certificate.Status != domain.CertificateIssueFailed || certificate.ServingStatus != domain.CertificateServingMissing || certificate.OperationStatus != domain.CertificateOperationIssueFailed || certificate.LastError != "dns failed" || certificate.FailureCount != 1 || certificate.NextAttemptAt == nil || certificate.LastAttemptedAt == nil {
		t.Fatalf("unexpected failure metadata: %+v", certificate)
	}
}

func TestServiceRecordsProviderCredentialFailure(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	service := Service{Store: db, Issuer: providerCheckingIssuer{}, DNSProvider: failingDNSProvider{err: errors.New("cloudflare token environment variable CF_DNS_API_TOKEN is not set")}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}

	if _, err := service.Issue(ctx, "proxy-1"); err == nil {
		t.Fatal("expected provider credential failure")
	}
	certificate, err := db.Certificates().ByProxyID(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("lookup certificate: %v", err)
	}
	if certificate.OperationStatus != domain.CertificateOperationIssueFailed || certificate.ServingStatus != domain.CertificateServingMissing || certificate.NextAttemptAt == nil || certificate.FailureCount != 1 {
		t.Fatalf("unexpected provider failure metadata: %+v", certificate)
	}
	if certificate.LastError == "" || certificate.LastError == "CF_DNS_API_TOKEN" {
		t.Fatalf("unexpected provider failure error summary: %+v", certificate)
	}
}

func TestServiceVerifyProviderCredentialDoesNotMutateDisabledCredential(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	credential := domain.ProviderCredential{ID: "cred-1", Name: "Cloudflare Origin", ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: "Zone SSL:Edit", TokenFingerprint: TokenFingerprint("cf-api-token"), SecretRef: "cred-1.secret", Status: domain.ProviderCredentialDisabled, LastError: "disabled by admin"}
	if err := db.ProviderCredentials().Create(ctx, credential); err != nil {
		t.Fatalf("create provider credential: %v", err)
	}
	service := Service{Store: db, ProviderSecretStore: testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}, OriginCAClient: testOriginCAClient{verify: func(context.Context, string) error {
		t.Fatal("disabled credential should not call origin ca verify")
		return nil
	}}, Now: func() time.Time { return now }}

	if err := service.VerifyProviderCredential(ctx, "cred-1"); !errors.Is(err, ErrProviderCredentialDisabled) {
		t.Fatalf("expected disabled credential error, got %v", err)
	}
	found, err := db.ProviderCredentials().ByID(ctx, "cred-1")
	if err != nil {
		t.Fatalf("read provider credential: %v", err)
	}
	if found.Status != domain.ProviderCredentialDisabled || found.LastError != "disabled by admin" || found.LastVerifiedAt != nil {
		t.Fatalf("disabled credential should not be mutated, got %+v", found)
	}
}

func TestServiceVerifyProviderCredentialCanRetryVerificationFailedCredential(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	now := time.Date(2026, 6, 11, 10, 5, 0, 0, time.UTC)
	credential := domain.ProviderCredential{ID: "cred-1", Name: "Cloudflare Origin", ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: "Zone SSL:Edit", TokenFingerprint: TokenFingerprint("cf-api-token"), SecretRef: "cred-1.secret", Status: domain.ProviderCredentialVerificationFailed, LastError: "cloudflare origin ca request failed: status 401"}
	if err := db.ProviderCredentials().Create(ctx, credential); err != nil {
		t.Fatalf("create provider credential: %v", err)
	}
	service := Service{Store: db, ProviderSecretStore: testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}, OriginCAClient: testOriginCAClient{}, Now: func() time.Time { return now }}
	if _, err := service.readSecretToken(ctx, credential, false); !errors.Is(err, ErrProviderCredentialVerificationFailed) {
		t.Fatalf("expected certificate use to reject verification failed credential, got %v", err)
	}

	if err := service.VerifyProviderCredential(ctx, "cred-1"); err != nil {
		t.Fatalf("verify provider credential retry: %v", err)
	}
	found, err := db.ProviderCredentials().ByID(ctx, "cred-1")
	if err != nil {
		t.Fatalf("read provider credential: %v", err)
	}
	if found.Status != domain.ProviderCredentialVerified || found.LastError != "" || found.LastVerifiedAt == nil || !found.LastVerifiedAt.Equal(now) {
		t.Fatalf("expected retry to verify credential, got %+v", found)
	}
}

func TestServiceRejectsUnsupportedProviderBeforeMutation(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	service := Service{Store: db, NewID: func() (string, error) { return "cert-1", nil }}

	if _, err := service.IssueWithProvider(ctx, ManagedCertificateRequest{ProxyID: "proxy-1", ProviderType: domain.CertificateProviderType("unsupported")}); err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if _, err := db.Certificates().ByProxyID(ctx, "proxy-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unsupported provider should not create certificate record, got %v", err)
	}
}

func TestServiceRenewsAndRetainsPreviousFiles(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	firstCert, firstKey := testCertificatePEM(t, "app.example.com", time.Now().Add(time.Hour))
	secondCert, secondKey := testCertificatePEM(t, "app.example.com", time.Now().Add(2*time.Hour))
	issuer := &sequenceIssuer{certs: []IssuedCertificate{{CertPEM: firstCert, KeyPEM: firstKey}, {CertPEM: secondCert, KeyPEM: secondKey}}}
	service := Service{Store: db, Issuer: issuer, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}

	issued, err := service.Issue(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	renewed, err := service.Renew(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("renew certificate: %v", err)
	}
	if renewed.PreviousCertFile == "" || renewed.PreviousKeyFile == "" || renewed.CertFile != issued.CertFile || renewed.KeyFile != issued.KeyFile {
		t.Fatalf("expected renewal to retain previous files: issued=%+v renewed=%+v", issued, renewed)
	}
}

func TestServiceRenewalFailurePreservesServingCertificate(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	firstCert, firstKey := testCertificatePEM(t, "app.example.com", time.Now().Add(time.Hour))
	issuer := &sequenceIssuer{certs: []IssuedCertificate{{CertPEM: firstCert, KeyPEM: firstKey}}}
	service := Service{Store: db, Issuer: issuer, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare", RenewalWindow: 30 * time.Minute}, NewID: func() (string, error) { return "cert-1", nil }}

	issued, err := service.Issue(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	issuer.err = errors.New("dns failed")
	if _, err := service.Renew(ctx, "proxy-1"); err == nil {
		t.Fatal("expected renewal failure")
	}
	certificate, err := db.Certificates().ByProxyID(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("lookup certificate: %v", err)
	}
	if certificate.CertFile != issued.CertFile || certificate.KeyFile != issued.KeyFile {
		t.Fatalf("renewal failure replaced active files: issued=%+v failed=%+v", issued, certificate)
	}
	if certificate.Status != domain.CertificateRenewalFailed || certificate.ServingStatus != domain.CertificateServingUsable || certificate.OperationStatus != domain.CertificateOperationRenewalFailed || certificate.FailureCount != 1 || certificate.NextAttemptAt == nil {
		t.Fatalf("unexpected renewal failure metadata: %+v", certificate)
	}
}

func TestServiceRenewCertificateReusesLoadedRecord(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	initialCert, initialKey := testCertificatePEM(t, "app.example.com", time.Now().Add(time.Hour))
	nextCert, nextKey := testCertificatePEM(t, "app.example.com", time.Now().Add(2*time.Hour))
	issuer := &sequenceIssuer{certs: []IssuedCertificate{{CertPEM: initialCert, KeyPEM: initialKey}, {CertPEM: nextCert, KeyPEM: nextKey}}}
	service := Service{Store: db, Issuer: issuer, DNSProvider: fakeDNSProvider{}, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, Settings: domain.ACMEProviderSettings{AccountEmail: "ops@example.com", TermsAccepted: true, DNSProvider: "cloudflare"}, NewID: func() (string, error) { return "cert-1", nil }}
	issued, err := service.Issue(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("issue certificate: %v", err)
	}
	countingRepo := &countingCertificateRepository{CertificateRepository: db.Certificates()}
	countingStore := countingStore{Store: db, certificates: countingRepo}
	service.Store = countingStore

	renewed, err := service.RenewCertificate(ctx, issued)
	if err != nil {
		t.Fatalf("renew loaded certificate: %v", err)
	}
	if renewed.LastRenewedAt == nil {
		t.Fatalf("expected renewal to complete: %+v", renewed)
	}
	// 续期复用已加载记录：不应按 proxy_id 重新拉取；最终仅按证书身份（id）回读一次。
	if countingRepo.byProxyIDCount != 0 {
		t.Fatalf("expected no certificate refresh by proxy id, got %d", countingRepo.byProxyIDCount)
	}
	if countingRepo.byIDCount != 1 {
		t.Fatalf("expected only final certificate refresh by certificate id, got %d", countingRepo.byIDCount)
	}
}

func TestServiceResolvesDefaultOriginCACredentialWithScopedQuery(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	secrets := testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}
	seedOriginCACredential(t, ctx, db)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	client := testOriginCAClient{create: func(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error) {
		return OriginCACertificate{ID: "cf-cert-1", CertificatePEM: originCATestCertificateFromCSR(t, request.CSR, request.Hostnames, now.Add(365*24*time.Hour)), Hostnames: request.Hostnames, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity}, nil
	}}
	countingCredentials := &countingProviderCredentialRepository{ProviderCredentialRepository: db.ProviderCredentials()}
	countingStore := countingStore{Store: db, credentials: countingCredentials}
	service := Service{Store: countingStore, OriginCAClient: client, ProviderSecretStore: secrets, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, OriginCASettings: domain.OriginCAProviderSettings{Enabled: true, DefaultRequestType: OriginCARequestTypeECC, RequestedValidity: 365}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return now }}

	issued, err := service.IssueWithProvider(ctx, ManagedCertificateRequest{ProxyID: "proxy-1", ProviderType: domain.CertificateProviderCloudflareOriginCA})
	if err != nil {
		t.Fatalf("issue with default origin ca credential: %v", err)
	}
	if issued.CredentialID != "cred-1" {
		t.Fatalf("expected default credential to be selected, got %+v", issued)
	}
	if countingCredentials.listCount != 0 || countingCredentials.listByProviderTypeCount != 1 {
		t.Fatalf("expected provider-scoped credential query, list=%d scoped=%d", countingCredentials.listCount, countingCredentials.listByProviderTypeCount)
	}
}

func TestServiceIssuesOriginCACertificateWithProviderMetadata(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	secrets := testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}
	seedOriginCACredential(t, ctx, db)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	var captured OriginCACreateRequest
	client := testOriginCAClient{create: func(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error) {
		if token != "cf-api-token" {
			t.Fatalf("unexpected token %q", token)
		}
		captured = request
		return OriginCACertificate{ID: "cf-cert-1", CertificatePEM: originCATestCertificateFromCSR(t, request.CSR, request.Hostnames, now.Add(365*24*time.Hour)), Hostnames: request.Hostnames, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity, Status: "active"}, nil
	}}
	service := Service{Store: db, OriginCAClient: client, ProviderSecretStore: secrets, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, OriginCASettings: domain.OriginCAProviderSettings{Enabled: true, DefaultRequestType: OriginCARequestTypeECC, RequestedValidity: 365, RotationWindow: 30 * 24 * time.Hour}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return now }}

	certificate, err := service.IssueWithProvider(ctx, ManagedCertificateRequest{ProxyID: "proxy-1", ProviderType: domain.CertificateProviderCloudflareOriginCA, CredentialID: "cred-1", Hostnames: []string{"API.Example.com"}})
	if err != nil {
		t.Fatalf("issue origin ca certificate: %v", err)
	}
	if captured.CSR == "" || strings.Contains(captured.CSR, "PRIVATE KEY") {
		t.Fatalf("unexpected csr material: %q", captured.CSR)
	}
	if certificate.ProviderType != domain.CertificateProviderCloudflareOriginCA || certificate.ProviderName != "cloudflare" || certificate.CredentialID != "cred-1" || certificate.ProviderStatus != domain.CertificateProviderStatusActive || certificate.CloudflareCertificateID != "cf-cert-1" {
		t.Fatalf("unexpected origin ca provider metadata: %+v", certificate)
	}
	if !stringSlicesEqual(certificate.Hostnames, []string{"app.example.com", "api.example.com"}) || certificate.RequestType != OriginCARequestTypeECC || certificate.RequestedValidity != 365 || certificate.LastSyncedAt == nil {
		t.Fatalf("unexpected origin ca request metadata: %+v", certificate)
	}
	if certificate.CertFile == "" || certificate.KeyFile == "" || certificate.Status != domain.CertificateValid || certificate.ServingStatus != domain.CertificateServingUsable || certificate.Fingerprint == "" {
		t.Fatalf("unexpected origin ca material metadata: %+v", certificate)
	}
}

func TestServiceOriginCAInitialIssueFailureRemovesUnusableCertificateRecord(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	secrets := testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}
	seedOriginCACredential(t, ctx, db)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	client := testOriginCAClient{create: func(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error) {
		return OriginCACertificate{}, &CloudflareAPIError{FailureMessage: "cloudflare origin ca request failed", StatusCode: 400, Errors: []CloudflareAPIErrorDetail{{Code: 1010}}}
	}}
	service := Service{Store: db, OriginCAClient: client, ProviderSecretStore: secrets, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir()}, OriginCASettings: domain.OriginCAProviderSettings{Enabled: true, DefaultRequestType: OriginCARequestTypeECC, RequestedValidity: 365}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return now }}

	if _, err := service.IssueCertificate(ctx, CertificateIssueRequest{Host: "www.example.com", ProviderType: domain.CertificateProviderCloudflareOriginCA, CredentialID: "cred-1"}); err == nil {
		t.Fatal("expected origin ca issue failure")
	}
	if _, err := db.Certificates().ByID(ctx, "cert-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected unusable initial certificate record to be removed, got %v", err)
	}
}

func TestServiceRotatesOriginCAAndPreservesActiveOnFailure(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	secrets := testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}
	seedOriginCACredential(t, ctx, db)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	createCount := 0
	var failNext bool
	client := testOriginCAClient{create: func(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error) {
		createCount++
		if failNext {
			return OriginCACertificate{ID: "cf-cert-bad", CertificatePEM: originCATestCertificateFromCSR(t, request.CSR, []string{"other.example.com"}, now.Add(365*24*time.Hour)), Hostnames: []string{"other.example.com"}, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity}, nil
		}
		return OriginCACertificate{ID: "cf-cert-" + string(rune('0'+createCount)), CertificatePEM: originCATestCertificateFromCSR(t, request.CSR, request.Hostnames, now.Add(time.Duration(createCount)*365*24*time.Hour)), Hostnames: request.Hostnames, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity}, nil
	}}
	service := Service{Store: db, OriginCAClient: client, ProviderSecretStore: secrets, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir(), Now: func() time.Time { return now }}, OriginCASettings: domain.OriginCAProviderSettings{Enabled: true, DefaultRequestType: OriginCARequestTypeECC, RequestedValidity: 365, RotationWindow: 30 * 24 * time.Hour}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return now }}

	issued, err := service.IssueWithProvider(ctx, ManagedCertificateRequest{ProxyID: "proxy-1", ProviderType: domain.CertificateProviderCloudflareOriginCA, CredentialID: "cred-1"})
	if err != nil {
		t.Fatalf("issue origin ca certificate: %v", err)
	}
	firstActive, err := os.ReadFile(issued.CertFile)
	if err != nil {
		t.Fatalf("read issued active cert: %v", err)
	}
	rotated, err := service.RotateOriginCA(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("rotate origin ca certificate: %v", err)
	}
	if rotated.CloudflareCertificateID != "cf-cert-2" || rotated.PreviousCloudflareCertificateID != "cf-cert-1" || rotated.PreviousCertFile == "" || rotated.PreviousKeyFile == "" || rotated.LastRenewedAt == nil {
		t.Fatalf("unexpected rotated certificate metadata: %+v", rotated)
	}
	previousActive, err := os.ReadFile(rotated.PreviousCertFile)
	if err != nil {
		t.Fatalf("read previous cert: %v", err)
	}
	if !bytes.Equal(previousActive, firstActive) {
		t.Fatal("rotation did not retain previous active certificate material")
	}
	activeBeforeFailure, err := os.ReadFile(rotated.CertFile)
	if err != nil {
		t.Fatalf("read rotated active cert: %v", err)
	}

	failNext = true
	if _, err := service.RotateOriginCA(ctx, "proxy-1"); err == nil {
		t.Fatal("expected failed origin ca rotation")
	}
	failed, err := db.Certificates().ByProxyID(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("lookup failed rotation certificate: %v", err)
	}
	activeAfterFailure, err := os.ReadFile(failed.CertFile)
	if err != nil {
		t.Fatalf("read failed active cert: %v", err)
	}
	if !bytes.Equal(activeAfterFailure, activeBeforeFailure) {
		t.Fatal("failed rotation replaced active certificate material")
	}
	if failed.CloudflareCertificateID != "cf-cert-2" || failed.Status != domain.CertificateRenewalFailed || failed.ServingStatus != domain.CertificateServingUsable || failed.OperationStatus != domain.CertificateOperationRenewalFailed || failed.FailureCount != 1 || failed.NextAttemptAt == nil {
		t.Fatalf("unexpected failed rotation metadata: %+v", failed)
	}
}

func TestServiceSyncAndRevokeOriginCAStatus(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	secrets := testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}
	seedOriginCACredential(t, ctx, db)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(time.Minute)
	revokeCalls := make([]string, 0)
	client := testOriginCAClient{
		create: func(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error) {
			return OriginCACertificate{ID: "cf-cert-1", CertificatePEM: originCATestCertificateFromCSR(t, request.CSR, request.Hostnames, now.Add(365*24*time.Hour)), Hostnames: request.Hostnames, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity}, nil
		},
		get: func(ctx context.Context, token string, certificateID string) (OriginCACertificate, error) {
			return OriginCACertificate{ID: certificateID, RevokedAt: &revokedAt, Status: "revoked"}, nil
		},
		revoke: func(ctx context.Context, token string, certificateID string) error {
			revokeCalls = append(revokeCalls, certificateID)
			return nil
		},
	}
	service := Service{Store: db, OriginCAClient: client, ProviderSecretStore: secrets, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: t.TempDir(), Now: func() time.Time { return now }}, OriginCASettings: domain.OriginCAProviderSettings{Enabled: true, DefaultRequestType: OriginCARequestTypeECC, RequestedValidity: 365}, NewID: func() (string, error) { return "cert-1", nil }, Now: func() time.Time { return now }}

	if _, err := service.IssueWithProvider(ctx, ManagedCertificateRequest{ProxyID: "proxy-1", ProviderType: domain.CertificateProviderCloudflareOriginCA, CredentialID: "cred-1"}); err != nil {
		t.Fatalf("issue origin ca certificate: %v", err)
	}
	synced, err := service.SyncOriginCA(ctx, "proxy-1")
	if err != nil {
		t.Fatalf("sync origin ca certificate: %v", err)
	}
	if synced.ProviderStatus != domain.CertificateProviderStatusRevoked || synced.LastSyncedAt == nil {
		t.Fatalf("unexpected synced provider status: %+v", synced)
	}
	if _, err := service.RevokeOriginCA(ctx, OriginCARevokeRequest{ProxyID: "proxy-1", Host: "other.example.com", CloudflareCertificateID: "cf-cert-1"}); err == nil {
		t.Fatal("expected revoke host confirmation failure")
	}
	if _, err := service.RevokeOriginCA(ctx, OriginCARevokeRequest{ProxyID: "proxy-1", Host: "app.example.com", CloudflareCertificateID: "cf-cert-other"}); err == nil {
		t.Fatal("expected revoke certificate id confirmation failure")
	}
	revoked, err := service.RevokeOriginCA(ctx, OriginCARevokeRequest{ProxyID: "proxy-1", Host: "app.example.com", CloudflareCertificateID: "cf-cert-1"})
	if err != nil {
		t.Fatalf("revoke origin ca certificate: %v", err)
	}
	if len(revokeCalls) != 1 || revokeCalls[0] != "cf-cert-1" {
		t.Fatalf("unexpected revoke calls: %+v", revokeCalls)
	}
	if revoked.ProviderStatus != domain.CertificateProviderStatusRevoked {
		t.Fatalf("expected active revoke to mark provider revoked: %+v", revoked)
	}
}

func TestServiceSyncOriginCAPreservesTerminalProviderStatusOnTransientError(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	seedHTTPSProxy(t, ctx, db)
	seedOriginCACredential(t, ctx, db)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	if err := db.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-1", ProxyID: "proxy-1", Host: "app.example.com", Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable, OperationStatus: domain.CertificateOperationIdle, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CredentialID: "cred-1", ProviderStatus: domain.CertificateProviderStatusRevoked, CloudflareCertificateID: "cf-cert-1"}); err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	client := testOriginCAClient{get: func(ctx context.Context, token string, certificateID string) (OriginCACertificate, error) {
		return OriginCACertificate{}, errors.New("cloudflare api unavailable")
	}}
	service := Service{Store: db, OriginCAClient: client, ProviderSecretStore: testSecretStore{values: map[string]string{"cred-1.secret": "cf-api-token"}}, Now: func() time.Time { return now }}

	updated, err := service.SyncOriginCA(ctx, "proxy-1")
	if err == nil {
		t.Fatal("expected transient sync error")
	}
	if updated.ProviderStatus != domain.CertificateProviderStatusRevoked || updated.LastSyncedAt == nil || !updated.LastSyncedAt.Equal(now) {
		t.Fatalf("expected revoked provider status to be preserved, got %+v", updated)
	}
	if updated.LastError != "cloudflare api unavailable" {
		t.Fatalf("expected sanitized sync error to be recorded, got %+v", updated)
	}
}

type fakeIssuer struct {
	certPEM []byte
	keyPEM  []byte
	err     error
}

func (issuer fakeIssuer) Issue(context.Context, IssueRequest) (IssuedCertificate, error) {
	if issuer.err != nil {
		return IssuedCertificate{}, issuer.err
	}
	return IssuedCertificate{CertPEM: issuer.certPEM, KeyPEM: issuer.keyPEM}, nil
}

type sequenceIssuer struct {
	certs []IssuedCertificate
	next  int
	err   error
}

func (issuer *sequenceIssuer) Issue(context.Context, IssueRequest) (IssuedCertificate, error) {
	if issuer.err != nil {
		return IssuedCertificate{}, issuer.err
	}
	if issuer.next >= len(issuer.certs) {
		return IssuedCertificate{}, errors.New("no certificate")
	}
	cert := issuer.certs[issuer.next]
	issuer.next++
	return cert, nil
}

type fakeDNSProvider struct{}

func (fakeDNSProvider) Present(context.Context, string, string) error { return nil }
func (fakeDNSProvider) CleanUp(context.Context, string, string) error { return nil }

type failingDNSProvider struct {
	err error
}

func (provider failingDNSProvider) Present(context.Context, string, string) error {
	return provider.err
}
func (provider failingDNSProvider) CleanUp(context.Context, string, string) error { return nil }

type providerCheckingIssuer struct{}

func (providerCheckingIssuer) Issue(ctx context.Context, request IssueRequest) (IssuedCertificate, error) {
	if request.DNSProvider == nil {
		return IssuedCertificate{}, errors.New("dns challenge provider is required")
	}
	if err := request.DNSProvider.Present(ctx, "_acme-challenge.app.example.com", "value"); err != nil {
		return IssuedCertificate{}, err
	}
	return IssuedCertificate{}, errors.New("unexpected provider success")
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

func seedHTTPSProxy(t *testing.T, ctx context.Context, db *sqlite.Store) {
	t.Helper()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	proxy := domain.Proxy{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
}

func testCertificatePEM(t *testing.T, host string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

type testSecretStore struct {
	values map[string]string
}

func (store testSecretStore) Write(context.Context, string, string) (string, error) {
	return "", errors.New("test secret store is read-only")
}

func (store testSecretStore) Read(_ context.Context, secretRef string) (string, error) {
	value, ok := store.values[secretRef]
	if !ok {
		return "", errors.New("secret material is missing")
	}
	return value, nil
}

func (store testSecretStore) Delete(context.Context, string) error {
	return nil
}

type testOriginCAClient struct {
	create func(context.Context, string, OriginCACreateRequest) (OriginCACertificate, error)
	get    func(context.Context, string, string) (OriginCACertificate, error)
	revoke func(context.Context, string, string) error
	verify func(context.Context, string) error
}

func (client testOriginCAClient) Create(ctx context.Context, token string, request OriginCACreateRequest) (OriginCACertificate, error) {
	if client.create == nil {
		return OriginCACertificate{}, errors.New("unexpected origin ca create")
	}
	return client.create(ctx, token, request)
}

func (client testOriginCAClient) Get(ctx context.Context, token string, certificateID string) (OriginCACertificate, error) {
	if client.get == nil {
		return OriginCACertificate{}, errors.New("unexpected origin ca get")
	}
	return client.get(ctx, token, certificateID)
}

func (client testOriginCAClient) List(context.Context, string) ([]OriginCACertificate, error) {
	return nil, errors.New("unexpected origin ca list")
}

func (client testOriginCAClient) Revoke(ctx context.Context, token string, certificateID string) error {
	if client.revoke == nil {
		return errors.New("unexpected origin ca revoke")
	}
	return client.revoke(ctx, token, certificateID)
}

func (client testOriginCAClient) VerifyToken(ctx context.Context, token string) error {
	if client.verify == nil {
		return nil
	}
	return client.verify(ctx, token)
}

func seedOriginCACredential(t *testing.T, ctx context.Context, db *sqlite.Store) {
	t.Helper()
	if err := db.ProviderCredentials().Create(ctx, domain.ProviderCredential{ID: "cred-1", Name: "Cloudflare Origin", ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: "Zone SSL:Edit", TokenFingerprint: TokenFingerprint("cf-api-token"), SecretRef: "cred-1.secret", Status: domain.ProviderCredentialVerified}); err != nil {
		t.Fatalf("create origin ca credential: %v", err)
	}
}

func originCATestCertificateFromCSR(t *testing.T, csrPEM string, hostnames []string, notAfter time.Time) []byte {
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
	if len(hostnames) == 0 {
		hostnames = csr.DNSNames
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: hostnames[0]}, DNSNames: hostnames, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, csr.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create certificate from csr: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type countingStore struct {
	*sqlite.Store
	certificates *countingCertificateRepository
	credentials  *countingProviderCredentialRepository
}

func (cs countingStore) Certificates() store.CertificateRepository {
	if cs.certificates != nil {
		return cs.certificates
	}
	return cs.Store.Certificates()
}

func (cs countingStore) ProviderCredentials() store.ProviderCredentialRepository {
	if cs.credentials != nil {
		return cs.credentials
	}
	return cs.Store.ProviderCredentials()
}

type countingCertificateRepository struct {
	store.CertificateRepository
	byProxyIDCount int
	byIDCount      int
}

func (repo *countingCertificateRepository) ByProxyID(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	repo.byProxyIDCount++
	return repo.CertificateRepository.ByProxyID(ctx, proxyID)
}

func (repo *countingCertificateRepository) ByID(ctx context.Context, id string) (domain.ManagedCertificate, error) {
	repo.byIDCount++
	return repo.CertificateRepository.ByID(ctx, id)
}

type countingProviderCredentialRepository struct {
	store.ProviderCredentialRepository
	listCount               int
	listByProviderTypeCount int
}

func (repo *countingProviderCredentialRepository) List(ctx context.Context) ([]domain.ProviderCredential, error) {
	repo.listCount++
	return repo.ProviderCredentialRepository.List(ctx)
}

func (repo *countingProviderCredentialRepository) ListByProviderType(ctx context.Context, providerType domain.CertificateProviderType, statuses []domain.ProviderCredentialStatus) ([]domain.ProviderCredential, error) {
	repo.listByProviderTypeCount++
	return repo.ProviderCredentialRepository.ListByProviderType(ctx, providerType, statuses)
}
