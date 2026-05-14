package adminquery

import (
	"context"
	"sort"
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
}

type UserDetail struct {
	UserListItem
	CreatedAt time.Time
	UpdatedAt time.Time
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
}

type ClientDetail struct {
	ClientListItem
	CreatedAt time.Time
	UpdatedAt time.Time
	ProxyIDs  []string
}

type ManagedCertificateSummary struct {
	ProxyID       string
	CertificateID string
	Host          string
	Status        domain.CertificateStatus
	CertFile      string
	KeyFile       string
	NotAfter      *time.Time
	LastIssuedAt  *time.Time
	LastRenewedAt *time.Time
	LastError     string
}

type ProxyListItem struct {
	ID                   string
	UserID               string
	ClientID             string
	Name                 string
	Type                 domain.ProxyType
	Status               domain.ProxyStatus
	EntryHost            string
	EntryPort            int
	TargetHost           string
	TargetPort           int
	Description          string
	CertFile             string
	KeyFile              string
	RuntimeStatus        domain.ProxyStatus
	ActiveTCPConnections int64
	UploadBytes          int64
	DownloadBytes        int64
	TCPErrorCount        int64
	UDPErrorCount        int64
	HTTPErrorCount       int64
	Certificate          *ManagedCertificateSummary
}

type ProxyDetail struct {
	ProxyListItem
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AuditListItem struct {
	ID           string
	ActorUserID  string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	CreatedAt    time.Time
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

func (service Service) ListUsers(ctx context.Context) ([]UserListItem, error) {
	users, err := service.Store.Users().List(ctx)
	if err != nil {
		return nil, err
	}
	clients, err := service.Store.Clients().List(ctx)
	if err != nil {
		return nil, err
	}
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return nil, err
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
		items = append(items, UserListItem{ID: user.ID, Username: user.Username, Role: user.Role, Status: user.Status, ClientCount: clientCountByUser[user.ID], ProxyCount: proxyCountByUser[user.ID], UploadBytes: uploadByUser[user.ID], DownloadBytes: downloadByUser[user.ID], LastActivityAt: lastActivityByUser[user.ID], HasPasswordHash: user.PasswordHash != ""})
	}
	return items, nil
}

func (service Service) UserDetail(ctx context.Context, userID string) (UserDetail, error) {
	user, err := service.Store.Users().ByID(ctx, userID)
	if err != nil {
		return UserDetail{}, err
	}
	items, err := service.ListUsers(ctx)
	if err != nil {
		return UserDetail{}, err
	}
	for _, item := range items {
		if item.ID == userID {
			return UserDetail{UserListItem: item, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}, nil
		}
	}
	return UserDetail{}, store.ErrNotFound
}

func (service Service) ListClients(ctx context.Context) ([]ClientListItem, error) {
	clients, err := service.Store.Clients().List(ctx)
	if err != nil {
		return nil, err
	}
	latestByClient := latestByClientID(service.latestSessions())
	items := make([]ClientListItem, 0, len(clients))
	for _, client := range clients {
		items = append(items, clientListItemFromDomain(client, latestByClient[client.ID]))
	}
	return items, nil
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
	latestByClient := latestByClientID(service.latestSessions())
	proxyIDs := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		proxyIDs = append(proxyIDs, proxy.ID)
	}
	return ClientDetail{ClientListItem: clientListItemFromDomain(client, latestByClient[clientID]), CreatedAt: client.CreatedAt, UpdatedAt: client.UpdatedAt, ProxyIDs: proxyIDs}, nil
}

func (service Service) ListProxies(ctx context.Context) ([]ProxyListItem, error) {
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return nil, err
	}
	statsByProxy := service.statsByProxy()
	latestByClient := latestByClientID(service.latestSessions())
	certificatesByProxy, err := service.certificatesByProxy(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ProxyListItem, 0, len(proxies))
	for _, proxy := range proxies {
		items = append(items, proxyListItemFromDomain(proxy, latestByClient[proxy.ClientID], statsByProxy[proxy.ID], certificatesByProxy[proxy.ID]))
	}
	return items, nil
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
	return ProxyDetail{ProxyListItem: proxyListItemFromDomain(proxy, latestByClient[proxy.ClientID], statsByProxy[proxy.ID], certificatesByProxy[proxy.ID]), CreatedAt: proxy.CreatedAt, UpdatedAt: proxy.UpdatedAt}, nil
}

func (service Service) ListManagedCertificates(ctx context.Context) ([]ManagedCertificateSummary, error) {
	proxies, err := service.Store.Proxies().List(ctx)
	if err != nil {
		return nil, err
	}
	certificatesByProxy, err := service.certificatesByProxy(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ManagedCertificateSummary, 0)
	for _, proxy := range proxies {
		if proxy.Type != domain.ProxyHTTPS {
			continue
		}
		if certificate, ok := certificatesByProxy[proxy.ID]; ok {
			items = append(items, *certificate)
			continue
		}
		items = append(items, ManagedCertificateSummary{ProxyID: proxy.ID, Host: proxy.EntryHost})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Host < items[j].Host })
	return items, nil
}

func (service Service) ListRecentAuditEvents(ctx context.Context, limit int) ([]AuditListItem, error) {
	events, err := service.Store.AuditEvents().ListRecent(ctx, limit)
	if err != nil {
		return nil, err
	}
	items := make([]AuditListItem, 0, len(events))
	for _, event := range events {
		items = append(items, AuditListItem{ID: event.ID, ActorUserID: event.ActorUserID, ResourceType: event.ResourceType, ResourceID: event.ResourceID, Action: event.Action, Result: event.Result, CreatedAt: event.CreatedAt})
	}
	return items, nil
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
	item := ClientListItem{ID: client.ID, UserID: client.UserID, Name: client.Name, Status: client.Status, Version: client.Version, LastOnlineAt: client.LastOnlineAt, LastOfflineAt: client.LastOfflineAt}
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
	return ProxyListItem{ID: proxy.ID, UserID: proxy.UserID, ClientID: proxy.ClientID, Name: proxy.Name, Type: proxy.Type, Status: proxy.Status, EntryHost: proxy.EntryHost, EntryPort: proxy.EntryPort, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort, Description: proxy.Description, CertFile: proxy.CertFile, KeyFile: proxy.KeyFile, RuntimeStatus: runtimeStatus, ActiveTCPConnections: proxyStats.TCPCurrentConnections, UploadBytes: proxyStats.TCPUploadBytes + proxyStats.UDPUploadBytes + proxyStats.HTTPUploadBytes, DownloadBytes: proxyStats.TCPDownloadBytes + proxyStats.UDPDownloadBytes + proxyStats.HTTPDownloadBytes, TCPErrorCount: proxyStats.TCPErrors, UDPErrorCount: proxyStats.UDPErrors, HTTPErrorCount: proxyStats.HTTPErrors, Certificate: certificate}
}

func certificateSummary(certificate domain.ManagedCertificate) ManagedCertificateSummary {
	return ManagedCertificateSummary{ProxyID: certificate.ProxyID, CertificateID: certificate.ID, Host: certificate.Host, Status: certificate.Status, CertFile: certificate.CertFile, KeyFile: certificate.KeyFile, NotAfter: certificate.NotAfter, LastIssuedAt: certificate.LastIssuedAt, LastRenewedAt: certificate.LastRenewedAt, LastError: certificate.LastError}
}
