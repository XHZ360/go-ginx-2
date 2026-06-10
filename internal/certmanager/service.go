package certmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Service struct {
	Store               store.Store
	Issuer              Issuer
	DNSProvider         DNSChallengeProvider
	OriginCAClient      OriginCAClient
	ProviderSecretStore SecretStore
	Storage             httpsproxy.ManagedCertificateStorage
	Settings            domain.ACMEProviderSettings
	OriginCASettings    domain.OriginCAProviderSettings
	NewID               func() (string, error)
	Now                 func() time.Time
}

var operationLocks sync.Map

var ErrOperationBusy = errors.New("certificate operation already in progress")

type CertificateStatus struct {
	Certificate domain.ManagedCertificate
}

type ManagedCertificateRequest struct {
	ProxyID           string
	ProviderType      domain.CertificateProviderType
	CredentialID      string
	Hostnames         []string
	RequestType       string
	RequestedValidity int
}

type OriginCARevokeRequest struct {
	ProxyID                 string
	Host                    string
	CloudflareCertificateID string
}

func (service Service) Issue(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	return service.issueACME(ctx, proxyID, domain.CertificateIssueFailed)
}

func (service Service) Renew(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err == nil && certificate.ProviderType == domain.CertificateProviderCloudflareOriginCA {
		return service.rotateOriginCA(ctx, proxyID)
	}
	return service.issueACME(ctx, proxyID, domain.CertificateRenewalFailed)
}

func (service Service) IssueWithProvider(ctx context.Context, request ManagedCertificateRequest) (domain.ManagedCertificate, error) {
	if request.ProviderType == "" || request.ProviderType == domain.CertificateProviderACMEDNS01 {
		return service.Issue(ctx, request.ProxyID)
	}
	if request.ProviderType == domain.CertificateProviderCloudflareOriginCA {
		return service.issueOriginCA(ctx, request, domain.CertificateIssueFailed)
	}
	return domain.ManagedCertificate{}, fmt.Errorf("unsupported certificate provider %q", request.ProviderType)
}

