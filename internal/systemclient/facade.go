// Package systemclient owns the system client identity and its management boundary.
package systemclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

const (
	ClientID       = "server-local"
	UserID         = "server-local-system"
	Username       = "__server_local_system__"
	ClientName     = "Server Local"
	disabledSecret = "system-client-has-no-control-credential"
)

type SystemClientFacade interface {
	Ensure(ctx context.Context) (domain.Client, error)
	Get(ctx context.Context) (domain.Client, error)
	IsSystemClient(clientID string) bool
}

type Service struct {
	Store store.Store
}

type internalMutationKey struct{}

// WithInternalMutation marks writes performed by the system identity and local
// proxy facades. Repository implementations reject protected object writes
// unless this marker is present.
func WithInternalMutation(ctx context.Context) context.Context {
	return context.WithValue(ctx, internalMutationKey{}, true)
}

func AllowsInternalMutation(ctx context.Context) bool {
	allowed, _ := ctx.Value(internalMutationKey{}).(bool)
	return allowed
}

func IsSystemClientID(clientID string) bool { return clientID == ClientID }

func IsSystemUserID(userID string) bool { return userID == UserID }

func IsSystemProxy(proxy domain.Proxy) bool { return IsSystemClientID(proxy.ClientID) }

func ProtectClientMutation(clientID string) error {
	if IsSystemClientID(clientID) {
		return &contracterr.Error{Code: contracterr.CodeForbidden, Message: "system client is immutable"}
	}
	return nil
}

func ProtectUserMutation(userID string) error {
	if IsSystemUserID(userID) {
		return &contracterr.Error{Code: contracterr.CodeForbidden, Message: "system user is immutable"}
	}
	return nil
}

func ProtectProxyMutation(proxy domain.Proxy) error {
	if IsSystemProxy(proxy) {
		return &contracterr.Error{Code: contracterr.CodeForbidden, Message: "system proxy must be managed through the local proxy API"}
	}
	return nil
}

func (service Service) Ensure(ctx context.Context) (domain.Client, error) {
	if service.Store == nil {
		return domain.Client{}, errors.New("store is required")
	}
	ctx = WithInternalMutation(ctx)
	user, err := service.Store.Users().ByID(ctx, UserID)
	if errors.Is(err, store.ErrNotFound) {
		user = domain.User{ID: UserID, Username: Username, Role: domain.RoleUser, Status: domain.UserDisabled}
		if err := service.Store.Users().Create(ctx, user); err != nil {
			return domain.Client{}, fmt.Errorf("create system user: %w", err)
		}
	} else if err != nil {
		return domain.Client{}, fmt.Errorf("load system user: %w", err)
	} else if user.Username != Username || user.Role != domain.RoleUser || user.Status != domain.UserDisabled || user.PasswordHash != "" {
		return domain.Client{}, errors.New("reserved system user has conflicting attributes")
	}

	client, err := service.Store.Clients().ByID(ctx, ClientID)
	if errors.Is(err, store.ErrNotFound) {
		client = domain.Client{ID: ClientID, UserID: UserID, Name: ClientName, Kind: domain.ClientKindProvider, Status: domain.ClientOffline, CredentialHash: disabledSecret}
		if err := service.Store.Clients().Create(ctx, client); err != nil {
			return domain.Client{}, fmt.Errorf("create system client: %w", err)
		}
		return service.Store.Clients().ByID(ctx, ClientID)
	}
	if err != nil {
		return domain.Client{}, fmt.Errorf("load system client: %w", err)
	}
	if client.UserID != UserID || client.Name != ClientName || domain.NormalizeClientKind(client.Kind) != domain.ClientKindProvider || client.Status != domain.ClientOffline || client.CredentialHash != disabledSecret {
		return domain.Client{}, errors.New("reserved system client has conflicting attributes")
	}
	return client, nil
}

func (service Service) Get(ctx context.Context) (domain.Client, error) {
	if service.Store == nil {
		return domain.Client{}, errors.New("store is required")
	}
	return service.Store.Clients().ByID(ctx, ClientID)
}

func (Service) IsSystemClient(clientID string) bool { return IsSystemClientID(clientID) }

var _ SystemClientFacade = Service{}
