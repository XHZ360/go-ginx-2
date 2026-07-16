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

var (
	ErrProviderCredentialDisabled           = errors.New("provider credential is disabled")
	ErrProviderCredentialVerificationFailed = errors.New("provider credential verification failed")
)

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

// CertificateIssueRequest 以证书身份（而非 proxyID）为核心描述一次签发/注册。
// CertificateID 为空表示创建新证书资源；否则在既有资源上重新签发。
// 对 provider_type=file，仅登记已有的 cert/key 文件路径（校验后写入元数据，不入库私钥）。
type CertificateIssueRequest struct {
	CertificateID     string
	Host              string
	ProviderType      domain.CertificateProviderType
	CredentialID      string
	Hostnames         []string
	RequestType       string
	RequestedValidity int
	// CertFile/KeyFile 仅用于 provider_type=file 的文件型证书登记。
	CertFile string
	KeyFile  string
}

type OriginCARevokeRequest struct {
	// CertificateID 优先；为空时回退到按 ProxyID 解析绑定证书。
	CertificateID           string
	ProxyID                 string
	Host                    string
	CloudflareCertificateID string
}

func (service Service) Issue(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	return service.IssueWithProvider(ctx, ManagedCertificateRequest{ProxyID: proxyID, ProviderType: domain.CertificateProviderACMEDNS01})
}

func (service Service) Renew(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.RenewCertificate(ctx, certificate)
}

