package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
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
	Kind       domain.ClientKind
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
	// CertificateID 选择一个既有证书资源进行绑定（权威绑定路径）。
	CertificateID string
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
	// CertificateID 选择/清除绑定的证书资源；CertificateIDSet 为 true 且值为空时表示清除绑定。
	CertificateID    string
	CertificateIDSet bool
	Description      string
	ActorID          string
}

type CreateProxyRouteInput struct {
	ID                 string
	ProxyID            string
	ClientID           string
	PathPrefix         string
	StripPrefix        bool
	UpstreamPathPrefix string
	TargetHost         string
	TargetPort         int
	ActorID            string
}

type UpdateProxyRouteInput struct {
	ID                 string
	ClientID           string
	PathPrefix         string
	StripPrefix        bool
	UpstreamPathPrefix string
	TargetHost         string
	TargetPort         int
	Status             domain.ProxyRouteStatus
	ActorID            string
}

type ProxyActivationResult struct {
	URL       string
	ExpiresAt time.Time
}

func (service Service) EnableProxyAccessAuthAndCreateActivation(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error) {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return ProxyActivationResult{}, err
	}
	if !proxy.Type.IsWeb() || strings.TrimSpace(proxy.DomainID) == "" {
		return ProxyActivationResult{}, contracterr.Validation("validation failed", map[string]string{"proxyId": "access authentication requires a web proxy with a domain"})
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return ProxyActivationResult{}, err
	}
	if !service.domainHasEnabledHTTPSEntry(ctx, webDomain.ID) {
		return ProxyActivationResult{}, contracterr.Conflict("domain has no enabled HTTPS entry", nil)
	}
	certificate, err := service.boundDomainCertificate(ctx, webDomain)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ProxyActivationResult{}, contracterr.Conflict("domain certificate is not bound or not serving TLS", nil)
		}
		return ProxyActivationResult{}, err
	}
	if !certificate.ServingStatus.ServesTLS() {
		return ProxyActivationResult{}, contracterr.Conflict("domain certificate is not serving TLS", nil)
	}
	access, ok := store.Access(service.Store)
	if !ok {
		return ProxyActivationResult{}, errors.New("proxy access repository is unavailable")
	}
	tokenValue := newCredential()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	token := domain.ProxyActivationToken{ID: newID("activation"), ProxyID: proxyID, AuthVersion: proxy.AccessAuthVersion + 1, TokenHash: hashAccessValue(tokenValue), ExpiresAt: expiresAt, CreatedBy: actorID}
	if err := access.EnableAuthAndCreateActivation(ctx, proxyID, token.AuthVersion, token); err != nil {
		return ProxyActivationResult{}, err
	}
	if err := service.audit(ctx, actorID, "proxy", proxyID, "enable_proxy_access_auth"); err != nil {
		return ProxyActivationResult{}, err
	}
	return ProxyActivationResult{URL: proxyActivationURL(webDomain, proxy, tokenValue), ExpiresAt: expiresAt}, nil
}

func (service Service) CreateProxyActivationLink(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error) {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return ProxyActivationResult{}, err
	}
	if !proxy.AccessAuthEnabled {
		return ProxyActivationResult{}, contracterr.Conflict("proxy access authentication is disabled", nil)
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return ProxyActivationResult{}, err
	}
	access, ok := store.Access(service.Store)
	if !ok {
		return ProxyActivationResult{}, errors.New("proxy access repository is unavailable")
	}
	tokenValue := newCredential()
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	token := domain.ProxyActivationToken{ID: newID("activation"), ProxyID: proxyID, AuthVersion: proxy.AccessAuthVersion, TokenHash: hashAccessValue(tokenValue), ExpiresAt: expiresAt, CreatedBy: actorID}
	if err := access.CreateActivationToken(ctx, token); err != nil {
		return ProxyActivationResult{}, err
	}
	if err := service.audit(ctx, actorID, "proxy", proxyID, "create_proxy_activation"); err != nil {
		return ProxyActivationResult{}, err
	}
	return ProxyActivationResult{URL: proxyActivationURL(webDomain, proxy, tokenValue), ExpiresAt: expiresAt}, nil
}

func (service Service) RevokeAllProxyAccess(ctx context.Context, proxyID string, actorID string) error {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return err
	}
	access, ok := store.Access(service.Store)
	if !ok {
		return errors.New("proxy access repository is unavailable")
	}
	if err := access.RevokeAllAccess(ctx, proxyID, proxy.AccessAuthVersion+1); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "proxy", proxyID, "revoke_proxy_access")
}

func (service Service) DisableProxyAccessAuth(ctx context.Context, proxyID string, actorID string) error {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return err
	}
	access, ok := store.Access(service.Store)
	if !ok {
		return errors.New("proxy access repository is unavailable")
	}
	if err := access.DisableAuth(ctx, proxyID, proxy.AccessAuthVersion+1); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "proxy", proxyID, "disable_proxy_access_auth")
}

func hashAccessValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func proxyActivationURL(webDomain domain.Domain, proxy domain.Proxy, token string) string {
	_ = proxy
	return fmt.Sprintf("https://%s/.well-known/goginx/activate/%s", webDomain.Host, token)
}

func (service Service) domainHasEnabledHTTPSEntry(ctx context.Context, domainID string) bool {
	entries, err := service.Store.DomainEntries().ListByDomainID(ctx, domainID)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Protocol == domain.DomainEntryHTTPS && entry.Status == domain.DomainEntryEnabled {
			return true
		}
	}
	return false
}

func (service Service) boundDomainCertificate(ctx context.Context, webDomain domain.Domain) (domain.ManagedCertificate, error) {
	if strings.TrimSpace(webDomain.CertificateID) == "" {
		return domain.ManagedCertificate{}, store.ErrNotFound
	}
	return service.Store.Certificates().ByID(ctx, webDomain.CertificateID)
}

type CertificateInput struct {
	// CertificateID 可选；提供时按证书身份寻址（用于未绑定证书的续期/轮换/同步）。
	CertificateID     string
	ProxyID           string
	ProviderType      domain.CertificateProviderType
	CredentialID      string
	RequestType       string
	RequestedValidity int
	ActorID           string
}

// CreateCertificateInput 描述创建一个证书资源（可未绑定代理）。
type CreateCertificateInput struct {
	Host              string
	ProviderType      domain.CertificateProviderType
	CredentialID      string
	RequestType       string
	RequestedValidity int
	// CertFile/KeyFile 仅用于 provider_type=file 的文件型证书登记。
	CertFile string
	KeyFile  string
	ActorID  string
}

// DeleteCertificateInput 描述删除一个证书资源；高风险删除需提供匹配的强确认。
type DeleteCertificateInput struct {
	CertificateID string
	// ConfirmHost 或 ConfirmCertificateID 任一与目标证书匹配即视为已确认。
	ConfirmHost          string
	ConfirmCertificateID string
	ActorID              string
}

// DeleteCertificateResult 返回删除影响的代理与是否曾要求强确认。
type DeleteCertificateResult struct {
	CertificateID    string
	AffectedProxyIDs []string
	RequiredConfirm  bool
}

// BindCertificateInput 将证书绑定到代理（一对一）。
type BindCertificateInput struct {
	ProxyID       string
	CertificateID string
	ActorID       string
}

// UnbindCertificateInput 清除代理的证书绑定（证书保留为未绑定资源）。
type UnbindCertificateInput struct {
	ProxyID string
	ActorID string
}

type ProviderCredentialInput struct {
	ID      string
	Name    string
	Scope   string
	Token   string
	ActorID string
}

type UpdateProviderCredentialInput struct {
	ID      string
	Name    string
	Scope   string
	Token   string
	ActorID string
}

type RevokeOriginCACertificateInput struct {
	// CertificateID 可选；提供时按证书身份寻址（支持未绑定证书的吊销）。
	CertificateID           string
	ProxyID                 string
	Host                    string
	CloudflareCertificateID string
	ActorID                 string
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
	kind := domain.NormalizeClientKind(input.Kind)
	client := domain.Client{ID: input.ID, UserID: input.UserID, Name: input.Name, Kind: kind, Status: domain.ClientOffline, CredentialHash: domain.HashCredential(input.Credential)}
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
	client, err := service.Store.Clients().ByID(ctx, input.ClientID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if client.UserID != input.UserID {
		return domain.Proxy{}, contracterr.Conflict("client does not belong to proxy user", nil)
	}
	if domain.NormalizeClientKind(client.Kind) != domain.ClientKindProvider {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"clientId": "client cannot provide proxy service"})
	}
	certificateID, err := service.resolveProxyCertificateSelection(ctx, input.Type, "", input.CertificateID, true, input.EntryHost, input.CertFile, input.KeyFile, input.ActorID)
	if err != nil {
		return domain.Proxy{}, err
	}
	// 证书绑定为权威路径：代理记录不再持久化原始静态证书路径。
	proxy := domain.Proxy{ID: input.ID, UserID: input.UserID, ClientID: input.ClientID, Name: input.Name, Type: input.Type, Status: domain.ProxyEnabled, EntryBindHost: input.EntryBindHost, EntryHost: input.EntryHost, EntryPort: input.EntryPort, TargetHost: input.TargetHost, TargetPort: input.TargetPort, CertificateID: certificateID, Description: input.Description}
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

