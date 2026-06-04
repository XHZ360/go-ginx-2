package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Service struct {
	Store                store.Store
	Certificates         certmanager.Service
	StaticListenerClaims []domain.ListenerClaim
	ProxyEntryDefaults   domain.ProxyEntryDefaults
	ListenerReconciler   ProxyListenerReconciler
	DefaultJoin          config.JoinServiceDefaults
}

type ProxyListenerReconciler interface {
	ReconcileProxyListeners(ctx context.Context) error
}

type CreateUserInput struct {
	ID       string
	Username string
	Password string
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

type CreateClientResult struct {
	Client     domain.Client
	Credential string
}

type CreateClientJoinInput struct {
	ID               string
	UserID           string
	Name             string
	ActorID          string
	EnrollmentURL    string
	ServerAddress    string
	ServerTLSAddress string
	ServerName       string
	ServerCAFile     string
	AllowedProtocols []domain.Protocol
	Reconnect        config.Reconnect
	TTL              time.Duration
}

type CreateClientJoinResult struct {
	Client domain.Client
	Token  string
}

type ReviewClientJoinTokenResult struct {
	Client    domain.Client
	Token     string
	ExpiresAt time.Time
}

type CreateProxyInput struct {
	ID            string
	UserID        string
	ClientID      string
	Name          string
	Type          domain.ProxyType
	EntryBindHost string
	EntryHost     string
	EntryPort     int
	TargetHost    string
	TargetPort    int
	CertFile      string
	KeyFile       string
	Description   string
	ActorID       string
}

type UpdateProxyInput struct {
	ID            string
	Type          domain.ProxyType
	Name          string
	EntryBindHost string
	EntryHost     string
	EntryPort     int
	TargetHost    string
	TargetPort    int
	CertFile      string
	KeyFile       string
	Description   string
	ActorID       string
}

type CertificateInput struct {
	ProxyID string
	ActorID string
}

type RotateClientCredentialInput struct {
	ClientID string
	ActorID  string
}

type RotateClientCredentialResult struct {
	Client     domain.Client
	Credential string
}

func (service Service) CreateUser(ctx context.Context, input CreateUserInput) (domain.User, error) {
	if service.Store == nil {
		return domain.User{}, errors.New("store is required")
	}
	if err := validateCreateUserInput(input); err != nil {
		return domain.User{}, err
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("user")
	}
	if input.Role == "" {
		input.Role = domain.RoleUser
	}
	passwordHash := ""
	if strings.TrimSpace(input.Password) != "" {
		passwordHashValue, err := domain.HashPassword(input.Password)
		if err != nil {
			return domain.User{}, err
		}
		passwordHash = passwordHashValue
	}
	user := domain.User{ID: input.ID, Username: input.Username, PasswordHash: passwordHash, Role: input.Role, Status: domain.UserEnabled}
	if err := service.Store.Users().Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, service.audit(ctx, input.ActorID, "user", user.ID, "create_user")
}

func (service Service) DisableUser(ctx context.Context, userID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := service.Store.Users().SetStatus(ctx, userID, domain.UserDisabled); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "user", userID, "disable_user")
}

func (service Service) EnableUser(ctx context.Context, userID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := service.Store.Users().SetStatus(ctx, userID, domain.UserEnabled); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "user", userID, "enable_user")
}

func (service Service) SetUserPassword(ctx context.Context, userID string, password string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := validateSetUserPassword(userID, password); err != nil {
		return err
	}
	passwordHash, err := domain.HashPassword(password)
	if err != nil {
		return contracterr.Validation("validation failed", map[string]string{"password": err.Error()})
	}
	if err := service.Store.Users().SetPassword(ctx, userID, passwordHash); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "user", userID, "set_user_password")
}

