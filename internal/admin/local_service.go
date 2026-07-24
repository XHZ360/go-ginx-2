package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/localproxy"
	"github.com/simp-frp/go-ginx-2/internal/systemclient"
)

type LocalAdministrationFacade interface {
	LocalAllowlist() localproxy.AllowlistSnapshot
	ReplaceLocalAllowlist(ctx context.Context, actorID string, input localproxy.AllowlistInput) (localproxy.AllowlistSnapshot, error)
	CreateLocalProxy(ctx context.Context, actorID string, input localproxy.LocalProxyInput) (domain.Proxy, error)
	UpdateLocalProxy(ctx context.Context, actorID string, input localproxy.LocalProxyInput) (domain.Proxy, error)
	EnableLocalProxy(ctx context.Context, actorID string, proxyID string) error
	DisableLocalProxy(ctx context.Context, actorID string, proxyID string) error
	DeleteLocalProxy(ctx context.Context, actorID string, proxyID string) error
}

func (service *LocalService) LocalAllowlist() localproxy.AllowlistSnapshot {
	if service == nil || service.Policy == nil {
		return localproxy.AllowlistSnapshot{}
	}
	return service.Policy.Snapshot()
}

func (service *LocalService) ReplaceLocalAllowlist(ctx context.Context, actorID string, input localproxy.AllowlistInput) (snapshot localproxy.AllowlistSnapshot, err error) {
	defer func() {
		err = finishAudit(ctx, service.Audit, actorID, "local_allowlist", systemclient.ClientID, "replace_local_allowlist", err)
	}()
	if service == nil || service.Policy == nil {
		return localproxy.AllowlistSnapshot{}, errors.New("local target policy is required")
	}
	if err := service.Policy.Replace(ctx, input); err != nil {
		return localproxy.AllowlistSnapshot{}, contracterr.Validation("invalid local target allowlist", map[string]string{"entries": err.Error()})
	}
	return service.Policy.Snapshot(), nil
}

func (service *LocalService) CreateLocalProxy(ctx context.Context, actorID string, input localproxy.LocalProxyInput) (proxy domain.Proxy, err error) {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("local-proxy")
	}
	defer func() { err = finishAudit(ctx, service.Audit, actorID, "proxy", input.ID, "create_local_proxy", err) }()
	if err := service.validateInput(ctx, input, false); err != nil {
		return domain.Proxy{}, err
	}
	proxy = localProxyFromInput(input)
	mutationCtx := systemclient.WithInternalMutation(ctx)
	if err := service.Admission.EnsureAdmission(ctx, proxy, ""); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Store.Proxies().Create(mutationCtx, proxy); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Proxies().Delete(mutationCtx, proxy.ID)
		_ = service.Admission.ReconcileListeners(ctx)
		return domain.Proxy{}, err
	}
	return proxy, nil
}

func (service *LocalService) UpdateLocalProxy(ctx context.Context, actorID string, input localproxy.LocalProxyInput) (proxy domain.Proxy, err error) {
	defer func() { err = finishAudit(ctx, service.Audit, actorID, "proxy", input.ID, "update_local_proxy", err) }()
	if strings.TrimSpace(input.ID) == "" {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"id": "proxy id is required"})
	}
	existing, err := service.systemProxy(ctx, input.ID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if input.Type == "" {
		input.Type = existing.Type
	}
	if input.Type != existing.Type {
		return domain.Proxy{}, contracterr.Validation("validation failed", map[string]string{"type": "proxy type is immutable"})
	}
	if err := service.validateInput(ctx, input, true); err != nil {
		return domain.Proxy{}, err
	}
	updated := localProxyFromInput(input)
	mutationCtx := systemclient.WithInternalMutation(ctx)
	updated.Status = existing.Status
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = existing.UpdatedAt
	if err := service.Admission.EnsureAdmission(ctx, updated, updated.ID); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Store.Proxies().Update(mutationCtx, updated); err != nil {
		return domain.Proxy{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Proxies().Update(mutationCtx, existing)
		_ = service.Admission.ReconcileListeners(ctx)
		return domain.Proxy{}, err
	}
	return updated, nil
}

func (service *LocalService) EnableLocalProxy(ctx context.Context, actorID string, proxyID string) (err error) {
	defer func() { err = finishAudit(ctx, service.Audit, actorID, "proxy", proxyID, "enable_local_proxy", err) }()
	proxy, err := service.systemProxy(ctx, proxyID)
	if err != nil {
		return err
	}
	proxy.Status = domain.ProxyEnabled
	mutationCtx := systemclient.WithInternalMutation(ctx)
	if err := service.Admission.EnsureAdmission(ctx, proxy, proxy.ID); err != nil {
		return err
	}
	if err := service.Store.Proxies().SetStatus(mutationCtx, proxyID, domain.ProxyEnabled); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Proxies().SetStatus(mutationCtx, proxyID, domain.ProxyDisabled)
		_ = service.Admission.ReconcileListeners(ctx)
		return err
	}
	return nil
}

