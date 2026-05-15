package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServerRemovedRoutesAndSessionBootstrap(t *testing.T) {
	server := startAdminTestServer(t, nil)

	response, err := http.Get("http://" + server.Addr().String() + "/")
	if err != nil {
		t.Fatalf("get removed root route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for removed root route, got %d", response.StatusCode)
	}

	response, err = http.Get("http://" + server.Addr().String() + "/users")
	if err != nil {
		t.Fatalf("get removed users route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for removed users route, got %d", response.StatusCode)
	}

	response, err = postJSON(http.DefaultClient, "http://"+server.Addr().String()+"/graphql", map[string]any{"query": `query { dashboardSummary { onlineClientCount } }`}, nil)
	if err != nil {
		t.Fatalf("post removed graphql route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for removed graphql route, got %d", response.StatusCode)
	}

	client := newAdminHTTPClient(t)
	bootstrap := readBootstrap(t, client, server.Addr().String())
	if bootstrap.Authenticated {
		t.Fatal("expected unauthenticated bootstrap without login")
	}

	response, err = postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/login", map[string]string{"username": "admin", "password": "wrong"}, nil)
	if err != nil {
		t.Fatalf("post invalid login: %v", err)
	}
	defer response.Body.Close()
	assertErrorCode(t, response, http.StatusUnauthorized, "UNAUTHENTICATED")

	login := loginAdmin(t, client, server.Addr().String(), "admin", "secret")
	if !login.Authenticated || login.Username != "admin" || login.CSRFToken == "" {
		t.Fatalf("unexpected login bootstrap: %+v", login)
	}

	bootstrap = readBootstrap(t, client, server.Addr().String())
	if !bootstrap.Authenticated || bootstrap.Username != "admin" || bootstrap.CSRFToken == "" {
		t.Fatalf("unexpected authenticated bootstrap: %+v", bootstrap)
	}
	if bootstrap.PollIntervalSecond != defaultPollSeconds {
		t.Fatalf("unexpected poll interval: %+v", bootstrap)
	}
}

