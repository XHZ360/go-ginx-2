package adminquery

import (
	"context"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type DomainFilter struct {
	Query  string
	UserID string
	Status string
}

type DomainListInput struct {
	Page   PageInput
	Filter DomainFilter
	Sort   SortInput
}

type DomainEntryItem struct {
	ID        string
	DomainID  string
	Protocol  domain.DomainEntryProtocol
	BindHost  string
	Port      int
	Status    domain.DomainEntryStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DomainListItem struct {
	ID              string
	UserID          string
	Host            string
	CertificateID   string
	Status          domain.DomainStatus
	ProxyCount      int
	HTTPEntryCount  int
	HTTPSEntryCount int
	Certificate     *ManagedCertificateSummary
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type DomainDetail struct {
	DomainListItem
	Entries []DomainEntryItem
	Proxies []ProxyListItem
}

type DomainPage struct {
	Items      []DomainListItem
	TotalCount int
	PageInfo   PageInfo
	Filter     DomainFilter
	Sort       SortInput
}

func (service Service) ListDomains(ctx context.Context, input DomainListInput) (DomainPage, error) {
	if service.Store == nil {
		return DomainPage{}, nil
	}
	domains, err := service.Store.Domains().List(ctx)
	if err != nil {
		return DomainPage{}, err
	}
	items := make([]DomainListItem, 0, len(domains))
	for _, webDomain := range domains {
		if !matchesDomainFilter(webDomain, input.Filter) {
			continue
		}
		item, err := service.domainListItem(ctx, webDomain)
		if err != nil {
			return DomainPage{}, err
		}
		items = append(items, item)
	}
	sortDomains(items, normalizeSort(input.Sort, "host", "asc"))
	paged, info := pageSlice(items, input.Page)
	return DomainPage{Items: paged, TotalCount: len(items), PageInfo: info, Filter: input.Filter, Sort: normalizeSort(input.Sort, "host", "asc")}, nil
}

func (service Service) DomainDetail(ctx context.Context, domainID string) (DomainDetail, error) {
	webDomain, err := service.Store.Domains().ByID(ctx, domainID)
	if err != nil {
		return DomainDetail{}, err
	}
	item, err := service.domainListItem(ctx, webDomain)
	if err != nil {
		return DomainDetail{}, err
	}
	entries, err := service.Store.DomainEntries().ListByDomainID(ctx, domainID)
	if err != nil {
		return DomainDetail{}, err
	}
	entryItems := make([]DomainEntryItem, 0, len(entries))
	for _, entry := range entries {
		entryItems = append(entryItems, DomainEntryItem{
			ID:        entry.ID,
			DomainID:  entry.DomainID,
			Protocol:  entry.Protocol,
			BindHost:  entry.BindHost,
			Port:      entry.Port,
			Status:    entry.Status,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.UpdatedAt,
		})
	}
	proxies, err := service.Store.Proxies().ByDomainID(ctx, domainID)
	if err != nil {
		return DomainDetail{}, err
	}
	statsByProxy := service.statsByProxy()
	latestByClient := latestByClientID(service.latestSessions())
	proxyItems := make([]ProxyListItem, 0, len(proxies))
	for _, proxy := range proxies {
		certificate, certErr := service.certificateByProxyID(ctx, proxy)
		if certErr != nil {
			return DomainDetail{}, certErr
		}
		proxyItems = append(proxyItems, proxyListItemFromDomain(proxy, latestByClient[proxy.ClientID], statsByProxy[proxy.ID], certificate))
	}
	return DomainDetail{DomainListItem: item, Entries: entryItems, Proxies: proxyItems}, nil
}

func (service Service) domainListItem(ctx context.Context, webDomain domain.Domain) (DomainListItem, error) {
	item := DomainListItem{
		ID:            webDomain.ID,
		UserID:        webDomain.UserID,
		Host:          webDomain.Host,
		CertificateID: webDomain.CertificateID,
		Status:        webDomain.Status,
		CreatedAt:     webDomain.CreatedAt,
		UpdatedAt:     webDomain.UpdatedAt,
	}
	proxies, err := service.Store.Proxies().ByDomainID(ctx, webDomain.ID)
	if err != nil {
		return DomainListItem{}, err
	}
	item.ProxyCount = len(proxies)
	entries, err := service.Store.DomainEntries().ListByDomainID(ctx, webDomain.ID)
	if err != nil {
		return DomainListItem{}, err
	}
	for _, entry := range entries {
		if entry.Protocol == domain.DomainEntryHTTP {
			item.HTTPEntryCount++
		}
		if entry.Protocol == domain.DomainEntryHTTPS {
			item.HTTPSEntryCount++
		}
	}
	if strings.TrimSpace(webDomain.CertificateID) != "" {
		certificate, certErr := service.Store.Certificates().ByID(ctx, webDomain.CertificateID)
		if certErr == nil {
			summary := service.managedCertificateSummary(certificate)
			service.applyCertificateReferenceFields(&summary, certificate, webDomain, true)
			item.Certificate = &summary
		}
	}
	return item, nil
}

func matchesDomainFilter(webDomain domain.Domain, filter DomainFilter) bool {
	if filter.UserID != "" && webDomain.UserID != filter.UserID {
		return false
	}
	if filter.Status != "" && string(webDomain.Status) != filter.Status {
		return false
	}
	if query := strings.ToLower(strings.TrimSpace(filter.Query)); query != "" {
		if !strings.Contains(strings.ToLower(webDomain.Host), query) && !strings.Contains(strings.ToLower(webDomain.ID), query) {
			return false
		}
	}
	return true
}

func sortDomains(items []DomainListItem, sortInput SortInput) {
	field := sortInput.Field
	asc := sortInput.Direction != "desc"
	less := func(i, j int) bool {
		switch field {
		case "status":
			if items[i].Status == items[j].Status {
				return items[i].Host < items[j].Host
			}
			return items[i].Status < items[j].Status
		case "createdAt":
			if items[i].CreatedAt.Equal(items[j].CreatedAt) {
				return items[i].Host < items[j].Host
			}
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		default:
			return items[i].Host < items[j].Host
		}
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if less(j, i) == asc {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}