func (service Service) CreateProxyRoute(ctx context.Context, input CreateProxyRouteInput) (domain.ProxyRoute, error) {
	if service.Store == nil {
		return domain.ProxyRoute{}, errors.New("store is required")
	}
	repository, ok := store.Routes(service.Store)
	if !ok {
		return domain.ProxyRoute{}, errors.New("proxy route repository is unavailable")
	}
	proxy, err := service.Store.Proxies().ByID(ctx, input.ProxyID)
	if err != nil {
		return domain.ProxyRoute{}, err
	}
	if proxy.Type != domain.ProxyHTTP && proxy.Type != domain.ProxyHTTPS {
		return domain.ProxyRoute{}, contracterr.Validation("validation failed", map[string]string{"proxyId": "routes require an HTTP or HTTPS proxy"})
	}
	client, err := service.Store.Clients().ByID(ctx, input.ClientID)
	if err != nil {
		return domain.ProxyRoute{}, err
	}
	if client.UserID != proxy.UserID {
		return domain.ProxyRoute{}, contracterr.Conflict("client does not belong to proxy user", nil)
	}
	if domain.NormalizeClientKind(client.Kind) != domain.ClientKindProvider {
		return domain.ProxyRoute{}, contracterr.Validation("validation failed", map[string]string{"clientId": "client cannot provide proxy service"})
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("route")
	}
	route := domain.ProxyRoute{ID: input.ID, ProxyID: input.ProxyID, ClientID: input.ClientID, PathPrefix: input.PathPrefix, StripPrefix: input.StripPrefix, UpstreamPathPrefix: input.UpstreamPathPrefix, TargetHost: input.TargetHost, TargetPort: input.TargetPort, Status: domain.ProxyRouteEnabled}
	if err := route.Validate(); err != nil {
		return domain.ProxyRoute{}, contracterr.Validation("validation failed", map[string]string{"route": err.Error()})
	}
	if err := repository.Create(ctx, route); err != nil {
		return domain.ProxyRoute{}, err
	}
	return route, service.audit(ctx, input.ActorID, "proxy_route", route.ID, "create_proxy_route")
}

func (service Service) UpdateProxyRoute(ctx context.Context, input UpdateProxyRouteInput) (domain.ProxyRoute, error) {
	if service.Store == nil {
		return domain.ProxyRoute{}, errors.New("store is required")
	}
	repository, ok := store.Routes(service.Store)
	if !ok {
		return domain.ProxyRoute{}, errors.New("proxy route repository is unavailable")
	}
	existing, err := repository.ByID(ctx, input.ID)
	if err != nil {
		return domain.ProxyRoute{}, err
	}
	proxy, err := service.Store.Proxies().ByID(ctx, existing.ProxyID)
	if err != nil {
		return domain.ProxyRoute{}, err
	}
	client, err := service.Store.Clients().ByID(ctx, input.ClientID)
	if err != nil {
		return domain.ProxyRoute{}, err
	}
	if client.UserID != proxy.UserID || domain.NormalizeClientKind(client.Kind) != domain.ClientKindProvider {
		return domain.ProxyRoute{}, contracterr.Conflict("client cannot be assigned to proxy route", nil)
	}
	existing.ClientID = input.ClientID
	existing.PathPrefix = input.PathPrefix
	existing.StripPrefix = input.StripPrefix
	existing.UpstreamPathPrefix = input.UpstreamPathPrefix
	existing.TargetHost = input.TargetHost
	existing.TargetPort = input.TargetPort
	if input.Status != "" {
		existing.Status = input.Status
	}
	if err := repository.Update(ctx, existing); err != nil {
		return domain.ProxyRoute{}, err
	}
	return existing, service.audit(ctx, input.ActorID, "proxy_route", existing.ID, "update_proxy_route")
}