func (service Service) DeleteUser(ctx context.Context, userID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(userID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "user id is required"})
	}
	if _, err := service.Store.Users().ByID(ctx, userID); err != nil {
		return err
	}
	clients, err := service.Store.Clients().List(ctx)
	if err != nil {
		return err
	}
	for _, client := range clients {
		if client.UserID == userID {
			return contracterr.Conflict("user has clients; disable and delete client resources before deleting the user", nil)
		}
	}
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		if proxy.UserID == userID {
			return contracterr.Conflict("user has proxies; disable and delete proxy resources before deleting the user", nil)
		}
	}
	if err := service.Store.Users().Delete(ctx, userID); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "user", userID, "delete_user")
}

func (service Service) CreateClient(ctx context.Context, input CreateClientInput) (domain.Client, error) {
	if service.Store == nil {
		return domain.Client{}, errors.New("store is required")
	}
	if err := validateCreateClientInput(input); err != nil {
		return domain.Client{}, err
	}
	if strings.TrimSpace(input.Credential) == "" {
		return domain.Client{}, contracterr.Validation("validation failed", map[string]string{"credential": "credential is required"})
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("client")
	}
	if _, err := service.Store.Users().ByID(ctx, input.UserID); err != nil {
		return domain.Client{}, err
	}
	client := domain.Client{ID: input.ID, UserID: input.UserID, Name: input.Name, Status: domain.ClientOffline, CredentialHash: domain.HashCredential(input.Credential)}
	if err := service.Store.Clients().Create(ctx, client); err != nil {
		return domain.Client{}, err
	}
	if err := service.audit(ctx, input.ActorID, "client", client.ID, "create_client"); err != nil {
		return domain.Client{}, err
	}
	return client, nil
}

func (service Service) CreateClientWithCredential(ctx context.Context, input CreateClientInput) (CreateClientResult, error) {
	if strings.TrimSpace(input.Credential) == "" {
		input.Credential = newCredential()
	}
	client, err := service.CreateClient(ctx, input)
	if err != nil {
		return CreateClientResult{}, err
	}
	client.CredentialHash = ""
	return CreateClientResult{Client: client, Credential: input.Credential}, nil
}

func (service Service) CreateClientJoin(ctx context.Context, input CreateClientJoinInput) (CreateClientJoinResult, error) {
	if service.Store == nil {
		return CreateClientJoinResult{}, errors.New("store is required")
	}
	if input.TTL <= 0 {
		input.TTL = time.Hour
	}
	if len(input.AllowedProtocols) == 0 {
		input.AllowedProtocols = config.DefaultClient().AllowedProtocols
	}
	if input.Reconnect.InitialDelay <= 0 || input.Reconnect.MaxDelay <= 0 {
		input.Reconnect = config.DefaultClient().Reconnect
	}
	if strings.TrimSpace(input.EnrollmentURL) == "" {
		input.EnrollmentURL = service.DefaultJoin.EnrollmentURL
	}
	if strings.TrimSpace(input.ServerAddress) == "" {
		input.ServerAddress = service.DefaultJoin.ServerAddress
	}
	if strings.TrimSpace(input.ServerTLSAddress) == "" {
		input.ServerTLSAddress = service.DefaultJoin.ServerTLSAddress
	}
	if strings.TrimSpace(input.ServerName) == "" {
		input.ServerName = service.DefaultJoin.ServerName
	}
	if strings.TrimSpace(input.ServerCAFile) == "" {
		input.ServerCAFile = service.DefaultJoin.ServerCAFile
	}
	if strings.TrimSpace(input.EnrollmentURL) == "" {
		return CreateClientJoinResult{}, contracterr.Validation("validation failed", map[string]string{"enrollmentUrl": "enrollment url is required"})
	}
	if strings.TrimSpace(input.ServerAddress) == "" {
		return CreateClientJoinResult{}, contracterr.Validation("validation failed", map[string]string{"serverAddress": "server address is required"})
	}
	if strings.TrimSpace(input.ServerName) == "" {
		return CreateClientJoinResult{}, contracterr.Validation("validation failed", map[string]string{"serverName": "server name is required"})
	}
	if strings.TrimSpace(input.ServerCAFile) == "" {
		input.ServerCAFile = config.DefaultServer().ControlTLSCAFile
	}
	caPEM, err := os.ReadFile(input.ServerCAFile)
	if err != nil {
		if os.IsNotExist(err) {
			return CreateClientJoinResult{}, contracterr.Validation("validation failed", map[string]string{"serverCAFile": "server CA file was not found"})
		}
		return CreateClientJoinResult{}, err
	}
	clientResult, err := service.CreateClientWithCredential(ctx, CreateClientInput{ID: input.ID, UserID: input.UserID, Name: input.Name, ActorID: input.ActorID})
	if err != nil {
		return CreateClientJoinResult{}, err
	}
	enrollmentID := newID("join")
	secret := newCredential()
	expiresAt := time.Now().UTC().Add(input.TTL)
	payload := enrollment.TokenPayload{
		EnrollmentID:     enrollmentID,
		Secret:           secret,
		EnrollmentURL:    input.EnrollmentURL,
		ServerAddress:    input.ServerAddress,
		ServerTLSAddress: input.ServerTLSAddress,
		ServerName:       input.ServerName,
		CAPEM:            string(caPEM),
		ClientID:         clientResult.Client.ID,
		Credential:       clientResult.Credential,
		AllowedProtocols: append([]domain.Protocol(nil), input.AllowedProtocols...),
		Reconnect:        input.Reconnect,
		ExpiresAt:        expiresAt,
	}
	token, err := enrollment.EncodeToken(payload)
	if err != nil {
		return CreateClientJoinResult{}, err
	}
	record := domain.ClientEnrollment{ID: enrollmentID, ClientID: clientResult.Client.ID, SecretHash: enrollment.HashSecret(secret), TokenHash: enrollment.HashToken(token), Token: token, ExpiresAt: expiresAt}
	if err := service.Store.ClientEnrollments().Create(ctx, record); err != nil {
		return CreateClientJoinResult{}, err
	}
	if err := service.audit(ctx, input.ActorID, "client", clientResult.Client.ID, "create_client_join"); err != nil {
		return CreateClientJoinResult{}, err
	}
	clientResult.Client.CredentialHash = ""
	return CreateClientJoinResult{Client: clientResult.Client, Token: token}, nil
}

