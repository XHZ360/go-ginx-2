package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type proxyAccessPolicy struct{ Store store.Store }

func (policy proxyAccessPolicy) RevokeIfEnabled(ctx context.Context, proxy *domain.Proxy) error {
	if proxy == nil || !proxy.AccessAuthEnabled {
		return nil
	}
	access, ok := store.Access(policy.Store)
	if !ok {
		return errors.New("proxy access repository is unavailable")
	}
	nextVersion := proxy.AccessAuthVersion + 1
	if err := access.RevokeAllAccess(ctx, proxy.ID, nextVersion); err != nil {
		return err
	}
	proxy.AccessAuthVersion = nextVersion
	return nil
}

func (policy proxyAccessPolicy) RevokeForDomain(ctx context.Context, domainID string) error {
	if strings.TrimSpace(domainID) == "" || policy.Store == nil {
		return nil
	}
	proxies, err := policy.Store.Proxies().ByDomainID(ctx, domainID)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		item := proxy
		if err := policy.RevokeIfEnabled(ctx, &item); err != nil {
			return err
		}
	}
	return nil
}

func (policy proxyAccessPolicy) DomainHasEnabledHTTPSEntry(ctx context.Context, domainID string) bool {
	entries, err := policy.Store.DomainEntries().ListByDomainID(ctx, domainID)
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

var _ ProxyAccessPolicy = proxyAccessPolicy{}