func (service Service) DeleteProxyRoute(ctx context.Context, routeID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	repository, ok := store.Routes(service.Store)
	if !ok {
		return errors.New("proxy route repository is unavailable")
	}
	if err := repository.Delete(ctx, routeID); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "proxy_route", routeID, "delete_proxy_route")
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
	if input.Type != "" && input.Type != existing.Type && !(existing.Type.IsWeb() && (input.Type == domain.ProxyHTTP || input.Type == domain.ProxyHTTPS || input.Type == domain.ProxyWeb)) {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"type": "proxy type is immutable"})
	}
	if existing.Type == domain.ProxyForward {
		return domain.Proxy{}, contracterr.Unsupported("forward proxy is not supported in this management batch")
	}
	existing.Name = input.Name
	existing.TargetHost = input.TargetHost
	existing.TargetPort = input.TargetPort
	existing.Description = input.Description
	if existing.Type.IsWeb() {
		// host rename updates Domain; entry fields are not stored on web proxy
		if strings.TrimSpace(input.EntryHost) != "" && strings.TrimSpace(existing.DomainID) != "" {
			webDomain, domainErr := service.Store.Domains().ByID(ctx, existing.DomainID)
			if domainErr != nil {
				return domain.Proxy{}, domainErr
			}
			webDomain.Host = domain.NormalizeRouteHost(input.EntryHost)
			if err := webDomain.Validate(); err != nil {
				return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"entryHost": err.Error()})
			}
			if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
				return domain.Proxy{}, err
			}
			existing.EntryHost = webDomain.Host
		}
		if input.CertificateIDSet {
			if strings.TrimSpace(existing.DomainID) != "" {
				webDomain, domainErr := service.Store.Domains().ByID(ctx, existing.DomainID)
				if domainErr != nil {
					return domain.Proxy{}, domainErr
				}
				if input.CertificateID == "" {
					webDomain.CertificateID = ""
				} else if err := service.validateCertificateBinding(ctx, input.CertificateID, webDomain.Host, webDomain.ID); err != nil {
					return domain.Proxy{}, err
				} else {
					webDomain.CertificateID = input.CertificateID
				}
				if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
					return domain.Proxy{}, err
				}
				existing.CertificateID = webDomain.CertificateID
			}
		}
	} else {
		existing.EntryBindHost = input.EntryBindHost
		existing.EntryHost = input.EntryHost
		existing.EntryPort = input.EntryPort
		if err := validateProxyEntryFields(existing.Type, existing.EntryBindHost, existing.EntryHost, existing.EntryPort, input.CertFile, input.KeyFile); err != nil {
			return domain.Proxy{}, err
		}
		certificateID, err := service.resolveProxyCertificateSelection(ctx, existing.Type, existing.ID, input.CertificateID, input.CertificateIDSet, existing.EntryHost, input.CertFile, input.KeyFile, input.ActorID)
		if err != nil {
			return domain.Proxy{}, err
		}
		existing.CertificateID = certificateID
		existing.CertFile = ""
		existing.KeyFile = ""
		if err := service.ensureProxyAdmission(ctx, existing, existing.ID); err != nil {
			return domain.Proxy{}, err
		}
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
	providerType := input.ProviderType
	var certificate domain.ManagedCertificate
	if strings.TrimSpace(input.CertificateID) != "" {
		if service.Store == nil {
			return domain.ManagedCertificate{}, errors.New("store is required")
		}
		existing, err := service.Store.Certificates().ByID(ctx, input.CertificateID)
		if err != nil {
			return domain.ManagedCertificate{}, err
		}
		if providerType == "" {
			providerType = existing.ProviderType
		}
		credentialID := input.CredentialID
		if credentialID == "" {
			credentialID = existing.CredentialID
		}
		requestType := input.RequestType
		if requestType == "" {
			requestType = existing.RequestType
		}
		requestedValidity := input.RequestedValidity
		if requestedValidity == 0 {
			requestedValidity = existing.RequestedValidity
		}
		certificate, err = manager.IssueCertificate(ctx, certmanager.CertificateIssueRequest{CertificateID: input.CertificateID, Host: existing.Host, ProviderType: providerType, CredentialID: credentialID, RequestType: requestType, RequestedValidity: requestedValidity})
	} else {
		certificate, err = manager.IssueWithProvider(ctx, certmanager.ManagedCertificateRequest{ProxyID: input.ProxyID, ProviderType: providerType, CredentialID: input.CredentialID, RequestType: input.RequestType, RequestedValidity: input.RequestedValidity})
	}
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	action := "issue_managed_certificate"
	if providerType == domain.CertificateProviderCloudflareOriginCA {
		action = "issue_cloudflare_origin_certificate"
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, action)
}

func (service Service) RenewManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := manager.RenewByID(ctx, input.CertificateID, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, "renew_managed_certificate")
}

func (service Service) RotateOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := manager.RotateOriginCAByID(ctx, input.CertificateID, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, "rotate_cloudflare_origin_certificate")
}

func (service Service) SyncOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := manager.SyncOriginCAByID(ctx, input.CertificateID, input.ProxyID)
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, "sync_cloudflare_origin_certificate")
}

func (service Service) RevokeOriginCACertificate(ctx context.Context, input RevokeOriginCACertificateInput) (domain.ManagedCertificate, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	certificate, err := manager.RevokeOriginCA(ctx, certmanager.OriginCARevokeRequest{CertificateID: input.CertificateID, ProxyID: input.ProxyID, Host: input.Host, CloudflareCertificateID: input.CloudflareCertificateID})
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, "revoke_cloudflare_origin_certificate")
}