func (service Service) ReviewClientJoinToken(ctx context.Context, clientID string, actorID string) (ReviewClientJoinTokenResult, error) {
	if service.Store == nil {
		return ReviewClientJoinTokenResult{}, errors.New("store is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return ReviewClientJoinTokenResult{}, contracterr.Validation("validation failed", map[string]string{"id": "client id is required"})
	}
	client, err := service.Store.Clients().ByID(ctx, clientID)
	if err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	now := time.Now().UTC()
	enrollmentRecord, err := service.Store.ClientEnrollments().LatestReviewableByClientID(ctx, clientID, now)
	if errors.Is(err, store.ErrNotFound) {
		return service.resetClientJoinToken(ctx, client, actorID, now, false)
	}
	if err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	if service.tokenUsesLegacyAdminEnrollmentURL(enrollmentRecord.Token, client.ID) {
		return service.resetClientJoinToken(ctx, client, actorID, now, true)
	}
	if err := service.audit(ctx, actorID, "client", client.ID, "review_client_join_token"); err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	return ReviewClientJoinTokenResult{Client: client, Token: enrollmentRecord.Token, ExpiresAt: enrollmentRecord.ExpiresAt}, nil
}

func (service Service) resetClientJoinToken(ctx context.Context, client domain.Client, actorID string, now time.Time, allowActive bool) (ReviewClientJoinTokenResult, error) {
	var basePayload enrollment.TokenPayload
	ttl := time.Hour
	latest, err := service.Store.ClientEnrollments().LatestUnusedByClientID(ctx, client.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return ReviewClientJoinTokenResult{}, err
	}
	if err == nil && latest.ExpiresAt.After(now) && !allowActive {
		return ReviewClientJoinTokenResult{}, unavailableJoinTokenError()
	}
	if err == nil {
		if decoded, decodeErr := enrollment.DecodeToken(latest.Token); decodeErr == nil && decoded.ClientID == client.ID {
			basePayload = decoded
			if service.usesLegacyAdminEnrollmentURL(basePayload.EnrollmentURL) {
				basePayload.EnrollmentURL = service.DefaultJoin.EnrollmentURL
			}
		}
		ttl = latest.ExpiresAt.Sub(latest.CreatedAt)
		if ttl <= 0 {
			ttl = time.Hour
		}
	}
	if basePayload.EnrollmentURL == "" {
		defaultPayload, err := service.defaultJoinTokenPayload()
		if err != nil {
			return ReviewClientJoinTokenResult{}, err
		}
		basePayload = defaultPayload
	}
	credential := newCredential()
	secret := newCredential()
	enrollmentID := newID("join")
	expiresAt := now.Add(ttl)
	payload := basePayload
	payload.EnrollmentID = enrollmentID
	payload.Secret = secret
	payload.ClientID = client.ID
	payload.Credential = credential
	payload.ExpiresAt = expiresAt
	token, err := enrollment.EncodeToken(payload)
	if err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	if err := service.Store.Clients().RotateCredential(ctx, client.ID, domain.HashCredential(credential)); err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	record := domain.ClientEnrollment{ID: enrollmentID, ClientID: client.ID, SecretHash: enrollment.HashSecret(secret), TokenHash: enrollment.HashToken(token), Token: token, ExpiresAt: expiresAt}
	if err := service.Store.ClientEnrollments().Create(ctx, record); err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	if err := service.audit(ctx, actorID, "client", client.ID, "reset_client_join_token"); err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	client.CredentialHash = ""
	return ReviewClientJoinTokenResult{Client: client, Token: token, ExpiresAt: expiresAt}, nil
}

func (service Service) tokenUsesLegacyAdminEnrollmentURL(token string, clientID string) bool {
	payload, err := enrollment.DecodeToken(token)
	if err != nil || payload.ClientID != clientID {
		return false
	}
	return service.usesLegacyAdminEnrollmentURL(payload.EnrollmentURL)
}

func (service Service) usesLegacyAdminEnrollmentURL(enrollmentURL string) bool {
	current := strings.TrimSpace(service.DefaultJoin.EnrollmentURL)
	legacy := strings.TrimSpace(service.DefaultJoin.LegacyAdminEnrollmentURL)
	return current != "" && legacy != "" && current != legacy && strings.TrimSpace(enrollmentURL) == legacy
}

func (service Service) defaultJoinTokenPayload() (enrollment.TokenPayload, error) {
	if strings.TrimSpace(service.DefaultJoin.EnrollmentURL) == "" {
		return enrollment.TokenPayload{}, contracterr.Validation("validation failed", map[string]string{"enrollmentUrl": "enrollment url is required"})
	}
	if strings.TrimSpace(service.DefaultJoin.ServerAddress) == "" {
		return enrollment.TokenPayload{}, contracterr.Validation("validation failed", map[string]string{"serverAddress": "server address is required"})
	}
	if strings.TrimSpace(service.DefaultJoin.ServerName) == "" {
		return enrollment.TokenPayload{}, contracterr.Validation("validation failed", map[string]string{"serverName": "server name is required"})
	}
	serverCAFile := service.DefaultJoin.ServerCAFile
	if strings.TrimSpace(serverCAFile) == "" {
		serverCAFile = config.DefaultServer().ControlTLSCAFile
	}
	caPEM, err := os.ReadFile(serverCAFile)
	if err != nil {
		if os.IsNotExist(err) {
			return enrollment.TokenPayload{}, contracterr.Validation("validation failed", map[string]string{"serverCAFile": "server CA file was not found"})
		}
		return enrollment.TokenPayload{}, err
	}
	defaultClient := config.DefaultClient()
	return enrollment.TokenPayload{
		EnrollmentURL:    service.DefaultJoin.EnrollmentURL,
		ServerAddress:    service.DefaultJoin.ServerAddress,
		ServerTLSAddress: service.DefaultJoin.ServerTLSAddress,
		ServerName:       service.DefaultJoin.ServerName,
		CAPEM:            string(caPEM),
		AllowedProtocols: append([]domain.Protocol(nil), defaultClient.AllowedProtocols...),
		Reconnect:        defaultClient.Reconnect,
	}, nil
}

func unavailableJoinTokenError() error {
	return contracterr.Conflict("join token is not available; generate a new join token", nil)
}

func (service Service) CreateProxy(ctx context.Context, input CreateProxyInput) (domain.Proxy, error) {
	if service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	if err := validateCreateProxyInput(input); err != nil {
		return domain.Proxy{}, err
	}
	if input.Type == domain.ProxyForward {
		return domain.Proxy{}, contracterr.Unsupported("forward proxy is not supported in this management batch")
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("proxy")
	}
	if _, err := service.Store.Users().ByID(ctx, input.UserID); err != nil {
		return domain.Proxy{}, err
	}
	if _, err := service.Store.Clients().ByID(ctx, input.ClientID); err != nil {
		return domain.Proxy{}, err
	}
	proxy := domain.Proxy{ID: input.ID, UserID: input.UserID, ClientID: input.ClientID, Name: input.Name, Type: input.Type, Status: domain.ProxyEnabled, EntryBindHost: input.EntryBindHost, EntryHost: input.EntryHost, EntryPort: input.EntryPort, TargetHost: input.TargetHost, TargetPort: input.TargetPort, CertFile: input.CertFile, KeyFile: input.KeyFile, Description: input.Description}
	if err := service.ensureProxyAdmission(ctx, proxy, ""); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Store.Proxies().Create(ctx, proxy); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.reconcileProxyListeners(ctx); err != nil {
		_ = service.Store.Proxies().Delete(ctx, proxy.ID)
		_ = service.reconcileProxyListeners(ctx)
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

func (service Service) UpdateProxy(ctx context.Context, input UpdateProxyInput) (domain.Proxy, error) {
	if service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	if err := validateUpdateProxyInput(input); err != nil {
		return domain.Proxy{}, err
	}
	existing, err := service.Store.Proxies().ByID(ctx, input.ID)
	if err != nil {
		return domain.Proxy{}, err
	}
	previous := existing
	if input.Type != "" && input.Type != existing.Type {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"type": "proxy type is immutable"})
	}
	if existing.Type == domain.ProxyForward {
		return domain.Proxy{}, contracterr.Unsupported("forward proxy is not supported in this management batch")
	}
	existing.Name = input.Name
	existing.EntryBindHost = input.EntryBindHost
	existing.EntryHost = input.EntryHost
	existing.EntryPort = input.EntryPort
	existing.TargetHost = input.TargetHost
	existing.TargetPort = input.TargetPort
	existing.CertFile = input.CertFile
	existing.KeyFile = input.KeyFile
	existing.Description = input.Description
	if err := validateProxyEntryFields(existing.Type, existing.EntryBindHost, existing.EntryHost, existing.EntryPort, existing.CertFile, existing.KeyFile); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.ensureProxyAdmission(ctx, existing, existing.ID); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Store.Proxies().Update(ctx, existing); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.reconcileProxyListeners(ctx); err != nil {
		_ = service.Store.Proxies().Update(ctx, previous)
		_ = service.reconcileProxyListeners(ctx)
		return domain.Proxy{}, err
	}
	return existing, service.audit(ctx, input.ActorID, "proxy", existing.ID, "update_proxy")
}

func (service Service) EnableProxy(ctx context.Context, proxyID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(proxyID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "proxy id is required"})
	}
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return err
	}
	if proxy.Type == domain.ProxyForward {
		return contracterr.Unsupported("forward proxy is not supported in this management batch")
	}
	proxy.Status = domain.ProxyEnabled
	if err := service.ensureProxyAdmission(ctx, proxy, proxy.ID); err != nil {
		return err
	}
	if err := service.Store.Proxies().SetStatus(ctx, proxyID, domain.ProxyEnabled); err != nil {
		return err
	}
	if err := service.reconcileProxyListeners(ctx); err != nil {
		_ = service.Store.Proxies().SetStatus(ctx, proxyID, domain.ProxyDisabled)
		_ = service.reconcileProxyListeners(ctx)
		return err
	}
	return service.audit(ctx, actorID, "proxy", proxyID, "enable_proxy")
}

