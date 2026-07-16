package certmanager

import (
	"context"
	"errors"
	"testing"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

func TestACMEReadinessBlocksIssueBeforeCertificateRecord(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	service := Service{Store: db, Settings: domain.ACMEProviderSettings{DNSProviderTokenEnv: "CF_DNS_API_TOKEN"}}

	_, err := service.IssueCertificate(ctx, CertificateIssueRequest{Host: "blocked.example.com", ProviderType: domain.CertificateProviderACMEDNS01})
	readinessErr, ok := IsProviderNotReady(err)
	if !ok {
		t.Fatalf("expected provider readiness error, got %v", err)
	}
	if readinessErr.Readiness.Ready || len(readinessErr.Readiness.MissingRequirements) != 4 {
		t.Fatalf("unexpected readiness: %+v", readinessErr.Readiness)
	}
	if _, lookupErr := db.Certificates().ByID(ctx, "blocked.example.com"); !errors.Is(lookupErr, store.ErrNotFound) {
		t.Fatal("readiness failure must not create a certificate record")
	}
}

func TestACMEReadinessDoesNotExposeTokenValue(t *testing.T) {
	t.Setenv("CF_DNS_API_TOKEN", "secret-token-value")
	service := Service{Settings: domain.ACMEProviderSettings{Enabled: true, AccountEmail: "ops@example.com", TermsAccepted: true, DNSProviderTokenEnv: "CF_DNS_API_TOKEN"}, Issuer: fakeIssuer{}, DNSProvider: fakeDNSProvider{}}
	readiness := service.ProviderReadiness(domain.CertificateProviderACMEDNS01)
	if !readiness.Ready || readiness.TokenEnvName != "CF_DNS_API_TOKEN" {
		t.Fatalf("unexpected readiness: %+v", readiness)
	}
	if readiness.Guidance == "secret-token-value" {
		t.Fatal("readiness leaked token value")
	}
}