func (service Service) RotateOriginCA(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	return service.rotateOriginCA(ctx, proxyID)
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

func (service Service) SyncOriginCA(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	if certificate.ProviderType != domain.CertificateProviderCloudflareOriginCA {
		return domain.ManagedCertificate{}, errors.New("certificate is not managed by cloudflare origin ca")
	}
	now := service.now()
	token, err := service.credentialToken(ctx, certificate.CredentialID)
	if err != nil {
		_ = service.Store.Certificates().UpdateProviderSync(ctx, certificate.ID, store.CertificateProviderSync{ProviderStatus: providerStatusAfterSyncError(certificate), LastError: httpsproxy.SafeCertificateError(err), SyncedAt: now, UpdatedAt: now})
		return domain.ManagedCertificate{}, err
	}
	remote, err := service.originCAClient().Get(ctx, token, certificate.CloudflareCertificateID)
	status := domain.CertificateProviderStatusActive
	lastError := ""
	if errors.Is(err, ErrOriginCACertificateMissing) {
		status = domain.CertificateProviderStatusMissingRemote
		lastError = providerStatusError(status)
	} else if err != nil {
		status = providerStatusAfterSyncError(certificate)
		lastError = httpsproxy.SafeCertificateError(err)
	} else if remote.RevokedAt != nil || strings.EqualFold(remote.Status, "revoked") {
		status = domain.CertificateProviderStatusRevoked
		lastError = providerStatusError(status)
	}
	if updateErr := service.Store.Certificates().UpdateProviderSync(ctx, certificate.ID, store.CertificateProviderSync{ProviderStatus: status, LastError: lastError, SyncedAt: now, UpdatedAt: now}); updateErr != nil {
		return domain.ManagedCertificate{}, updateErr
	}
	updated, readErr := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if readErr != nil {
		return domain.ManagedCertificate{}, readErr
	}
	if err != nil && !errors.Is(err, ErrOriginCACertificateMissing) {
		return updated, err
	}
	return updated, nil
}

func (service Service) RevokeOriginCA(ctx context.Context, request OriginCARevokeRequest) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	certificate, err := service.Store.Certificates().ByProxyID(ctx, request.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	host := strings.ToLower(strings.TrimSpace(request.Host))
	if host == "" || host != strings.ToLower(strings.TrimSpace(certificate.Host)) {
		return domain.ManagedCertificate{}, errors.New("revoke confirmation host does not match certificate")
	}
	cloudflareID := strings.TrimSpace(request.CloudflareCertificateID)
	if cloudflareID == "" {
		return domain.ManagedCertificate{}, errors.New("cloudflare certificate id confirmation is required")
	}
	active := cloudflareID == certificate.CloudflareCertificateID
	previous := cloudflareID == certificate.PreviousCloudflareCertificateID && certificate.PreviousCloudflareCertificateID != ""
	if !active && !previous {
		return domain.ManagedCertificate{}, errors.New("revoke confirmation cloudflare certificate id does not match active or previous certificate")
	}
	token, err := service.credentialToken(ctx, certificate.CredentialID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	if err := service.originCAClient().Revoke(ctx, token, cloudflareID); err != nil {
		return domain.ManagedCertificate{}, err
	}
	now := service.now()
	if active {
		if err := service.Store.Certificates().UpdateProviderSync(ctx, certificate.ID, store.CertificateProviderSync{ProviderStatus: domain.CertificateProviderStatusRevoked, LastError: providerStatusError(domain.CertificateProviderStatusRevoked), SyncedAt: now, UpdatedAt: now}); err != nil {
			return domain.ManagedCertificate{}, err
		}
	}
	return service.Store.Certificates().ByProxyID(ctx, request.ProxyID)
}

func (service Service) VerifyProviderCredential(ctx context.Context, credentialID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	credential, err := service.Store.ProviderCredentials().ByID(ctx, credentialID)
	if err != nil {
		return err
	}
	token, err := service.readSecretToken(ctx, credential)
	now := service.now()
	if err == nil {
		err = service.originCAClient().VerifyToken(ctx, token)
	}
	if err != nil {
		_ = service.Store.ProviderCredentials().SetStatus(ctx, credential.ID, domain.ProviderCredentialVerificationFailed, &now, httpsproxy.SafeCertificateError(err))
		return err
	}
	return service.Store.ProviderCredentials().SetStatus(ctx, credential.ID, domain.ProviderCredentialVerified, &now, "")
}

func (service Service) issueACME(ctx context.Context, proxyID string, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
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
	release, err := acquireOperationLock(ctx, certificateOperationKey(proxy.ID, host))
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	defer release()
	providerName := defaultString(service.Settings.DNSProvider, "cloudflare")
	certificate, err := service.ensureCertificateRecord(ctx, proxy.ID, host, domain.CertificateProviderACMEDNS01, providerName, "")
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	issued, err := service.Issuer.Issue(ctx, IssueRequest{Host: host, AccountEmail: service.Settings.AccountEmail, DirectoryURL: service.Settings.DirectoryURL, TermsAccepted: service.Settings.TermsAccepted, DNSProvider: service.DNSProvider})
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	storage := service.Storage
	if storage.Now == nil {
		storage.Now = service.now
	}
	stored, err := storage.Store(host, issued.CertPEM, issued.KeyPEM)
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	now := service.now()
	if err := service.Store.Certificates().UpdateSuccess(ctx, certificate.ID, store.CertificateSuccess{CertFile: stored.CertFile, KeyFile: stored.KeyFile, PreviousCertFile: stored.PreviousCertFile, PreviousKeyFile: stored.PreviousKeyFile, NotAfter: stored.NotAfter, ServingStatus: service.servingStatus(stored.NotAfter, now, domain.CertificateProviderACMEDNS01), ProviderStatus: domain.CertificateProviderStatusUnknown, ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: providerName, Fingerprint: stored.Fingerprint, LastCheckedAt: now, LastAttemptedAt: now, CompletedAt: now}); err != nil {
		return domain.ManagedCertificate{}, err
	}
	updated, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return updated, nil
}

func (service Service) issueOriginCA(ctx context.Context, request ManagedCertificateRequest, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	if !service.OriginCASettings.Enabled {
		return domain.ManagedCertificate{}, errors.New("cloudflare origin ca is disabled")
	}
	proxy, err := service.Store.Proxies().ByID(ctx, request.ProxyID)
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
	release, err := acquireOperationLock(ctx, certificateOperationKey(proxy.ID, host))
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	defer release()
	credentialID, err := service.resolveCredentialID(ctx, request.CredentialID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := service.ensureCertificateRecord(ctx, proxy.ID, host, domain.CertificateProviderCloudflareOriginCA, "cloudflare", credentialID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	hostnames := normalizeOriginCAHostnames(append([]string{host}, request.Hostnames...))
	requestType := defaultString(request.RequestType, service.OriginCASettings.DefaultRequestType)
	if requestType == "" {
		requestType = OriginCARequestTypeECC
	}
	requestedValidity := request.RequestedValidity
	if requestedValidity <= 0 {
		requestedValidity = service.OriginCASettings.RequestedValidity
	}
	if requestedValidity <= 0 {
		requestedValidity = 5475
	}
	csrPEM, keyPEM, err := BuildOriginCACSR(hostnames, requestType)
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	token, err := service.credentialToken(ctx, credentialID)
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	remote, err := service.originCAClient().Create(ctx, token, OriginCACreateRequest{CSR: string(csrPEM), Hostnames: hostnames, RequestType: requestType, RequestedValidity: requestedValidity})
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	if len(remote.CertificatePEM) == 0 {
		err := errors.New("cloudflare origin ca response missing certificate")
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	storage := service.Storage
	if storage.Now == nil {
		storage.Now = service.now
	}
	stored, err := storage.Store(host, remote.CertificatePEM, keyPEM)
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	now := service.now()
	previousCloudflareID := ""
	if certificate.CloudflareCertificateID != "" && certificate.CloudflareCertificateID != remote.ID {
		previousCloudflareID = certificate.CloudflareCertificateID
	}
	if len(remote.Hostnames) > 0 {
		hostnames = remote.Hostnames
	}
	if remote.RequestType != "" {
		requestType = remote.RequestType
	}
	if remote.RequestedValidity > 0 {
		requestedValidity = remote.RequestedValidity
	}
	lastSyncedAt := now
	if err := service.Store.Certificates().UpdateSuccess(ctx, certificate.ID, store.CertificateSuccess{CertFile: stored.CertFile, KeyFile: stored.KeyFile, PreviousCertFile: stored.PreviousCertFile, PreviousKeyFile: stored.PreviousKeyFile, NotAfter: stored.NotAfter, ServingStatus: service.servingStatus(stored.NotAfter, now, domain.CertificateProviderCloudflareOriginCA), ProviderStatus: domain.CertificateProviderStatusActive, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CredentialID: credentialID, CloudflareID: remote.ID, PreviousCloudflareID: previousCloudflareID, Hostnames: hostnames, RequestType: requestType, RequestedValidity: requestedValidity, Fingerprint: stored.Fingerprint, LastCheckedAt: now, LastAttemptedAt: now, LastSyncedAt: &lastSyncedAt, CompletedAt: now}); err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.Store.Certificates().ByProxyID(ctx, proxy.ID)
}

func (service Service) rotateOriginCA(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.issueOriginCA(ctx, ManagedCertificateRequest{ProxyID: proxyID, ProviderType: domain.CertificateProviderCloudflareOriginCA, CredentialID: certificate.CredentialID, Hostnames: certificate.Hostnames, RequestType: certificate.RequestType, RequestedValidity: certificate.RequestedValidity}, domain.CertificateRenewalFailed)
}

func (service Service) recordFailure(ctx context.Context, certificate domain.ManagedCertificate, host string, failureStatus domain.CertificateStatus, cause error) error {
	now := service.now()
	health := service.activeHealth(host, certificate, now)
	_ = service.Store.Certificates().UpdateHealth(ctx, certificate.ID, store.CertificateHealth{ServingStatus: health.ServingStatus, NotAfter: health.NotAfter, Fingerprint: health.Fingerprint, LastError: health.ErrorSummary, CheckedAt: now})
	failureCount := certificate.FailureCount + 1
	nextAttemptAt := service.nextAttemptAt(now, failureCount, certificate.NotAfter)
	return service.Store.Certificates().UpdateFailure(ctx, certificate.ID, store.CertificateFailure{
		Status:          failureStatus,
		ServingStatus:   health.ServingStatus,
		OperationStatus: operationStatusFromFailure(failureStatus),
		ProviderStatus:  providerStatusAfterFailure(certificate),
		LastError:       httpsproxy.SafeCertificateError(cause),
		LastCheckedAt:   now,
		LastAttemptedAt: now,
		NextAttemptAt:   &nextAttemptAt,
		FailureCount:    failureCount,
		CompletedAt:     now,
	})
}

func (service Service) activeHealth(host string, certificate domain.ManagedCertificate, now time.Time) httpsproxy.CertificateMaterialHealth {
	if certificate.CertFile == "" || certificate.KeyFile == "" {
		return httpsproxy.CertificateMaterialHealth{ServingStatus: domain.CertificateServingMissing, ErrorSummary: "certificate active material is missing"}
	}
	return httpsproxy.CheckCertificateFiles(host, certificate.CertFile, certificate.KeyFile, service.Storage.CertificateDir, service.renewalWindow(certificate.ProviderType), now)
}

func (service Service) servingStatus(notAfter time.Time, now time.Time, providerType domain.CertificateProviderType) domain.CertificateServingStatus {
	if !notAfter.After(now) {
		return domain.CertificateServingExpired
	}
	if window := service.renewalWindow(providerType); window > 0 && !notAfter.After(now.Add(window)) {
		return domain.CertificateServingExpiringSoon
	}
	return domain.CertificateServingUsable
}

func (service Service) renewalWindow(providerType domain.CertificateProviderType) time.Duration {
	if providerType == domain.CertificateProviderCloudflareOriginCA && service.OriginCASettings.RotationWindow > 0 {
		return service.OriginCASettings.RotationWindow
	}
	return service.Settings.RenewalWindow
}

func (service Service) nextAttemptAt(now time.Time, failureCount int, notAfter *time.Time) time.Time {
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

func operationStatusFromFailure(status domain.CertificateStatus) domain.CertificateOperationStatus {
	if status == domain.CertificateRenewalFailed {
		return domain.CertificateOperationRenewalFailed
	}
	return domain.CertificateOperationIssueFailed
}

func acquireOperationLock(ctx context.Context, key string) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("certificate operation key is required")
	}
	if _, loaded := operationLocks.LoadOrStore(key, struct{}{}); loaded {
		return nil, ErrOperationBusy
	}
	return func() { operationLocks.Delete(key) }, nil
}

func (service Service) ensureCertificateRecord(ctx context.Context, proxyID string, host string, providerType domain.CertificateProviderType, providerName string, credentialID string) (domain.ManagedCertificate, error) {
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
	certificate = domain.ManagedCertificate{ID: id, ProxyID: proxyID, Host: host, Status: domain.CertificatePending, Provider: providerName, ProviderType: providerType, ProviderName: providerName, CredentialID: credentialID, ProviderStatus: domain.CertificateProviderStatusUnknown, CreatedAt: now, UpdatedAt: now}
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

func (service Service) originCAClient() OriginCAClient {
	if service.OriginCAClient != nil {
		return service.OriginCAClient
	}
	return CloudflareOriginCAClient{}
}

func (service Service) credentialToken(ctx context.Context, credentialID string) (string, error) {
	if service.Store == nil {
		return "", errors.New("store is required")
	}
	credential, err := service.Store.ProviderCredentials().ByID(ctx, credentialID)
	if err != nil {
		return "", err
	}
	return service.readSecretToken(ctx, credential)
}

func (service Service) readSecretToken(ctx context.Context, credential domain.ProviderCredential) (string, error) {
	if credential.ProviderType != domain.CertificateProviderCloudflareOriginCA {
		return "", errors.New("provider credential is not for cloudflare origin ca")
	}
	if credential.Status == domain.ProviderCredentialDisabled {
		return "", errors.New("provider credential is disabled")
	}
	if credential.Status == domain.ProviderCredentialVerificationFailed {
		return "", errors.New("provider credential verification failed")
	}
	if service.ProviderSecretStore == nil {
		return "", errors.New("provider secret store is required")
	}
	token, err := service.ProviderSecretStore.Read(ctx, credential.SecretRef)
	if err != nil {
		return "", err
	}
	if err := RejectOriginCAServiceKey(token); err != nil {
		return "", err
	}
	return token, nil
}

func (service Service) resolveCredentialID(ctx context.Context, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	if service.Store == nil {
		return "", errors.New("store is required")
	}
	credentials, err := service.Store.ProviderCredentials().List(ctx)
	if err != nil {
		return "", err
	}
	candidates := make([]domain.ProviderCredential, 0, 1)
	for _, credential := range credentials {
		if credential.ProviderType != domain.CertificateProviderCloudflareOriginCA {
			continue
		}
		if credential.Status == domain.ProviderCredentialDisabled || credential.Status == domain.ProviderCredentialVerificationFailed {
			continue
		}
		candidates = append(candidates, credential)
	}
	if len(candidates) == 0 {
		return "", errors.New("cloudflare origin ca credential is required")
	}
	if len(candidates) > 1 {
		return "", errors.New("cloudflare origin ca credential id is required when multiple credentials exist")
	}
	return candidates[0].ID, nil
}

func providerStatusAfterFailure(certificate domain.ManagedCertificate) domain.CertificateProviderStatus {
	if certificate.ProviderType == domain.CertificateProviderCloudflareOriginCA {
		if certificate.ProviderStatus == domain.CertificateProviderStatusRevoked || certificate.ProviderStatus == domain.CertificateProviderStatusMissingRemote {
			return certificate.ProviderStatus
		}
		return domain.CertificateProviderStatusUnknown
	}
	return certificate.ProviderStatus
}

func providerStatusAfterSyncError(certificate domain.ManagedCertificate) domain.CertificateProviderStatus {
	if certificate.ProviderStatus.BlocksServing() {
		return certificate.ProviderStatus
	}
	return domain.CertificateProviderStatusUnknown
}

func providerStatusError(status domain.CertificateProviderStatus) string {
	switch status {
	case domain.CertificateProviderStatusRevoked:
		return "certificate provider marked active material revoked"
	case domain.CertificateProviderStatusMissingRemote:
		return "certificate provider active material is missing remotely"
	default:
		return ""
	}
}

func certificateOperationKey(proxyID string, host string) string {
	return strings.TrimSpace(proxyID) + "\x00" + strings.ToLower(strings.TrimSpace(host))
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}