func (service Service) DisableProxy(ctx context.Context, proxyID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(proxyID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "proxy id is required"})
	}
	if err := service.Store.Proxies().SetStatus(ctx, proxyID, domain.ProxyDisabled); err != nil {
		return err
	}
	if err := service.reconcileProxyListeners(ctx); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "proxy", proxyID, "disable_proxy")
}

func (service Service) DeleteProxy(ctx context.Context, proxyID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(proxyID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "proxy id is required"})
	}
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return err
	}
	if proxy.Status != domain.ProxyDisabled {
		return contracterr.Conflict("proxy must be disabled before delete", nil)
	}
	if err := service.Store.Proxies().Delete(ctx, proxyID); err != nil {
		return err
	}
	if err := service.reconcileProxyListeners(ctx); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "proxy", proxyID, "delete_proxy")
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

func (service Service) ensureProxyAdmission(ctx context.Context, proxy domain.Proxy, ignoreProxyID string) error {
	if !proxyRequiresListenerAdmission(proxy.Type) || proxy.Status != domain.ProxyEnabled {
		return nil
	}
	proposedEntry, ok := domain.EffectiveProxyEntry(proxy, service.ProxyEntryDefaults)
	if !ok {
		return nil
	}
	if conflict, ok, err := service.findActiveRouteConflict(ctx, proposedEntry, ignoreProxyID); err != nil {
		return err
	} else if ok {
		return &contracterr.Error{Code: contracterr.CodeEntryConflict, Message: fmt.Sprintf("%s route %s on %s:%d conflicts with proxy %s", proposedEntry.Protocol, proposedEntry.RouteHost, displayBindHost(proposedEntry.BindHost), proposedEntry.Port, conflict.ID), Err: domain.ErrEntryConflict}
	}
	proposed, ok := domain.ListenerClaimForProxy(proxy, service.ProxyEntryDefaults)
	if !ok {
		return nil
	}
	claims, err := service.activeListenerClaims(ctx, ignoreProxyID)
	if err != nil {
		return err
	}
	if conflict, ok := domain.FindListenerConflict(claims, proposed); ok {
		return &domain.ListenerAdmissionError{Proposed: proposed, Conflict: conflict}
	}
	return nil
}

