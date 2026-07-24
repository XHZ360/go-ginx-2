package admin

import (
	"context"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type UserFacade interface {
	CreateUser(ctx context.Context, input CreateUserInput) (domain.User, error)
	DisableUser(ctx context.Context, userID string, actorID string) error
	EnableUser(ctx context.Context, userID string, actorID string) error
	SetUserPassword(ctx context.Context, userID string, password string, actorID string) error
	DeleteUser(ctx context.Context, userID string, actorID string) error
}

type ClientFacade interface {
	CreateClient(ctx context.Context, input CreateClientInput) (domain.Client, error)
	CreateClientWithCredential(ctx context.Context, input CreateClientInput) (CreateClientResult, error)
	CreateClientJoin(ctx context.Context, input CreateClientJoinInput) (CreateClientJoinResult, error)
	ReviewClientJoinToken(ctx context.Context, clientID string, actorID string) (ReviewClientJoinTokenResult, error)
	EnableClient(ctx context.Context, clientID string, actorID string) error
	DisableClient(ctx context.Context, clientID string, actorID string) error
	DeleteClient(ctx context.Context, clientID string, actorID string) error
	RotateClientCredential(ctx context.Context, input RotateClientCredentialInput) (RotateClientCredentialResult, error)
}

type DomainFacade interface {
	CreateDomain(ctx context.Context, input CreateDomainInput) (domain.Domain, error)
	UpdateDomain(ctx context.Context, input UpdateDomainInput) (domain.Domain, error)
	EnableDomain(ctx context.Context, domainID string, actorID string) error
	DisableDomain(ctx context.Context, domainID string, actorID string) error
	DeleteDomain(ctx context.Context, domainID string, actorID string) error
	CreateDomainEntry(ctx context.Context, input CreateDomainEntryInput) (domain.DomainEntry, error)
	UpdateDomainEntry(ctx context.Context, input UpdateDomainEntryInput) (domain.DomainEntry, error)
	DeleteDomainEntry(ctx context.Context, entryID string, actorID string) error
	BindDomainCertificate(ctx context.Context, domainID string, certificateID string, actorID string) (domain.Domain, error)
	UnbindDomainCertificate(ctx context.Context, domainID string, actorID string) (domain.Domain, error)
}

type ProxyFacade interface {
	CreateProxy(ctx context.Context, input CreateProxyInput) (domain.Proxy, error)
	UpdateProxy(ctx context.Context, input UpdateProxyInput) (domain.Proxy, error)
	EnableProxy(ctx context.Context, proxyID string, actorID string) error
	DisableProxy(ctx context.Context, proxyID string, actorID string) error
	DeleteProxy(ctx context.Context, proxyID string, actorID string) error
	EnableProxyAccessAuthAndCreateActivation(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error)
	CreateProxyActivationLink(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error)
	RevokeAllProxyAccess(ctx context.Context, proxyID string, actorID string) error
	DisableProxyAccessAuth(ctx context.Context, proxyID string, actorID string) error
}

type CertificateFacade interface {
	IssueManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
	RenewManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
	CreateCertificate(ctx context.Context, input CreateCertificateInput) (domain.ManagedCertificate, error)
	DeleteCertificate(ctx context.Context, input DeleteCertificateInput) (DeleteCertificateResult, error)
	BindCertificate(ctx context.Context, input BindCertificateInput) (domain.Proxy, error)
	UnbindCertificate(ctx context.Context, input UnbindCertificateInput) (domain.Proxy, error)
	RotateOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
	SyncOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
	RevokeOriginCACertificate(ctx context.Context, input RevokeOriginCACertificateInput) (domain.ManagedCertificate, error)
	CertificateProviderReadiness() []certmanager.ProviderReadiness
	ManagedCertificateStatus(ctx context.Context, proxyID string) (certmanager.CertificateStatus, error)
}

type ProviderCredentialFacade interface {
	CreateProviderCredential(ctx context.Context, input ProviderCredentialInput) (domain.ProviderCredential, error)
	UpdateProviderCredential(ctx context.Context, input UpdateProviderCredentialInput) (domain.ProviderCredential, error)
	VerifyProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error)
	DisableProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error)
	DeleteProviderCredential(ctx context.Context, credentialID string, actorID string) error
}

// CommandFacades is the command boundary consumed by management adapters.
type CommandFacades interface {
	UserFacade
	ClientFacade
	DomainFacade
	ProxyFacade
	CertificateFacade
	ProviderCredentialFacade
}

var _ UserFacade = Service{}
var _ ClientFacade = Service{}
var _ DomainFacade = Service{}
var _ ProxyFacade = Service{}
var _ CertificateFacade = Service{}
var _ ProviderCredentialFacade = Service{}
var _ CommandFacades = Service{}
var _ CommandFacades = (*Service)(nil)