// RenewByID 按证书身份（certificateID 优先，否则 proxyID 绑定）续期/重新签发。
func (service Service) RenewByID(ctx context.Context, certificateID string, proxyID string) (domain.ManagedCertificate, error) {
	certificate, err := service.resolveCertificate(ctx, certificateID, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.RenewCertificate(ctx, certificate)
}

// RotateOriginCAByID 按证书身份轮换 Origin CA 证书。
func (service Service) RotateOriginCAByID(ctx context.Context, certificateID string, proxyID string) (domain.ManagedCertificate, error) {
	certificate, err := service.resolveCertificate(ctx, certificateID, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.rotateOriginCACertificate(ctx, certificate)
}

// SyncOriginCAByID 按证书身份同步 Origin CA 证书状态。
func (service Service) SyncOriginCAByID(ctx context.Context, certificateID string, proxyID string) (domain.ManagedCertificate, error) {
	certificate, err := service.resolveCertificate(ctx, certificateID, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.SyncOriginCACertificate(ctx, certificate)
}

func (service Service) IssueWithProvider(ctx context.Context, request ManagedCertificateRequest) (domain.ManagedCertificate, error) {
	if request.ProviderType == "" {
		request.ProviderType = domain.CertificateProviderACMEDNS01
	}
	if err := service.requireProviderReady(request.ProviderType); err != nil {
		return domain.ManagedCertificate{}, err
	}
	provider, err := service.providerFor(request.ProviderType)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return provider.Issue(ctx, service, request, domain.CertificateIssueFailed)
}

// IssueCertificate 以证书身份签发/注册证书资源（可未绑定代理）。
//   - acme_dns01：发起 ACME 签发；
//   - cloudflare_origin_ca：发起 Origin CA 签发；
//   - file：仅登记已有的 cert/key 文件路径（校验证书对，写入元数据，不入库私钥）。
//
// CertificateID 为空表示创建新资源；否则在既有资源上重新签发/重新登记。
func (service Service) IssueCertificate(ctx context.Context, request CertificateIssueRequest) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	host := strings.ToLower(strings.TrimSpace(request.Host))
	if host == "" && strings.TrimSpace(request.CertificateID) != "" {
		existing, err := service.Store.Certificates().ByID(ctx, request.CertificateID)
		if err != nil {
			return domain.ManagedCertificate{}, err
		}
		host = strings.ToLower(strings.TrimSpace(existing.Host))
	}
	if host == "" {
		return domain.ManagedCertificate{}, errors.New("certificate host is required")
	}
	providerType := request.ProviderType
	if providerType == "" {
		providerType = domain.CertificateProviderACMEDNS01
	}
	proxyID := ""
	if strings.TrimSpace(request.CertificateID) != "" {
		if existing, err := service.Store.Certificates().ByID(ctx, request.CertificateID); err == nil {
			proxyID = existing.ProxyID
		} else if !errors.Is(err, store.ErrNotFound) {
			return domain.ManagedCertificate{}, err
		}
	}
	switch providerType {
	case domain.CertificateProviderACMEDNS01:
		if err := service.requireProviderReady(providerType); err != nil {
			return domain.ManagedCertificate{}, err
		}
		return service.issueACMEForIdentity(ctx, request.CertificateID, proxyID, host, nil, domain.CertificateIssueFailed)
	case domain.CertificateProviderCloudflareOriginCA:
		if err := service.requireProviderReady(providerType); err != nil {
			return domain.ManagedCertificate{}, err
		}
		req := ManagedCertificateRequest{ProviderType: providerType, CredentialID: request.CredentialID, Hostnames: request.Hostnames, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity}
		return service.issueOriginCAForIdentity(ctx, request.CertificateID, proxyID, host, req, nil, domain.CertificateIssueFailed)
	case domain.CertificateProviderFile:
		return service.registerFileCertificate(ctx, request.CertificateID, proxyID, host, request.CertFile, request.KeyFile)
	default:
		return domain.ManagedCertificate{}, errors.New("unsupported certificate provider " + string(providerType))
	}
}

// registerFileCertificate 登记一对已存在的静态证书/私钥文件为文件型证书资源（provider_type=file）。
// 仅写入文件路径与从文件派生的健康元数据，私钥内容不入库。
func (service Service) registerFileCertificate(ctx context.Context, certificateID string, proxyID string, host string, certFile string, keyFile string) (domain.ManagedCertificate, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return domain.ManagedCertificate{}, errors.New("certificate host is required")
	}
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" || keyFile == "" {
		return domain.ManagedCertificate{}, errors.New("file certificate requires both cert file and key file")
	}
	now := service.now()
	health := httpsproxy.CheckCertificateFiles(host, certFile, keyFile, service.Storage.CertificateDir, service.scheduler().WindowFor(domain.CertificateProviderFile), now)
	certificate, err := service.ensureCertificateRecordByIdentity(ctx, certificateID, proxyID, host, domain.CertificateProviderFile, string(domain.CertificateProviderFile), "", nil)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	notAfter := now
	if health.NotAfter != nil {
		notAfter = *health.NotAfter
	}
	servingStatus := health.ServingStatus
	if servingStatus == "" {
		servingStatus = domain.CertificateServingMissing
	}
	// 先登记文件路径与基础元数据（provider_type=file），UpdateSuccess 会写入 cert_file/key_file。
	result := store.CertificateSuccess{CertFile: certFile, KeyFile: keyFile, NotAfter: notAfter, ServingStatus: servingStatus, ProviderStatus: domain.CertificateProviderStatusActive, ProviderType: domain.CertificateProviderFile, ProviderName: string(domain.CertificateProviderFile), Fingerprint: health.Fingerprint, LastCheckedAt: now, LastAttemptedAt: now, CompletedAt: now}
	if err := service.Store.Certificates().UpdateSuccess(ctx, certificate.ID, result); err != nil {
		return domain.ManagedCertificate{}, err
	}
	if !health.Usable() {
		// 文件不可用：重放真实健康，使 serving/status 反映问题（代理将被视为 needs_config）。
		if err := service.Store.Certificates().UpdateHealth(ctx, certificate.ID, store.CertificateHealth{ServingStatus: servingStatus, NotAfter: health.NotAfter, Fingerprint: health.Fingerprint, LastError: defaultString(health.ErrorSummary, "certificate active material is not usable"), CheckedAt: now}); err != nil {
			return domain.ManagedCertificate{}, err
		}
	}
	return service.Store.Certificates().ByID(ctx, certificate.ID)
}

func (service Service) RotateOriginCA(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.rotateOriginCACertificate(ctx, certificate)
}