func (service Service) activeListenerClaims(ctx context.Context, ignoreProxyID string) ([]domain.ListenerClaim, error) {
	claims := append([]domain.ListenerClaim(nil), service.StaticListenerClaims...)
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return nil, err
	}
	for _, proxy := range proxies {
		if proxy.ID == ignoreProxyID || proxy.Status != domain.ProxyEnabled {
			continue
		}
		claim, ok := domain.ListenerClaimForProxy(proxy, service.ProxyEntryDefaults)
		if !ok {
			continue
		}
		claims = append(claims, claim)
	}
	return claims, nil
}

func proxyRequiresListenerAdmission(proxyType domain.ProxyType) bool {
	return proxyType == domain.ProxyTCP || proxyType == domain.ProxyUDP || proxyType == domain.ProxyHTTP || proxyType == domain.ProxyHTTPS
}

func (service Service) reconcileProxyListeners(ctx context.Context) error {
	if service.ListenerReconciler == nil {
		return nil
	}
	if err := service.ListenerReconciler.ReconcileProxyListeners(ctx); err != nil {
		return &contracterr.Error{Code: contracterr.CodeEntryConflict, Message: "proxy listener reconcile failed", Err: err}
	}
	return nil
}

func (service Service) findActiveRouteConflict(ctx context.Context, proposed domain.ProxyEntry, ignoreProxyID string) (domain.Proxy, bool, error) {
	if proposed.Protocol != domain.ListenerProtocolHTTP && proposed.Protocol != domain.ListenerProtocolHTTPS {
		return domain.Proxy{}, false, nil
	}
	proxies, err := service.Store.Proxies().EnabledByType(ctx, domain.ProxyType(proposed.Protocol))
	if err != nil {
		return domain.Proxy{}, false, err
	}
	for _, proxy := range proxies {
		if proxy.ID == ignoreProxyID {
			continue
		}
		entry, ok := domain.EffectiveProxyEntry(proxy, service.ProxyEntryDefaults)
		if !ok {
			continue
		}
		if entry.Protocol == proposed.Protocol && entry.Port == proposed.Port && domain.NormalizeBindHost(entry.BindHost) == domain.NormalizeBindHost(proposed.BindHost) && domain.NormalizeRouteHost(entry.RouteHost) == domain.NormalizeRouteHost(proposed.RouteHost) {
			return proxy, true, nil
		}
	}
	return domain.Proxy{}, false, nil
}

