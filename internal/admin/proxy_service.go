package admin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/systemclient"
)

func (service *ProxyService) CreateProxy(ctx context.Context, input CreateProxyInput) (domain.Proxy, error) {
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
	if systemclient.IsSystemClientID(client.ID) {
		err := &contracterr.Error{Code: contracterr.CodeForbidden, Message: "system proxies must be created through the local proxy API"}
		recordRejectedAudit(ctx, service.Audit, input.ActorID, "proxy", input.ID, "create_proxy", err)
		return domain.Proxy{}, err
	}
	if client.UserID != input.UserID {
		return domain.Proxy{}, contracterr.Conflict("client does not belong to proxy user", nil)
	}
	if domain.NormalizeClientKind(client.Kind) != domain.ClientKindProvider {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"clientId": "client cannot provide proxy service"})
	}
	if input.Type == domain.ProxyWeb || (input.Type.IsWeb() && strings.TrimSpace(input.DomainID) != "") {
		proxy, err := service.createWebProxy(ctx, input)
		if err != nil {
			return domain.Proxy{}, err
		}
		if err := service.Admission.ReconcileListeners(ctx); err != nil {
			_ = service.Store.Proxies().Delete(ctx, proxy.ID)
			_ = service.Admission.ReconcileListeners(ctx)
			return domain.Proxy{}, err
		}
		return proxy, nil
	}
	certificateID, err := service.Binding.ResolveProxySelection(ctx, input.Type, "", input.CertificateID, true, input.EntryHost, input.CertFile, input.KeyFile, input.ActorID)
	if err != nil {
		return domain.Proxy{}, err
	}
	proxy := domain.Proxy{ID: input.ID, UserID: input.UserID, ClientID: input.ClientID, Name: input.Name, Type: input.Type, Status: domain.ProxyEnabled, EntryBindHost: input.EntryBindHost, EntryHost: input.EntryHost, EntryPort: input.EntryPort, TargetHost: input.TargetHost, TargetPort: input.TargetPort, CertificateID: certificateID, Description: input.Description}
	if err := service.Admission.EnsureAdmission(ctx, proxy, ""); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Store.Proxies().Create(ctx, proxy); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Proxies().Delete(ctx, proxy.ID)
		_ = service.Admission.ReconcileListeners(ctx)
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
	return proxy, service.Audit.Record(ctx, input.ActorID, "proxy", proxy.ID, action)
}

func (service *ProxyService) createWebProxy(ctx context.Context, input CreateProxyInput) (domain.Proxy, error) {
	if strings.TrimSpace(input.DomainID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"domainId": "domain id is required"})
	}
	webDomain, err := service.Store.Domains().ByID(ctx, input.DomainID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if webDomain.UserID != input.UserID {
		return domain.Proxy{}, contracterr.Conflict("domain does not belong to proxy user", nil)
	}
	pathPrefix := input.PathPrefix
	if pathPrefix == "" {
		pathPrefix = "/"
	}
	normalized, err := domain.NormalizeProxyRoutePrefix(pathPrefix)
	if err != nil {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"pathPrefix": err.Error()})
	}
	upstream, err := domain.NormalizeProxyUpstreamPathPrefix(input.UpstreamPathPrefix)
	if err != nil {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"upstreamPathPrefix": err.Error()})
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("proxy")
	}
	proxy := domain.Proxy{ID: input.ID, UserID: input.UserID, ClientID: input.ClientID, Name: input.Name, Type: domain.ProxyWeb, Status: domain.ProxyEnabled, DomainID: webDomain.ID, PathPrefix: normalized, StripPrefix: input.StripPrefix, UpstreamPathPrefix: upstream, TargetHost: input.TargetHost, TargetPort: input.TargetPort, Description: input.Description}
	if err := proxy.Validate(); err != nil {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"proxy": err.Error()})
	}
	if err := service.Admission.EnsureAdmission(ctx, proxy, ""); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Store.Proxies().Create(ctx, proxy); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			return domain.Proxy{}, contracterr.Conflict("domain path is already used by another proxy", err)
		}
		return domain.Proxy{}, err
	}
	return proxy, service.Audit.Record(ctx, input.ActorID, "proxy", proxy.ID, "create_web_proxy")
}