func (service Service) RenewCertificate(ctx context.Context, certificate domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	provider, err := service.providerFor(certificate.ProviderType)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return provider.Renew(ctx, service, certificate)
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
	return service.SyncOriginCACertificate(ctx, certificate)
}

func (service Service) SyncOriginCACertificate(ctx context.Context, certificate domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	provider, err := service.providerFor(certificate.ProviderType)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return provider.Sync(ctx, service, certificate)
}

func (service Service) syncOriginCACertificate(ctx context.Context, certificate domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
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
	updated, readErr := service.Store.Certificates().ByID(ctx, certificate.ID)
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
	certificate, err := service.resolveCertificate(ctx, request.CertificateID, request.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.RevokeOriginCACertificate(ctx, certificate, request)
}

// resolveCertificate 按证书身份解析证书：优先 certificateID，其次按绑定 proxyID 反向查找。
func (service Service) resolveCertificate(ctx context.Context, certificateID string, proxyID string) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	if strings.TrimSpace(certificateID) != "" {
		return service.Store.Certificates().ByID(ctx, certificateID)
	}
	if strings.TrimSpace(proxyID) != "" {
		proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
		if err == nil && strings.TrimSpace(proxy.CertificateID) != "" {
			if certificate, certErr := service.Store.Certificates().ByID(ctx, proxy.CertificateID); certErr == nil {
				return certificate, nil
			}
		}
		return service.Store.Certificates().ByProxyID(ctx, proxyID)
	}
	return domain.ManagedCertificate{}, errors.New("certificate id or proxy id is required")
}

func (service Service) RevokeOriginCACertificate(ctx context.Context, certificate domain.ManagedCertificate, request OriginCARevokeRequest) (domain.ManagedCertificate, error) {
	provider, err := service.providerFor(certificate.ProviderType)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return provider.Revoke(ctx, service, certificate, request)
}

func (service Service) revokeOriginCACertificate(ctx context.Context, certificate domain.ManagedCertificate, request OriginCARevokeRequest) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
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
	return service.Store.Certificates().ByID(ctx, certificate.ID)
}

func (service Service) VerifyProviderCredential(ctx context.Context, credentialID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	credential, err := service.Store.ProviderCredentials().ByID(ctx, credentialID)
	if err != nil {
		return err
	}
	if credential.Status == domain.ProviderCredentialDisabled {
		return ErrProviderCredentialDisabled
	}
	token, err := service.readSecretToken(ctx, credential, true)
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
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.issueACMEForProxy(ctx, proxy, nil, failureStatus)
}

func (service Service) issueACMEForCertificate(ctx context.Context, certificate domain.ManagedCertificate, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	host := strings.ToLower(strings.TrimSpace(certificate.Host))
	if host == "" {
		return domain.ManagedCertificate{}, errors.New("certificate host is required")
	}
	return service.issueACMEForIdentity(ctx, certificate.ID, certificate.ProxyID, host, &certificate, failureStatus)
}

func (service Service) issueACMEForProxy(ctx context.Context, proxy domain.Proxy, existing *domain.ManagedCertificate, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	host, err := service.proxyCertificateHost(ctx, proxy)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificateID := ""
	if existing != nil {
		certificateID = existing.ID
	}
	return service.issueACMEForIdentity(ctx, certificateID, proxy.ID, host, existing, failureStatus)
}