func displayBindHost(host string) string {
	if domain.NormalizeBindHost(host) == "" {
		return "*"
	}
	return domain.NormalizeBindHost(host)
}

func (service Service) audit(ctx context.Context, actorID string, resourceType string, resourceID string, action string) error {
	if strings.TrimSpace(actorID) == "" {
		actorID = "system"
	}
	event := domain.AuditEvent{ID: newID("audit"), ActorUserID: actorID, ResourceType: resourceType, ResourceID: resourceID, Action: action, Result: "success"}
	return service.Store.AuditEvents().Create(ctx, event)
}

func (service Service) EnableClient(ctx context.Context, clientID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "client id is required"})
	}
	if err := service.Store.Clients().SetStatus(ctx, clientID, domain.ClientOffline); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "client", clientID, "enable_client")
}

func (service Service) DisableClient(ctx context.Context, clientID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "client id is required"})
	}
	if err := service.Store.Clients().SetStatus(ctx, clientID, domain.ClientDisabled); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "client", clientID, "disable_client")
}

func (service Service) DeleteClient(ctx context.Context, clientID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "client id is required"})
	}
	if _, err := service.Store.Clients().ByID(ctx, clientID); err != nil {
		return err
	}
	proxies, err := service.Store.Proxies().ByClientID(ctx, clientID)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		if proxy.Status == domain.ProxyEnabled {
			return contracterr.Conflict("client has enabled proxies; disable proxies before deleting the client", nil)
		}
	}
	if err := service.Store.Clients().Delete(ctx, clientID); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "client", clientID, "delete_client")
}

