package adminquery

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type Service struct {
	Store    store.Store
	Sessions *session.Manager
	Stats    *stats.Memory
}

type DashboardSummary struct {
	OnlineClientCount        int
	EnabledProxyCount        int
	ActiveTCPConnectionCount int64
	CumulativeUploadBytes    int64
	CumulativeDownloadBytes  int64
	CumulativeTCPErrorCount  int64
	CumulativeUDPErrorCount  int64
	CumulativeHTTPErrorCount int64
}

type PageInput struct {
	Page     int
	PageSize int
}

type SortInput struct {
	Field     string
	Direction string
}

type PageInfo struct {
	Page       int
	PageSize   int
	TotalCount int
	TotalPages int
	HasNext    bool
	HasPrev    bool
}

type UserFilter struct {
	Query  string
	Role   string
	Status string
}

type ClientFilter struct {
	Query  string
	UserID string
	Status string
	Online *bool
}

type ProxyFilter struct {
	Query    string
	UserID   string
	ClientID string
	Type     string
	Status   string
}

type CertificateFilter struct {
	Query  string
	Status string
}

type AuditFilter struct {
	Query        string
	ActorType    string
	ActorID      string
	ResourceType string
	Action       string
	Result       string
}

type UserListInput struct {
	Page   PageInput
	Filter UserFilter
	Sort   SortInput
}

type ClientListInput struct {
	Page   PageInput
	Filter ClientFilter
	Sort   SortInput
}

type ProxyListInput struct {
	Page   PageInput
	Filter ProxyFilter
	Sort   SortInput
}

type CertificateListInput struct {
	Page   PageInput
	Filter CertificateFilter
	Sort   SortInput
}

type AuditListInput struct {
	Page   PageInput
	Filter AuditFilter
	Sort   SortInput
}

