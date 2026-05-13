package certmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Service struct {
	Store       store.Store
	Issuer      Issuer
	DNSProvider DNSChallengeProvider
	Storage     httpsproxy.ManagedCertificateStorage
	Settings    domain.ACMEProviderSettings
	NewID       func() (string, error)
	Now         func() time.Time
}

type CertificateStatus struct {
	Certificate domain.ManagedCertificate
}

func (service Service) Issue(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	return service.issue(ctx, proxyID, domain.CertificateIssueFailed)
}

func (service Service) Renew(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	return service.issue(ctx, proxyID, domain.CertificateRenewalFailed)
}

func (service Service) Status(ctx context.Context, proxyID string) (CertificateStatus, error) {
	if service.Store == nil {
		return CertificateStatus{}, errors.New("store is required")
	}
	certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err != nil {
		return CertificateStatus{}, err
	}
	return CertificateStatus{Certificate: certificate}, nil
}

func (service Service) issue(ctx context.Context, proxyID string, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	if service.Issuer == nil {
		return domain.ManagedCertificate{}, errors.New("issuer is required")
	}
	if service.DNSProvider == nil {
		return domain.ManagedCertificate{}, errors.New("dns challenge provider is required")
	}
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	if proxy.Type != domain.ProxyHTTPS {
		return domain.ManagedCertificate{}, errors.New("managed certificates require an https proxy")
	}
	host := strings.ToLower(strings.TrimSpace(proxy.EntryHost))
	if host == "" {
		return domain.ManagedCertificate{}, errors.New("https proxy host is required")
	}
	certificate, err := service.ensureCertificateRecord(ctx, proxy.ID, host)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	issued, err := service.Issuer.Issue(ctx, IssueRequest{Host: host, AccountEmail: service.Settings.AccountEmail, DirectoryURL: service.Settings.DirectoryURL, TermsAccepted: service.Settings.TermsAccepted, DNSProvider: service.DNSProvider})
	if err != nil {
		_ = service.Store.Certificates().UpdateFailure(ctx, certificate.ID, store.CertificateFailure{Status: failureStatus, LastError: safeError(err), CompletedAt: service.now()})
		return domain.ManagedCertificate{}, err
	}
	storage := service.Storage
	if storage.Now == nil {
		storage.Now = service.now
	}
	stored, err := storage.Store(host, issued.CertPEM, issued.KeyPEM)
	if err != nil {
		_ = service.Store.Certificates().UpdateFailure(ctx, certificate.ID, store.CertificateFailure{Status: failureStatus, LastError: safeError(err), CompletedAt: service.now()})
		return domain.ManagedCertificate{}, err
	}
	if err := service.Store.Certificates().UpdateSuccess(ctx, certificate.ID, store.CertificateSuccess{CertFile: stored.CertFile, KeyFile: stored.KeyFile, PreviousCertFile: stored.PreviousCertFile, PreviousKeyFile: stored.PreviousKeyFile, NotAfter: stored.NotAfter, CompletedAt: service.now()}); err != nil {
		return domain.ManagedCertificate{}, err
	}
	updated, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return updated, nil
}

func (service Service) ensureCertificateRecord(ctx context.Context, proxyID string, host string) (domain.ManagedCertificate, error) {
	certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err == nil {
		return certificate, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return domain.ManagedCertificate{}, err
	}
	id, err := service.newID()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	now := service.now()
	certificate = domain.ManagedCertificate{ID: id, ProxyID: proxyID, Host: host, Status: domain.CertificatePending, Provider: service.Settings.DNSProvider, CreatedAt: now, UpdatedAt: now}
	if certificate.Provider == "" {
		certificate.Provider = "cloudflare"
	}
	if err := service.Store.Certificates().Create(ctx, certificate); err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, nil
}

func (service Service) newID() (string, error) {
	if service.NewID != nil {
		return service.NewID()
	}
	return "cert_" + fmt.Sprint(service.now().UnixNano()), nil
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 512 {
		message = message[:512]
	}
	return strings.ReplaceAll(message, "\n", " ")
}
