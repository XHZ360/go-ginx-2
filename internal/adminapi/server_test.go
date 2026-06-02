package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServerRemovedRoutesAndSessionBootstrap(t *testing.T) {
	server := startAdminTestServer(t, nil)

	response, err := http.Get("http://" + server.Addr().String() + "/")
	if err != nil {
		t.Fatalf("get frontend root route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected frontend for root route, got %d", response.StatusCode)
	}

	response, err = http.Get("http://" + server.Addr().String() + "/users")
	if err != nil {
		t.Fatalf("get frontend users route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected frontend for users route, got %d", response.StatusCode)
	}

	response, err = postJSON(http.DefaultClient, "http://"+server.Addr().String()+"/graphql", map[string]any{"query": `query { dashboard { onlineClientCount } }`}, nil)
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

func TestServerFrontendRoutesAndAssetsFromDefaultAdminUIDirectory(t *testing.T) {
	deploymentRoot := t.TempDir()
	writeAdminFrontendFixtureAt(t, filepath.Join(deploymentRoot, defaultAdminFrontendDir))
	setDefaultAdminExecutable(t, deploymentRoot)
	t.Chdir(t.TempDir())
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.AdminFrontendDir = ""
	})

	assertFrontendResponse(t, "http://"+server.Addr().String()+"/", "<title>Admin UI</title>", "text/html; charset=utf-8")
	assertFrontendResponse(t, "http://"+server.Addr().String()+"/dashboard", "<title>Admin UI</title>", "text/html; charset=utf-8")
	assertFrontendResponse(t, "http://"+server.Addr().String()+"/assets/app.js", "window.__adminFrontend = true;", "text/javascript; charset=utf-8")
}

func TestServerFrontendRoutesAndAssetsWhenConfigured(t *testing.T) {
	frontendDir := writeAdminFrontendFixture(t)
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.AdminFrontendDir = frontendDir
	})

	assertFrontendResponse(t, "http://"+server.Addr().String()+"/", "<title>Admin UI</title>", "text/html; charset=utf-8")
	assertFrontendResponse(t, "http://"+server.Addr().String()+"/login", "<title>Admin UI</title>", "text/html; charset=utf-8")
	assertFrontendResponse(t, "http://"+server.Addr().String()+"/users/user-1", "<title>Admin UI</title>", "text/html; charset=utf-8")
	assertFrontendResponse(t, "http://"+server.Addr().String()+"/assets/app.js", "window.__adminFrontend = true;", "text/javascript; charset=utf-8")

	response, err := http.Get("http://" + server.Addr().String() + "/assets/missing.js")
	if err != nil {
		t.Fatalf("get missing asset: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d", response.StatusCode)
	}

	response, err = postJSON(http.DefaultClient, "http://"+server.Addr().String()+"/api/admin/unknown", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("post unknown admin api route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown admin api route, got %d", response.StatusCode)
	}

	client := newAdminHTTPClient(t)
	bootstrap := readBootstrap(t, client, server.Addr().String())
	if bootstrap.Authenticated {
		t.Fatal("expected unauthenticated bootstrap without login")
	}
	login := loginAdmin(t, client, server.Addr().String(), "admin", "secret")
	if !login.Authenticated {
		t.Fatalf("expected authenticated login bootstrap, got %+v", login)
	}
	result := postAdminGraphQL(t, client, server.Addr().String(), `query { dashboard { onlineClientCount } }`, "", http.StatusOK)
	if result["data"].(map[string]any)["dashboard"].(map[string]any)["onlineClientCount"].(float64) != 1 {
		t.Fatalf("unexpected dashboard result with frontend configured: %+v", result)
	}
}

