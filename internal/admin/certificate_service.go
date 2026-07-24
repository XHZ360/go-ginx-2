package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

func (service *CertificateService) IssueManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	providerType := input.ProviderType
	var certificate domain.ManagedCertificate
	var err error
	if strings.TrimSpace(input.CertificateID) != "" {
		if service.Store == nil {
			return domain.ManagedCertificate{}, errors.New("store is required")
		}
		existing, readErr := service.Store.Certificates().ByID(ctx, input.CertificateID)
		if readErr != nil {
			return domain.ManagedCertificate{}, readErr
		}
		if providerType == "" {
			providerType = existing.ProviderType
		}
		credentialID := input.CredentialID
		if credentialID == "" {
			credentialID = existing.CredentialID
		}
		requestType := input.RequestType
		if requestType == "" {
			requestType = existing.RequestType
		}
		requestedValidity := input.RequestedValidity
		if requestedValidity == 0 {
			requestedValidity = existing.RequestedValidity
		}
		certificate, err = service.Certificates.IssueCertificate(ctx, certmanager.CertificateIssueRequest{CertificateID: input.CertificateID, Host: existing.Host, ProviderType: providerType, CredentialID: credentialID, RequestType: requestType, RequestedValidity: requestedValidity})
	} else {
		certificate, err = service.Certificates.IssueWithProvider(ctx, certmanager.ManagedCertificateRequest{ProxyID: input.ProxyID, ProviderType: providerType, CredentialID: input.CredentialID, RequestType: input.RequestType, RequestedValidity: input.RequestedValidity})
	}
	if err != nil {
		return domain.ManagedCertificate{}, providerReadinessError(err)
	}
	action := "issue_managed_certificate"
	if providerType == domain.CertificateProviderCloudflareOriginCA {
		action = "issue_cloudflare_origin_certificate"
	}
	return certificate, service.Audit.Record(ctx, input.ActorID, "certificate", certificate.ID, action)
}

func (service *CertificateService) CertificateProviderReadiness() []certmanager.ProviderReadiness {
	if service.Store == nil {
		return nil
	}
	return []certmanager.ProviderReadiness{service.Certificates.ProviderReadiness(domain.CertificateProviderACMEDNS01), service.Certificates.ProviderReadiness(domain.CertificateProviderCloudflareOriginCA)}
}

func (service *CertificateService) RenewManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	certificate, err := service.Certificates.RenewByID(ctx, input.CertificateID, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.Audit.Record(ctx, input.ActorID, "certificate", certificate.ID, "renew_managed_certificate")
}

func (service *CertificateService) RotateOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	certificate, err := service.Certificates.RotateOriginCAByID(ctx, input.CertificateID, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.Audit.Record(ctx, input.ActorID, "certificate", certificate.ID, "rotate_cloudflare_origin_certificate")
}

func (service *CertificateService) SyncOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	certificate, err := service.Certificates.SyncOriginCAByID(ctx, input.CertificateID, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.Audit.Record(ctx, input.ActorID, "certificate", certificate.ID, "sync_cloudflare_origin_certificate")
}

func (service *CertificateService) RevokeOriginCACertificate(ctx context.Context, input RevokeOriginCACertificateInput) (domain.ManagedCertificate, error) {
	certificate, err := service.Certificates.RevokeOriginCA(ctx, certmanager.OriginCARevokeRequest{CertificateID: input.CertificateID, ProxyID: input.ProxyID, Host: input.Host, CloudflareCertificateID: input.CloudflareCertificateID})
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.Audit.Record(ctx, input.ActorID, "certificate", certificate.ID, "revoke_cloudflare_origin_certificate")
}

func (service *CertificateService) CreateCertificate(ctx context.Context, input CreateCertificateInput) (domain.ManagedCertificate, error) {
	host := strings.ToLower(strings.TrimSpace(input.Host))
	if host == "" {
		return domain.ManagedCertificate{}, contracterr.Validation("validation failed", map[string]string{"host": "certificate host is required"})
	}
	providerType := input.ProviderType
	if providerType == "" {
		providerType = domain.CertificateProviderACMEDNS01
	}
	if !providerType.Valid() {
		return domain.ManagedCertificate{}, contracterr.Validation("validation failed", map[string]string{"providerType": "certificate provider type is invalid"})
	}
	if providerType == domain.CertificateProviderFile && (strings.TrimSpace(input.CertFile) == "" || strings.TrimSpace(input.KeyFile) == "") {
		return domain.ManagedCertificate{}, contracterr.Validation("validation failed", map[string]string{"certFile": "file certificate requires both cert file and key file", "keyFile": "file certificate requires both cert file and key file"})
	}
	certificate, err := service.Certificates.IssueCertificate(ctx, certmanager.CertificateIssueRequest{Host: host, ProviderType: providerType, CredentialID: input.CredentialID, RequestType: input.RequestType, RequestedValidity: input.RequestedValidity, CertFile: input.CertFile, KeyFile: input.KeyFile})
	if err != nil {
		return domain.ManagedCertificate{}, providerReadinessError(err)
	}
	action := "create_managed_certificate"
	if providerType == domain.CertificateProviderCloudflareOriginCA {
		action = "create_cloudflare_origin_certificate"
	}
	if providerType == domain.CertificateProviderFile {
		action = "create_file_certificate"
	}
	return certificate, service.Audit.Record(ctx, input.ActorID, "certificate", certificate.ID, action)
}

