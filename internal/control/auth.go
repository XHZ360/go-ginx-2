package control

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrProtocolUnavailable  = errors.New("protocol unavailable")
)

type Authenticator struct {
	Store             store.Store
	AllowedProtocols  []domain.Protocol
	HeartbeatInterval time.Duration
	Now               func() time.Time
}

type AuthResult struct {
	Client            domain.Client
	User              domain.User
	SelectedProtocol  domain.Protocol
	HeartbeatInterval time.Duration
	ConfigVersion     int64
}

func (auth Authenticator) Authenticate(ctx context.Context, request AuthRequest) (AuthResult, error) {
	if auth.Store == nil {
		return AuthResult{}, errors.New("store is required")
	}
	if strings.TrimSpace(request.ClientID) == "" || strings.TrimSpace(request.Credential) == "" {
		return AuthResult{}, fmt.Errorf("%w: missing client credentials", ErrAuthenticationFailed)
	}
	if len(request.Protocols) == 0 {
		return AuthResult{}, fmt.Errorf("%w: no offered protocols", ErrProtocolUnavailable)
	}
	if err := auth.validateTimestamp(request.Timestamp); err != nil {
		return AuthResult{}, err
	}

	client, err := auth.Store.Clients().ByID(ctx, request.ClientID)
	if err != nil {
		return AuthResult{}, fmt.Errorf("%w: client lookup failed", ErrAuthenticationFailed)
	}
	user, err := auth.Store.Users().ByID(ctx, client.UserID)
	if err != nil {
		return AuthResult{}, fmt.Errorf("%w: user lookup failed", ErrAuthenticationFailed)
	}
	if user.Status != domain.UserEnabled {
		return AuthResult{}, fmt.Errorf("%w: user disabled", ErrAuthenticationFailed)
	}
	if client.Status == domain.ClientDisabled {
		return AuthResult{}, fmt.Errorf("%w: client disabled", ErrAuthenticationFailed)
	}
	if subtle.ConstantTimeCompare([]byte(domain.HashCredential(request.Credential)), []byte(client.CredentialHash)) != 1 {
		return AuthResult{}, fmt.Errorf("%w: credential mismatch", ErrAuthenticationFailed)
	}

	selected, err := selectProtocol(auth.allowedProtocols(), request.Protocols)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		Client:            client,
		User:              user,
		SelectedProtocol:  selected,
		HeartbeatInterval: auth.heartbeatInterval(),
		ConfigVersion:     client.Version,
	}, nil
}

func (auth Authenticator) allowedProtocols() []domain.Protocol {
	if len(auth.AllowedProtocols) > 0 {
		return auth.AllowedProtocols
	}
	return []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS}
}

func (auth Authenticator) heartbeatInterval() time.Duration {
	if auth.HeartbeatInterval > 0 {
		return auth.HeartbeatInterval
	}
	return 15 * time.Second
}

func (auth Authenticator) validateTimestamp(timestamp time.Time) error {
	if timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp is required", ErrAuthenticationFailed)
	}
	now := time.Now().UTC()
	if auth.Now != nil {
		now = auth.Now().UTC()
	}
	if timestamp.Before(now.Add(-5*time.Minute)) || timestamp.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("%w: timestamp outside allowed window", ErrAuthenticationFailed)
	}
	return nil
}

func selectProtocol(allowed []domain.Protocol, offered []domain.Protocol) (domain.Protocol, error) {
	for _, candidate := range allowed {
		if !candidate.Valid() {
			continue
		}
		if slices.Contains(offered, candidate) {
			return candidate, nil
		}
	}
	return "", ErrProtocolUnavailable
}
