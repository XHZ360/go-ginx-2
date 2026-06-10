package certmanager

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type managedCertificateProvider interface {
	ProviderType() domain.CertificateProviderType
	Issue(context.Context, Service, ManagedCertificateRequest, domain.CertificateStatus) (domain.ManagedCertificate, error)
	Renew(context.Context, Service, domain.ManagedCertificate) (domain.ManagedCertificate, error)
	Sync(context.Context, Service, domain.ManagedCertificate) (domain.ManagedCertificate, error)
	Revoke(context.Context, Service, domain.ManagedCertificate, OriginCARevokeRequest) (domain.ManagedCertificate, error)
}

type acmeDNS01Provider struct{}

func (provider acmeDNS01Provider) ProviderType() domain.CertificateProviderType {
	return domain.CertificateProviderACMEDNS01
}

func (provider acmeDNS01Provider) Issue(ctx context.Context, service Service, request ManagedCertificateRequest, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	return service.issueACME(ctx, request.ProxyID, failureStatus)
}

func (provider acmeDNS01Provider) Renew(ctx context.Context, service Service, certificate domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	return service.issueACMEForCertificate(ctx, certificate, domain.CertificateRenewalFailed)
}

func (provider acmeDNS01Provider) Sync(context.Context, Service, domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	return domain.ManagedCertificate{}, errors.New("provider sync is not supported for acme dns-01 certificates")
}

func (provider acmeDNS01Provider) Revoke(context.Context, Service, domain.ManagedCertificate, OriginCARevokeRequest) (domain.ManagedCertificate, error) {
	return domain.ManagedCertificate{}, errors.New("provider revoke is not supported for acme dns-01 certificates")
}

type cloudflareOriginCAProvider struct{}

func (provider cloudflareOriginCAProvider) ProviderType() domain.CertificateProviderType {
	return domain.CertificateProviderCloudflareOriginCA
}

func (provider cloudflareOriginCAProvider) Issue(ctx context.Context, service Service, request ManagedCertificateRequest, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	return service.issueOriginCA(ctx, request, failureStatus)
}

func (provider cloudflareOriginCAProvider) Renew(ctx context.Context, service Service, certificate domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	return service.rotateOriginCACertificate(ctx, certificate)
}

func (provider cloudflareOriginCAProvider) Sync(ctx context.Context, service Service, certificate domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	return service.syncOriginCACertificate(ctx, certificate)
}

func (provider cloudflareOriginCAProvider) Revoke(ctx context.Context, service Service, certificate domain.ManagedCertificate, request OriginCARevokeRequest) (domain.ManagedCertificate, error) {
	return service.revokeOriginCACertificate(ctx, certificate, request)
}

func (service Service) providerFor(providerType domain.CertificateProviderType) (managedCertificateProvider, error) {
	if providerType == "" {
		providerType = domain.CertificateProviderACMEDNS01
	}
	switch providerType {
	case domain.CertificateProviderACMEDNS01:
		return acmeDNS01Provider{}, nil
	case domain.CertificateProviderCloudflareOriginCA:
		return cloudflareOriginCAProvider{}, nil
	default:
		return nil, errors.New("unsupported certificate provider " + string(providerType))
	}
}

func validateProviderSuccess(result store.CertificateSuccess) error {
	if result.ProviderType == "" {
		return errors.New("certificate provider type is required")
	}
	if !result.ProviderType.Valid() {
		return errors.New("certificate provider type is invalid")
	}
	if strings.TrimSpace(result.CertFile) == "" || strings.TrimSpace(result.KeyFile) == "" {
		return errors.New("certificate active material paths are required")
	}
	if result.NotAfter.IsZero() {
		return errors.New("certificate not_after is required")
	}
	if strings.TrimSpace(result.ProviderName) == "" {
		return errors.New("certificate provider name is required")
	}
	switch result.ProviderType {
	case domain.CertificateProviderACMEDNS01:
		return validateACMESuccess(result)
	case domain.CertificateProviderCloudflareOriginCA:
		return validateOriginCASuccess(result)
	default:
		return errors.New("unsupported certificate provider " + string(result.ProviderType))
	}
}

func validateACMESuccess(result store.CertificateSuccess) error {
	if result.CredentialID != "" || result.CloudflareID != "" || len(result.Hostnames) > 0 || result.RequestType != "" || result.RequestedValidity > 0 {
		return errors.New("acme dns-01 certificate result contains origin ca metadata")
	}
	return nil
}

func validateOriginCASuccess(result store.CertificateSuccess) error {
	if strings.TrimSpace(result.CredentialID) == "" {
		return errors.New("origin ca credential id is required")
	}
	if strings.TrimSpace(result.CloudflareID) == "" {
		return errors.New("origin ca cloudflare certificate id is required")
	}
	if len(result.Hostnames) == 0 {
		return errors.New("origin ca hostnames are required")
	}
	for _, hostname := range result.Hostnames {
		if strings.TrimSpace(hostname) == "" {
			return errors.New("origin ca hostnames are required")
		}
	}
	switch result.RequestType {
	case OriginCARequestTypeECC, OriginCARequestTypeRSA:
	default:
		return errors.New("origin ca request type is invalid")
	}
	if result.RequestedValidity <= 0 {
		return errors.New("origin ca requested validity is required")
	}
	if result.ProviderStatus != domain.CertificateProviderStatusActive {
		return errors.New("origin ca provider status must be active for success")
	}
	return nil
}
