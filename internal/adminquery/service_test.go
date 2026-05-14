package adminquery

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServiceBuildsDashboardSummary(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	sessions := session.NewManager()
	registered, _, err := sessions.Register(session.RegisterInput{SessionID: "session-1", ClientID: "client-1", UserID: "user-1", Protocol: domain.ProtocolQUIC, ConfigVersion: 1})
	if err != nil {
		t.Fatalf("register session: %v", err)
	}
	_, err = sessions.Heartbeat(session.HeartbeatInput{SessionID: registered.ID, ConfigVersion: 1, Stats: session.HeartbeatStats{ActiveProxies: 1, ActiveStreams: 2, UploadBytes: 3, DownloadBytes: 4, ErrorSummary: "ok"}})
	if err != nil {
		t.Fatalf("heartbeat session: %v", err)
	}
	memory := stats.NewMemory()
	memory.RecordTCPStart("proxy-1")
	memory.RecordTCPEnd("proxy-1", 10, 20, true)
	memory.RecordUDP("proxy-2", 5, 6, true)
	memory.RecordHTTP("proxy-3", 200, 7, 8, true)

	service := Service{Store: db, Sessions: sessions, Stats: memory}
	summary, err := service.DashboardSummary(ctx)
	if err != nil {
		t.Fatalf("dashboard summary: %v", err)
	}
	if summary.OnlineClientCount != 1 || summary.EnabledProxyCount != 3 || summary.ActiveTCPConnectionCount != 0 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.CumulativeUploadBytes != 22 || summary.CumulativeDownloadBytes != 34 {
		t.Fatalf("unexpected summary bytes: %+v", summary)
	}
	if summary.CumulativeTCPErrorCount != 1 || summary.CumulativeUDPErrorCount != 1 || summary.CumulativeHTTPErrorCount != 1 {
		t.Fatalf("unexpected summary errors: %+v", summary)
	}
}

func TestServiceListsRecentAuditEvents(t *testing.T) {
	ctx := context.Background()
	db := openQueryTestStore(t)
	seedQueryTestData(t, ctx, db)
	now := time.Now().UTC()
	if err := db.AuditEvents().Create(ctx, domain.AuditEvent{ID: "audit-1", ActorUserID: "admin-1", ResourceType: "proxy", ResourceID: "proxy-1", Action: "create_proxy", Result: "success", CreatedAt: now.Add(-time.Minute)}); err != nil {
		t.Fatalf("create audit 1: %v", err)
	}
	if err := db.AuditEvents().Create(ctx, domain.AuditEvent{ID: "audit-2", ActorUserID: "admin-1", ResourceType: "user", ResourceID: "user-1", Action: "disable_user", Result: "success", CreatedAt: now}); err != nil {
		t.Fatalf("create audit 2: %v", err)
	}
	service := Service{Store: db}
	events, err := service.ListRecentAuditEvents(ctx, 10)
	if err != nil {
		t.Fatalf("list recent audit: %v", err)
	}
	if len(events) != 2 || events[0].ID != "audit-2" || events[1].ID != "audit-1" {
		t.Fatalf("unexpected audit ordering: %+v", events)
	}
}

func openQueryTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "query.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedQueryTestData(t *testing.T, ctx context.Context, db *sqlite.Store) {
	t.Helper()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOnline, CredentialHash: domain.HashCredential("secret")}
	proxies := []domain.Proxy{
		{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "tcp", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22},
		{ID: "proxy-2", UserID: user.ID, ClientID: client.ID, Name: "udp", Type: domain.ProxyUDP, Status: domain.ProxyEnabled, EntryPort: 10053, TargetHost: "127.0.0.1", TargetPort: 53},
		{ID: "proxy-3", UserID: user.ID, ClientID: client.ID, Name: "http", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080},
	}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	for _, proxy := range proxies {
		if err := db.Proxies().Create(ctx, proxy); err != nil {
			t.Fatalf("create proxy %s: %v", proxy.ID, err)
		}
	}
}
