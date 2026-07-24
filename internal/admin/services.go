package admin

import (
	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Options struct {
	Store                store.Store
	Certificates         certmanager.Service
	StaticListenerClaims []domain.ListenerClaim
	ProxyEntryDefaults   domain.ProxyEntryDefaults
	ListenerReconciler   ListenerReconciler
	DefaultJoin          config.JoinServiceDefaults
}

type Commands struct {
	UserFacade
	ClientFacade
	DomainFacade
	ProxyFacade
	CertificateFacade
	ProviderCredentialFacade
}

type Services struct {
	Commands
	Store               store.Store
	Users               *UserService
	Clients             *ClientService
	Domains             *DomainService
	Proxies             *ProxyService
	Certificates        *CertificateService
	ProviderCredentials *ProviderCredentialService
}

type UserService struct {
	Store store.Store
	Audit AuditRecorder
}

type ClientService struct {
	Store       store.Store
	DefaultJoin config.JoinServiceDefaults
	Audit       AuditRecorder
}

type ProviderCredentialService struct {
	Store        store.Store
	Certificates certmanager.Service
	Audit        AuditRecorder
}

type DomainService struct {
	Store     store.Store
	Audit     AuditRecorder
	Admission ProxyAdmissionPolicy
	Binding   CertificateBindingPolicy
	Access    ProxyAccessPolicy
}

type ProxyService struct {
	Store     store.Store
	Audit     AuditRecorder
	Admission ProxyAdmissionPolicy
	Binding   CertificateBindingPolicy
	Access    ProxyAccessPolicy
}

type CertificateService struct {
	Store        store.Store
	Certificates certmanager.Service
	Audit        AuditRecorder
	Admission    ProxyAdmissionPolicy
	Binding      CertificateBindingPolicy
}

func NewServices(options Options) Services {
	certificates := options.Certificates
	certificates.Store = options.Store
	audit := storeAuditRecorder{store: options.Store}
	admission := newProxyAdmissionPolicy(options.Store, options.StaticListenerClaims, options.ProxyEntryDefaults, options.ListenerReconciler)
	binding := newCertificateBindingPolicy(options.Store, certificates, audit)
	access := newProxyAccessPolicy(options.Store)

	users := &UserService{Store: options.Store, Audit: audit}
	clients := &ClientService{Store: options.Store, DefaultJoin: options.DefaultJoin, Audit: audit}
	domains := &DomainService{Store: options.Store, Audit: audit, Admission: admission, Binding: binding, Access: access}
	proxies := &ProxyService{Store: options.Store, Audit: audit, Admission: admission, Binding: binding, Access: access}
	certificatesService := &CertificateService{Store: options.Store, Certificates: certificates, Audit: audit, Admission: admission, Binding: binding}
	providerCredentials := &ProviderCredentialService{Store: options.Store, Certificates: certificates, Audit: audit}

	return Services{
		Commands: Commands{
			UserFacade:               users,
			ClientFacade:             clients,
			DomainFacade:             domains,
			ProxyFacade:              proxies,
			CertificateFacade:        certificatesService,
			ProviderCredentialFacade: providerCredentials,
		},
		Store: options.Store, Users: users, Clients: clients, Domains: domains, Proxies: proxies,
		Certificates: certificatesService, ProviderCredentials: providerCredentials,
	}
}