// issueACMEForIdentity 以证书身份（certificateID/proxyID/host）执行 ACME 签发。
// proxyID 可为空，表示对未绑定证书资源签发。
func (service Service) issueACMEForIdentity(ctx context.Context, certificateID string, proxyID string, host string, existing *domain.ManagedCertificate, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	if service.Issuer == nil {
		return domain.ManagedCertificate{}, errors.New("issuer is required")
	}
	if service.DNSProvider == nil {
		return domain.ManagedCertificate{}, errors.New("dns challenge provider is required")
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return domain.ManagedCertificate{}, errors.New("certificate host is required")
	}
	release, err := acquireOperationLock(ctx, certificateOperationKey(host, host))
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	defer release()
	providerName := defaultString(service.Settings.DNSProvider, "cloudflare")
	certificate, err := service.ensureCertificateRecordByIdentity(ctx, certificateID, proxyID, host, domain.CertificateProviderACMEDNS01, providerName, "", existing)
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
	result := store.CertificateSuccess{CertFile: stored.CertFile, KeyFile: stored.KeyFile, PreviousCertFile: stored.PreviousCertFile, PreviousKeyFile: stored.PreviousKeyFile, NotAfter: stored.NotAfter, ServingStatus: service.scheduler().ServingStatus(stored.NotAfter, domain.CertificateProviderACMEDNS01, now), ProviderStatus: domain.CertificateProviderStatusUnknown, ProviderType: domain.CertificateProviderACMEDNS01, ProviderName: providerName, Fingerprint: stored.Fingerprint, LastCheckedAt: now, LastAttemptedAt: now, CompletedAt: now}
	if err := validateProviderSuccess(result); err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		return domain.ManagedCertificate{}, err
	}
	if err := service.Store.Certificates().UpdateSuccess(ctx, certificate.ID, result); err != nil {
		return domain.ManagedCertificate{}, err
	}
	updated, err := service.Store.Certificates().ByID(ctx, certificate.ID)
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
	return service.issueOriginCAForProxy(ctx, proxy, request, nil, failureStatus)
}

func (service Service) issueOriginCAForProxy(ctx context.Context, proxy domain.Proxy, request ManagedCertificateRequest, existing *domain.ManagedCertificate, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	host, err := service.proxyCertificateHost(ctx, proxy)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificateID := ""
	if existing != nil {
		certificateID = existing.ID
	}
	return service.issueOriginCAForIdentity(ctx, certificateID, proxy.ID, host, request, existing, failureStatus)
}

func (service Service) proxyCertificateHost(ctx context.Context, proxy domain.Proxy) (string, error) {
	if proxy.Type.IsWeb() {
		if strings.TrimSpace(proxy.DomainID) != "" && service.Store != nil {
			webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
			if err == nil {
				host := strings.ToLower(strings.TrimSpace(webDomain.Host))
				if host != "" {
					return host, nil
				}
			}
		}
		host := strings.ToLower(strings.TrimSpace(proxy.EntryHost))
		if host != "" {
			return host, nil
		}
		return "", errors.New("web proxy domain host is required")
	}
	if proxy.Type != domain.ProxyHTTPS {
		return "", errors.New("managed certificates require a web or https proxy")
	}
	host := strings.ToLower(strings.TrimSpace(proxy.EntryHost))
	if host == "" {
		return "", errors.New("https proxy host is required")
	}
	return host, nil
}