func (service *ProxyService) UpdateProxy(ctx context.Context, input UpdateProxyInput) (domain.Proxy, error) {
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
	if err := systemclient.ProtectProxyMutation(existing); err != nil {
		recordRejectedAudit(ctx, service.Audit, input.ActorID, "proxy", input.ID, "update_proxy", err)
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
		identityChanged := false
		if input.DomainIDSet && strings.TrimSpace(input.DomainID) != "" && input.DomainID != existing.DomainID {
			webDomain, domainErr := service.Store.Domains().ByID(ctx, input.DomainID)
			if domainErr != nil {
				return domain.Proxy{}, domainErr
			}
			if webDomain.UserID != existing.UserID {
				return domain.Proxy{}, contracterr.Conflict("domain does not belong to proxy user", nil)
			}
			existing.DomainID = webDomain.ID
			identityChanged = true
		}
		if input.PathPrefixSet && strings.TrimSpace(input.PathPrefix) != "" {
			normalized, pathErr := domain.NormalizeProxyRoutePrefix(input.PathPrefix)
			if pathErr != nil {
				return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"pathPrefix": pathErr.Error()})
			}
			if normalized != existing.PathPrefix {
				existing.PathPrefix = normalized
				identityChanged = true
			}
		}
		if input.StripPrefixSet {
			existing.StripPrefix = input.StripPrefix
		}
		if input.UpstreamPathPrefixSet {
			upstream, upErr := domain.NormalizeProxyUpstreamPathPrefix(input.UpstreamPathPrefix)
			if upErr != nil {
				return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"upstreamPathPrefix": upErr.Error()})
			}
			existing.UpstreamPathPrefix = upstream
		}
		if strings.TrimSpace(input.EntryHost) != "" && strings.TrimSpace(existing.DomainID) != "" {
			webDomain, domainErr := service.Store.Domains().ByID(ctx, existing.DomainID)
			if domainErr != nil {
				return domain.Proxy{}, domainErr
			}
			nextHost := domain.NormalizeRouteHost(input.EntryHost)
			hostChanged := nextHost != webDomain.Host
			webDomain.Host = nextHost
			if err := webDomain.Validate(); err != nil {
				return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"entryHost": err.Error()})
			}
			if webDomain.CertificateID != "" {
				if err := service.Binding.ValidateBinding(ctx, webDomain.CertificateID, webDomain.Host, webDomain.ID); err != nil {
					return domain.Proxy{}, err
				}
			}
			if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
				return domain.Proxy{}, err
			}
			if hostChanged {
				if err := service.Access.RevokeForDomain(ctx, webDomain.ID); err != nil {
					return domain.Proxy{}, err
				}
				if existing.AccessAuthEnabled {
					reloaded, reloadErr := service.Store.Proxies().ByID(ctx, existing.ID)
					if reloadErr != nil {
						return domain.Proxy{}, reloadErr
					}
					existing.AccessAuthVersion = reloaded.AccessAuthVersion
				}
			}
		}
		if input.CertificateIDSet && strings.TrimSpace(existing.DomainID) != "" {
			webDomain, domainErr := service.Store.Domains().ByID(ctx, existing.DomainID)
			if domainErr != nil {
				return domain.Proxy{}, domainErr
			}
			if input.CertificateID == "" {
				webDomain.CertificateID = ""
			} else if err := service.Binding.ValidateBinding(ctx, input.CertificateID, webDomain.Host, webDomain.ID); err != nil {
				return domain.Proxy{}, err
			} else {
				webDomain.CertificateID = input.CertificateID
			}
			if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
				return domain.Proxy{}, err
			}
		}
		if identityChanged {
			if err := service.Access.RevokeIfEnabled(ctx, &existing); err != nil {
				return domain.Proxy{}, err
			}
		}
	} else {
		existing.EntryBindHost = input.EntryBindHost
		existing.EntryHost = input.EntryHost
		existing.EntryPort = input.EntryPort
		if err := validateProxyEntryFields(existing.Type, existing.EntryBindHost, existing.EntryHost, existing.EntryPort, input.CertFile, input.KeyFile); err != nil {
			return domain.Proxy{}, err
		}
		certificateID, err := service.Binding.ResolveProxySelection(ctx, existing.Type, existing.ID, input.CertificateID, input.CertificateIDSet, existing.EntryHost, input.CertFile, input.KeyFile, input.ActorID)
		if err != nil {
			return domain.Proxy{}, err
		}
		existing.CertificateID = certificateID
		existing.CertFile = ""
		existing.KeyFile = ""
		if err := service.Admission.EnsureAdmission(ctx, existing, existing.ID); err != nil {
			return domain.Proxy{}, err
		}
	}
	if err := service.Admission.EnsureAdmission(ctx, existing, existing.ID); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Store.Proxies().Update(ctx, existing); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Proxies().Update(ctx, previous)
		_ = service.Admission.ReconcileListeners(ctx)
		return domain.Proxy{}, err
	}
	return existing, service.Audit.Record(ctx, input.ActorID, "proxy", existing.ID, "update_proxy")
}