func TestServerSessionGraphQLAndCSRF(t *testing.T) {
	server := startAdminTestServer(t, nil)
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	queryResult := postAdminGraphQL(t, client, server.Addr().String(), `query { dashboardSummary { onlineClientCount enabledProxyCount cumulativeUploadBytes cumulativeDownloadBytes } auditEvents(limit: 10) { action result } }`, "", http.StatusOK)
	data := queryResult["data"].(map[string]any)
	dashboard := data["dashboardSummary"].(map[string]any)
	if dashboard["onlineClientCount"].(float64) != 1 || dashboard["enabledProxyCount"].(float64) != 1 {
		t.Fatalf("unexpected dashboard: %+v", dashboard)
	}
	if dashboard["cumulativeUploadBytes"].(float64) != 10 || dashboard["cumulativeDownloadBytes"].(float64) != 20 {
		t.Fatalf("unexpected dashboard bytes: %+v", dashboard)
	}

	mutationResponse, err := postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/graphql", map[string]any{"query": `mutation { createUser(username: "bob", password: "secret-2", role: "user") { id username hasPasswordHash } }`}, nil)
	if err != nil {
		t.Fatalf("post graphql mutation without csrf: %v", err)
	}
	defer mutationResponse.Body.Close()
	assertErrorCode(t, mutationResponse, http.StatusForbidden, "INVALID_CSRF")

	mutationResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation { createUser(username: "bob", password: "secret-2", role: "user") { id username hasPasswordHash } }`, bootstrap.CSRFToken, http.StatusOK)
	mutationData := mutationResult["data"].(map[string]any)["createUser"].(map[string]any)
	if mutationData["username"].(string) != "bob" || mutationData["hasPasswordHash"].(bool) != true {
		t.Fatalf("unexpected create user mutation: %+v", mutationData)
	}

	auditResult := postAdminGraphQL(t, client, server.Addr().String(), `query { auditEvents(limit: 10) { action result } }`, "", http.StatusOK)
	auditItems := auditResult["data"].(map[string]any)["auditEvents"].([]any)
	if len(auditItems) == 0 {
		t.Fatal("expected audit events after mutation")
	}
	firstAudit := auditItems[0].(map[string]any)
	if firstAudit["action"].(string) != "create_user" || firstAudit["result"].(string) != "success" {
		t.Fatalf("unexpected audit event: %+v", firstAudit)
	}
}

func TestServerSessionExpiryAndLogout(t *testing.T) {
	now := time.Now().UTC()
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Now = func() time.Time { return now }
		entry.SessionIdleTimeout = time.Minute
		entry.SessionAbsoluteLifetime = 5 * time.Minute
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	response, err := postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/logout", map[string]any{}, map[string]string{adminCSRFHeader: bootstrap.CSRFToken})
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected logout status: %d", response.StatusCode)
	}
	loggedOut := decodeBootstrapResponse(t, response.Body)
	if loggedOut.Authenticated {
		t.Fatalf("expected logged out bootstrap, got %+v", loggedOut)
	}

	bootstrap = readBootstrap(t, client, server.Addr().String())
	if bootstrap.Authenticated {
		t.Fatal("expected unauthenticated bootstrap after logout")
	}

	bootstrap = loginAdmin(t, client, server.Addr().String(), "admin", "secret")
	now = now.Add(2 * time.Minute)
	bootstrap = readBootstrap(t, client, server.Addr().String())
	if bootstrap.Authenticated {
		t.Fatal("expected session to expire from bootstrap")
	}

	response, err = postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/graphql", map[string]any{"query": `query { dashboardSummary { onlineClientCount } }`}, nil)
	if err != nil {
		t.Fatalf("graphql after expiry: %v", err)
	}
	defer response.Body.Close()
	assertErrorCode(t, response, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func startAdminTestServer(t *testing.T, mutateEntry func(*Entry)) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, sessions, memory := adminAPITestRuntime(t)
	credentialsFile := writeAdminCredentials(t)
	entry := Entry{ListenAddress: "127.0.0.1:0", AdminCredentialsFile: credentialsFile, Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}}
	if mutateEntry != nil {
		mutateEntry(&entry)
	}
	server, err := Listen(entry)
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

func newAdminHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func loginAdmin(t *testing.T, client *http.Client, addr string, username string, password string) sessionBootstrapResponse {
	t.Helper()
	response, err := postJSON(client, "http://"+addr+adminAPIPrefix+"/login", map[string]string{"username": username, "password": password}, nil)
	if err != nil {
		t.Fatalf("post login: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected login status %d body=%s", response.StatusCode, string(body))
	}
	return decodeBootstrapResponse(t, response.Body)
}

func readBootstrap(t *testing.T, client *http.Client, addr string) sessionBootstrapResponse {
	t.Helper()
	response, err := client.Get("http://" + addr + adminAPIPrefix + "/session")
	if err != nil {
		t.Fatalf("get session bootstrap: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected bootstrap status %d body=%s", response.StatusCode, string(body))
	}
	return decodeBootstrapResponse(t, response.Body)
}

func postAdminGraphQL(t *testing.T, client *http.Client, addr string, query string, csrfToken string, expectedStatus int) map[string]any {
	t.Helper()
	headers := map[string]string{"Content-Type": "application/json"}
	if csrfToken != "" {
		headers[adminCSRFHeader] = csrfToken
	}
	response, err := postJSON(client, "http://"+addr+adminAPIPrefix+"/graphql", map[string]any{"query": query}, headers)
	if err != nil {
		t.Fatalf("post graphql: %v", err)
	}
	defer response.Body.Close()
	decoded := make(map[string]any)
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expectedStatus {
		t.Fatalf("unexpected graphql status %d: %+v", response.StatusCode, decoded)
	}
	if errors, ok := decoded["errors"]; ok {
		t.Fatalf("graphql returned errors: %+v", errors)
	}
	return decoded
}

func postJSON(client *http.Client, url string, payload any, headers map[string]string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return client.Do(request)
}

func assertErrorCode(t *testing.T, response *http.Response, expectedStatus int, expectedCode string) {
	t.Helper()
	var decoded apiErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.StatusCode != expectedStatus {
		t.Fatalf("unexpected status %d response=%+v", response.StatusCode, decoded)
	}
	if decoded.Error.Code != expectedCode {
		t.Fatalf("unexpected error code %q response=%+v", decoded.Error.Code, decoded)
	}
}

func decodeBootstrapResponse(t *testing.T, reader io.Reader) sessionBootstrapResponse {
	t.Helper()
	var response sessionBootstrapResponse
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}
