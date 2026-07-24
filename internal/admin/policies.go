package admin

import (
	"context"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type ProxyAdmissionPolicy interface {
	EnsureAdmission(ctx context.Context, proxy domain.Proxy, ignoreProxyID string) error
	ReconcileListeners(ctx context.Context) error
}

type CertificateBindingPolicy interface {
	ResolveProxySelection(ctx context.Context, proxyType domain.ProxyType, proxyID, certificateID string, certificateIDSet bool, entryHost, certFile, keyFile, actorID string) (string, error)
	ValidateBinding(ctx context.Context, certificateID, host, domainID string) error
	ProxyBoundToCertificate(ctx context.Context, certificateID string) (domain.Proxy, bool, error)
	UnbindProxy(ctx context.Context, proxy domain.Proxy) error
	UnbindDomain(ctx context.Context, webDomain domain.Domain) error
	BoundDomain(ctx context.Context, webDomain domain.Domain) (domain.ManagedCertificate, error)
	BoundProxy(ctx context.Context, proxy domain.Proxy) (domain.ManagedCertificate, error)
	CertificateServable(certificate domain.ManagedCertificate) bool
	CleanupManagedFiles(certificate domain.ManagedCertificate)
}

type ProxyAccessPolicy interface {
	RevokeIfEnabled(ctx context.Context, proxy *domain.Proxy) error
	RevokeForDomain(ctx context.Context, domainID string) error
	DomainHasEnabledHTTPSEntry(ctx context.Context, domainID string) bool
}

func newProxyAdmissionPolicy(store store.Store, claims []domain.ListenerClaim, defaults domain.ProxyEntryDefaults, reconciler ListenerReconciler) ProxyAdmissionPolicy {
	return proxyAdmissionPolicy{Store: store, StaticListenerClaims: claims, ProxyEntryDefaults: defaults, ListenerReconciler: reconciler}
}
func newCertificateBindingPolicy(store store.Store, certificates certmanager.Service, audit AuditRecorder) CertificateBindingPolicy {
	return certificateBindingPolicy{Store: store, Certificates: certificates, Audit: audit}
}
func newProxyAccessPolicy(store store.Store) ProxyAccessPolicy {
	return proxyAccessPolicy{Store: store}
}
