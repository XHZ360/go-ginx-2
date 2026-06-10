package certmanager

import (
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

func TestLifecycleSchedulerUsesProviderSpecificWindows(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	scheduler := LifecycleScheduler{RenewalWindow: time.Hour, OriginCARotationWindow: 24 * time.Hour}

	if got := scheduler.WindowFor(domain.CertificateProviderACMEDNS01); got != time.Hour {
		t.Fatalf("unexpected acme window: %v", got)
	}
	if got := scheduler.WindowFor(domain.CertificateProviderCloudflareOriginCA); got != 24*time.Hour {
		t.Fatalf("unexpected origin ca window: %v", got)
	}
	if got := scheduler.ServingStatus(now.Add(30*time.Minute), domain.CertificateProviderACMEDNS01, now); got != domain.CertificateServingExpiringSoon {
		t.Fatalf("expected acme certificate to be expiring soon, got %s", got)
	}
	if got := scheduler.ServingStatus(now.Add(2*time.Hour), domain.CertificateProviderACMEDNS01, now); got != domain.CertificateServingUsable {
		t.Fatalf("expected acme certificate to be usable, got %s", got)
	}
	if got := scheduler.ServingStatus(now.Add(2*time.Hour), domain.CertificateProviderCloudflareOriginCA, now); got != domain.CertificateServingExpiringSoon {
		t.Fatalf("expected origin ca certificate to be expiring soon, got %s", got)
	}
	if got := scheduler.ServingStatus(now.Add(-time.Second), domain.CertificateProviderCloudflareOriginCA, now); got != domain.CertificateServingExpired {
		t.Fatalf("expected expired status, got %s", got)
	}
}

func TestLifecycleSchedulerCandidateQueryAndBackoff(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	scheduler := LifecycleScheduler{RenewalWindow: time.Hour, OriginCARotationWindow: 24 * time.Hour}

	query := scheduler.CandidateQuery(now)
	if query.ACMEBefore == nil || !query.ACMEBefore.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected acme candidate boundary: %+v", query.ACMEBefore)
	}
	if query.OriginCABefore == nil || !query.OriginCABefore.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("unexpected origin ca candidate boundary: %+v", query.OriginCABefore)
	}
	if got := scheduler.MaxLookahead(); got != 24*time.Hour {
		t.Fatalf("unexpected max lookahead: %v", got)
	}
	if got := scheduler.NextAttemptAt(now, 3, nil); !got.Equal(now.Add(4 * time.Minute)) {
		t.Fatalf("unexpected exponential backoff: %v", got)
	}
	notAfter := now.Add(2 * time.Hour)
	if got := scheduler.NextAttemptAt(now, 8, &notAfter); !got.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expected urgent expiry retry cap, got %v", got)
	}
}

func TestValidateProviderSuccessRejectsCrossProviderMetadata(t *testing.T) {
	notAfter := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	if err := validateProviderSuccess(store.CertificateSuccess{ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: "cloudflare", CertFile: "active.crt", KeyFile: "active.key", NotAfter: notAfter, CredentialID: "cred-1"}); err == nil {
		t.Fatal("expected acme success with origin ca credential metadata to be rejected")
	}
	if err := validateProviderSuccess(store.CertificateSuccess{ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", ProviderStatus: domain.CertificateProviderStatusActive, CredentialID: "cred-1", CertFile: "active.crt", KeyFile: "active.key", NotAfter: notAfter, Hostnames: []string{"app.example.com"}, RequestType: OriginCARequestTypeECC, RequestedValidity: 365}); err == nil {
		t.Fatal("expected origin ca success without cloudflare certificate id to be rejected")
	}
	if err := validateProviderSuccess(store.CertificateSuccess{ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", ProviderStatus: domain.CertificateProviderStatusActive, CredentialID: "cred-1", CloudflareID: "cf-cert-1", CertFile: "active.crt", KeyFile: "active.key", NotAfter: notAfter, Hostnames: []string{"app.example.com"}, RequestType: OriginCARequestTypeECC, RequestedValidity: 365}); err != nil {
		t.Fatalf("expected valid origin ca success: %v", err)
	}
}
