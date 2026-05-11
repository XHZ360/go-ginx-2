package store

import (
	"context"
	"errors"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
)

type Store interface {
	Users() UserRepository
	Clients() ClientRepository
	Proxies() ProxyRepository
	AuditEvents() AuditRepository
	Close() error
}

type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	ByID(ctx context.Context, id string) (domain.User, error)
	ByUsername(ctx context.Context, username string) (domain.User, error)
	SetStatus(ctx context.Context, id string, status domain.UserStatus) error
}

type ClientRepository interface {
	Create(ctx context.Context, client domain.Client) error
	ByID(ctx context.Context, id string) (domain.Client, error)
	SetStatus(ctx context.Context, id string, status domain.ClientStatus) error
	RotateCredential(ctx context.Context, id string, credentialHash string) error
}

type ProxyRepository interface {
	Create(ctx context.Context, proxy domain.Proxy) error
	ByID(ctx context.Context, id string) (domain.Proxy, error)
	ByClientID(ctx context.Context, clientID string) ([]domain.Proxy, error)
	EnabledByType(ctx context.Context, proxyType domain.ProxyType) ([]domain.Proxy, error)
	ByTCPEntryPort(ctx context.Context, port int) (domain.Proxy, error)
	ByHTTPHost(ctx context.Context, host string) (domain.Proxy, error)
	SetStatus(ctx context.Context, id string, status domain.ProxyStatus) error
}

type AuditRepository interface {
	Create(ctx context.Context, event domain.AuditEvent) error
}
