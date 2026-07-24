package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type CreateDomainInput struct {
	ID            string
	UserID        string
	Host          string
	CertificateID string
	ActorID       string
}

type UpdateDomainInput struct {
	ID               string
	Host             string
	CertificateID    string
	CertificateIDSet bool
	ActorID          string
}

type CreateDomainEntryInput struct {
	ID       string
	DomainID string
	Protocol domain.DomainEntryProtocol
	BindHost string
	Port     int
	ActorID  string
}

type UpdateDomainEntryInput struct {
	ID       string
	BindHost string
	Port     int
	Status   domain.DomainEntryStatus
	ActorID  string
}

func (service *DomainService) CreateDomain(ctx context.Context, input CreateDomainInput) (domain.Domain, error) {
	if service.Store == nil {
		return domain.Domain{}, errors.New("store is required")
	}
	fields := map[string]string{}
	if strings.TrimSpace(input.UserID) == "" {
		fields["userId"] = "user id is required"
	}
	host := domain.NormalizeRouteHost(input.Host)
	if host == "" {
		fields["host"] = "domain host is required"
	}
	if len(fields) > 0 {
		return domain.Domain{}, contracterr.Validation("validation failed", fields)
	}
	if _, err := service.Store.Users().ByID(ctx, input.UserID); err != nil {
		return domain.Domain{}, err
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("domain")
	}
	if input.CertificateID != "" {
		if err := service.Binding.ValidateBinding(ctx, input.CertificateID, host, input.ID); err != nil {
			return domain.Domain{}, err
		}
	}
	value := domain.Domain{
		ID:            input.ID,
		UserID:        input.UserID,
		Host:          host,
		CertificateID: input.CertificateID,
		Status:        domain.DomainEnabled,
	}
	if err := service.Store.Domains().Create(ctx, value); err != nil {
		return domain.Domain{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Domains().Delete(ctx, value.ID)
		_ = service.Admission.ReconcileListeners(ctx)
		return domain.Domain{}, err
	}
	return value, service.Audit.Record(ctx, input.ActorID, "domain", value.ID, "create_domain")
}

func (service *DomainService) UpdateDomain(ctx context.Context, input UpdateDomainInput) (domain.Domain, error) {
	if service.Store == nil {
		return domain.Domain{}, errors.New("store is required")
	}
	existing, err := service.Store.Domains().ByID(ctx, input.ID)
	if err != nil {
		return domain.Domain{}, err
	}
	previous := existing
	hostChanged := false
	if strings.TrimSpace(input.Host) != "" {
		nextHost := domain.NormalizeRouteHost(input.Host)
		hostChanged = nextHost != existing.Host
		existing.Host = nextHost
	}
	if input.CertificateIDSet {
		if input.CertificateID == "" {
			existing.CertificateID = ""
		} else {
			existing.CertificateID = input.CertificateID
		}
	}
	if existing.CertificateID != "" {
		if err := service.Binding.ValidateBinding(ctx, existing.CertificateID, existing.Host, existing.ID); err != nil {
			return domain.Domain{}, err
		}
	}
	if err := existing.Validate(); err != nil {
		return domain.Domain{}, contracterr.Validation("validation failed", map[string]string{"domain": err.Error()})
	}
	if err := service.Store.Domains().Update(ctx, existing); err != nil {
		return domain.Domain{}, err
	}
	if hostChanged {
		if err := service.Access.RevokeForDomain(ctx, existing.ID); err != nil {
			_ = service.Store.Domains().Update(ctx, previous)
			return domain.Domain{}, err
		}
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.Domains().Update(ctx, previous)
		_ = service.Admission.ReconcileListeners(ctx)
		return domain.Domain{}, err
	}
	return existing, service.Audit.Record(ctx, input.ActorID, "domain", existing.ID, "update_domain")
}

func (service *DomainService) EnableDomain(ctx context.Context, domainID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := service.Store.Domains().SetStatus(ctx, domainID, domain.DomainEnabled); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "domain", domainID, "enable_domain")
}

func (service *DomainService) DisableDomain(ctx context.Context, domainID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := service.Store.Domains().SetStatus(ctx, domainID, domain.DomainDisabled); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "domain", domainID, "disable_domain")
}

func (service *DomainService) DeleteDomain(ctx context.Context, domainID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	webDomain, err := service.Store.Domains().ByID(ctx, domainID)
	if err != nil {
		return err
	}
	if webDomain.Status != domain.DomainDisabled {
		return contracterr.Conflict("domain must be disabled before delete", nil)
	}
	proxies, err := service.Store.Proxies().ByDomainID(ctx, domainID)
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		if proxy.Status == domain.ProxyEnabled {
			return contracterr.Conflict("domain has enabled proxies; disable or move proxies before deleting the domain", nil)
		}
	}
	if err := service.Store.DomainEntries().DeleteByDomainID(ctx, domainID); err != nil {
		return err
	}
	if err := service.Store.Domains().Delete(ctx, domainID); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "domain", domainID, "delete_domain")
}