// issueOriginCAForIdentity 以证书身份执行 Cloudflare Origin CA 签发/轮换。proxyID 可为空（未绑定证书）。
func (service Service) issueOriginCAForIdentity(ctx context.Context, certificateID string, proxyID string, host string, request ManagedCertificateRequest, existing *domain.ManagedCertificate, failureStatus domain.CertificateStatus) (domain.ManagedCertificate, error) {
	if !service.OriginCASettings.Enabled {
		return domain.ManagedCertificate{}, errors.New("cloudflare origin ca is disabled")
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return domain.ManagedCertificate{}, errors.New("certificate host is required")
	}
	release, err := acquireOperationLock(ctx, certificateOperationKey(host, host))
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	defer release()
	credentialID, err := service.resolveCredentialID(ctx, request.CredentialID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := service.ensureCertificateRecordByIdentity(ctx, certificateID, proxyID, host, domain.CertificateProviderCloudflareOriginCA, "cloudflare", credentialID, existing)
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
		_ = service.cleanupUnusableInitialIssue(ctx, certificate)
		return domain.ManagedCertificate{}, err
	}
	token, err := service.credentialToken(ctx, credentialID)
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		_ = service.cleanupUnusableInitialIssue(ctx, certificate)
		return domain.ManagedCertificate{}, err
	}
	remote, err := service.originCAClient().Create(ctx, token, OriginCACreateRequest{CSR: string(csrPEM), Hostnames: hostnames, RequestType: requestType, RequestedValidity: requestedValidity})
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		_ = service.cleanupUnusableInitialIssue(ctx, certificate)
		return domain.ManagedCertificate{}, err
	}
	if len(remote.CertificatePEM) == 0 {
		err := errors.New("cloudflare origin ca response missing certificate")
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		_ = service.cleanupUnusableInitialIssue(ctx, certificate)
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
	if err := validateOriginCASuccess(store.CertificateSuccess{ProviderStatus: domain.CertificateProviderStatusActive, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CredentialID: credentialID, CloudflareID: remote.ID, Hostnames: hostnames, RequestType: requestType, RequestedValidity: requestedValidity, CertFile: "pending.crt", KeyFile: "pending.key", NotAfter: now}); err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		_ = service.cleanupUnusableInitialIssue(ctx, certificate)
		return domain.ManagedCertificate{}, err
	}
	storage := service.Storage
	if storage.Now == nil {
		storage.Now = service.now
	}
	stored, err := storage.Store(host, remote.CertificatePEM, keyPEM)
	if err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		_ = service.cleanupUnusableInitialIssue(ctx, certificate)
		return domain.ManagedCertificate{}, err
	}
	lastSyncedAt := now
	result := store.CertificateSuccess{CertFile: stored.CertFile, KeyFile: stored.KeyFile, PreviousCertFile: stored.PreviousCertFile, PreviousKeyFile: stored.PreviousKeyFile, NotAfter: stored.NotAfter, ServingStatus: service.scheduler().ServingStatus(stored.NotAfter, domain.CertificateProviderCloudflareOriginCA, now), ProviderStatus: domain.CertificateProviderStatusActive, ProviderType: domain.CertificateProviderCloudflareOriginCA, ProviderName: "cloudflare", CredentialID: credentialID, CloudflareID: remote.ID, PreviousCloudflareID: previousCloudflareID, Hostnames: hostnames, RequestType: requestType, RequestedValidity: requestedValidity, Fingerprint: stored.Fingerprint, LastCheckedAt: now, LastAttemptedAt: now, LastSyncedAt: &lastSyncedAt, CompletedAt: now}
	if err := validateProviderSuccess(result); err != nil {
		_ = service.recordFailure(ctx, certificate, host, failureStatus, err)
		_ = service.cleanupUnusableInitialIssue(ctx, certificate)
		return domain.ManagedCertificate{}, err
	}
	if err := service.Store.Certificates().UpdateSuccess(ctx, certificate.ID, result); err != nil {
		return domain.ManagedCertificate{}, err
	}
	return service.Store.Certificates().ByID(ctx, certificate.ID)
}

func (service Service) rotateOriginCACertificate(ctx context.Context, certificate domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	if service.Store == nil {
		return domain.ManagedCertificate{}, errors.New("store is required")
	}
	request := ManagedCertificateRequest{ProxyID: certificate.ProxyID, ProviderType: domain.CertificateProviderCloudflareOriginCA, CredentialID: certificate.CredentialID, Hostnames: certificate.Hostnames, RequestType: certificate.RequestType, RequestedValidity: certificate.RequestedValidity}
	return service.issueOriginCAForIdentity(ctx, certificate.ID, certificate.ProxyID, certificate.Host, request, &certificate, domain.CertificateRenewalFailed)
}

