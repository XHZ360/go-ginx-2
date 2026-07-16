package certmanager

import (
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type LifecycleScheduler struct {
	RenewalWindow          time.Duration
	OriginCARotationWindow time.Duration
}

func (scheduler LifecycleScheduler) WindowFor(providerType domain.CertificateProviderType) time.Duration {
	if providerType == domain.CertificateProviderCloudflareOriginCA && scheduler.OriginCARotationWindow > 0 {
		return scheduler.OriginCARotationWindow
	}
	return scheduler.RenewalWindow
}

func (scheduler LifecycleScheduler) ServingStatus(notAfter time.Time, providerType domain.CertificateProviderType, now time.Time) domain.CertificateServingStatus {
	if !notAfter.After(now) {
		return domain.CertificateServingExpired
	}
	if window := scheduler.WindowFor(providerType); window > 0 && !notAfter.After(now.Add(window)) {
		return domain.CertificateServingExpiringSoon
	}
	return domain.CertificateServingUsable
}

func (scheduler LifecycleScheduler) IsDue(certificate domain.ManagedCertificate, now time.Time) bool {
	if certificate.NotAfter == nil {
		return false
	}
	window := scheduler.WindowFor(certificate.ProviderType)
	if window <= 0 {
		return false
	}
	return !certificate.NotAfter.After(now.Add(window))
}

func (scheduler LifecycleScheduler) NextAttemptAt(now time.Time, failureCount int, notAfter *time.Time) time.Time {
	if failureCount < 1 {
		failureCount = 1
	}
	delay := time.Minute
	for i := 1; i < failureCount && delay < time.Hour; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	if notAfter != nil {
		remaining := notAfter.Sub(now)
		if remaining > 0 && remaining <= 24*time.Hour && delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
	}
	return now.Add(delay)
}

func (scheduler LifecycleScheduler) MaxLookahead() time.Duration {
	window := max(scheduler.OriginCARotationWindow, scheduler.RenewalWindow)
	return window
}

func (scheduler LifecycleScheduler) CandidateQuery(now time.Time) store.CertificateLifecycleCandidateQuery {
	query := store.CertificateLifecycleCandidateQuery{Now: now}
	if scheduler.RenewalWindow > 0 {
		before := now.Add(scheduler.RenewalWindow)
		query.ACMEBefore = &before
	}
	if scheduler.OriginCARotationWindow > 0 {
		before := now.Add(scheduler.OriginCARotationWindow)
		query.OriginCABefore = &before
	}
	return query
}