// CreateCertificate 创建一个证书资源（可未绑定代理），委托 certmanager 的身份化签发/登记。
func (service Service) CreateCertificate(ctx context.Context, input CreateCertificateInput) (domain.ManagedCertificate, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	host := strings.ToLower(strings.TrimSpace(input.Host))
	if host == "" {
		return domain.ManagedCertificate{}, contracterr.Validation("validation failed", map[string]string{"host": "certificate host is required"})
	}
	providerType := input.ProviderType
	if providerType == "" {
		providerType = domain.CertificateProviderACMEDNS01
	}
	if !providerType.Valid() {
		return domain.ManagedCertificate{}, contracterr.Validation("validation failed", map[string]string{"providerType": "certificate provider type is invalid"})
	}
	if providerType == domain.CertificateProviderFile && (strings.TrimSpace(input.CertFile) == "" || strings.TrimSpace(input.KeyFile) == "") {
		return domain.ManagedCertificate{}, contracterr.Validation("validation failed", map[string]string{"certFile": "file certificate requires both cert file and key file", "keyFile": "file certificate requires both cert file and key file"})
	}
	certificate, err := manager.IssueCertificate(ctx, certmanager.CertificateIssueRequest{Host: host, ProviderType: providerType, CredentialID: input.CredentialID, RequestType: input.RequestType, RequestedValidity: input.RequestedValidity, CertFile: input.CertFile, KeyFile: input.KeyFile})
	if err != nil {
		return domain.ManagedCertificate{}, err
	}
	action := "create_managed_certificate"
	if providerType == domain.CertificateProviderCloudflareOriginCA {
		action = "create_cloudflare_origin_certificate"
	}
	if providerType == domain.CertificateProviderFile {
		action = "create_file_certificate"
	}
	return certificate, service.audit(ctx, input.ActorID, "certificate", certificate.ID, action)
}

// DeleteCertificate 删除证书资源，按风险分级要求强确认。
// 当证书既绑定代理又当前可服务时（移除会影响正在服务的活跃材料）必须提供匹配的 ConfirmHost/ConfirmCertificateID；
// 未绑定、无效/过期/缺失材料或被 provider 状态阻断的证书可直接删除。
func (service Service) DeleteCertificate(ctx context.Context, input DeleteCertificateInput) (DeleteCertificateResult, error) {
	if service.Store == nil {
		return DeleteCertificateResult{}, errors.New("store is required")
	}
	certificateID := strings.TrimSpace(input.CertificateID)
	if certificateID == "" {
		return DeleteCertificateResult{}, contracterr.Validation("validation failed", map[string]string{"id": "certificate id is required"})
	}
	certificate, err := service.Store.Certificates().ByID(ctx, certificateID)
	if err != nil {
		return DeleteCertificateResult{}, err
	}
	boundProxy, hasBinding, err := service.proxyBoundToCertificate(ctx, certificateID)
	if err != nil {
		return DeleteCertificateResult{}, err
	}
	servable := service.certificateServable(certificate)
	requireConfirm := hasBinding && servable
	if requireConfirm && !service.deleteConfirmationMatches(certificate, input) {
		return DeleteCertificateResult{RequiredConfirm: true, CertificateID: certificateID}, contracterr.ConfirmationRequired("certificate is bound to a proxy and currently serving; confirm deletion with a matching host or certificate id", map[string]string{"confirmHost": certificate.Host, "confirmCertificateId": certificate.ID})
	}
	affected := make([]string, 0, 1)
	if hasBinding {
		if err := service.unbindProxyCertificate(ctx, boundProxy); err != nil {
			return DeleteCertificateResult{}, err
		}
		affected = append(affected, boundProxy.ID)
	}
	if err := service.Store.Certificates().Delete(ctx, certificateID); err != nil {
		return DeleteCertificateResult{}, err
	}
	service.cleanupManagedCertificateFiles(certificate)
	if err := service.reconcileProxyListeners(ctx); err != nil {
		return DeleteCertificateResult{}, err
	}
	if err := service.audit(ctx, input.ActorID, "certificate", certificateID, "delete_managed_certificate"); err != nil {
		return DeleteCertificateResult{}, err
	}
	return DeleteCertificateResult{CertificateID: certificateID, AffectedProxyIDs: affected, RequiredConfirm: requireConfirm}, nil
}

// BindCertificate 将证书绑定到 Web Proxy 所属 Domain（一对一）。
// 兼容输入仍使用 ProxyID，运行时解析 Domain。
func (service Service) BindCertificate(ctx context.Context, input BindCertificateInput) (domain.Proxy, error) {
	if service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.ProxyID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"proxyId": "proxy id is required"})
	}
	if strings.TrimSpace(input.CertificateID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"certificateId": "certificate id is required"})
	}
	proxy, err := service.Store.Proxies().ByID(ctx, input.ProxyID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if !proxy.Type.IsWeb() || strings.TrimSpace(proxy.DomainID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"type": "certificate binding requires a web proxy with domain"})
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if err := service.validateCertificateBinding(ctx, input.CertificateID, webDomain.Host, webDomain.ID); err != nil {
		return domain.Proxy{}, err
	}
	webDomain.CertificateID = input.CertificateID
	if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
		return domain.Proxy{}, err
	}
	// Surface domain binding on returned proxy for API/test compatibility.
	proxy.CertificateID = input.CertificateID
	if err := service.reconcileProxyListeners(ctx); err != nil {
		return domain.Proxy{}, err
	}
	return proxy, service.audit(ctx, input.ActorID, "domain", webDomain.ID, "bind_certificate")
}