func (service Service) recordFailure(ctx context.Context, certificate domain.ManagedCertificate, host string, failureStatus domain.CertificateStatus, cause error) error {
	now := service.now()
	health := service.activeHealth(host, certificate, now)
	_ = service.Store.Certificates().UpdateHealth(ctx, certificate.ID, store.CertificateHealth{ServingStatus: health.ServingStatus, NotAfter: health.NotAfter, Fingerprint: health.Fingerprint, LastError: health.ErrorSummary, CheckedAt: now})
	failureCount := certificate.FailureCount + 1
	nextAttemptAt := service.scheduler().NextAttemptAt(now, failureCount, certificate.NotAfter)
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

func (service Service) cleanupUnusableInitialIssue(ctx context.Context, certificate domain.ManagedCertificate) error {
	if strings.TrimSpace(certificate.ProxyID) != "" {
		return nil
	}
	if strings.TrimSpace(certificate.CertFile) != "" || strings.TrimSpace(certificate.KeyFile) != "" {
		return nil
	}
	if strings.TrimSpace(certificate.CloudflareCertificateID) != "" {
		return nil
	}
	return service.Store.Certificates().Delete(ctx, certificate.ID)
}

func (service Service) activeHealth(host string, certificate domain.ManagedCertificate, now time.Time) httpsproxy.CertificateMaterialHealth {
	if certificate.CertFile == "" || certificate.KeyFile == "" {
		return httpsproxy.CertificateMaterialHealth{ServingStatus: domain.CertificateServingMissing, ErrorSummary: "certificate active material is missing"}
	}
	return httpsproxy.CheckCertificateFiles(host, certificate.CertFile, certificate.KeyFile, service.Storage.CertificateDir, service.scheduler().WindowFor(certificate.ProviderType), now)
}

func (service Service) scheduler() LifecycleScheduler {
	return LifecycleScheduler{RenewalWindow: service.Settings.RenewalWindow, OriginCARotationWindow: service.OriginCASettings.RotationWindow}
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

func (service Service) ensureCertificateRecord(ctx context.Context, proxyID string, host string, providerType domain.CertificateProviderType, providerName string, credentialID string, existing *domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	return service.ensureCertificateRecordByIdentity(ctx, "", proxyID, host, providerType, providerName, credentialID, existing)
}

// ensureCertificateRecordByIdentity 解析或创建证书记录。优先级：
//  1. existing（调用方已持有）；
//  2. certificateID（按证书身份查找，未绑定证书亦可）；
//  3. proxyID（遗留：按绑定代理反向引用查找）；
//  4. 创建新资源（proxyID 可为空，表示未绑定证书）。
func (service Service) ensureCertificateRecordByIdentity(ctx context.Context, certificateID string, proxyID string, host string, providerType domain.CertificateProviderType, providerName string, credentialID string, existing *domain.ManagedCertificate) (domain.ManagedCertificate, error) {
	if existing != nil && existing.ID != "" {
		return *existing, nil
	}
	if strings.TrimSpace(certificateID) != "" {
		certificate, err := service.Store.Certificates().ByID(ctx, certificateID)
		if err == nil {
			return certificate, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.ManagedCertificate{}, err
		}
	} else if strings.TrimSpace(proxyID) != "" {
		certificate, err := service.Store.Certificates().ByProxyID(ctx, proxyID)
		if err == nil {
			return certificate, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.ManagedCertificate{}, err
		}
	}
	id := strings.TrimSpace(certificateID)
	if id == "" {
		newID, err := service.newID()
		if err != nil {
			return domain.ManagedCertificate{}, err
		}
		id = newID
	}
	now := service.now()
	certificate := domain.ManagedCertificate{ID: id, ProxyID: proxyID, Host: host, Status: domain.CertificatePending, Provider: providerName, ProviderType: providerType, ProviderName: providerName, CredentialID: credentialID, ProviderStatus: domain.CertificateProviderStatusUnknown, CreatedAt: now, UpdatedAt: now}
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
	return service.readSecretToken(ctx, credential, false)
}

func (service Service) readSecretToken(ctx context.Context, credential domain.ProviderCredential, allowVerificationFailed bool) (string, error) {
	if credential.ProviderType != domain.CertificateProviderCloudflareOriginCA {
		return "", errors.New("provider credential is not for cloudflare origin ca")
	}
	if credential.Status == domain.ProviderCredentialDisabled {
		return "", ErrProviderCredentialDisabled
	}
	if credential.Status == domain.ProviderCredentialVerificationFailed && !allowVerificationFailed {
		return "", ErrProviderCredentialVerificationFailed
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
	credentials, err := service.Store.ProviderCredentials().ListByProviderType(ctx, domain.CertificateProviderCloudflareOriginCA, []domain.ProviderCredentialStatus{
		domain.ProviderCredentialPending,
		domain.ProviderCredentialVerified,
	})
	if err != nil {
		return "", err
	}
	if len(credentials) == 0 {
		return "", errors.New("cloudflare origin ca credential is required")
	}
	if len(credentials) > 1 {
		return "", errors.New("cloudflare origin ca credential id is required when multiple credentials exist")
	}
	return credentials[0].ID, nil
}

// MigrateLegacyFileCertificates 将旧代理静态证书（cert_file/key_file 非空且尚未绑定 certificate_id）
// 迁移为 provider_type=file 的证书资源，并将代理的 certificate_id 指向该资源。幂等：已绑定的代理跳过。
// 返回新建并绑定的证书数量。
func (service Service) MigrateLegacyFileCertificates(ctx context.Context) (int, error) {
	if service.Store == nil {
		return 0, errors.New("store is required")
	}
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return 0, err
	}
	migrated := 0
	for _, proxy := range proxies {
		if strings.TrimSpace(proxy.CertificateID) != "" {
			continue
		}
		if strings.TrimSpace(proxy.CertFile) == "" && strings.TrimSpace(proxy.KeyFile) == "" {
			// also migrate domain-bound static leftovers via domain host if domain has no cert
			if proxy.Type.IsWeb() && strings.TrimSpace(proxy.DomainID) != "" {
				webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
				if err != nil || strings.TrimSpace(webDomain.CertificateID) != "" {
					continue
				}
			}
			continue
		}
		host := strings.ToLower(strings.TrimSpace(proxy.EntryHost))
		if host == "" && proxy.Type.IsWeb() && strings.TrimSpace(proxy.DomainID) != "" {
			if webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID); err == nil {
				host = strings.ToLower(strings.TrimSpace(webDomain.Host))
			}
		}
		if host == "" {
			continue
		}
		certificate, err := service.registerFileCertificate(ctx, "", proxy.ID, host, proxy.CertFile, proxy.KeyFile)
		if err != nil {
			return migrated, err
		}
		if proxy.Type.IsWeb() && strings.TrimSpace(proxy.DomainID) != "" {
			webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
			if err != nil {
				return migrated, err
			}
			if strings.TrimSpace(webDomain.CertificateID) == "" {
				webDomain.CertificateID = certificate.ID
				if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
					return migrated, err
				}
			}
		} else {
			bound, err := service.Store.Proxies().ByID(ctx, proxy.ID)
			if err != nil {
				return migrated, err
			}
			bound.CertificateID = certificate.ID
			if err := service.Store.Proxies().Update(ctx, bound); err != nil {
				return migrated, err
			}
		}
		// clear static paths so re-run is idempotent
		bound, err := service.Store.Proxies().ByID(ctx, proxy.ID)
		if err != nil {
			return migrated, err
		}
		bound.CertFile = ""
		bound.KeyFile = ""
		if err := service.Store.Proxies().Update(ctx, bound); err != nil {
			return migrated, err
		}
		migrated++
	}
	return migrated, nil
}

// CertificateCoversHost 校验给定证书资源的活跃材料是否覆盖指定 SNI 主机（VerifyHostname 语义）。
// 仅当证书材料可用且主机匹配时返回 nil；否则返回描述性错误。
func (service Service) CertificateCoversHost(certificate domain.ManagedCertificate, host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return errors.New("certificate host is required")
	}
	if strings.TrimSpace(certificate.CertFile) == "" || strings.TrimSpace(certificate.KeyFile) == "" {
		return errors.New("certificate active material is missing")
	}
	health := httpsproxy.CheckCertificateFiles(host, certificate.CertFile, certificate.KeyFile, service.Storage.CertificateDir, 0, service.now())
	if health.ServingStatus == domain.CertificateServingInvalid {
		summary := health.ErrorSummary
		if summary == "" {
			summary = "certificate does not cover host " + host
		}
		return errors.New(summary)
	}
	return nil
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
