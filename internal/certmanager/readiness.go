package certmanager

import (
	"errors"
	"os"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

const (
	ReadinessACMEDisabled        = "acme_disabled"
	ReadinessAccountEmailMissing = "account_email_missing"
	ReadinessTermsNotAccepted    = "terms_not_accepted"
	ReadinessDNSTokenMissing     = "dns_token_missing"
	ReadinessOriginCADisabled    = "origin_ca_disabled"
	ReadinessCredentialMissing   = "credential_missing"
)

type ProviderReadiness struct {
	ProviderType        domain.CertificateProviderType
	Ready               bool
	MissingRequirements []string
	TokenEnvName        string
	Guidance            string
}

type ProviderNotReadyError struct {
	Readiness ProviderReadiness
}

func (err *ProviderNotReadyError) Error() string {
	return "certificate provider is not ready"
}

func IsProviderNotReady(err error) (*ProviderNotReadyError, bool) {
	var readinessErr *ProviderNotReadyError
	ok := errors.As(err, &readinessErr)
	return readinessErr, ok
}

func (service Service) ProviderReadiness(providerType domain.CertificateProviderType) ProviderReadiness {
	if providerType == "" {
		providerType = domain.CertificateProviderACMEDNS01
	}
	readiness := ProviderReadiness{ProviderType: providerType, Ready: true}
	switch providerType {
	case domain.CertificateProviderACMEDNS01:
		readiness.TokenEnvName = strings.TrimSpace(service.Settings.DNSProviderTokenEnv)
		if !service.Settings.Enabled && service.Issuer == nil && service.DNSProvider == nil {
			readiness.MissingRequirements = append(readiness.MissingRequirements, ReadinessACMEDisabled)
		}
		if strings.TrimSpace(service.Settings.AccountEmail) == "" {
			readiness.MissingRequirements = append(readiness.MissingRequirements, ReadinessAccountEmailMissing)
		}
		if !service.Settings.TermsAccepted {
			readiness.MissingRequirements = append(readiness.MissingRequirements, ReadinessTermsNotAccepted)
		}
		if readiness.TokenEnvName != "" && (service.Settings.Enabled || (service.Issuer == nil && service.DNSProvider == nil)) && strings.TrimSpace(os.Getenv(readiness.TokenEnvName)) == "" {
			readiness.MissingRequirements = append(readiness.MissingRequirements, ReadinessDNSTokenMissing)
		}
		readiness.Guidance = "修改服务端 ACME 配置，注入所列环境变量后重启服务。"
	case domain.CertificateProviderCloudflareOriginCA:
		if !service.OriginCASettings.Enabled {
			readiness.MissingRequirements = append(readiness.MissingRequirements, ReadinessOriginCADisabled)
		}
		if service.ProviderSecretStore == nil {
			readiness.MissingRequirements = append(readiness.MissingRequirements, ReadinessCredentialMissing)
		}
		readiness.Guidance = "在服务端启用 Origin CA 并通过管理面配置可用的 Cloudflare 凭据。"
	}
	readiness.Ready = len(readiness.MissingRequirements) == 0
	return readiness
}

func (service Service) requireProviderReady(providerType domain.CertificateProviderType) error {
	readiness := service.ProviderReadiness(providerType)
	if !readiness.Ready {
		return &ProviderNotReadyError{Readiness: readiness}
	}
	return nil
}
