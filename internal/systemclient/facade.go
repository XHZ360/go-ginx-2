// Package systemclient owns the system client identity and its management boundary.
package systemclient

import (
	"context"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type SystemClientFacade interface {
	Ensure(ctx context.Context) (domain.Client, error)
	Get(ctx context.Context) (domain.Client, error)
	IsSystemClient(clientID string) bool
}