func (service *LocalService) DisableLocalProxy(ctx context.Context, actorID string, proxyID string) (err error) {
	defer func() { err = finishAudit(ctx, service.Audit, actorID, "proxy", proxyID, "disable_local_proxy", err) }()
	if _, err := service.systemProxy(ctx, proxyID); err != nil {
		return err
	}
	if err := service.Store.Proxies().SetStatus(systemclient.WithInternalMutation(ctx), proxyID, domain.ProxyDisabled); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Proxies().SetStatus(systemclient.WithInternalMutation(ctx), proxyID, domain.ProxyEnabled)
		_ = service.Admission.ReconcileListeners(ctx)
		return err
	}
	return nil
}

func (service *LocalService) DeleteLocalProxy(ctx context.Context, actorID string, proxyID string) (err error) {
	defer func() { err = finishAudit(ctx, service.Audit, actorID, "proxy", proxyID, "delete_local_proxy", err) }()
	proxy, err := service.systemProxy(ctx, proxyID)
	if err != nil {
		return err
	}
	if proxy.Status != domain.ProxyDisabled {
		return contracterr.Conflict("local proxy must be disabled before delete", nil)
	}
	if err := service.Store.Proxies().Delete(systemclient.WithInternalMutation(ctx), proxyID); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return err
	}
	return nil
}

func (service *LocalService) validateInput(ctx context.Context, input localproxy.LocalProxyInput, requireID bool) error {
	if service == nil || service.Store == nil || service.Policy == nil {
		return errors.New("local proxy service dependencies are required")
	}
	if requireID && strings.TrimSpace(input.ID) == "" {
		return contracterr.Validation("validation failed", map[string]string{"id": "proxy id is required"})
	}
	if input.Type != domain.ProxyTCP && input.Type != domain.ProxyUDP {
		return contracterr.Validation("validation failed", map[string]string{"type": "local proxy type must be tcp or udp"})
	}
	if err := service.Policy.ValidateTarget(ctx, input.TargetHost, input.TargetPort); err != nil {
		return contracterr.Validation("validation failed", map[string]string{"target": err.Error()})
	}
	proxy := localProxyFromInput(input)
	if strings.TrimSpace(proxy.ID) == "" {
		proxy.ID = "validation-only"
	}
	if err := proxy.Validate(); err != nil {
		return contracterr.Validation("validation failed", map[string]string{"proxy": err.Error()})
	}
	return nil
}

func (service *LocalService) systemProxy(ctx context.Context, proxyID string) (domain.Proxy, error) {
	if service == nil || service.Store == nil {
		return domain.Proxy{}, errors.New("store is required")
	}
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return domain.Proxy{}, err
	}
	if !systemclient.IsSystemProxy(proxy) {
		return domain.Proxy{}, &contracterr.Error{Code: contracterr.CodeForbidden, Message: "proxy is not a server-local proxy"}
	}
	return proxy, nil
}

func localProxyFromInput(input localproxy.LocalProxyInput) domain.Proxy {
	return domain.Proxy{ID: input.ID, UserID: systemclient.UserID, ClientID: systemclient.ClientID, Name: input.Name, Type: input.Type, Status: domain.ProxyEnabled, EntryBindHost: input.EntryBindHost, EntryPort: input.EntryPort, TargetHost: input.TargetHost, TargetPort: input.TargetPort, Description: input.Description}
}

var _ LocalAdministrationFacade = (*LocalService)(nil)
var _ localproxy.LocalProxyFacade = (*LocalService)(nil)

func (service *LocalService) Create(ctx context.Context, actorID string, input localproxy.LocalProxyInput) (domain.Proxy, error) {
	return service.CreateLocalProxy(ctx, actorID, input)
}

func (service *LocalService) Update(ctx context.Context, actorID string, input localproxy.LocalProxyInput) (domain.Proxy, error) {
	return service.UpdateLocalProxy(ctx, actorID, input)
}

func (service *LocalService) Enable(ctx context.Context, actorID string, proxyID string) error {
	return service.EnableLocalProxy(ctx, actorID, proxyID)
}

func (service *LocalService) Disable(ctx context.Context, actorID string, proxyID string) error {
	return service.DisableLocalProxy(ctx, actorID, proxyID)
}

func (service *LocalService) Delete(ctx context.Context, actorID string, proxyID string) error {
	return service.DeleteLocalProxy(ctx, actorID, proxyID)
}
