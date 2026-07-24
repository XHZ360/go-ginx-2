package admin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

func (service *ProviderCredentialService) CreateProviderCredential(ctx context.Context, input ProviderCredentialInput) (domain.ProviderCredential, error) {
	if service.Store == nil {
		return domain.ProviderCredential{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return domain.ProviderCredential{}, contracterr.Validation("validation failed", map[string]string{"name": "credential name is required"})
	}
	if strings.TrimSpace(input.Token) == "" {
		return domain.ProviderCredential{}, contracterr.Validation("validation failed", map[string]string{"token": "cloudflare api token is required"})
	}
	if err := certmanager.RejectOriginCAServiceKey(input.Token); err != nil {
		return domain.ProviderCredential{}, contracterr.Validation("validation failed", map[string]string{"token": err.Error()})
	}
	if service.Certificates.ProviderSecretStore == nil {
		return domain.ProviderCredential{}, providerCredentialStorageUnavailableError()
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = newID("cfcred")
	}
	secretRef, err := service.Certificates.ProviderSecretStore.Write(ctx, id, strings.TrimSpace(input.Token))
	if err != nil {
		return domain.ProviderCredential{}, err
	}
	now := time.Now().UTC()
	credential := domain.ProviderCredential{ID: id, Name: input.Name, ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: input.Scope, TokenFingerprint: certmanager.TokenFingerprint(input.Token), SecretRef: secretRef, Status: domain.ProviderCredentialPending, CreatedAt: now, UpdatedAt: now}
	if err := service.Store.ProviderCredentials().Create(ctx, credential); err != nil {
		_ = service.Certificates.ProviderSecretStore.Delete(ctx, secretRef)
		return domain.ProviderCredential{}, err
	}
	return credential, service.Audit.Record(ctx, input.ActorID, "provider_credential", credential.ID, "create_provider_credential")
}

func (service *ProviderCredentialService) UpdateProviderCredential(ctx context.Context, input UpdateProviderCredentialInput) (domain.ProviderCredential, error) {
	if service.Store == nil {
		return domain.ProviderCredential{}, errors.New("store is required")
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return domain.ProviderCredential{}, contracterr.Validation("validation failed", map[string]string{"id": "credential id is required"})
	}
	credential, err := service.Store.ProviderCredentials().ByID(ctx, id)
	if err != nil {
		return domain.ProviderCredential{}, err
	}
	if strings.TrimSpace(input.Name) != "" {
		credential.Name = input.Name
	}
	credential.Scope = input.Scope
	if strings.TrimSpace(input.Token) != "" {
		if err := certmanager.RejectOriginCAServiceKey(input.Token); err != nil {
			return domain.ProviderCredential{}, contracterr.Validation("validation failed", map[string]string{"token": err.Error()})
		}
		if service.Certificates.ProviderSecretStore == nil {
			return domain.ProviderCredential{}, providerCredentialStorageUnavailableError()
		}
		secretRef, err := service.Certificates.ProviderSecretStore.Write(ctx, credential.ID, strings.TrimSpace(input.Token))
		if err != nil {
			return domain.ProviderCredential{}, err
		}
		credential.SecretRef = secretRef
		credential.TokenFingerprint = certmanager.TokenFingerprint(input.Token)
		credential.Status = domain.ProviderCredentialPending
		credential.LastVerifiedAt = nil
		credential.LastError = ""
	}
	credential.UpdatedAt = time.Now().UTC()
	if err := service.Store.ProviderCredentials().Update(ctx, credential); err != nil {
		return domain.ProviderCredential{}, err
	}
	return credential, service.Audit.Record(ctx, input.ActorID, "provider_credential", credential.ID, "update_provider_credential")
}

func (service *ProviderCredentialService) VerifyProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error) {
	if service.Store == nil {
		return domain.ProviderCredential{}, errors.New("store is required")
	}
	if service.Certificates.ProviderSecretStore == nil {
		return domain.ProviderCredential{}, providerCredentialStorageUnavailableError()
	}
	verifyErr := service.Certificates.VerifyProviderCredential(ctx, credentialID)
	credential, readErr := service.Store.ProviderCredentials().ByID(ctx, credentialID)
	if readErr != nil {
		return domain.ProviderCredential{}, readErr
	}
	auditErr := service.Audit.Record(ctx, actorID, "provider_credential", credential.ID, "verify_provider_credential")
	if verifyErr != nil {
		return credential, providerCredentialVerificationError(verifyErr)
	}
	return credential, auditErr
}

func (service *ProviderCredentialService) DisableProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error) {
	if service.Store == nil {
		return domain.ProviderCredential{}, errors.New("store is required")
	}
	if err := service.Store.ProviderCredentials().SetStatus(ctx, credentialID, domain.ProviderCredentialDisabled, nil, ""); err != nil {
		return domain.ProviderCredential{}, err
	}
	credential, err := service.Store.ProviderCredentials().ByID(ctx, credentialID)
	if err != nil {
		return domain.ProviderCredential{}, err
	}
	return credential, service.Audit.Record(ctx, actorID, "provider_credential", credential.ID, "disable_provider_credential")
}

func (service *ProviderCredentialService) DeleteProviderCredential(ctx context.Context, credentialID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "credential id is required"})
	}
	credential, err := service.Store.ProviderCredentials().ByID(ctx, credentialID)
	if err != nil {
		return err
	}
	certificates, err := service.Store.Certificates().List(ctx)
	if err != nil {
		return err
	}
	for _, certificate := range certificates {
		if certificate.CredentialID == credential.ID {
			return contracterr.Conflict("provider credential is used by managed certificates; update or remove those certificates before deleting it", nil)
		}
	}
	if service.Certificates.ProviderSecretStore != nil {
		_ = service.Certificates.ProviderSecretStore.Delete(ctx, credential.SecretRef)
	}
	if err := service.Store.ProviderCredentials().Delete(ctx, credentialID); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "provider_credential", credential.ID, "delete_provider_credential")
}

var _ ProviderCredentialFacade = (*ProviderCredentialService)(nil)
