package admin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type certificateBindingPolicy struct {
	Store        store.Store
	Certificates certmanager.Service
	Audit        AuditRecorder
}

func (policy certificateBindingPolicy) ResolveProxySelection(ctx context.Context, proxyType domain.ProxyType, proxyID, certificateID string, certificateIDSet bool, entryHost, certFile, keyFile, actorID string) (string, error) {
	if proxyType != domain.ProxyHTTPS && proxyType != domain.ProxyWeb {
		return "", nil
	}
	certificateID = strings.TrimSpace(certificateID)
	host := strings.ToLower(strings.TrimSpace(entryHost))
	if certificateID != "" {
		if err := policy.ValidateBinding(ctx, certificateID, host, proxyID); err != nil {
			return "", err
		}
		return certificateID, nil
	}
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" && keyFile == "" {
		if certificateIDSet {
			return "", nil
		}
		if strings.TrimSpace(proxyID) != "" {
			if existing, err := policy.Store.Proxies().ByID(ctx, proxyID); err == nil {
				return existing.CertificateID, nil
			}
		}
		return "", nil
	}
	certificate, err := policy.Certificates.IssueCertificate(ctx, certmanager.CertificateIssueRequest{Host: host, ProviderType: domain.CertificateProviderFile, CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		return "", err
	}
	_ = policy.Audit.Record(ctx, actorID, "certificate", certificate.ID, "migrate_file_certificate")
	return certificate.ID, nil
}

func (policy certificateBindingPolicy) ValidateBinding(ctx context.Context, certificateID, host, domainID string) error {
	_ = domainID
	certificate, err := policy.Store.Certificates().ByID(ctx, certificateID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return contracterr.Validation("validation failed", map[string]string{"certificateId": "certificate was not found"})
		}
		return err
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return contracterr.Validation("validation failed", map[string]string{"host": "domain host is required"})
	}
	if !hostnameWithinCertificate(certificate, host) {
		return contracterr.CertificateIncompatible("certificate does not cover domain host "+host, map[string]string{"host": host, "certificateId": certificateID})
	}
	return nil
}

func (policy certificateBindingPolicy) ProxyBoundToCertificate(ctx context.Context, certificateID string) (domain.Proxy, bool, error) {
	webDomain, err := policy.Store.Domains().ByCertificateID(ctx, certificateID)
	if errors.Is(err, store.ErrNotFound) {
		return domain.Proxy{}, false, nil
	}
	if err != nil {
		return domain.Proxy{}, false, err
	}
	proxies, err := policy.Store.Proxies().ByDomainID(ctx, webDomain.ID)
	if err != nil {
		return domain.Proxy{}, false, err
	}
	if len(proxies) == 0 {
		return domain.Proxy{}, true, nil
	}
	return proxies[0], true, nil
}

func (policy certificateBindingPolicy) UnbindProxy(ctx context.Context, proxy domain.Proxy) error {
	if strings.TrimSpace(proxy.DomainID) == "" {
		proxy.CertificateID = ""
		proxy.CertFile = ""
		proxy.KeyFile = ""
		return policy.Store.Proxies().Update(ctx, proxy)
	}
	webDomain, err := policy.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return err
	}
	return policy.UnbindDomain(ctx, webDomain)
}

func (policy certificateBindingPolicy) UnbindDomain(ctx context.Context, webDomain domain.Domain) error {
	webDomain.CertificateID = ""
	if err := policy.Store.Domains().Update(ctx, webDomain); err != nil {
		return err
	}
	proxies, err := policy.Store.Proxies().ByDomainID(ctx, webDomain.ID)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		if proxy.Status == domain.ProxyEnabled {
			proxy.Status = domain.ProxyNeedsConf
			proxy.CertificateID = ""
			if err := policy.Store.Proxies().Update(ctx, proxy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (policy certificateBindingPolicy) BoundDomain(ctx context.Context, webDomain domain.Domain) (domain.ManagedCertificate, error) {
	if strings.TrimSpace(webDomain.CertificateID) == "" {
		return domain.ManagedCertificate{}, store.ErrNotFound
	}
	return policy.Store.Certificates().ByID(ctx, webDomain.CertificateID)
}

func (policy certificateBindingPolicy) BoundProxy(ctx context.Context, proxy domain.Proxy) (domain.ManagedCertificate, error) {
	if strings.TrimSpace(proxy.DomainID) != "" {
		webDomain, err := policy.Store.Domains().ByID(ctx, proxy.DomainID)
		if err == nil {
			return policy.BoundDomain(ctx, webDomain)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.ManagedCertificate{}, err
		}
	}
	if strings.TrimSpace(proxy.CertificateID) != "" {
		certificate, err := policy.Store.Certificates().ByID(ctx, proxy.CertificateID)
		if err == nil {
			return certificate, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.ManagedCertificate{}, err
		}
	}
	return policy.Store.Certificates().ByProxyID(ctx, proxy.ID)
}

func (policy certificateBindingPolicy) CertificateServable(certificate domain.ManagedCertificate) bool {
	if certificate.ProviderStatus.BlocksServing() || strings.TrimSpace(certificate.CertFile) == "" || strings.TrimSpace(certificate.KeyFile) == "" {
		return false
	}
	health := httpsproxy.CheckCertificateFiles(certificate.Host, certificate.CertFile, certificate.KeyFile, policy.Certificates.Storage.CertificateDir, 0, time.Now().UTC())
	return health.ServingStatus.ServesTLS()
}

func (policy certificateBindingPolicy) CleanupManagedFiles(certificate domain.ManagedCertificate) {
	certificateDir := strings.TrimSpace(policy.Certificates.Storage.CertificateDir)
	if certificateDir == "" {
		return
	}
	for _, path := range []string{certificate.CertFile, certificate.KeyFile, certificate.PreviousCertFile, certificate.PreviousKeyFile} {
		removeManagedFile(certificateDir, path)
	}
}

var _ CertificateBindingPolicy = certificateBindingPolicy{}
