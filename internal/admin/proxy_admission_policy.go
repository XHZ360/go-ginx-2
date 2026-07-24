package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type proxyAdmissionPolicy struct {
	Store                store.Store
	StaticListenerClaims []domain.ListenerClaim
	ProxyEntryDefaults   domain.ProxyEntryDefaults
	ListenerReconciler   ListenerReconciler
}

func (policy proxyAdmissionPolicy) EnsureAdmission(ctx context.Context, proxy domain.Proxy, ignoreProxyID string) error {
	if proxy.Status != domain.ProxyEnabled {
		return nil
	}
	if proxy.Type.IsWeb() {
		webProxy := proxy
		if proxy.Type != domain.ProxyWeb {
			webDomain, err := policy.Store.Domains().ByHost(ctx, proxy.EntryHost)
			if errors.Is(err, store.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			webProxy.Type = domain.ProxyWeb
			webProxy.DomainID = webDomain.ID
			webProxy.PathPrefix = "/"
		}
		conflict, ok, err := policy.findActiveWebRouteConflict(ctx, webProxy, ignoreProxyID)
		if err != nil {
			return err
		}
		if ok {
			return &contracterr.Error{Code: contracterr.CodeEntryConflict, Message: fmt.Sprintf("domain path %s conflicts with proxy %s", webProxy.PathPrefix, conflict.ID), Err: domain.ErrEntryConflict}
		}
		return nil
	}
	if !proxyRequiresListenerAdmission(proxy.Type) {
		return nil
	}
	proposedEntry, ok := domain.EffectiveProxyEntry(proxy, policy.ProxyEntryDefaults)
	if !ok {
		return nil
	}
	if conflict, ok, err := policy.findActiveRouteConflict(ctx, proposedEntry, ignoreProxyID); err != nil {
		return err
	} else if ok {
		return &contracterr.Error{Code: contracterr.CodeEntryConflict, Message: fmt.Sprintf("%s route %s on %s:%d conflicts with proxy %s", proposedEntry.Protocol, proposedEntry.RouteHost, displayBindHost(proposedEntry.BindHost), proposedEntry.Port, conflict.ID), Err: domain.ErrEntryConflict}
	}
	proposed, ok := domain.ListenerClaimForProxy(proxy, policy.ProxyEntryDefaults)
	if !ok {
		return nil
	}
	claims, err := policy.activeListenerClaims(ctx, ignoreProxyID)
	if err != nil {
		return err
	}
	if conflict, ok := domain.FindListenerConflict(claims, proposed); ok {
		return &domain.ListenerAdmissionError{Proposed: proposed, Conflict: conflict}
	}
	return nil
}

func (policy proxyAdmissionPolicy) activeListenerClaims(ctx context.Context, ignoreProxyID string) ([]domain.ListenerClaim, error) {
	claims := append([]domain.ListenerClaim(nil), policy.StaticListenerClaims...)
	proxies, err := policy.Store.Proxies().List(ctx)
	if err != nil {
		return nil, err
	}
	for _, proxy := range proxies {
		if proxy.ID == ignoreProxyID || proxy.Status != domain.ProxyEnabled {
			continue
		}
		claim, ok := domain.ListenerClaimForProxy(proxy, policy.ProxyEntryDefaults)
		if !ok {
			continue
		}
		claims = append(claims, claim)
	}
	return claims, nil
}

func (policy proxyAdmissionPolicy) ReconcileListeners(ctx context.Context) error {
	if policy.ListenerReconciler == nil {
		return nil
	}
	if err := policy.ListenerReconciler.ReconcileProxyListeners(ctx); err != nil {
		return &contracterr.Error{Code: contracterr.CodeEntryConflict, Message: "proxy listener reconcile failed", Err: err}
	}
	return nil
}

func (policy proxyAdmissionPolicy) findActiveRouteConflict(ctx context.Context, proposed domain.ProxyEntry, ignoreProxyID string) (domain.Proxy, bool, error) {
	if proposed.Protocol != domain.ListenerProtocolHTTP && proposed.Protocol != domain.ListenerProtocolHTTPS {
		return domain.Proxy{}, false, nil
	}
	proxies, err := policy.Store.Proxies().EnabledByType(ctx, domain.ProxyType(proposed.Protocol))
	if err != nil {
		return domain.Proxy{}, false, err
	}
	for _, proxy := range proxies {
		if proxy.ID == ignoreProxyID {
			continue
		}
		entry, ok := domain.EffectiveProxyEntry(proxy, policy.ProxyEntryDefaults)
		if !ok {
			continue
		}
		if entry.Protocol == proposed.Protocol && entry.Port == proposed.Port && domain.NormalizeBindHost(entry.BindHost) == domain.NormalizeBindHost(proposed.BindHost) && domain.NormalizeRouteHost(entry.RouteHost) == domain.NormalizeRouteHost(proposed.RouteHost) {
			return proxy, true, nil
		}
	}
	return domain.Proxy{}, false, nil
}

func (policy proxyAdmissionPolicy) findActiveWebRouteConflict(ctx context.Context, proposed domain.Proxy, ignoreProxyID string) (domain.Proxy, bool, error) {
	proxies, err := policy.Store.Proxies().EnabledWebByDomainID(ctx, proposed.DomainID)
	if err != nil {
		return domain.Proxy{}, false, err
	}
	for _, proxy := range proxies {
		if proxy.ID != ignoreProxyID && proxy.PathPrefix == proposed.PathPrefix {
			return proxy, true, nil
		}
	}
	return domain.Proxy{}, false, nil
}

var _ ProxyAdmissionPolicy = proxyAdmissionPolicy{}