// UnbindCertificate 清除 Domain 的证书绑定（证书保留为未绑定资源）。
func (service Service) UnbindCertificate(ctx context.Context, input UnbindCertificateInput) (domain.Proxy, error) {
	if service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	if strings.TrimSpace(input.ProxyID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"proxyId": "proxy id is required"})
	}
	proxy, err := service.Store.Proxies().ByID(ctx, input.ProxyID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if strings.TrimSpace(proxy.DomainID) == "" {
		return proxy, nil
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if strings.TrimSpace(webDomain.CertificateID) == "" {
		return proxy, nil
	}
	if err := service.unbindDomainCertificate(ctx, webDomain); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.reconcileProxyListeners(ctx); err != nil {
		return domain.Proxy{}, err
	}
	return proxy, service.audit(ctx, input.ActorID, "domain", webDomain.ID, "unbind_certificate")
}

// MigrateLegacyFileCertificates 在启动时将旧代理静态证书迁移为文件型证书资源并绑定。幂等。
func (service Service) MigrateLegacyFileCertificates(ctx context.Context) (int, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return 0, err
	}
	return manager.MigrateLegacyFileCertificates(ctx)
}

// resolveProxyCertificateSelection 解析代理的证书选择并返回应绑定的 certificateID：
//   - 显式 certificateID：校验存在/主机覆盖/一对一后返回；
//   - 否则若提供遗留 certFile/keyFile：登记为文件型证书资源并返回其 ID（兼容旧客户端）；
//   - 否则若是更新且未显式提供 certificateID：保留既有绑定；
//   - 否则返回空（无绑定/清除绑定）。
//
// 非 HTTPS 代理始终返回空。
func (service Service) resolveProxyCertificateSelection(ctx context.Context, proxyType domain.ProxyType, proxyID string, certificateID string, certificateIDSet bool, entryHost string, certFile string, keyFile string, actorID string) (string, error) {
	if proxyType != domain.ProxyHTTPS && proxyType != domain.ProxyWeb {
		return "", nil
	}
	certificateID = strings.TrimSpace(certificateID)
	host := strings.ToLower(strings.TrimSpace(entryHost))
	if certificateID != "" {
		if err := service.validateCertificateBinding(ctx, certificateID, host, proxyID); err != nil {
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
		// 未选择证书：若代理已绑定既有证书（更新场景），保留绑定。
		if strings.TrimSpace(proxyID) != "" {
			if existing, err := service.Store.Proxies().ByID(ctx, proxyID); err == nil {
				return existing.CertificateID, nil
			}
		}
		return "", nil
	}
	// 遗留兼容：将静态 cert/key 文件登记为文件型证书资源并绑定。
	manager, err := service.certificateManager()
	if err != nil {
		return "", err
	}
	certificate, err := manager.IssueCertificate(ctx, certmanager.CertificateIssueRequest{Host: host, ProviderType: domain.CertificateProviderFile, CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		return "", err
	}
	_ = service.audit(ctx, actorID, "certificate", certificate.ID, "migrate_file_certificate")
	return certificate.ID, nil
}

// validateCertificateBinding 校验证书可绑定到指定 Domain：
//
//	(a) 证书存在；(b) 主机覆盖；(c) 一对一（未被其他 Domain 绑定）。
func (service Service) validateCertificateBinding(ctx context.Context, certificateID string, host string, domainID string) error {
	certificate, err := service.Store.Certificates().ByID(ctx, certificateID)
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
	bound, err := service.Store.Domains().ByCertificateID(ctx, certificateID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err == nil && bound.ID != domainID {
		return contracterr.CertificateIncompatible("certificate is already bound to domain "+bound.ID, map[string]string{"certificateId": certificateID, "domainId": bound.ID})
	}
	return nil
}

// proxyBoundToCertificate 返回绑定该证书的 Domain 下任一 Proxy（兼容旧调用方）。
func (service Service) proxyBoundToCertificate(ctx context.Context, certificateID string) (domain.Proxy, bool, error) {
	webDomain, err := service.Store.Domains().ByCertificateID(ctx, certificateID)
	if errors.Is(err, store.ErrNotFound) {
		return domain.Proxy{}, false, nil
	}
	if err != nil {
		return domain.Proxy{}, false, err
	}
	proxies, err := service.Store.Proxies().ByDomainID(ctx, webDomain.ID)
	if err != nil {
		return domain.Proxy{}, false, err
	}
	if len(proxies) == 0 {
		return domain.Proxy{}, true, nil
	}
	return proxies[0], true, nil
}

// unbindProxyCertificate 兼容旧调用：通过 proxy 找到 Domain 并解绑证书。
func (service Service) unbindProxyCertificate(ctx context.Context, proxy domain.Proxy) error {
	if strings.TrimSpace(proxy.DomainID) == "" {
		proxy.CertificateID = ""
		proxy.CertFile = ""
		proxy.KeyFile = ""
		return service.Store.Proxies().Update(ctx, proxy)
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return err
	}
	return service.unbindDomainCertificate(ctx, webDomain)
}

func (service Service) unbindDomainCertificate(ctx context.Context, webDomain domain.Domain) error {
	webDomain.CertificateID = ""
	if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
		return err
	}
	// keep proxy status enabled; tests historically expected needs_config when cert unbound from https proxy
	proxies, err := service.Store.Proxies().ByDomainID(ctx, webDomain.ID)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		if proxy.Status == domain.ProxyEnabled {
			proxy.Status = domain.ProxyNeedsConf
			proxy.CertificateID = ""
			if err := service.Store.Proxies().Update(ctx, proxy); err != nil {
				return err
			}
		}
	}
	return nil
}

// boundProxyCertificate 解析 Web Proxy 所属 Domain 的绑定证书。
func (service Service) boundProxyCertificate(ctx context.Context, proxy domain.Proxy) (domain.ManagedCertificate, error) {
	if strings.TrimSpace(proxy.DomainID) != "" {
		webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
		if err == nil {
			return service.boundDomainCertificate(ctx, webDomain)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.ManagedCertificate{}, err
		}
	}
	if strings.TrimSpace(proxy.CertificateID) != "" {
		certificate, err := service.Store.Certificates().ByID(ctx, proxy.CertificateID)
		if err == nil {
			return certificate, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.ManagedCertificate{}, err
		}
	}
	return service.Store.Certificates().ByProxyID(ctx, proxy.ID)
}

// certificateServable 判断证书当前是否可服务（serving 可用且未被 provider 状态阻断且材料存在）。
func (service Service) certificateServable(certificate domain.ManagedCertificate) bool {
	if certificate.ProviderStatus.BlocksServing() {
		return false
	}
	if strings.TrimSpace(certificate.CertFile) == "" || strings.TrimSpace(certificate.KeyFile) == "" {
		return false
	}
	manager, err := service.certificateManager()
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	health := httpsproxy.CheckCertificateFiles(certificate.Host, certificate.CertFile, certificate.KeyFile, manager.Storage.CertificateDir, 0, now)
	return health.ServingStatus.ServesTLS()
}

// deleteConfirmationMatches 校验强确认是否匹配目标证书（host 或 cert id 任一相等即可）。
func (service Service) deleteConfirmationMatches(certificate domain.ManagedCertificate, input DeleteCertificateInput) bool {
	if strings.EqualFold(strings.TrimSpace(input.ConfirmCertificateID), strings.TrimSpace(certificate.ID)) && strings.TrimSpace(input.ConfirmCertificateID) != "" {
		return true
	}
	confirmHost := strings.ToLower(strings.TrimSpace(input.ConfirmHost))
	return confirmHost != "" && confirmHost == strings.ToLower(strings.TrimSpace(certificate.Host))
}

// cleanupManagedCertificateFiles 仅清理位于受管证书目录下的活跃/历史材料文件，绝不删除任意外部路径。
func (service Service) cleanupManagedCertificateFiles(certificate domain.ManagedCertificate) {
	manager, err := service.certificateManager()
	if err != nil {
		return
	}
	certificateDir := strings.TrimSpace(manager.Storage.CertificateDir)
	if certificateDir == "" {
		return
	}
	for _, path := range []string{certificate.CertFile, certificate.KeyFile, certificate.PreviousCertFile, certificate.PreviousKeyFile} {
		removeManagedFile(certificateDir, path)
	}
}

// removeManagedFile 仅删除位于受管证书目录下的文件，绝不删除目录外的任意路径。
func removeManagedFile(certificateDir string, path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	absDir, err := filepath.Abs(certificateDir)
	if err != nil {
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}
	relative, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return
	}
	if relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return
	}
	_ = os.Remove(absPath)
}