func (service *DomainService) CreateDomainEntry(ctx context.Context, input CreateDomainEntryInput) (domain.DomainEntry, error) {
	if service.Store == nil {
		return domain.DomainEntry{}, errors.New("store is required")
	}
	webDomain, err := service.Store.Domains().ByID(ctx, input.DomainID)
	if err != nil {
		return domain.DomainEntry{}, err
	}
	if !input.Protocol.Valid() {
		return domain.DomainEntry{}, contracterr.Validation("validation failed", map[string]string{"protocol": "domain entry protocol is invalid"})
	}
	if input.Protocol == domain.DomainEntryHTTPS && strings.TrimSpace(webDomain.CertificateID) == "" {
		return domain.DomainEntry{}, contracterr.Conflict("https domain entry requires a bound certificate", nil)
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("dentry")
	}
	entry := domain.DomainEntry{
		ID:       input.ID,
		DomainID: input.DomainID,
		Protocol: input.Protocol,
		BindHost: domain.NormalizeBindHost(input.BindHost),
		Port:     input.Port,
		Status:   domain.DomainEntryEnabled,
	}
	if err := entry.Validate(); err != nil {
		return domain.DomainEntry{}, contracterr.Validation("validation failed", map[string]string{"entry": err.Error()})
	}
	if err := service.Store.DomainEntries().Create(ctx, entry); err != nil {
		return domain.DomainEntry{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.DomainEntries().Delete(ctx, entry.ID)
		_ = service.Admission.ReconcileListeners(ctx)
		return domain.DomainEntry{}, err
	}
	return entry, service.Audit.Record(ctx, input.ActorID, "domain_entry", entry.ID, "create_domain_entry")
}

func (service *DomainService) UpdateDomainEntry(ctx context.Context, input UpdateDomainEntryInput) (domain.DomainEntry, error) {
	if service.Store == nil {
		return domain.DomainEntry{}, errors.New("store is required")
	}
	existing, err := service.Store.DomainEntries().ByID(ctx, input.ID)
	if err != nil {
		return domain.DomainEntry{}, err
	}
	previous := existing
	if input.BindHost != "" || input.Port != 0 {
		existing.BindHost = domain.NormalizeBindHost(input.BindHost)
		if input.Port != 0 {
			existing.Port = input.Port
		}
	}
	if input.Status != "" {
		existing.Status = input.Status
	}
	if existing.Protocol == domain.DomainEntryHTTPS && existing.Status == domain.DomainEntryEnabled {
		webDomain, err := service.Store.Domains().ByID(ctx, existing.DomainID)
		if err != nil {
			return domain.DomainEntry{}, err
		}
		if strings.TrimSpace(webDomain.CertificateID) == "" {
			return domain.DomainEntry{}, contracterr.Conflict("https domain entry requires a bound certificate", nil)
		}
	}
	if err := existing.Validate(); err != nil {
		return domain.DomainEntry{}, contracterr.Validation("validation failed", map[string]string{"entry": err.Error()})
	}
	if err := service.Store.DomainEntries().Update(ctx, existing); err != nil {
		return domain.DomainEntry{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		_ = service.Store.DomainEntries().Update(ctx, previous)
		_ = service.Admission.ReconcileListeners(ctx)
		return domain.DomainEntry{}, err
	}
	return existing, service.Audit.Record(ctx, input.ActorID, "domain_entry", existing.ID, "update_domain_entry")
}

func (service *DomainService) DeleteDomainEntry(ctx context.Context, entryID string, actorID string) error {
	if service.Store == nil {
		return errors.New("store is required")
	}
	if err := service.Store.DomainEntries().Delete(ctx, entryID); err != nil {
		return err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return err
	}
	return service.Audit.Record(ctx, actorID, "domain_entry", entryID, "delete_domain_entry")
}

func (service *DomainService) BindDomainCertificate(ctx context.Context, domainID string, certificateID string, actorID string) (domain.Domain, error) {
	if service.Store == nil {
		return domain.Domain{}, errors.New("store is required")
	}
	webDomain, err := service.Store.Domains().ByID(ctx, domainID)
	if err != nil {
		return domain.Domain{}, err
	}
	if err := service.Binding.ValidateBinding(ctx, certificateID, webDomain.Host, webDomain.ID); err != nil {
		return domain.Domain{}, err
	}
	webDomain.CertificateID = certificateID
	if err := service.Store.Domains().Update(ctx, webDomain); err != nil {
		return domain.Domain{}, err
	}
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return domain.Domain{}, err
	}
	return webDomain, service.Audit.Record(ctx, actorID, "domain", webDomain.ID, "bind_certificate")
}

func (service *DomainService) UnbindDomainCertificate(ctx context.Context, domainID string, actorID string) (domain.Domain, error) {
	if service.Store == nil {
		return domain.Domain{}, errors.New("store is required")
	}
	webDomain, err := service.Store.Domains().ByID(ctx, domainID)
	if err != nil {
		return domain.Domain{}, err
	}
	if strings.TrimSpace(webDomain.CertificateID) == "" {
		return webDomain, nil
	}
	if err := service.Binding.UnbindDomain(ctx, webDomain); err != nil {
		return domain.Domain{}, err
	}
	webDomain.CertificateID = ""
	if err := service.Admission.ReconcileListeners(ctx); err != nil {
		return domain.Domain{}, err
	}
	return webDomain, service.Audit.Record(ctx, actorID, "domain", webDomain.ID, "unbind_certificate")
}

// Ensure CreateProxy accepts web domain+path without relying only on legacy http conversion.
