package admin

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

func (service *ClientService) CreateClient(ctx context.Context, input CreateClientInput) (domain.Client, error) {
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
	if err := service.Audit.Record(ctx, input.ActorID, "client", client.ID, "create_client"); err != nil {
		return domain.Client{}, err
	}
	return client, nil
}

func (service *ClientService) CreateClientWithCredential(ctx context.Context, input CreateClientInput) (CreateClientResult, error) {
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

func (service *ClientService) CreateClientJoin(ctx context.Context, input CreateClientJoinInput) (CreateClientJoinResult, error) {
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
	payload := enrollment.TokenPayload{EnrollmentID: enrollmentID, Secret: secret, EnrollmentURL: input.EnrollmentURL, ServerAddress: input.ServerAddress, ServerTLSAddress: input.ServerTLSAddress, ServerName: input.ServerName, CAPEM: string(caPEM), ClientID: clientResult.Client.ID, Credential: clientResult.Credential, AllowedProtocols: append([]domain.Protocol(nil), input.AllowedProtocols...), Reconnect: input.Reconnect, ExpiresAt: expiresAt}
	token, err := enrollment.EncodeToken(payload)
	if err != nil {
		return CreateClientJoinResult{}, err
	}
	record := domain.ClientEnrollment{ID: enrollmentID, ClientID: clientResult.Client.ID, SecretHash: enrollment.HashSecret(secret), TokenHash: enrollment.HashToken(token), Token: token, ExpiresAt: expiresAt}
	if err := service.Store.ClientEnrollments().Create(ctx, record); err != nil {
		return CreateClientJoinResult{}, err
	}
	if err := service.Audit.Record(ctx, input.ActorID, "client", clientResult.Client.ID, "create_client_join"); err != nil {
		return CreateClientJoinResult{}, err
	}
	clientResult.Client.CredentialHash = ""
	return CreateClientJoinResult{Client: clientResult.Client, Token: token}, nil
}

func (service *ClientService) ReviewClientJoinToken(ctx context.Context, clientID string, actorID string) (ReviewClientJoinTokenResult, error) {
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
	if err := service.Audit.Record(ctx, actorID, "client", client.ID, "review_client_join_token"); err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	return ReviewClientJoinTokenResult{Client: client, Token: enrollmentRecord.Token, ExpiresAt: enrollmentRecord.ExpiresAt}, nil
}

func (service *ClientService) resetClientJoinToken(ctx context.Context, client domain.Client, actorID string, now time.Time, allowActive bool) (ReviewClientJoinTokenResult, error) {
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
	if err := service.Audit.Record(ctx, actorID, "client", client.ID, "reset_client_join_token"); err != nil {
		return ReviewClientJoinTokenResult{}, err
	}
	client.CredentialHash = ""
	return ReviewClientJoinTokenResult{Client: client, Token: token, ExpiresAt: expiresAt}, nil
}

func (service *ClientService) tokenUsesLegacyAdminEnrollmentURL(token string, clientID string) bool {
	payload, err := enrollment.DecodeToken(token)
	if err != nil || payload.ClientID != clientID {
		return false
	}
	return service.usesLegacyAdminEnrollmentURL(payload.EnrollmentURL)
}

func (service *ClientService) usesLegacyAdminEnrollmentURL(enrollmentURL string) bool {
	current := strings.TrimSpace(service.DefaultJoin.EnrollmentURL)
	legacy := strings.TrimSpace(service.DefaultJoin.LegacyAdminEnrollmentURL)
	return current != "" && legacy != "" && current != legacy && strings.TrimSpace(enrollmentURL) == legacy
}

func (service *ClientService) defaultJoinTokenPayload() (enrollment.TokenPayload, error) {
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
	return enrollment.TokenPayload{EnrollmentURL: service.DefaultJoin.EnrollmentURL, ServerAddress: service.DefaultJoin.ServerAddress, ServerTLSAddress: service.DefaultJoin.ServerTLSAddress, ServerName: service.DefaultJoin.ServerName, CAPEM: string(caPEM), AllowedProtocols: append([]domain.Protocol(nil), defaultClient.AllowedProtocols...), Reconnect: defaultClient.Reconnect}, nil
}

func (service *ClientService) EnableClient(ctx context.Context, clientID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "client id is required"})
	}
	if err := service.Store.Clients().SetStatus(ctx, clientID, domain.ClientOffline); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "client", clientID, "enable_client")
}

func (service *ClientService) DisableClient(ctx context.Context, clientID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "client id is required"})
	}
	if err := service.Store.Clients().SetStatus(ctx, clientID, domain.ClientDisabled); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "client", clientID, "disable_client")
}

func (service *ClientService) DeleteClient(ctx context.Context, clientID string, actorID string) error {
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
	return service.Audit.Record(ctx, actorID, "client", clientID, "delete_client")
}

func (service *ClientService) RotateClientCredential(ctx context.Context, input RotateClientCredentialInput) (RotateClientCredentialResult, error) {
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
	if err := service.Audit.Record(ctx, input.ActorID, "client", client.ID, "rotate_client_credential"); err != nil {
		return RotateClientCredentialResult{}, err
	}
	client.CredentialHash = ""
	return RotateClientCredentialResult{Client: client, Credential: credential}, nil
}

var _ ClientFacade = (*ClientService)(nil)