// hostnameWithinCertificate 以 VerifyHostname 风格（含通配符）判断证书元数据声明的主机集合是否覆盖 host。
// 用于绑定校验：此时证书材料文件可能尚未签发，因此基于 Host/Hostnames 元数据判断。
func hostnameWithinCertificate(certificate domain.ManagedCertificate, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	candidates := append([]string{certificate.Host}, certificate.Hostnames...)
	for _, candidate := range candidates {
		if hostnameMatchesPattern(host, candidate) {
			return true
		}
	}
	return false
}

func hostnameMatchesPattern(host string, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		if !strings.HasSuffix(host, suffix) {
			return false
		}
		label := host[:len(host)-len(suffix)]
		return label != "" && !strings.Contains(label, ".")
	}
	return false
}

func (service Service) CreateProviderCredential(ctx context.Context, input ProviderCredentialInput) (domain.ProviderCredential, error) {
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
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ProviderCredential{}, err
	}
	if manager.ProviderSecretStore == nil {
		return domain.ProviderCredential{}, providerCredentialStorageUnavailableError()
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = newID("cfcred")
	}
	secretRef, err := manager.ProviderSecretStore.Write(ctx, id, strings.TrimSpace(input.Token))
	if err != nil {
		return domain.ProviderCredential{}, err
	}
	now := time.Now().UTC()
	credential := domain.ProviderCredential{ID: id, Name: input.Name, ProviderType: domain.CertificateProviderCloudflareOriginCA, Scope: input.Scope, TokenFingerprint: certmanager.TokenFingerprint(input.Token), SecretRef: secretRef, Status: domain.ProviderCredentialPending, CreatedAt: now, UpdatedAt: now}
	if err := service.Store.ProviderCredentials().Create(ctx, credential); err != nil {
		_ = manager.ProviderSecretStore.Delete(ctx, secretRef)
		return domain.ProviderCredential{}, err
	}
	return credential, service.audit(ctx, input.ActorID, "provider_credential", credential.ID, "create_provider_credential")
}