type UserListItem struct {
	ID              string
	Username        string
	Role            domain.Role
	Status          domain.UserStatus
	ClientCount     int
	ProxyCount      int
	UploadBytes     int64
	DownloadBytes   int64
	LastActivityAt  *time.Time
	HasPasswordHash bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type UserDetail struct {
	UserListItem
}

type UserPage struct {
	Items     []UserListItem
	TotalCount int
	PageInfo  PageInfo
	Filter    UserFilter
	Sort      SortInput
}

type ClientRuntime struct {
	Online        bool
	Protocol      domain.Protocol
	ConnectedAt   *time.Time
	LastHeartbeat *time.Time
	ConfigVersion int64
	ActiveProxies int
	ActiveStreams int
	UploadBytes   int64
	DownloadBytes int64
	ErrorSummary  string
}

type ClientListItem struct {
	ID            string
	UserID        string
	Name          string
	Status        domain.ClientStatus
	Version       int64
	LastOnlineAt  *time.Time
	LastOfflineAt *time.Time
	Runtime       ClientRuntime
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ClientDetail struct {
	ID             string
	UserID         string
	Name           string
	Status         domain.ClientStatus
	Version        int64
	LastOnlineAt   *time.Time
	LastOfflineAt  *time.Time
	Runtime        ClientRuntime
	ManagedProxies []ProxySummary
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ClientPage struct {
	Items      []ClientListItem
	TotalCount int
	PageInfo   PageInfo
	Filter     ClientFilter
	Sort       SortInput
}

type ManagedCertificateSummary struct {
	ProxyID       string
	CertificateID string
	Host          string
	Status        domain.CertificateStatus
	NotAfter      *time.Time
	LastIssuedAt  *time.Time
	LastRenewedAt *time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ManagedCertificatePage struct {
	Items      []ManagedCertificateSummary
	TotalCount int
	PageInfo   PageInfo
	Filter     CertificateFilter
	Sort       SortInput
}

type ProxyTypeConfig struct {
	EntryHost  string
	EntryPort  int
	TargetHost string
	TargetPort int
}

type ProxySummary struct {
	ID                   string
	Name                 string
	Type                 domain.ProxyType
	Status               domain.ProxyStatus
	RuntimeStatus        domain.ProxyStatus
	EntryHost            string
	EntryPort            int
	TargetHost           string
	TargetPort           int
	ActiveTCPConnections int64
}

type ProxyListItem struct {
	ID                   string
	UserID               string
	ClientID             string
	Name                 string
	Type                 domain.ProxyType
	Status               domain.ProxyStatus
	Description          string
	RuntimeStatus        domain.ProxyStatus
	ActiveTCPConnections int64
	UploadBytes          int64
	DownloadBytes        int64
	TCPErrorCount        int64
	UDPErrorCount        int64
	HTTPErrorCount       int64
	Config               ProxyTypeConfig
	Certificate          *ManagedCertificateSummary
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ProxyDetail struct {
	ID                   string
	UserID               string
	ClientID             string
	Name                 string
	Type                 domain.ProxyType
	Status               domain.ProxyStatus
	Description          string
	RuntimeStatus        domain.ProxyStatus
	ActiveTCPConnections int64
	UploadBytes          int64
	DownloadBytes        int64
	TCPErrorCount        int64
	UDPErrorCount        int64
	HTTPErrorCount       int64
	Config               ProxyTypeConfig
	Certificate          *ManagedCertificateSummary
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ProxyPage struct {
	Items      []ProxyListItem
	TotalCount int
	PageInfo   PageInfo
	Filter     ProxyFilter
	Sort       SortInput
}

type AuditListItem struct {
	ID           string
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	CreatedAt    time.Time
}

type AuditPage struct {
	Items      []AuditListItem
	TotalCount int
	PageInfo   PageInfo
	Filter     AuditFilter
	Sort       SortInput
}

func (service Service) DashboardSummary(ctx context.Context) (DashboardSummary, error) {
	if service.Store == nil {
		return DashboardSummary{}, nil
	}
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return DashboardSummary{}, err
	}
	statsByProxy := service.statsByProxy()
	summary := DashboardSummary{OnlineClientCount: len(service.latestSessions())}
	for _, proxy := range proxies {
		if proxy.Status == domain.ProxyEnabled || proxy.Status == domain.ProxyOnline {
			summary.EnabledProxyCount++
		}
		proxyStats := statsByProxy[proxy.ID]
		summary.ActiveTCPConnectionCount += proxyStats.TCPCurrentConnections
		summary.CumulativeUploadBytes += proxyStats.TCPUploadBytes + proxyStats.UDPUploadBytes + proxyStats.HTTPUploadBytes
		summary.CumulativeDownloadBytes += proxyStats.TCPDownloadBytes + proxyStats.UDPDownloadBytes + proxyStats.HTTPDownloadBytes
		summary.CumulativeTCPErrorCount += proxyStats.TCPErrors
		summary.CumulativeUDPErrorCount += proxyStats.UDPErrors
		summary.CumulativeHTTPErrorCount += proxyStats.HTTPErrors
	}
	return summary, nil
}

func (service Service) ListUsers(ctx context.Context, input UserListInput) (UserPage, error) {
	users, err := service.Store.Users().List(ctx)
	if err != nil {
		return UserPage{}, err
	}
	clients, err := service.Store.Clients().List(ctx)
	if err != nil {
		return UserPage{}, err
	}
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return UserPage{}, err
	}
	statsByProxy := service.statsByProxy()
	clientCountByUser := make(map[string]int)
	for _, client := range clients {
		clientCountByUser[client.UserID]++
	}
	proxyCountByUser := make(map[string]int)
	uploadByUser := make(map[string]int64)
	downloadByUser := make(map[string]int64)
	lastActivityByUser := make(map[string]*time.Time)
	for _, proxy := range proxies {
		proxyCountByUser[proxy.UserID]++
		proxyStats := statsByProxy[proxy.ID]
		uploadByUser[proxy.UserID] += proxyStats.TCPUploadBytes + proxyStats.UDPUploadBytes + proxyStats.HTTPUploadBytes
		downloadByUser[proxy.UserID] += proxyStats.TCPDownloadBytes + proxyStats.UDPDownloadBytes + proxyStats.HTTPDownloadBytes
	}
	for _, client := range clients {
		mergeLastActivity(lastActivityByUser, client.UserID, client.LastOnlineAt)
		mergeLastActivity(lastActivityByUser, client.UserID, client.LastOfflineAt)
	}
	items := make([]UserListItem, 0, len(users))
	for _, user := range users {
		item := UserListItem{ID: user.ID, Username: user.Username, Role: user.Role, Status: user.Status, ClientCount: clientCountByUser[user.ID], ProxyCount: proxyCountByUser[user.ID], UploadBytes: uploadByUser[user.ID], DownloadBytes: downloadByUser[user.ID], LastActivityAt: lastActivityByUser[user.ID], HasPasswordHash: user.PasswordHash != "", CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
		if !matchesUserFilter(item, input.Filter) {
			continue
		}
		items = append(items, item)
	}
	sortUsers(items, normalizeSort(input.Sort, "username", "asc"))
	paged, info := pageSlice(items, input.Page)
	return UserPage{Items: paged, TotalCount: len(items), PageInfo: info, Filter: input.Filter, Sort: normalizeSort(input.Sort, "username", "asc")}, nil
}

func (service Service) UserDetail(ctx context.Context, userID string) (UserDetail, error) {
	user, err := service.Store.Users().ByID(ctx, userID)
	if err != nil {
		return UserDetail{}, err
	}
	items, err := service.ListUsers(ctx, UserListInput{Page: PageInput{Page: 1, PageSize: 1000000}})
	if err != nil {
		return UserDetail{}, err
	}
	for _, item := range items.Items {
		if item.ID == userID {
			item.CreatedAt = user.CreatedAt
			item.UpdatedAt = user.UpdatedAt
			return UserDetail{UserListItem: item}, nil
		}
	}
	return UserDetail{}, store.ErrNotFound
}

func (service Service) ListClients(ctx context.Context, input ClientListInput) (ClientPage, error) {
	clients, err := service.Store.Clients().List(ctx)
	if err != nil {
		return ClientPage{}, err
	}
	latestByClient := latestByClientID(service.latestSessions())
	items := make([]ClientListItem, 0, len(clients))
	for _, client := range clients {
		item := clientListItemFromDomain(client, latestByClient[client.ID])
		if !matchesClientFilter(item, input.Filter) {
			continue
		}
		items = append(items, item)
	}
	sortClients(items, normalizeSort(input.Sort, "name", "asc"))
	paged, info := pageSlice(items, input.Page)
	return ClientPage{Items: paged, TotalCount: len(items), PageInfo: info, Filter: input.Filter, Sort: normalizeSort(input.Sort, "name", "asc")}, nil
}

func (service Service) ClientDetail(ctx context.Context, clientID string) (ClientDetail, error) {
	client, err := service.Store.Clients().ByID(ctx, clientID)
	if err != nil {
		return ClientDetail{}, err
	}
	proxies, err := service.Store.Proxies().ByClientID(ctx, clientID)
	if err != nil {
		return ClientDetail{}, err
	}
	statsByProxy := service.statsByProxy()
	latestByClient := latestByClientID(service.latestSessions())
	managedProxies := make([]ProxySummary, 0, len(proxies))
	for _, proxy := range proxies {
		summary := proxySummaryFromDomain(proxy, latestByClient[proxy.ClientID], statsByProxy[proxy.ID])
		managedProxies = append(managedProxies, summary)
	}
	sort.Slice(managedProxies, func(i, j int) bool { return managedProxies[i].Name < managedProxies[j].Name })
	item := clientListItemFromDomain(client, latestByClient[clientID])
	return ClientDetail{ID: item.ID, UserID: item.UserID, Name: item.Name, Status: item.Status, Version: item.Version, LastOnlineAt: item.LastOnlineAt, LastOfflineAt: item.LastOfflineAt, Runtime: item.Runtime, ManagedProxies: managedProxies, CreatedAt: client.CreatedAt, UpdatedAt: client.UpdatedAt}, nil
}

func (service Service) ListProxies(ctx context.Context, input ProxyListInput) (ProxyPage, error) {
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return ProxyPage{}, err
	}
	statsByProxy := service.statsByProxy()
	latestByClient := latestByClientID(service.latestSessions())
	certificatesByProxy, err := service.certificatesByProxy(ctx)
	if err != nil {
		return ProxyPage{}, err
	}
	items := make([]ProxyListItem, 0, len(proxies))
	for _, proxy := range proxies {
		item := proxyListItemFromDomain(proxy, latestByClient[proxy.ClientID], statsByProxy[proxy.ID], certificatesByProxy[proxy.ID])
		if !matchesProxyFilter(item, input.Filter) {
			continue
		}
		items = append(items, item)
	}
	sortProxies(items, normalizeSort(input.Sort, "name", "asc"))
	paged, info := pageSlice(items, input.Page)
	return ProxyPage{Items: paged, TotalCount: len(items), PageInfo: info, Filter: input.Filter, Sort: normalizeSort(input.Sort, "name", "asc")}, nil
}

func (service Service) ProxyDetail(ctx context.Context, proxyID string) (ProxyDetail, error) {
	proxy, err := service.Store.Proxies().ByID(ctx, proxyID)
	if err != nil {
		return ProxyDetail{}, err
	}
	statsByProxy := service.statsByProxy()
	latestByClient := latestByClientID(service.latestSessions())
	certificatesByProxy, err := service.certificatesByProxy(ctx)
	if err != nil {
		return ProxyDetail{}, err
	}
	item := proxyListItemFromDomain(proxy, latestByClient[proxy.ClientID], statsByProxy[proxy.ID], certificatesByProxy[proxy.ID])
	return ProxyDetail{ID: item.ID, UserID: item.UserID, ClientID: item.ClientID, Name: item.Name, Type: item.Type, Status: item.Status, Description: item.Description, RuntimeStatus: item.RuntimeStatus, ActiveTCPConnections: item.ActiveTCPConnections, UploadBytes: item.UploadBytes, DownloadBytes: item.DownloadBytes, TCPErrorCount: item.TCPErrorCount, UDPErrorCount: item.UDPErrorCount, HTTPErrorCount: item.HTTPErrorCount, Config: item.Config, Certificate: item.Certificate, CreatedAt: proxy.CreatedAt, UpdatedAt: proxy.UpdatedAt}, nil
}

func (service Service) ListManagedCertificates(ctx context.Context, input CertificateListInput) (ManagedCertificatePage, error) {
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return ManagedCertificatePage{}, err
	}
	certificatesByProxy, err := service.certificatesByProxy(ctx)
	if err != nil {
		return ManagedCertificatePage{}, err
	}
	items := make([]ManagedCertificateSummary, 0)
	for _, proxy := range proxies {
		if proxy.Type != domain.ProxyHTTPS {
			continue
		}
		item, ok := certificatesByProxy[proxy.ID]
		if !ok {
			item = &ManagedCertificateSummary{ProxyID: proxy.ID, Host: proxy.EntryHost}
		}
		if !matchesCertificateFilter(*item, input.Filter) {
			continue
		}
		items = append(items, *item)
	}
	sortCertificates(items, normalizeSort(input.Sort, "host", "asc"))
	paged, info := pageSlice(items, input.Page)
	return ManagedCertificatePage{Items: paged, TotalCount: len(items), PageInfo: info, Filter: input.Filter, Sort: normalizeSort(input.Sort, "host", "asc")}, nil
}

func (service Service) ListRecentAuditEvents(ctx context.Context, input AuditListInput) (AuditPage, error) {
	limit := normalizePage(input.Page).PageSize * normalizePage(input.Page).Page
	if limit < 50 {
		limit = 50
	}
	events, err := service.Store.AuditEvents().ListRecent(ctx, limit)
	if err != nil {
		return AuditPage{}, err
	}
	items := make([]AuditListItem, 0, len(events))
	for _, event := range events {
		item := AuditListItem{ID: event.ID, ActorType: auditActorType(event.ActorUserID), ActorID: event.ActorUserID, ResourceType: event.ResourceType, ResourceID: event.ResourceID, Action: event.Action, Result: event.Result, CreatedAt: event.CreatedAt}
		if !matchesAuditFilter(item, input.Filter) {
			continue
		}
		items = append(items, item)
	}
	sortAudit(items, normalizeSort(input.Sort, "createdAt", "desc"))
	paged, info := pageSlice(items, input.Page)
	return AuditPage{Items: paged, TotalCount: len(items), PageInfo: info, Filter: input.Filter, Sort: normalizeSort(input.Sort, "createdAt", "desc")}, nil
}

func (service Service) latestSessions() []session.Session {
	if service.Sessions == nil {
		return nil
	}
	return service.Sessions.SnapshotLatest()
}

func (service Service) statsByProxy() map[string]stats.ProxyStats {
	byProxy := make(map[string]stats.ProxyStats)
	if service.Stats == nil {
		return byProxy
	}
	for _, proxyStats := range service.Stats.List() {
		byProxy[proxyStats.ProxyID] = proxyStats
	}
	return byProxy
}

func (service Service) certificatesByProxy(ctx context.Context) (map[string]*ManagedCertificateSummary, error) {
	byProxy := make(map[string]*ManagedCertificateSummary)
	if service.Store == nil {
		return byProxy, nil
	}
	certificates, err := service.Store.Certificates().List(ctx)
	if err != nil {
		return nil, err
	}
	for _, certificate := range certificates {
		summary := certificateSummary(certificate)
		byProxy[certificate.ProxyID] = &summary
	}
	return byProxy, nil
}

func mergeLastActivity(target map[string]*time.Time, userID string, value *time.Time) {
	if value == nil {
		return
	}
	existing := target[userID]
	if existing == nil || existing.Before(*value) {
		copy := *value
		target[userID] = &copy
	}
}

func latestByClientID(sessions []session.Session) map[string]session.Session {
	byClient := make(map[string]session.Session, len(sessions))
	for _, current := range sessions {
		byClient[current.ClientID] = current
	}
	return byClient
}

func clientListItemFromDomain(client domain.Client, runtimeSession session.Session) ClientListItem {
	item := ClientListItem{ID: client.ID, UserID: client.UserID, Name: client.Name, Status: client.Status, Version: client.Version, LastOnlineAt: client.LastOnlineAt, LastOfflineAt: client.LastOfflineAt, CreatedAt: client.CreatedAt, UpdatedAt: client.UpdatedAt}
	if runtimeSession.ID == "" {
		return item
	}
	item.Runtime = ClientRuntime{Online: true, Protocol: runtimeSession.Protocol, ConnectedAt: &runtimeSession.ConnectedAt, LastHeartbeat: &runtimeSession.LastHeartbeat, ConfigVersion: runtimeSession.ConfigVersion, ActiveProxies: runtimeSession.Stats.ActiveProxies, ActiveStreams: runtimeSession.Stats.ActiveStreams, UploadBytes: runtimeSession.Stats.UploadBytes, DownloadBytes: runtimeSession.Stats.DownloadBytes, ErrorSummary: runtimeSession.Stats.ErrorSummary}
	return item
}

func proxyListItemFromDomain(proxy domain.Proxy, runtimeSession session.Session, proxyStats stats.ProxyStats, certificate *ManagedCertificateSummary) ProxyListItem {
	runtimeStatus := proxy.Status
	if proxy.Status == domain.ProxyEnabled {
		if runtimeSession.ID == "" {
			runtimeStatus = domain.ProxyOffline
		} else {
			runtimeStatus = domain.ProxyOnline
		}
	}
	return ProxyListItem{ID: proxy.ID, UserID: proxy.UserID, ClientID: proxy.ClientID, Name: proxy.Name, Type: proxy.Type, Status: proxy.Status, Description: proxy.Description, RuntimeStatus: runtimeStatus, ActiveTCPConnections: proxyStats.TCPCurrentConnections, UploadBytes: proxyStats.TCPUploadBytes + proxyStats.UDPUploadBytes + proxyStats.HTTPUploadBytes, DownloadBytes: proxyStats.TCPDownloadBytes + proxyStats.UDPDownloadBytes + proxyStats.HTTPDownloadBytes, TCPErrorCount: proxyStats.TCPErrors, UDPErrorCount: proxyStats.UDPErrors, HTTPErrorCount: proxyStats.HTTPErrors, Config: ProxyTypeConfig{EntryHost: proxy.EntryHost, EntryPort: proxy.EntryPort, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort}, Certificate: certificate, CreatedAt: proxy.CreatedAt, UpdatedAt: proxy.UpdatedAt}
}

func proxySummaryFromDomain(proxy domain.Proxy, runtimeSession session.Session, proxyStats stats.ProxyStats) ProxySummary {
	runtimeStatus := proxy.Status
	if proxy.Status == domain.ProxyEnabled {
		if runtimeSession.ID == "" {
			runtimeStatus = domain.ProxyOffline
		} else {
			runtimeStatus = domain.ProxyOnline
		}
	}
	return ProxySummary{ID: proxy.ID, Name: proxy.Name, Type: proxy.Type, Status: proxy.Status, RuntimeStatus: runtimeStatus, EntryHost: proxy.EntryHost, EntryPort: proxy.EntryPort, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort, ActiveTCPConnections: proxyStats.TCPCurrentConnections}
}

func certificateSummary(certificate domain.ManagedCertificate) ManagedCertificateSummary {
	return ManagedCertificateSummary{ProxyID: certificate.ProxyID, CertificateID: certificate.ID, Host: certificate.Host, Status: certificate.Status, NotAfter: certificate.NotAfter, LastIssuedAt: certificate.LastIssuedAt, LastRenewedAt: certificate.LastRenewedAt, LastError: certificate.LastError, CreatedAt: certificate.CreatedAt, UpdatedAt: certificate.UpdatedAt}
}

func normalizePage(input PageInput) PageInput {
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = 25
	}
	if input.PageSize > 100 {
		input.PageSize = 100
	}
	return input
}

func normalizeSort(input SortInput, field string, direction string) SortInput {
	if strings.TrimSpace(input.Field) == "" {
		input.Field = field
	}
	if strings.TrimSpace(input.Direction) == "" {
		input.Direction = direction
	}
	input.Direction = strings.ToLower(input.Direction)
	if input.Direction != "desc" {
		input.Direction = "asc"
	}
	return input
}

func pageSlice[T any](items []T, input PageInput) ([]T, PageInfo) {
	input = normalizePage(input)
	totalCount := len(items)
	totalPages := totalCount / input.PageSize
	if totalCount%input.PageSize != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	if input.Page > totalPages {
		input.Page = totalPages
	}
	start := (input.Page - 1) * input.PageSize
	if start > totalCount {
		start = totalCount
	}
	end := start + input.PageSize
	if end > totalCount {
		end = totalCount
	}
	pageItems := append([]T(nil), items[start:end]...)
	return pageItems, PageInfo{Page: input.Page, PageSize: input.PageSize, TotalCount: totalCount, TotalPages: totalPages, HasNext: input.Page < totalPages, HasPrev: input.Page > 1}
}

func matchesUserFilter(item UserListItem, filter UserFilter) bool {
	if filter.Role != "" && string(item.Role) != filter.Role {
		return false
	}
	if filter.Status != "" && string(item.Status) != filter.Status {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	return query == "" || strings.Contains(strings.ToLower(item.Username), query) || strings.Contains(strings.ToLower(item.ID), query)
}

func matchesClientFilter(item ClientListItem, filter ClientFilter) bool {
	if filter.UserID != "" && item.UserID != filter.UserID {
		return false
	}
	if filter.Status != "" && string(item.Status) != filter.Status {
		return false
	}
	if filter.Online != nil && item.Runtime.Online != *filter.Online {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	return query == "" || strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.ID), query)
}

func matchesProxyFilter(item ProxyListItem, filter ProxyFilter) bool {
	if filter.UserID != "" && item.UserID != filter.UserID {
		return false
	}
	if filter.ClientID != "" && item.ClientID != filter.ClientID {
		return false
	}
	if filter.Type != "" && string(item.Type) != filter.Type {
		return false
	}
	if filter.Status != "" && string(item.Status) != filter.Status {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	return query == "" || strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.ID), query)
}

func matchesCertificateFilter(item ManagedCertificateSummary, filter CertificateFilter) bool {
	if filter.Status != "" && string(item.Status) != filter.Status {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	return query == "" || strings.Contains(strings.ToLower(item.Host), query) || strings.Contains(strings.ToLower(item.ProxyID), query)
}

func matchesAuditFilter(item AuditListItem, filter AuditFilter) bool {
	if filter.ActorType != "" && item.ActorType != filter.ActorType {
		return false
	}
	if filter.ActorID != "" && item.ActorID != filter.ActorID {
		return false
	}
	if filter.ResourceType != "" && item.ResourceType != filter.ResourceType {
		return false
	}
	if filter.Action != "" && item.Action != filter.Action {
		return false
	}
	if filter.Result != "" && item.Result != filter.Result {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	return query == "" || strings.Contains(strings.ToLower(item.ResourceID), query) || strings.Contains(strings.ToLower(item.Action), query) || strings.Contains(strings.ToLower(item.ActorID), query)
}

func sortUsers(items []UserListItem, sortInput SortInput) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		less := false
		switch sortInput.Field {
		case "createdAt":
			less = left.CreatedAt.Before(right.CreatedAt)
		case "clientCount":
			less = left.ClientCount < right.ClientCount
		default:
			less = left.Username < right.Username
		}
		if sortInput.Direction == "desc" {
			return !less
		}
		return less
	})
}