func (service Service) RotateClientCredential(ctx context.Context, input RotateClientCredentialInput) (RotateClientCredentialResult, error) {
	if service.Store == nil {
		return RotateClientCredentialResult{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.ClientID) == "" {
		return RotateClientCredentialResult{}, contracterr.Validation("validation failed", map[string]string{"id": "client id is required"})
	}
	client, err := service.Store.Clients().ByID(ctx, input.ClientID)
	if err != nil {
		return RotateClientCredentialResult{}, err
	}
	credential := newCredential()
	if err := service.Store.Clients().RotateCredential(ctx, input.ClientID, domain.HashCredential(credential)); err != nil {
		return RotateClientCredentialResult{}, err
	}
	client, err = service.Store.Clients().ByID(ctx, input.ClientID)
	if err != nil {
		return RotateClientCredentialResult{}, err
	}
	if err := service.audit(ctx, input.ActorID, "client", client.ID, "rotate_client_credential"); err != nil {
		return RotateClientCredentialResult{}, err
	}
	client.CredentialHash = ""
	return RotateClientCredentialResult{Client: client, Credential: credential}, nil
}

func validateCreateUserInput(input CreateUserInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.Username) == "" {
		fields["username"] = "username is required"
	}
	if input.Role != "" && !input.Role.Valid() {
		fields["role"] = "role is invalid"
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}

func validateSetUserPassword(userID string, password string) error {
	fields := map[string]string{}
	if strings.TrimSpace(userID) == "" {
		fields["id"] = "user id is required"
	}
	if strings.TrimSpace(password) == "" {
		fields["password"] = "password is required"
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}

func validateCreateClientInput(input CreateClientInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.UserID) == "" {
		fields["userId"] = "user id is required"
	}
	if strings.TrimSpace(input.Name) == "" {
		fields["name"] = "client name is required"
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}

func validateCreateProxyInput(input CreateProxyInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.UserID) == "" {
		fields["userId"] = "user id is required"
	}
	if strings.TrimSpace(input.ClientID) == "" {
		fields["clientId"] = "client id is required"
	}
	if strings.TrimSpace(input.Name) == "" {
		fields["name"] = "proxy name is required"
	}
	if !input.Type.Valid() {
		fields["type"] = "proxy type is invalid"
	}
	if strings.TrimSpace(input.TargetHost) == "" {
		fields["targetHost"] = "proxy target host is required"
	}
	if input.TargetPort <= 0 || input.TargetPort > 65535 {
		fields["targetPort"] = "proxy target port is invalid"
	}
	if err := collectProxyEntryFieldErrors(fields, input.Type, input.EntryBindHost, input.EntryHost, input.EntryPort, input.CertFile, input.KeyFile); err != nil {
		return err
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}

func validateUpdateProxyInput(input UpdateProxyInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.ID) == "" {
		fields["id"] = "proxy id is required"
	}
	if strings.TrimSpace(input.Name) == "" {
		fields["name"] = "proxy name is required"
	}
	if strings.TrimSpace(input.TargetHost) == "" {
		fields["targetHost"] = "proxy target host is required"
	}
	if input.TargetPort <= 0 || input.TargetPort > 65535 {
		fields["targetPort"] = "proxy target port is invalid"
	}
	if input.Type != "" && !input.Type.Valid() {
		fields["type"] = "proxy type is invalid"
	}
	if input.Type.Valid() {
		if err := collectProxyEntryFieldErrors(fields, input.Type, input.EntryBindHost, input.EntryHost, input.EntryPort, input.CertFile, input.KeyFile); err != nil {
			return err
		}
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}

func validateProxyEntryFields(proxyType domain.ProxyType, entryBindHost string, entryHost string, entryPort int, certFile string, keyFile string) error {
	fields := map[string]string{}
	if err := collectProxyEntryFieldErrors(fields, proxyType, entryBindHost, entryHost, entryPort, certFile, keyFile); err != nil {
		return err
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}

func collectProxyEntryFieldErrors(fields map[string]string, proxyType domain.ProxyType, entryBindHost string, entryHost string, entryPort int, certFile string, keyFile string) error {
	if strings.TrimSpace(entryBindHost) != "" && !domain.ValidBindHost(entryBindHost) {
		fields["entryBindHost"] = "proxy entry bind host is invalid"
	}
	switch proxyType {
	case domain.ProxyTCP, domain.ProxyUDP:
		if entryPort <= 0 || entryPort > 65535 {
			fields["entryPort"] = fmt.Sprintf("%s proxy entry port is required", proxyType)
		}
	case domain.ProxyHTTP, domain.ProxyHTTPS:
		if strings.TrimSpace(entryHost) == "" {
			fields["entryHost"] = fmt.Sprintf("%s proxy route host is required", proxyType)
		} else if !domain.ValidBindHost(entryHost) {
			fields["entryHost"] = fmt.Sprintf("%s proxy route host is invalid", proxyType)
		}
		if entryPort < 0 || entryPort > 65535 {
			fields["entryPort"] = fmt.Sprintf("%s proxy entry port is invalid", proxyType)
		}
	}
	if proxyType == domain.ProxyHTTPS && (strings.TrimSpace(certFile) == "") != (strings.TrimSpace(keyFile) == "") {
		fields["certFile"] = "https proxy cert file and key file must be provided together"
		fields["keyFile"] = "https proxy cert file and key file must be provided together"
	}
	return nil
}

func newCredential() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate credential: %v", err))
	}
	return hex.EncodeToString(bytes[:])
}

func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate %s id: %v", prefix, err))
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
