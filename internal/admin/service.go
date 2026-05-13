package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Service struct {
	Store        store.Store
	Certificates certmanager.Service
}

type CreateUserInput struct {
	ID       string
	Username string
	Role     domain.Role
	ActorID  string
}

type CreateClientInput struct {
	ID         string
	UserID     string
	Name       string
	Credential string
	ActorID    string
}

type CreateProxyInput struct {
	ID          string
	UserID      string
	ClientID    string
	Name        string
	Type        domain.ProxyType
	EntryHost   string
	EntryPort   int
	TargetHost  string
	TargetPort  int
	CertFile    string
	KeyFile     string
	Description string
	ActorID     string
}

type CertificateInput struct {
	ProxyID string
	ActorID string
}

func (service Service) CreateUser(ctx context.Context, input CreateUserInput) (domain.User, error) {
	if service.Store == nil {
		return domain.User{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("user")
	}
	if input.Role == "" {
		input.Role = domain.RoleUser
	}
	user := domain.User{ID: input.ID, Username: input.Username, Role: input.Role, Status: domain.UserEnabled}
	if err := service.Store.Users().Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, service.audit(ctx, input.ActorID, "user", user.ID, "create_user")
}

func (service Service) CreateClient(ctx context.Context, input CreateClientInput) (domain.Client, error) {
	if service.Store == nil {
		return domain.Client{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.Credential) == "" {
		return domain.Client{}, errors.New("credential is required")
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("client")
	}
	client := domain.Client{ID: input.ID, UserID: input.UserID, Name: input.Name, Status: domain.ClientOffline, CredentialHash: domain.HashCredential(input.Credential)}
	if err := service.Store.Clients().Create(ctx, client); err != nil {
		return domain.Client{}, err
	}
	return client, service.audit(ctx, input.ActorID, "client", client.ID, "create_client")
}

func (service Service) CreateProxy(ctx context.Context, input CreateProxyInput) (domain.Proxy, error) {
	if service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	if input.Type == domain.ProxyTCP && input.EntryPort == 0 {
		return domain.Proxy{}, errors.New("tcp proxy entry port is required")
	}
	if input.Type == domain.ProxyUDP && input.EntryPort == 0 {
		return domain.Proxy{}, errors.New("udp proxy entry port is required")
	}
	if input.Type == domain.ProxyHTTP && strings.TrimSpace(input.EntryHost) == "" {
		return domain.Proxy{}, errors.New("http proxy entry host is required")
	}
	if input.Type == domain.ProxyHTTPS && strings.TrimSpace(input.EntryHost) == "" {
		return domain.Proxy{}, errors.New("https proxy entry host is required")
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("proxy")
	}
	if input.Type == domain.ProxyHTTPS && (strings.TrimSpace(input.CertFile) == "") != (strings.TrimSpace(input.KeyFile) == "") {
		return domain.Proxy{}, errors.New("https proxy cert file and key file must be provided together")
	}
	proxy := domain.Proxy{ID: input.ID, UserID: input.UserID, ClientID: input.ClientID, Name: input.Name, Type: input.Type, Status: domain.ProxyEnabled, EntryHost: input.EntryHost, EntryPort: input.EntryPort, TargetHost: input.TargetHost, TargetPort: input.TargetPort, CertFile: input.CertFile, KeyFile: input.KeyFile, Description: input.Description}
	if err := service.Store.Proxies().Create(ctx, proxy); err != nil {
		return domain.Proxy{}, err
	}
	action := "create_proxy"
	if input.Type == domain.ProxyTCP {
		action = "create_tcp_proxy"
	}
	if input.Type == domain.ProxyHTTP {
		action = "create_http_proxy"
	}
	if input.Type == domain.ProxyHTTPS {
		action = "create_https_proxy"
	}
	if input.Type == domain.ProxyUDP {
		action = "create_udp_proxy"
	}
	return proxy, service.audit(ctx, input.ActorID, "proxy", proxy.ID, action)
}

func (service Service) IssueManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := manager.Issue(ctx, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, "issue_managed_certificate")
}

func (service Service) RenewManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := manager.Renew(ctx, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, "renew_managed_certificate")
}

func (service Service) ManagedCertificateStatus(ctx context.Context, proxyID string) (certmanager.CertificateStatus, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return certmanager.CertificateStatus{}, err
	}
	return manager.Status(ctx, proxyID)
}

func (service Service) certificateManager() (certmanager.Service, error) {
	if service.Store == nil {
		return certmanager.Service{}, errors.New("store is required")
	}
	manager := service.Certificates
	manager.Store = service.Store
	return manager, nil
}

func (service Service) audit(ctx context.Context, actorID string, resourceType string, resourceID string, action string) error {
	if strings.TrimSpace(actorID) == "" {
		actorID = "system"
	}
	event := domain.AuditEvent{ID: newID("audit"), ActorUserID: actorID, ResourceType: resourceType, ResourceID: resourceID, Action: action, Result: "success"}
	return service.Store.AuditEvents().Create(ctx, event)
}

func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate %s id: %v", prefix, err))
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