func sortClients(items []ClientListItem, sortInput SortInput) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		less := false
		switch sortInput.Field {
		case "status":
			less = left.Status < right.Status
		case "updatedAt":
			less = left.UpdatedAt.Before(right.UpdatedAt)
		default:
			less = left.Name < right.Name
		}
		if sortInput.Direction == "desc" {
			return !less
		}
		return less
	})
}

func sortProxies(items []ProxyListItem, sortInput SortInput) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		less := false
		switch sortInput.Field {
		case "type":
			less = left.Type < right.Type
		case "status":
			less = left.Status < right.Status
		case "updatedAt":
			less = left.UpdatedAt.Before(right.UpdatedAt)
		default:
			less = left.Name < right.Name
		}
		if sortInput.Direction == "desc" {
			return !less
		}
		return less
	})
}

func sortCertificates(items []ManagedCertificateSummary, sortInput SortInput) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		less := false
		switch sortInput.Field {
		case "status":
			less = left.Status < right.Status
		case "notAfter":
			less = timePtrBefore(left.NotAfter, right.NotAfter)
		default:
			less = left.Host < right.Host
		}
		if sortInput.Direction == "desc" {
			return !less
		}
		return less
	})
}

func sortAudit(items []AuditListItem, sortInput SortInput) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		less := false
		switch sortInput.Field {
		case "action":
			less = left.Action < right.Action
		default:
			less = left.CreatedAt.Before(right.CreatedAt)
		}
		if sortInput.Direction == "desc" {
			return !less
		}
		return less
	})
}

func timePtrBefore(left *time.Time, right *time.Time) bool {
	if left == nil {
		return right != nil
	}
	if right == nil {
		return false
	}
	return left.Before(*right)
}

func auditActorType(actorID string) string {
	if strings.TrimSpace(actorID) == "" || actorID == "system" {
		return "system"
	}
	return "admin"
}
