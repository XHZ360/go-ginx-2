// Package localproxy owns server-local proxy policy, dialing, and management ports.
package localproxy

import (
	"context"
	"net"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type LocalProxyInput struct {
	ID            string
	Name          string
	Type          domain.ProxyType
	EntryBindHost string
	EntryPort     int
	TargetHost    string
	TargetPort    int
	Description   string
}

type AllowlistEntry struct {
	CIDR      string
	PortStart int
	PortEnd   int
}

type AllowlistSnapshot struct {
	Entries []AllowlistEntry
}

type AllowlistInput struct {
	Entries []AllowlistEntry
}

type AllowlistRepository interface {
	List(ctx context.Context) ([]AllowlistEntry, error)
	Replace(ctx context.Context, entries []AllowlistEntry) error
}

type AllowlistRepositoryStore interface {
	LocalAllowlist() AllowlistRepository
}

type LocalProxyFacade interface {
	Create(ctx context.Context, actorID string, input LocalProxyInput) (domain.Proxy, error)
	Update(ctx context.Context, actorID string, input LocalProxyInput) (domain.Proxy, error)
	Enable(ctx context.Context, actorID string, proxyID string) error
	Disable(ctx context.Context, actorID string, proxyID string) error
	Delete(ctx context.Context, actorID string, proxyID string) error
}

type LocalTargetPolicy interface {
	ValidateTarget(ctx context.Context, host string, port int) error
	Snapshot() AllowlistSnapshot
	Replace(ctx context.Context, input AllowlistInput) error
}

type LocalDialer interface {
	DialContext(ctx context.Context, network string, address string) (net.Conn, error)
}
