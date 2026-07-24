// Package localproxy owns ports for future server-local proxy support.
package localproxy

import (
	"context"
	"net"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type LocalProxyInput struct{}

type AllowlistSnapshot struct{}

type AllowlistInput struct{}

type LocalProxyFacade interface {
	Create(ctx context.Context, actorID string, input LocalProxyInput) (domain.Proxy, error)
	Update(ctx context.Context, actorID string, input LocalProxyInput) (domain.Proxy, error)
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