func (service Service) UpdateProviderCredential(ctx context.Context, input UpdateProviderCredentialInput) (domain.ProviderCredential, error) {
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
		manager, err := service.certificateManager()
		if err != nil {
			return domain.ProviderCredential{}, err
		}
		if manager.ProviderSecretStore == nil {
			return domain.ProviderCredential{}, providerCredentialStorageUnavailableError()
		}
		secretRef, err := manager.ProviderSecretStore.Write(ctx, credential.ID, strings.TrimSpace(input.Token))
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
	return credential, service.audit(ctx, input.ActorID, "provider_credential", credential.ID, "update_provider_credential")
}

func (service Service) VerifyProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error) {
	manager, err := service.certificateManager()
	if err != nil {
		return domain.ProviderCredential{}, err
	}
	if manager.ProviderSecretStore == nil {
		return domain.ProviderCredential{}, providerCredentialStorageUnavailableError()
	}
	verifyErr := manager.VerifyProviderCredential(ctx, credentialID)
	credential, readErr := service.Store.ProviderCredentials().ByID(ctx, credentialID)
	if readErr != nil {
		return domain.ProviderCredential{}, readErr
	}
	auditErr := service.audit(ctx, actorID, "provider_credential", credential.ID, "verify_provider_credential")
	if verifyErr != nil {
		return credential, providerCredentialVerificationError(verifyErr)
	}
	return credential, auditErr
}

func (service Service) DisableProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error) {
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
	return credential, service.audit(ctx, actorID, "provider_credential", credential.ID, "disable_provider_credential")
}

func (service Service) DeleteProviderCredential(ctx context.Context, credentialID string, actorID string) error {
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
	manager, _ := service.certificateManager()
	if manager.ProviderSecretStore != nil {
		_ = manager.ProviderSecretStore.Delete(ctx, credential.SecretRef)
	}
	if err := service.Store.ProviderCredentials().Delete(ctx, credentialID); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "provider_credential", credential.ID, "delete_provider_credential")
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

func providerCredentialStorageUnavailableError() error {
	return contracterr.Unsupported("cloudflare origin ca credential storage is not configured; enable origin_ca_enabled and set origin_ca_secret_store_path")
}

func providerCredentialVerificationError(err error) error {
	if errors.Is(err, certmanager.ErrProviderCredentialDisabled) {
		return contracterr.Conflict("provider credential is disabled", err)
	}
	message := httpsproxy.SafeCertificateError(err)
	if strings.TrimSpace(message) == "" {
		message = "provider credential verification failed"
	}
	return contracterr.Conflict("provider credential verification failed: "+message, err)
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
	// Web listeners are admitted via DomainEntry, not Proxy entry fields.
	return proxyType == domain.ProxyTCP || proxyType == domain.ProxyUDP
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
	kind := domain.NormalizeClientKind(input.Kind)
	if !kind.Valid() {
		fields["kind"] = "client kind is invalid"
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