func TestLoadAdminFrontendDefaultDirectoryFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
		want  string
	}{
		{name: "missing directory", want: defaultAdminFrontendDir},
		{name: "not directory", setup: func(t *testing.T, workDir string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(workDir, defaultAdminFrontendDir), []byte("not a dir"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "must be a directory"},
		{name: "missing index", setup: func(t *testing.T, workDir string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(workDir, defaultAdminFrontendDir, "assets"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, want: "index.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deploymentRoot := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, deploymentRoot)
			}
			setDefaultAdminExecutable(t, deploymentRoot)
			t.Chdir(t.TempDir())

			_, err := loadAdminFrontend("")
			if err == nil {
				t.Fatal("expected default admin frontend load to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestServerFrontendRoutesStillRequireProtectedTransport(t *testing.T) {
	frontendDir := writeAdminFrontendFixture(t)
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.AdminFrontendDir = frontendDir
	})

	request := httptest.NewRequest(http.MethodGet, "http://admin.example.test/users", nil)
	request.RemoteAddr = "203.0.113.9:1234"
	response := httptest.NewRecorder()

	server.routeHandler(http.NewServeMux()).ServeHTTP(response, request)

	if response.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected protected transport rejection, got %d body=%s", response.Code, response.Body.String())
	}
	var decoded apiErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode protected transport response: %v", err)
	}
	if decoded.Error.Code != "PROTECTED_TRANSPORT_REQUIRED" {
		t.Fatalf("unexpected protected transport error: %+v", decoded)
	}
}

func TestServerAuthenticatesSQLiteAdministrators(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, sessions, memory := adminAPITestRuntime(t)
	hash, err := domain.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Users().Create(context.Background(), domain.User{ID: "admin-1", Username: "admin-sqlite", PasswordHash: hash, Role: domain.RoleAdmin, Status: domain.UserEnabled}); err != nil {
		t.Fatal(err)
	}
	ordinaryHash, err := domain.HashPassword("user-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Users().SetPassword(context.Background(), "user-1", ordinaryHash); err != nil {
		t.Fatal(err)
	}
	server, err := Listen(Entry{ListenAddress: "127.0.0.1:0", AdminFrontendDir: writeAdminFrontendFixture(t), Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}})
	if err != nil {
		t.Fatalf("listen sqlite admin server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(ctx) }()
	client := newAdminHTTPClient(t)

	ordinaryLogin, err := postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/login", map[string]string{"username": "alice", "password": "user-secret"}, nil)
	if err != nil {
		t.Fatalf("ordinary login: %v", err)
	}
	defer ordinaryLogin.Body.Close()
	assertErrorCode(t, ordinaryLogin, http.StatusUnauthorized, "UNAUTHENTICATED")

	login := loginAdmin(t, client, server.Addr().String(), "admin-sqlite", "secret")
	if !login.Authenticated || login.Username != "admin-sqlite" {
		t.Fatalf("unexpected sqlite admin login: %+v", login)
	}
}

func TestServerRedeemsClientEnrollmentToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, sessions, memory := adminAPITestRuntime(t)
	expiresAt := time.Now().UTC().Add(time.Hour)
	payload := enrollment.TokenPayload{EnrollmentID: "join-1", Secret: "join-secret", EnrollmentURL: "http://127.0.0.1:8080/api/client/enroll", ServerAddress: "127.0.0.1:8443", ServerTLSAddress: "127.0.0.1:9443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS}, Reconnect: config.DefaultClient().Reconnect, ExpiresAt: expiresAt}
	token, err := enrollment.EncodeToken(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ClientEnrollments().Create(context.Background(), domain.ClientEnrollment{ID: payload.EnrollmentID, ClientID: payload.ClientID, SecretHash: enrollment.HashSecret(payload.Secret), TokenHash: enrollment.HashToken(token), Token: token, ExpiresAt: expiresAt}); err != nil {
		t.Fatal(err)
	}
	server, err := Listen(Entry{ListenAddress: "127.0.0.1:0", AdminFrontendDir: writeAdminFrontendFixture(t), Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}, Enrollment: enrollment.Service{Store: db}})
	if err != nil {
		t.Fatalf("listen enrollment server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(ctx) }()

	response, err := postJSON(http.DefaultClient, "http://"+server.Addr().String()+clientEnrollmentPrefix+"/enroll", enrollment.RedeemRequest{Token: token}, nil)
	if err != nil {
		t.Fatalf("post enrollment: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected enrollment status %d body=%s", response.StatusCode, string(body))
	}
	var decoded enrollment.RedeemResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ClientID != "client-1" || decoded.Credential != "secret" {
		t.Fatalf("unexpected enrollment response: %+v", decoded)
	}

	duplicate, err := postJSON(http.DefaultClient, "http://"+server.Addr().String()+clientEnrollmentPrefix+"/enroll", enrollment.RedeemRequest{Token: token}, nil)
	if err != nil {
		t.Fatalf("post duplicate enrollment: %v", err)
	}
	defer duplicate.Body.Close()
	assertErrorCode(t, duplicate, http.StatusConflict, "UNAUTHENTICATED")
}

func TestServerSessionGraphQLAndCanonicalQueries(t *testing.T) {
	server := startAdminTestServer(t, nil)
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	queryResult := postAdminGraphQL(t, client, server.Addr().String(), `query {
  dashboard {
    onlineClientCount
    enabledProxyCount
    cumulativeUploadBytes
    cumulativeDownloadBytes
  }
  users(input: { page: { page: 1, pageSize: 10 }, sort: { field: "username", direction: "asc" } }) {
    totalCount
    pageInfo { page pageSize totalCount totalPages hasNext hasPrev }
    items { id username hasPasswordHash }
  }
  clients(input: { page: { page: 1, pageSize: 10 } }) {
    items { id name status runtime { online } }
  }
  proxies(input: { page: { page: 1, pageSize: 10 } }) {
    items { id name type status runtimeStatus config { entryHost entryPort targetHost targetPort } }
  }
  certificates(input: { page: { page: 1, pageSize: 10 } }) {
    totalCount
    items { proxyId host status }
  }
  audit(input: { page: { page: 1, pageSize: 10 } }) {
    items { action result actorType actorId }
  }
}`, "", http.StatusOK)
	data := queryResult["data"].(map[string]any)
	dashboard := data["dashboard"].(map[string]any)
	if dashboard["onlineClientCount"].(float64) != 1 || dashboard["enabledProxyCount"].(float64) != 1 {
		t.Fatalf("unexpected dashboard: %+v", dashboard)
	}
	users := data["users"].(map[string]any)
	if users["totalCount"].(float64) != 1 {
		t.Fatalf("unexpected users page: %+v", users)
	}
	clients := data["clients"].(map[string]any)["items"].([]any)
	if len(clients) != 1 || clients[0].(map[string]any)["status"].(string) != string(domain.ClientOnline) || !clients[0].(map[string]any)["runtime"].(map[string]any)["online"].(bool) {
		t.Fatalf("unexpected clients page: %+v", clients)
	}
	certificates := data["certificates"].(map[string]any)
	if certificates["totalCount"].(float64) != 0 {
		t.Fatalf("unexpected certificates page: %+v", certificates)
	}
	auditItems := data["audit"].(map[string]any)["items"].([]any)
	if len(auditItems) == 0 {
		t.Fatal("expected audit events")
	}
	firstAudit := auditItems[0].(map[string]any)
	if firstAudit["actorType"].(string) != "admin" || firstAudit["actorId"].(string) != "admin" {
		t.Fatalf("unexpected audit identity: %+v", firstAudit)
	}

	detailResult := postAdminGraphQL(t, client, server.Addr().String(), `query {
  user(id: "user-1") { id username status createdAt updatedAt }
  client(id: "client-1") { id name managedProxies { id name type } }
  proxy(id: "proxy-1") { id name type config { entryHost targetHost targetPort } }
}`, "", http.StatusOK)
	detailData := detailResult["data"].(map[string]any)
	userDetail := detailData["user"].(map[string]any)
	if userDetail["id"].(string) != "user-1" {
		t.Fatalf("unexpected user detail id: %+v", userDetail)
	}
	managedProxies := detailData["client"].(map[string]any)["managedProxies"].([]any)
	if len(managedProxies) != 1 {
		t.Fatalf("unexpected managed proxies: %+v", managedProxies)
	}

	mutationResponse, err := postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/graphql", map[string]any{"query": `mutation { createUser(input: { username: "bob", password: "secret-2", role: "user" }) { user { id } } }`}, nil)
	if err != nil {
		t.Fatalf("post graphql mutation without csrf: %v", err)
	}
	defer mutationResponse.Body.Close()
	assertGraphQLErrorCode(t, mutationResponse, http.StatusForbidden, "FORBIDDEN")

	mutationResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createUser(input: { username: "bob", password: "secret-2", role: "user" }) {
    userId
    status
    user { username hasPasswordHash }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	mutationData := mutationResult["data"].(map[string]any)["createUser"].(map[string]any)
	userValue, ok := mutationData["user"].(map[string]any)
	if !ok || len(userValue) == 0 {
		t.Fatalf("unexpected create user payload: %+v", mutationData)
	}
}

func TestServerClientCredentialAndValidationContract(t *testing.T) {
	deploymentRoot := t.TempDir()
	setDefaultAdminExecutable(t, deploymentRoot)
	t.Chdir(t.TempDir())
	server := startAdminTestServer(t, nil)
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	createResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createClient(input: { userId: "user-1", name: "branch-node" }) {
    clientId
    credential
    client { id name status }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	createPayload := createResult["data"].(map[string]any)["createClient"].(map[string]any)
	clientID := createPayload["clientId"].(string)
	credential := createPayload["credential"].(string)
	if clientID == "" || credential == "" {
		t.Fatalf("expected one-time credential in create payload: %+v", createPayload)
	}

	caFile := filepath.ToSlash(filepath.Join("data", "certs", "control-ca.crt"))
	if err := os.MkdirAll(filepath.Join(deploymentRoot, "data", "certs"), 0o755); err != nil {
		t.Fatalf("create control ca dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deploymentRoot, "data", "certs", "control-ca.crt"), []byte("ca-pem"), 0o600); err != nil {
		t.Fatalf("write control ca: %v", err)
	}
	joinResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createClientJoin(input: {
    userId: "user-1"
    name: "join-node"
    enrollmentUrl: "http://127.0.0.1:8080/api/client/enroll"
    serverAddress: "127.0.0.1:8443"
    serverTLSAddress: "127.0.0.1:9443"
    serverName: "go-ginx-control.test"
    serverCAFile: "`+caFile+`"
    ttlSeconds: 600
  }) {
    clientId
    token
    client { id name status }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	joinPayload := joinResult["data"].(map[string]any)["createClientJoin"].(map[string]any)
	joinToken := joinPayload["token"].(string)
	if joinPayload["clientId"].(string) == "" || joinToken == "" {
		t.Fatalf("expected join token payload: %+v", joinPayload)
	}
	tokenPayload, err := enrollment.DecodeToken(joinToken)
	if err != nil {
		t.Fatalf("decode join token: %v", err)
	}
	if tokenPayload.ClientID != joinPayload["clientId"].(string) || tokenPayload.ServerAddress != "127.0.0.1:8443" || tokenPayload.CAPEM != "ca-pem" {
		t.Fatalf("unexpected token payload: %+v", tokenPayload)
	}

	rotateResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  rotateClientCredential(input: { id: "`+clientID+`" }) {
    clientId
    credential
    client { id version }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	rotatedCredential := rotateResult["data"].(map[string]any)["rotateClientCredential"].(map[string]any)["credential"].(string)
	if rotatedCredential == "" || rotatedCredential == credential {
		t.Fatalf("expected rotated one-time credential, got %+v", rotateResult)
	}

	queryResult := postAdminGraphQL(t, client, server.Addr().String(), `query {
  clients(input: { page: { page: 1, pageSize: 20 } }) { items { id name status version } }
  client(id: "`+clientID+`") { id name status version }
}`, "", http.StatusOK)
	encoded, err := json.Marshal(queryResult)
	if err != nil {
		t.Fatalf("marshal query result: %v", err)
	}
	if bytes.Contains(encoded, []byte(credential)) || bytes.Contains(encoded, []byte(rotatedCredential)) {
		t.Fatalf("query response leaked client credential: %s", string(encoded))
	}

	validationResponse, err := postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/graphql", map[string]any{"query": `mutation { createClient(input: { userId: "", name: "" }) { clientId } }`}, map[string]string{"Content-Type": "application/json", adminCSRFHeader: bootstrap.CSRFToken})
	if err != nil {
		t.Fatalf("post invalid create client: %v", err)
	}
	defer validationResponse.Body.Close()
	assertGraphQLErrorFields(t, validationResponse, http.StatusOK, "VALIDATION_FAILED", []string{"userId", "name"})
}

func TestServerProxyEntryConflictAndDeleteAfterDisable(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.StaticListenerClaims = []domain.ListenerClaim{{Network: domain.ListenerNetworkTCP, Port: 10022, Source: "admin_listen", ResourceID: "admin_listen"}}
		ctx := context.Background()
		if err := entry.Commands.Store.Proxies().Create(ctx, domain.Proxy{ID: "proxy-update", UserID: "user-1", ClientID: "client-1", Name: "ssh-update", Type: domain.ProxyTCP, Status: domain.ProxyEnabled, EntryPort: 10024, TargetHost: "127.0.0.1", TargetPort: 24}); err != nil {
			t.Fatalf("seed update proxy: %v", err)
		}
		if err := entry.Commands.Store.Proxies().Create(ctx, domain.Proxy{ID: "proxy-enable", UserID: "user-1", ClientID: "client-1", Name: "ssh-enable", Type: domain.ProxyTCP, Status: domain.ProxyDisabled, EntryPort: 10022, TargetHost: "127.0.0.1", TargetPort: 22}); err != nil {
			t.Fatalf("seed enable proxy: %v", err)
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  createProxy(input: {
    userId: "user-1"
    clientId: "client-1"
    name: "ssh"
    type: "tcp"
    config: { entryPort: 10022, targetHost: "127.0.0.1", targetPort: 22 }
  }) { proxyId }
}`, bootstrap.CSRFToken, "ENTRY_CONFLICT")
	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  updateProxy(input: {
    id: "proxy-update"
    name: "ssh-update"
    type: "tcp"
    config: { entryPort: 10022, targetHost: "127.0.0.1", targetPort: 24 }
  }) { proxyId }
}`, bootstrap.CSRFToken, "ENTRY_CONFLICT")
	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  enableProxy(input: { id: "proxy-enable" }) { proxyId }
}`, bootstrap.CSRFToken, "ENTRY_CONFLICT")

	deleteBlocked, err := postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/graphql", map[string]any{"query": `mutation { deleteProxy(input: { id: "proxy-1" }) { proxyId } }`}, map[string]string{"Content-Type": "application/json", adminCSRFHeader: bootstrap.CSRFToken})
	if err != nil {
		t.Fatalf("delete enabled proxy: %v", err)
	}
	defer deleteBlocked.Body.Close()
	assertGraphQLErrorCode(t, deleteBlocked, http.StatusOK, "CONFLICT")
}

func TestServerClientJoinDefaultsAndDeleteContract(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "control-ca.crt")
	if err := os.WriteFile(caFile, []byte("ca-pem"), 0o600); err != nil {
		t.Fatalf("write control ca: %v", err)
	}
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.DefaultJoin = config.JoinServiceDefaults{
			EnrollmentURL:    "http://server.example.com:8080/api/client/enroll",
			ServerAddress:    "server.example.com:8443",
			ServerTLSAddress: "server.example.com:9443",
			ServerName:       "go-ginx-control.test",
			ServerCAFile:     caFile,
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	joinResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createClientJoin(input: {
    userId: "user-1"
    name: "join-default-node"
  }) {
    clientId
    token
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	joinPayload := joinResult["data"].(map[string]any)["createClientJoin"].(map[string]any)
	tokenPayload, err := enrollment.DecodeToken(joinPayload["token"].(string))
	if err != nil {
		t.Fatalf("decode join token: %v", err)
	}
	if tokenPayload.ServerAddress != "server.example.com:8443" || tokenPayload.ServerTLSAddress != "server.example.com:9443" || tokenPayload.EnrollmentURL != "http://server.example.com:8080/api/client/enroll" {
		t.Fatalf("join token did not use defaults: %+v", tokenPayload)
	}

	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  deleteClient(input: { id: "client-1" }) { clientId }
}`, bootstrap.CSRFToken, "CONFLICT")

	postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  disableProxy(input: { id: "proxy-1" }) { proxyId }
}`, bootstrap.CSRFToken, http.StatusOK)
	deleteResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  deleteClient(input: { id: "client-1" }) { clientId }
}`, bootstrap.CSRFToken, http.StatusOK)
	deletePayload := deleteResult["data"].(map[string]any)["deleteClient"].(map[string]any)
	if deletePayload["clientId"] != "client-1" {
		t.Fatalf("unexpected delete payload: %+v", deletePayload)
	}
	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `query {
  client(id: "client-1") { id }
}`, "", "NOT_FOUND")
}

func TestServerCertificateResponsesStaySecretSafe(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		ctx := context.Background()
		if err := entry.Commands.Store.Proxies().Create(ctx, domain.Proxy{ID: "proxy-https", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443, CertFile: "secret-cert.pem", KeyFile: "secret-key.pem"}); err != nil {
			t.Fatalf("seed https proxy: %v", err)
		}
		notAfter := time.Now().UTC().Add(time.Hour)
		if err := entry.Commands.Store.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-1", ProxyID: "proxy-https", Host: "secure.example.com", Status: domain.CertificateValid, CertFile: "secret-cert.pem", KeyFile: "secret-key.pem", PreviousCertFile: "old-cert.pem", PreviousKeyFile: "old-key.pem", NotAfter: &notAfter}); err != nil {
			t.Fatalf("seed certificate: %v", err)
		}
	})
	client := newAdminHTTPClient(t)
	loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	queryResult := postAdminGraphQL(t, client, server.Addr().String(), `query {
  certificates(input: { page: { page: 1, pageSize: 10 } }) {
    items { certificateId proxyId host status notAfter lastError }
  }
}`, "", http.StatusOK)
	encoded, err := json.Marshal(queryResult)
	if err != nil {
		t.Fatalf("marshal certificate result: %v", err)
	}
	for _, secret := range []string{"secret-cert.pem", "secret-key.pem", "old-key.pem", "old-cert.pem"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("certificate response leaked secret material %q: %s", secret, string(encoded))
		}
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

	loginAdmin(t, client, server.Addr().String(), "admin", "secret")
	now = now.Add(2 * time.Minute)
	bootstrap = readBootstrap(t, client, server.Addr().String())
	if bootstrap.Authenticated {
		t.Fatal("expected session to expire from bootstrap")
	}

	response, err = postJSON(client, "http://"+server.Addr().String()+adminAPIPrefix+"/graphql", map[string]any{"query": `query { dashboard { onlineClientCount } }`}, nil)
	if err != nil {
		t.Fatalf("graphql after expiry: %v", err)
	}
	defer response.Body.Close()
	assertGraphQLErrorCode(t, response, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func startAdminTestServer(t *testing.T, mutateEntry func(*Entry)) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, sessions, memory := adminAPITestRuntime(t)
	credentialsFile := writeAdminCredentials(t)
	entry := Entry{ListenAddress: "127.0.0.1:0", AdminCredentialsFile: credentialsFile, AdminFrontendDir: writeAdminFrontendFixture(t), Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}}
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

func setDefaultAdminExecutable(t *testing.T, deploymentRoot string) {
	t.Helper()
	previous := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(deploymentRoot, defaultBinaryDir, "goginx-server"), nil
	}
	t.Cleanup(func() {
		executablePath = previous
	})
}

func writeAdminFrontendFixture(t *testing.T) string {
	t.Helper()
	return writeAdminFrontendFixtureAt(t, t.TempDir())
}

func writeAdminFrontendFixtureAt(t *testing.T, dir string) string {
	t.Helper()
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><html><head><title>Admin UI</title></head><body><div id=app>admin frontend shell</div></body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("window.__adminFrontend = true;"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
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
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
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

func assertGraphQLErrorCodeForQuery(t *testing.T, client *http.Client, addr string, query string, csrfToken string, expectedCode string) {
	t.Helper()
	headers := map[string]string{"Content-Type": "application/json", adminCSRFHeader: csrfToken}
	response, err := postJSON(client, "http://"+addr+adminAPIPrefix+"/graphql", map[string]any{"query": query}, headers)
	if err != nil {
		t.Fatalf("post graphql error query: %v", err)
	}
	defer response.Body.Close()
	assertGraphQLErrorCode(t, response, http.StatusOK, expectedCode)
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

func assertGraphQLErrorCode(t *testing.T, response *http.Response, expectedStatus int, expectedCode string) {
	t.Helper()
	decoded := decodeGraphQLResponse(t, response)
	if response.StatusCode != expectedStatus {
		t.Fatalf("unexpected graphql status %d response=%+v", response.StatusCode, decoded)
	}
	code, _ := firstGraphQLErrorCode(decoded)
	if code != expectedCode {
		t.Fatalf("unexpected graphql error code %q response=%+v", code, decoded)
	}
}

func assertGraphQLErrorFields(t *testing.T, response *http.Response, expectedStatus int, expectedCode string, expectedFields []string) {
	t.Helper()
	decoded := decodeGraphQLResponse(t, response)
	if response.StatusCode != expectedStatus {
		t.Fatalf("unexpected graphql status %d response=%+v", response.StatusCode, decoded)
	}
	errorCode, fields := firstGraphQLErrorCode(decoded)
	if errorCode != expectedCode {
		t.Fatalf("unexpected graphql error code %q response=%+v", errorCode, decoded)
	}
	for _, field := range expectedFields {
		if _, ok := fields[field]; !ok {
			t.Fatalf("expected field error for %q response=%+v", field, decoded)
		}
	}
}

func decodeGraphQLResponse(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode graphql response: %v", err)
	}
	return decoded
}

func firstGraphQLErrorCode(decoded map[string]any) (string, map[string]any) {
	errorsValue, ok := decoded["errors"].([]any)
	if !ok || len(errorsValue) == 0 {
		return "", nil
	}
	firstError, _ := errorsValue[0].(map[string]any)
	extensions, _ := firstError["extensions"].(map[string]any)
	code, _ := extensions["code"].(string)
	fields, _ := extensions["fields"].(map[string]any)
	return code, fields
}

func decodeBootstrapResponse(t *testing.T, reader io.Reader) sessionBootstrapResponse {
	t.Helper()
	var response sessionBootstrapResponse
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}

func assertFrontendResponse(t *testing.T, url string, wantBodyFragment string, wantContentTypePrefix string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("get frontend route %s: %v", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected frontend status %d body=%s", response.StatusCode, string(body))
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read frontend body: %v", err)
	}
	if !strings.Contains(string(body), wantBodyFragment) {
		t.Fatalf("expected body fragment %q in %q", wantBodyFragment, string(body))
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, wantContentTypePrefix) {
		t.Fatalf("unexpected content type %q, want prefix %q", contentType, wantContentTypePrefix)
	}
}