func (service *CertificateService) DeleteCertificate(ctx context.Context, input DeleteCertificateInput) (DeleteCertificateResult, error) {
	if service.Store == nil {
		return DeleteCertificateResult{}, errors.New("store is required")
	}
	certificateID := strings.TrimSpace(input.CertificateID)
	if certificateID == "" {
		return DeleteCertificateResult{}, contracterr.Validation("validation failed", map[string]string{"id": "certificate id is required"})
	}
	certificate, err := service.Store.Certificates().ByID(ctx, certificateID)
	if err != nil {
		return DeleteCertificateResult{}, err
	}
	boundProxy, hasBinding, err := service.Binding.ProxyBoundToCertificate(ctx, certificateID)
	if err != nil {
		return DeleteCertificateResult{}, err
	}
	servable := service.Binding.CertificateServable(certificate)
	requireConfirm := hasBinding && servable
	if requireConfirm && !service.deleteConfirmationMatches(certificate, input) {
		return DeleteCertificateResult{RequiredConfirm: true, CertificateID: certificateID}, contracterr.ConfirmationRequired("certificate is bound to a proxy and currently serving; confirm deletion with a matching host or certificate id", map[string]string{"confirmHost": certificate.Host, "confirmCertificateId": certificate.ID})
	}
	affected := make([]string, 0, 1)
	if hasBinding {
		if err := service.Binding.UnbindProxy(ctx, boundProxy); err != nil {
			return DeleteCertificateResult{}, err
		}
		affected = append(affected, boundProxy.ID)
	}
	if err := service.Store.Certificates().Delete(ctx, certificateID); err != nil {
		return DeleteCertificateResult{}, err
	}
	service.Binding.CleanupManagedFiles(certificate)
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return DeleteCertificateResult{}, err
	}
	if err := service.Audit.Record(ctx, input.ActorID, "certificate", certificateID, "delete_managed_certificate"); err != nil {
		return DeleteCertificateResult{}, err
	}
	return DeleteCertificateResult{CertificateID: certificateID, AffectedProxyIDs: affected, RequiredConfirm: requireConfirm}, nil
}

func (service *CertificateService) BindCertificate(ctx context.Context, input BindCertificateInput) (domain.Proxy, error) {
	if service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.ProxyID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"proxyId": "proxy id is required"})
	}
	if strings.TrimSpace(input.CertificateID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"certificateId": "certificate id is required"})
	}
	proxy, err := service.Store.Proxies().ByID(ctx, input.ProxyID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if !proxy.Type.IsWeb() || strings.TrimSpace(proxy.DomainID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"type": "certificate binding requires a web proxy with domain"})
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Binding.ValidateBinding(ctx, input.CertificateID, webDomain.Host, webDomain.ID); err != nil {
		return domain.Proxy{}, err
	}
	webDomain.CertificateID = input.CertificateID
	if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
		return domain.Proxy{}, err
	}
	proxy.CertificateID = input.CertificateID
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return domain.Proxy{}, err
	}
	return proxy, service.Audit.Record(ctx, input.ActorID, "domain", webDomain.ID, "bind_certificate")
}

func (service *CertificateService) UnbindCertificate(ctx context.Context, input UnbindCertificateInput) (domain.Proxy, error) {
	if service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.ProxyID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"proxyId": "proxy id is required"})
	}
	proxy, err := service.Store.Proxies().ByID(ctx, input.ProxyID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if strings.TrimSpace(proxy.DomainID) == "" {
		return proxy, nil
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if strings.TrimSpace(webDomain.CertificateID) == "" {
		return proxy, nil
	}
	if err := service.Binding.UnbindDomain(ctx, webDomain); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return domain.Proxy{}, err
	}
	return proxy, service.Audit.Record(ctx, input.ActorID, "domain", webDomain.ID, "unbind_certificate")
}

func (service *CertificateService) MigrateLegacyFileCertificates(ctx context.Context) (int, error) {
	return service.Certificates.MigrateLegacyFileCertificates(ctx)
}

func (service *CertificateService) ManagedCertificateStatus(ctx context.Context, proxyID string) (certmanager.CertificateStatus, error) {
	return service.Certificates.Status(ctx, proxyID)
}

func (service *CertificateService) deleteConfirmationMatches(certificate domain.ManagedCertificate, input DeleteCertificateInput) bool {
	if strings.EqualFold(strings.TrimSpace(input.ConfirmCertificateID), strings.TrimSpace(certificate.ID)) && strings.TrimSpace(input.ConfirmCertificateID) != "" {
		return true
	}
	confirmHost := strings.ToLower(strings.TrimSpace(input.ConfirmHost))
	return confirmHost != "" && confirmHost == strings.ToLower(strings.TrimSpace(certificate.Host))
}

var _ CertificateFacade = (*CertificateService)(nil)