func (service *ProxyService) EnableProxy(ctx context.Context, proxyID string, actorID string) error {
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
	if err := systemclient.ProtectProxyMutation(proxy); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "proxy", proxyID, "enable_proxy", err)
		return err
	}
	if proxy.Type == domain.ProxyForward {
		return contracterr.Unsupported("forward proxy is not supported in this management batch")
	}
	proxy.Status = domain.ProxyEnabled
	if err := service.Admission.EnsureAdmission(ctx, proxy, proxy.ID); err != nil {
		return err
	}
	if err := service.Store.Proxies().SetStatus(ctx, proxyID, domain.ProxyEnabled); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Proxies().SetStatus(ctx, proxyID, domain.ProxyDisabled)
		_ = service.Admission.ReconcileListeners(ctx)
		return err
	}
	return service.Audit.Record(ctx, actorID, "proxy", proxyID, "enable_proxy")
}

func (service *ProxyService) DisableProxy(ctx context.Context, proxyID string, actorID string) error {
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
	if err := systemclient.ProtectProxyMutation(proxy); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "proxy", proxyID, "disable_proxy", err)
		return err
	}
	if err := service.Store.Proxies().SetStatus(ctx, proxyID, domain.ProxyDisabled); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "proxy", proxyID, "disable_proxy")
}

func (service *ProxyService) DeleteProxy(ctx context.Context, proxyID string, actorID string) error {
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
	if err := systemclient.ProtectProxyMutation(proxy); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "proxy", proxyID, "delete_proxy", err)
		return err
	}
	if proxy.Status != domain.ProxyDisabled {
		return contracterr.Conflict("proxy must be disabled before delete", nil)
	}
	if err := service.Store.Proxies().Delete(ctx, proxyID); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "proxy", proxyID, "delete_proxy")
}

func (service *ProxyService) EnableProxyAccessAuthAndCreateActivation(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error) {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return ProxyActivationResult{}, err
	}
	if err := systemclient.ProtectProxyMutation(proxy); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "proxy", proxyID, "enable_proxy_access_auth", err)
		return ProxyActivationResult{}, err
	}
	if !proxy.Type.IsWeb() || strings.TrimSpace(proxy.DomainID) == "" {
		return ProxyActivationResult{}, contracterr.Validation("validation failed", map[string]string{"proxyId": "access authentication requires a web proxy with a domain"})
	}
	webDomain, err := service.Store.Domains().ByID(ctx, proxy.DomainID)
	if err != nil {
		return ProxyActivationResult{}, err
	}
	if !service.Access.DomainHasEnabledHTTPSEntry(ctx, webDomain.ID) {
		return ProxyActivationResult{}, contracterr.Conflict("domain has no enabled HTTPS entry", nil)
	}
	certificate, err := service.Binding.BoundDomain(ctx, webDomain)
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
	if err := service.Audit.Record(ctx, actorID, "proxy", proxyID, "enable_proxy_access_auth"); err != nil {
		return ProxyActivationResult{}, err
	}
	return ProxyActivationResult{URL: proxyActivationURL(webDomain, proxy, tokenValue), ExpiresAt: expiresAt}, nil
}

func (service *ProxyService) CreateProxyActivationLink(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error) {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return ProxyActivationResult{}, err
	}
	if err := systemclient.ProtectProxyMutation(proxy); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "proxy", proxyID, "create_proxy_activation", err)
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
	if err := service.Audit.Record(ctx, actorID, "proxy", proxyID, "create_proxy_activation"); err != nil {
		return ProxyActivationResult{}, err
	}
	return ProxyActivationResult{URL: proxyActivationURL(webDomain, proxy, tokenValue), ExpiresAt: expiresAt}, nil
}

func (service *ProxyService) RevokeAllProxyAccess(ctx context.Context, proxyID string, actorID string) error {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return err
	}
	if err := systemclient.ProtectProxyMutation(proxy); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "proxy", proxyID, "revoke_proxy_access", err)
		return err
	}
	access, ok := store.Access(service.Store)
	if !ok {
		return errors.New("proxy access repository is unavailable")
	}
	if err := access.RevokeAllAccess(ctx, proxyID, proxy.AccessAuthVersion+1); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "proxy", proxyID, "revoke_proxy_access")
}

func (service *ProxyService) DisableProxyAccessAuth(ctx context.Context, proxyID string, actorID string) error {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return err
	}
	if err := systemclient.ProtectProxyMutation(proxy); err != nil {
		recordRejectedAudit(ctx, service.Audit, actorID, "proxy", proxyID, "disable_proxy_access_auth", err)
		return err
	}
	access, ok := store.Access(service.Store)
	if !ok {
		return errors.New("proxy access repository is unavailable")
	}
	if err := access.DisableAuth(ctx, proxyID, proxy.AccessAuthVersion+1); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "proxy", proxyID, "disable_proxy_access_auth")
}

var _ ProxyFacade = (*ProxyService)(nil)
