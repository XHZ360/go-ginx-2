package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServerRequiresAndAcceptsBasicAuth(t *testing.T) {
	server := startAdminTestServer(t)
	response, err := http.Get("http://" + server.Addr().String() + "/")
	if err != nil {
		t.Fatalf("get unauthenticated page: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, "http://"+server.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("admin", "wrong")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get invalid auth page: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 invalid auth, got %d", response.StatusCode)
	}

	request, err = http.NewRequest(http.MethodGet, "http://"+server.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("admin", "secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get authenticated page: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "Dashboard") {
		t.Fatalf("unexpected authenticated page status=%d body=%q", response.StatusCode, string(body))
	}
}

func TestServerGraphQLQueriesAndMutations(t *testing.T) {
	server := startAdminTestServer(t)
	queryResult := postGraphQL(t, server.Addr().String(), "admin", "secret", `query { dashboardSummary { onlineClientCount enabledProxyCount cumulativeUploadBytes cumulativeDownloadBytes } auditEvents(limit: 10) { action result } }`)
	data := queryResult["data"].(map[string]any)
	dashboard := data["dashboardSummary"].(map[string]any)
	if dashboard["onlineClientCount"].(float64) != 1 || dashboard["enabledProxyCount"].(float64) != 1 {
		t.Fatalf("unexpected dashboard: %+v", dashboard)
	}
	if dashboard["cumulativeUploadBytes"].(float64) != 10 || dashboard["cumulativeDownloadBytes"].(float64) != 20 {
		t.Fatalf("unexpected dashboard bytes: %+v", dashboard)
	}

	mutationResult := postGraphQL(t, server.Addr().String(), "admin", "secret", `mutation { createUser(username: "bob", password: "secret-2", role: "user") { id username hasPasswordHash } }`)
	mutationData := mutationResult["data"].(map[string]any)["createUser"].(map[string]any)
	if mutationData["username"].(string) != "bob" || mutationData["hasPasswordHash"].(bool) != true {
		t.Fatalf("unexpected create user mutation: %+v", mutationData)
	}

	auditResult := postGraphQL(t, server.Addr().String(), "admin", "secret", `query { auditEvents(limit: 10) { action result } }`)
	auditItems := auditResult["data"].(map[string]any)["auditEvents"].([]any)
	if len(auditItems) == 0 {
		t.Fatal("expected audit events after mutation")
	}
	firstAudit := auditItems[0].(map[string]any)
	if firstAudit["action"].(string) != "create_user" || firstAudit["result"].(string) != "success" {
		t.Fatalf("unexpected audit event: %+v", firstAudit)
	}
}

func startAdminTestServer(t *testing.T) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, sessions, memory := adminAPITestRuntime(t)
	credentialsFile := writeAdminCredentials(t)
	server, err := Listen(Entry{ListenAddress: "127.0.0.1:0", AdminCredentialsFile: credentialsFile, Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}})
	if err != nil {
		t.Fatalf("listen admin server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(ctx) }()
	return server
}

func adminAPITestRuntime(t *testing.T) (*sqlite.Store, *session.Manager, *stats.Memory) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "adminapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOnline, CredentialHash: domain.HashCredential("secret")}
	proxy := domain.Proxy{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "web", Type: domain.ProxyHTTP, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	sessions := session.NewManager()
	registered, _, err := sessions.Register(session.RegisterInput{SessionID: "session-1", ClientID: client.ID, UserID: user.ID, Protocol: domain.ProtocolQUIC, ConfigVersion: 1})
	if err != nil {
		t.Fatalf("register session: %v", err)
	}
	if _, err := sessions.Heartbeat(session.HeartbeatInput{SessionID: registered.ID, ConfigVersion: 1, Stats: session.HeartbeatStats{ActiveProxies: 1, ActiveStreams: 1}}); err != nil {
		t.Fatalf("heartbeat session: %v", err)
	}
	memory := stats.NewMemory()
	memory.RecordTCPStart(proxy.ID)
	memory.RecordTCPEnd(proxy.ID, 10, 20, false)
	if err := db.AuditEvents().Create(ctx, domain.AuditEvent{ID: "audit-1", ActorUserID: "admin", ResourceType: "proxy", ResourceID: proxy.ID, Action: "create_proxy", Result: "success", CreatedAt: time.Now().UTC().Add(-time.Minute)}); err != nil {
		t.Fatalf("create audit: %v", err)
	}
	return db, sessions, memory
}

func writeAdminCredentials(t *testing.T) string {
	t.Helper()
	hash, err := domain.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "admins.json")
	content, err := json.Marshal(map[string]any{"administrators": []map[string]string{{"username": "admin", "password_hash": hash}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func postGraphQL(t *testing.T, addr string, username string, password string, query string) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, "http://"+addr+"/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(username, password)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoded := make(map[string]any)
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected graphQL status %d: %+v", response.StatusCode, decoded)
	}
	if errors, ok := decoded["errors"]; ok {
		t.Fatalf("graphql returned errors: %+v", errors)
	}
	return decoded
}
