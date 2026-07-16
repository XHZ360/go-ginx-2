package adminapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
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
	server, err := Listen(Entry{ListenAddress: "127.0.0.1:0", AdminFrontendDir: writeAdminFrontendFixture(t), AdminJWTSecret: testAdminJWTSecret(), Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}})
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

func TestServerDoesNotRedeemClientEnrollmentToken(t *testing.T) {
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
	server, err := Listen(Entry{ListenAddress: "127.0.0.1:0", AdminFrontendDir: writeAdminFrontendFixture(t), AdminJWTSecret: testAdminJWTSecret(), Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}})
	if err != nil {
		t.Fatalf("listen admin server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(ctx) }()

	response, err := postJSON(http.DefaultClient, "http://"+server.Addr().String()+clientEnrollmentPrefix+"/enroll", enrollment.RedeemRequest{Token: token}, nil)
	if err != nil {
		t.Fatalf("post admin enrollment route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("expected admin enrollment route to be absent, status=%d body=%s", response.StatusCode, string(body))
	}
	record, err := db.ClientEnrollments().ByID(context.Background(), payload.EnrollmentID)
	if err != nil {
		t.Fatalf("lookup enrollment: %v", err)
	}
	if record.UsedAt != nil {
		t.Fatal("admin listener must not consume enrollment token")
	}
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
		entry.Commands.StaticListenerClaims = []domain.ListenerClaim{
			{Network: domain.ListenerNetworkTCP, Port: 10022, Source: "admin_listen", ResourceID: "admin_listen"},
			{Network: domain.ListenerNetworkTCP, Port: 10081, Source: "client_enrollment_listen", ResourceID: "client_enrollment_listen"},
		}
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
  createProxy(input: {
    userId: "user-1"
    clientId: "client-1"
    name: "join-port"
    type: "tcp"
    config: { entryPort: 10081, targetHost: "127.0.0.1", targetPort: 8081 }
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
			EnrollmentURL:    "http://server.example.com:8081/api/client/enroll",
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
	if tokenPayload.ServerAddress != "server.example.com:8443" || tokenPayload.ServerTLSAddress != "server.example.com:9443" || tokenPayload.EnrollmentURL != "http://server.example.com:8081/api/client/enroll" {
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
		if err := entry.Commands.Store.Proxies().Create(ctx, domain.Proxy{ID: "proxy-https", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443}); err != nil {
			t.Fatalf("seed https proxy: %v", err)
		}
		// remove auto-registered static cert if any, then seed secret-path certificate for leak checks
		proxy, err := entry.Commands.Store.Proxies().ByID(ctx, "proxy-https")
		if err != nil {
			t.Fatalf("reload proxy: %v", err)
		}
		webDomain, err := entry.Commands.Store.Domains().ByID(ctx, proxy.DomainID)
		if err != nil {
			t.Fatalf("reload domain: %v", err)
		}
		if webDomain.CertificateID != "" {
			_ = entry.Commands.Store.Certificates().Delete(ctx, webDomain.CertificateID)
			webDomain.CertificateID = ""
			_ = entry.Commands.Store.Domains().Update(ctx, webDomain)
		}
		notAfter := time.Now().UTC().Add(time.Hour)
		if err := entry.Commands.Store.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-1", ProxyID: "proxy-https", Host: "secure.example.com", Status: domain.CertificateValid, CertFile: "secret-cert.pem", KeyFile: "secret-key.pem", PreviousCertFile: "old-cert.pem", PreviousKeyFile: "old-key.pem", NotAfter: &notAfter}); err != nil {
			t.Fatalf("seed certificate: %v", err)
		}
		webDomain.CertificateID = "cert-1"
		if err := entry.Commands.Store.Domains().Update(ctx, webDomain); err != nil {
			t.Fatalf("bind domain certificate: %v", err)
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

func TestServerProviderCredentialGraphQLLifecycleIsSecretSafe(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.Certificates = certmanager.Service{
			ProviderSecretStore: certmanager.FileSecretStore{Dir: t.TempDir()},
			OriginCAClient:      adminAPIOriginCAClient{},
			OriginCASettings:    domain.OriginCAProviderSettings{Enabled: true},
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	createResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createProviderCredential(input: { id: "cred-1", name: "Production Origin CA", scope: "Zone SSL:Edit", token: "cf-token-secret" }) {
    id
    status
    credential { id name providerType scope tokenFingerprint status lastVerifiedAt lastError }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	createPayload := createResult["data"].(map[string]any)["createProviderCredential"].(map[string]any)
	created := createPayload["credential"].(map[string]any)
	if created["id"] != "cred-1" || created["providerType"] != string(domain.CertificateProviderCloudflareOriginCA) || created["status"] != string(domain.ProviderCredentialPending) || created["tokenFingerprint"] == "" {
		t.Fatalf("unexpected created credential payload: %+v", createPayload)
	}

	verifyResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  verifyProviderCredential(input: { id: "cred-1" }) {
    credential { id status lastVerifiedAt lastError }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	verified := verifyResult["data"].(map[string]any)["verifyProviderCredential"].(map[string]any)["credential"].(map[string]any)
	if verified["status"] != string(domain.ProviderCredentialVerified) || verified["lastVerifiedAt"] == nil {
		t.Fatalf("unexpected verified credential payload: %+v", verified)
	}

	updateResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  updateProviderCredential(input: { id: "cred-1", name: "Rotated Origin CA", scope: "Zone SSL:Read,Edit", token: "cf-token-rotated" }) {
    credential { id name tokenFingerprint status lastVerifiedAt }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	updated := updateResult["data"].(map[string]any)["updateProviderCredential"].(map[string]any)["credential"].(map[string]any)
	if updated["name"] != "Rotated Origin CA" || updated["status"] != string(domain.ProviderCredentialPending) || updated["lastVerifiedAt"] != nil {
		t.Fatalf("unexpected updated credential payload: %+v", updated)
	}

	queryResult := postAdminGraphQL(t, client, server.Addr().String(), `query {
  providerCredentials(input: { page: { page: 1, pageSize: 10 } }) {
    items { id name providerType scope tokenFingerprint status lastVerifiedAt lastError }
  }
  providerCredential(id: "cred-1") { id name providerType scope tokenFingerprint status lastVerifiedAt lastError }
  audit(input: { page: { page: 1, pageSize: 20 } }) { items { action resourceId result } }
}`, "", http.StatusOK)
	encoded, err := json.Marshal(queryResult)
	if err != nil {
		t.Fatalf("marshal provider credential query: %v", err)
	}
	for _, secret := range []string{"cf-token-secret", "cf-token-rotated", "secretRef", "token:"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("provider credential response leaked %q: %s", secret, string(encoded))
		}
	}
	auditItems := queryResult["data"].(map[string]any)["audit"].(map[string]any)["items"].([]any)
	auditActions := make(map[string]bool, len(auditItems))
	for _, item := range auditItems {
		auditActions[item.(map[string]any)["action"].(string)] = true
	}
	for _, action := range []string{"create_provider_credential", "verify_provider_credential", "update_provider_credential"} {
		if !auditActions[action] {
			t.Fatalf("expected audit action %q in %+v", action, auditItems)
		}
	}

	disableResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  disableProviderCredential(input: { id: "cred-1" }) { credential { id status } }
}`, bootstrap.CSRFToken, http.StatusOK)
	disabled := disableResult["data"].(map[string]any)["disableProviderCredential"].(map[string]any)["credential"].(map[string]any)
	if disabled["status"] != string(domain.ProviderCredentialDisabled) {
		t.Fatalf("unexpected disabled credential payload: %+v", disabled)
	}
	deleteResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  deleteProviderCredential(input: { id: "cred-1" }) { id status }
}`, bootstrap.CSRFToken, http.StatusOK)
	deleted := deleteResult["data"].(map[string]any)["deleteProviderCredential"].(map[string]any)
	if deleted["id"] != "cred-1" || deleted["status"] != string(domain.ProviderCredentialDisabled) {
		t.Fatalf("unexpected delete credential payload: %+v", deleted)
	}
}

func TestServerProviderCredentialGraphQLReportsUnavailableStorage(t *testing.T) {
	server := startAdminTestServer(t, nil)
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  createProviderCredential(input: { id: "cred-1", name: "Production Origin CA", scope: "Zone SSL:Edit", token: "cf-token-secret" }) {
    id
  }
}`, bootstrap.CSRFToken, "UNSUPPORTED")
}

func TestServerProviderCredentialGraphQLReportsVerificationFailure(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.Certificates = certmanager.Service{
			ProviderSecretStore: certmanager.FileSecretStore{Dir: t.TempDir()},
			OriginCAClient:      adminAPIOriginCAClient{verifyErr: io.ErrUnexpectedEOF},
			OriginCASettings:    domain.OriginCAProviderSettings{Enabled: true},
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")
	postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createProviderCredential(input: { id: "cred-1", name: "Production Origin CA", scope: "Zone SSL:Edit", token: "cf-token-secret" }) {
    id
  }
}`, bootstrap.CSRFToken, http.StatusOK)

	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  verifyProviderCredential(input: { id: "cred-1" }) {
    id
  }
}`, bootstrap.CSRFToken, "CONFLICT")
}

func TestServerCloudflareOriginCAIssueFailureIsConsumable(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.Certificates = certmanager.Service{
			ProviderSecretStore: certmanager.FileSecretStore{Dir: t.TempDir()},
			OriginCAClient: adminAPIOriginCAClient{createErr: &certmanager.CloudflareAPIError{
				FailureMessage: "cloudflare origin ca request failed",
				StatusCode:     http.StatusBadRequest,
				Errors:         []certmanager.CloudflareAPIErrorDetail{{Code: 1010}},
			}},
			OriginCASettings: domain.OriginCAProviderSettings{Enabled: true},
			NewID:            func() (string, error) { return "cert-origin-failed", nil },
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")
	postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createProviderCredential(input: { id: "cred-1", name: "Production Origin CA", scope: "Zone SSL:Edit", token: "cf-token-secret" }) {
    id
  }
}`, bootstrap.CSRFToken, http.StatusOK)

	response := postGraphQLErrorQuery(t, client, server.Addr().String(), `mutation {
  createCertificate(input: { host: "www.example.com", providerType: "cloudflare_origin_ca", credentialId: "cred-1", requestType: "origin-ecc", requestedValidity: 5475 }) {
    certificate { certificateId }
  }
}`, bootstrap.CSRFToken)
	code, _ := firstGraphQLErrorCode(response)
	if code != "CONFLICT" {
		t.Fatalf("expected conflict error, got %+v", response)
	}
	message := firstGraphQLErrorMessage(response)
	if !strings.Contains(message, "cloudflare origin ca request failed: status 400") || !strings.Contains(message, "1010") {
		t.Fatalf("expected cloudflare error message, got %q response=%+v", message, response)
	}
}

func TestServerReportsACMEReadinessAndBlocksCreate(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.Certificates = certmanager.Service{Settings: domain.ACMEProviderSettings{DNSProviderTokenEnv: "CF_DNS_API_TOKEN"}}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	result := postAdminGraphQL(t, client, server.Addr().String(), `query {
  certificateProviderReadiness { providerType ready missingRequirements tokenEnvName }
}`, "", http.StatusOK)
	readiness := result["data"].(map[string]any)["certificateProviderReadiness"].([]any)[0].(map[string]any)
	if readiness["providerType"] != "acme_dns01" || readiness["ready"] != false || readiness["tokenEnvName"] != "CF_DNS_API_TOKEN" {
		t.Fatalf("unexpected acme readiness: %+v", readiness)
	}

	response := postGraphQLErrorQuery(t, client, server.Addr().String(), `mutation {
  createCertificate(input: { host: "blocked.example.com", providerType: "acme_dns01" }) { certificate { certificateId } }
}`, bootstrap.CSRFToken)
	code, _ := firstGraphQLErrorCode(response)
	if code != "PROVIDER_NOT_READY" {
		t.Fatalf("expected provider readiness error, got %+v", response)
	}
}

func TestServerCreatesWildcardOriginCACertificateThroughGraphQL(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	certificateDir := t.TempDir()
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.Certificates = certmanager.Service{
			ProviderSecretStore: certmanager.FileSecretStore{Dir: t.TempDir()},
			OriginCAClient: adminAPIOriginCAClient{create: func(_ context.Context, _ string, request certmanager.OriginCACreateRequest) (certmanager.OriginCACertificate, error) {
				return certmanager.OriginCACertificate{ID: "cf-cert-wildcard", CertificatePEM: adminAPIOriginCATestCertificateFromCSR(t, request.CSR, request.Hostnames, now.Add(365*24*time.Hour)), Hostnames: request.Hostnames, RequestType: request.RequestType, RequestedValidity: request.RequestedValidity, Status: "active"}, nil
			}},
			OriginCASettings: domain.OriginCAProviderSettings{Enabled: true, DefaultRequestType: certmanager.OriginCARequestTypeECC, RequestedValidity: 5475},
			Storage:          httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir, Now: func() time.Time { return now }},
			NewID:            func() (string, error) { return "cert-origin-wildcard", nil },
			Now:              func() time.Time { return now },
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")
	postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createProviderCredential(input: { id: "cred-1", name: "Production Origin CA", scope: "Zone SSL:Edit", token: "cf-token-secret" }) {
    id
  }
}`, bootstrap.CSRFToken, http.StatusOK)

	result := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createCertificate(input: { host: "*.example.com", providerType: "cloudflare_origin_ca", credentialId: "cred-1", requestType: "origin-ecc", requestedValidity: 5475 }) {
    certificate { certificateId host status servingStatus providerType cloudflareCertificateId hostnames }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	certificate := result["data"].(map[string]any)["createCertificate"].(map[string]any)["certificate"].(map[string]any)
	if certificate["certificateId"] != "cert-origin-wildcard" || certificate["host"] != "*.example.com" || certificate["status"] != string(domain.CertificateValid) || certificate["servingStatus"] != string(domain.CertificateServingUsable) {
		t.Fatalf("unexpected wildcard origin ca certificate: %+v", certificate)
	}
	hostnames := certificate["hostnames"].([]any)
	if len(hostnames) != 1 || hostnames[0] != "*.example.com" {
		t.Fatalf("unexpected wildcard hostnames: %+v", hostnames)
	}
	stored, err := server.commands.Store.Certificates().ByID(context.Background(), "cert-origin-wildcard")
	if err != nil {
		t.Fatalf("read stored wildcard certificate: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(stored.CertFile), "/_wildcard.example.com/") || !strings.Contains(filepath.ToSlash(stored.KeyFile), "/_wildcard.example.com/") {
		t.Fatalf("expected safe wildcard storage paths: %+v", stored)
	}
}

func TestServerSessionExpiryAndLogout(t *testing.T) {
	now := time.Now().UTC()
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Now = func() time.Time { return now }
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
	if !bootstrap.Authenticated {
		t.Fatal("expected jwt session to survive inactivity before absolute expiry")
	}

	now = now.Add(4 * time.Minute)
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

func TestServerJWTSessionSurvivesRestartWithSameSigningKey(t *testing.T) {
	client := newAdminHTTPClient(t)
	first := startAdminTestServer(t, nil)
	login := loginAdmin(t, client, first.Addr().String(), "admin", "secret")
	if !login.Authenticated {
		t.Fatalf("expected authenticated login: %+v", login)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first server: %v", err)
	}

	second := startAdminTestServer(t, nil)
	bootstrap := readBootstrap(t, client, second.Addr().String())
	if !bootstrap.Authenticated || bootstrap.Username != "admin" || bootstrap.CSRFToken != login.CSRFToken {
		t.Fatalf("expected jwt bootstrap after restart, got %+v login=%+v", bootstrap, login)
	}
	result := postAdminGraphQL(t, client, second.Addr().String(), `query { dashboard { onlineClientCount } }`, "", http.StatusOK)
	if result["data"].(map[string]any)["dashboard"].(map[string]any)["onlineClientCount"].(float64) != 1 {
		t.Fatalf("unexpected dashboard after restart: %+v", result)
	}
}

func TestServerJWTSessionRejectsOldSigningKeyAfterRotation(t *testing.T) {
	client := newAdminHTTPClient(t)
	first := startAdminTestServer(t, nil)
	loginAdmin(t, client, first.Addr().String(), "admin", "secret")
	if err := first.Close(); err != nil {
		t.Fatalf("close first server: %v", err)
	}

	second := startAdminTestServer(t, func(entry *Entry) {
		entry.AdminJWTSecret = []byte("fedcba9876543210fedcba9876543210")
	})
	bootstrap := readBootstrap(t, client, second.Addr().String())
	if bootstrap.Authenticated {
		t.Fatalf("expected rotated key to reject old jwt, got %+v", bootstrap)
	}
}

func TestServerInvalidJWTBootstrapClearsCookie(t *testing.T) {
	server := startAdminTestServer(t, nil)
	client := newAdminHTTPClient(t)
	cookieURL, err := url.Parse("http://" + server.Addr().String() + adminAPIPrefix + "/session")
	if err != nil {
		t.Fatal(err)
	}
	client.Jar.SetCookies(cookieURL, []*http.Cookie{{Name: adminSessionCookieName, Value: "not-a-jwt", Path: adminSessionCookiePath}})
	bootstrap := readBootstrap(t, client, server.Addr().String())
	if bootstrap.Authenticated {
		t.Fatalf("expected invalid jwt to be unauthenticated, got %+v", bootstrap)
	}
	cookies := client.Jar.Cookies(cookieURL)
	for _, cookie := range cookies {
		if cookie.Name == adminSessionCookieName && cookie.Value != "" {
			t.Fatalf("expected invalid jwt cookie to be cleared, got %+v", cookies)
		}
	}
}

func startAdminTestServer(t *testing.T, mutateEntry func(*Entry)) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, sessions, memory := adminAPITestRuntime(t)
	credentialsFile := writeAdminCredentials(t)
	entry := Entry{ListenAddress: "127.0.0.1:0", AdminCredentialsFile: credentialsFile, AdminFrontendDir: writeAdminFrontendFixture(t), AdminJWTSecret: testAdminJWTSecret(), Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memory}, Commands: admin.Service{Store: db}}
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

func testAdminJWTSecret() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

type adminAPIOriginCAClient struct {
	create    func(context.Context, string, certmanager.OriginCACreateRequest) (certmanager.OriginCACertificate, error)
	createErr error
	verifyErr error
}

func (client adminAPIOriginCAClient) Create(ctx context.Context, token string, request certmanager.OriginCACreateRequest) (certmanager.OriginCACertificate, error) {
	if client.create != nil {
		return client.create(ctx, token, request)
	}
	if client.createErr != nil {
		return certmanager.OriginCACertificate{}, client.createErr
	}
	return certmanager.OriginCACertificate{}, io.ErrUnexpectedEOF
}

func (adminAPIOriginCAClient) Get(context.Context, string, string) (certmanager.OriginCACertificate, error) {
	return certmanager.OriginCACertificate{}, io.ErrUnexpectedEOF
}

func (adminAPIOriginCAClient) List(context.Context, string) ([]certmanager.OriginCACertificate, error) {
	return nil, io.ErrUnexpectedEOF
}

func (adminAPIOriginCAClient) Revoke(context.Context, string, string) error {
	return io.ErrUnexpectedEOF
}

func (client adminAPIOriginCAClient) VerifyToken(context.Context, string) error {
	return client.verifyErr
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
	decoded := postGraphQLErrorQuery(t, client, addr, query, csrfToken)
	code, _ := firstGraphQLErrorCode(decoded)
	if code != expectedCode {
		t.Fatalf("unexpected graphql error code %q response=%+v", code, decoded)
	}
}

func postGraphQLErrorQuery(t *testing.T, client *http.Client, addr string, query string, csrfToken string) map[string]any {
	t.Helper()
	headers := map[string]string{"Content-Type": "application/json", adminCSRFHeader: csrfToken}
	response, err := postJSON(client, "http://"+addr+adminAPIPrefix+"/graphql", map[string]any{"query": query}, headers)
	if err != nil {
		t.Fatalf("post graphql error query: %v", err)
	}
	defer response.Body.Close()
	decoded := decodeGraphQLResponse(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected graphql status %d response=%+v", response.StatusCode, decoded)
	}
	if _, ok := decoded["errors"].([]any); !ok {
		t.Fatalf("expected graphql errors response=%+v", decoded)
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

func firstGraphQLErrorMessage(decoded map[string]any) string {
	errorsValue, ok := decoded["errors"].([]any)
	if !ok || len(errorsValue) == 0 {
		return ""
	}
	firstError, _ := errorsValue[0].(map[string]any)
	message, _ := firstError["message"].(string)
	return message
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

func TestServerDomainDetailResolvesEmbeddedListFields(t *testing.T) {
	// DomainDetail embeds DomainListItem; graphql-go default resolve does not promote
	// anonymous fields, so certificateId/host must use explicit resolvers.
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "domain-detail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Certificates().Create(ctx, domain.ManagedCertificate{
		ID: "cert-ww1", Host: "ww1.example.com", Status: domain.CertificateValid, ServingStatus: domain.CertificateServingUsable,
		ProviderStatus: domain.CertificateProviderStatusActive, Hostnames: []string{"ww1.example.com"}, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Domains().Create(ctx, domain.Domain{
		ID: "domain-ww1", UserID: user.ID, Host: "ww1.example.com", CertificateID: "cert-ww1", Status: domain.DomainEnabled, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Query = adminquery.Service{Store: db}
		entry.Commands = admin.Service{Store: db}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	decoded := postAdminGraphQL(t, client, server.Addr().String(), `
query {
  domain(id: "domain-ww1") {
    id
    host
    certificateId
    status
    certificate { certificateId host boundDomainId }
  }
}
`, bootstrap.CSRFToken, http.StatusOK)
	data, _ := decoded["data"].(map[string]any)
	domainNode, _ := data["domain"].(map[string]any)
	if domainNode["host"] != "ww1.example.com" {
		t.Fatalf("expected domain host from embedded list item, got %+v", domainNode)
	}
	if domainNode["certificateId"] != "cert-ww1" {
		t.Fatalf("expected bound certificateId on domain detail, got %+v", domainNode)
	}
	certificate, _ := domainNode["certificate"].(map[string]any)
	if certificate["certificateId"] != "cert-ww1" || certificate["boundDomainId"] != "domain-ww1" {
		t.Fatalf("expected certificate summary on domain detail, got %+v", certificate)
	}
}

func TestServerCertificateBindingMutationsAndReferenceFields(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		ctx := context.Background()
		if err := entry.Commands.Store.Proxies().Create(ctx, domain.Proxy{ID: "proxy-https", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443}); err != nil {
			t.Fatalf("seed https proxy: %v", err)
		}
		if err := entry.Commands.Store.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-bind", Host: "secure.example.com", Hostnames: []string{"secure.example.com"}, Status: domain.CertificateValid, CertFile: "secret-cert.pem", KeyFile: "secret-key.pem"}); err != nil {
			t.Fatalf("seed certificate: %v", err)
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	bindResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  bindCertificate(input: { proxyId: "proxy-https", certificateId: "cert-bind" }) {
    proxyId
    proxy { id config { certificateId certFile keyFile } certificate { certificateId } }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	bindPayload := bindResult["data"].(map[string]any)["bindCertificate"].(map[string]any)
	if bindPayload["proxyId"] != "proxy-https" {
		t.Fatalf("unexpected bind payload: %+v", bindPayload)
	}
	boundProxy := bindPayload["proxy"].(map[string]any)
	boundConfig := boundProxy["config"].(map[string]any)
	if boundConfig["certificateId"] != "cert-bind" {
		t.Fatalf("expected proxy config certificateId to be cert-bind: %+v", boundConfig)
	}

	queryResult := postAdminGraphQL(t, client, server.Addr().String(), `query {
  certificates(input: { page: { page: 1, pageSize: 10 } }) {
    items { certificateId referenced boundProxyId servable deletionRisk lastError }
  }
}`, "", http.StatusOK)
	items := queryResult["data"].(map[string]any)["certificates"].(map[string]any)["items"].([]any)
	var bound map[string]any
	for _, item := range items {
		entry := item.(map[string]any)
		if entry["certificateId"] == "cert-bind" {
			bound = entry
		}
	}
	if bound == nil {
		t.Fatalf("certificate cert-bind missing from list: %+v", items)
	}
	if bound["referenced"] != true {
		t.Fatalf("expected certificate to be referenced after binding: %+v", bound)
	}
	if bound["boundProxyId"] != "proxy-https" {
		t.Fatalf("expected boundProxyId proxy-https: %+v", bound)
	}
	if _, ok := bound["servable"].(bool); !ok {
		t.Fatalf("expected servable boolean field: %+v", bound)
	}
	if risk, ok := bound["deletionRisk"].(string); !ok || risk == "" {
		t.Fatalf("expected non-empty deletionRisk string: %+v", bound)
	}

	encoded, err := json.Marshal(queryResult)
	if err != nil {
		t.Fatalf("marshal certificate query: %v", err)
	}
	for _, secret := range []string{"secret-cert.pem", "secret-key.pem"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("certificate response leaked secret material %q: %s", secret, string(encoded))
		}
	}

	unbindResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  unbindCertificate(input: { proxyId: "proxy-https" }) {
    proxy { config { certificateId } }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	unboundConfig := unbindResult["data"].(map[string]any)["unbindCertificate"].(map[string]any)["proxy"].(map[string]any)["config"].(map[string]any)
	if certificateID, _ := unboundConfig["certificateId"].(string); certificateID != "" {
		t.Fatalf("expected certificateId cleared after unbind: %+v", unboundConfig)
	}
}

func TestServerBindCertificateIncompatibleHostReportsTypedCode(t *testing.T) {
	server := startAdminTestServer(t, func(entry *Entry) {
		ctx := context.Background()
		if err := entry.Commands.Store.Proxies().Create(ctx, domain.Proxy{ID: "proxy-https", UserID: "user-1", ClientID: "client-1", Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "secure.example.com", TargetHost: "127.0.0.1", TargetPort: 8443}); err != nil {
			t.Fatalf("seed https proxy: %v", err)
		}
		if err := entry.Commands.Store.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-other", Host: "other.example.com", Hostnames: []string{"other.example.com"}, Status: domain.CertificateValid, CertFile: "c.pem", KeyFile: "k.pem"}); err != nil {
			t.Fatalf("seed certificate: %v", err)
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  bindCertificate(input: { proxyId: "proxy-https", certificateId: "cert-other" }) { proxyId }
}`, bootstrap.CSRFToken, "CERTIFICATE_INCOMPATIBLE")
}

// TestServerCertificateCreateProxySelectionAndRiskBasedDeleteThroughSchema 端到端覆盖
// 新 GraphQL mutation：createCertificate（file provider，未绑定）、proxy create 通过
// config.certificateId 建立绑定，以及风险分级的 deleteCertificate（强确认失败 ->
// CONFIRMATION_REQUIRED；确认后成功并返回 requiredConfirm/affectedProxyIds），且响应不泄露 secret。
func TestServerCertificateCreateProxySelectionAndRiskBasedDeleteThroughSchema(t *testing.T) {
	certificateDir := t.TempDir()
	certFile, keyFile := writeAdminAPICertPair(t, certificateDir, "secure.example.com", time.Now().Add(24*time.Hour))
	server := startAdminTestServer(t, func(entry *Entry) {
		entry.Commands.Certificates = certmanager.Service{
			Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir},
			NewID:   func() (string, error) { return "cert-file-graphql", nil },
			Now:     func() time.Time { return time.Now().UTC() },
		}
	})
	client := newAdminHTTPClient(t)
	bootstrap := loginAdmin(t, client, server.Addr().String(), "admin", "secret")

	// createCertificate（file provider）创建未绑定证书资源。
	createResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createCertificate(input: { host: "secure.example.com", providerType: "file", certFile: "`+filepath.ToSlash(certFile)+`", keyFile: "`+filepath.ToSlash(keyFile)+`" }) {
    proxyId
    certificate { certificateId host providerType status }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	createPayload := createResult["data"].(map[string]any)["createCertificate"].(map[string]any)
	createdCert := createPayload["certificate"].(map[string]any)
	certificateID := createdCert["certificateId"].(string)
	if certificateID == "" || createdCert["host"] != "secure.example.com" || createdCert["providerType"] != string(domain.CertificateProviderFile) {
		t.Fatalf("unexpected created certificate payload: %+v", createPayload)
	}
	if proxyID, _ := createPayload["proxyId"].(string); proxyID != "" {
		t.Fatalf("expected created certificate to be unbound, got proxyId %q", proxyID)
	}

	// 清单中确认为未绑定、低风险。
	listResult := postAdminGraphQL(t, client, server.Addr().String(), `query {
  certificates(input: { page: { page: 1, pageSize: 10 } }) {
    items { certificateId referenced boundProxyId deletionRisk }
  }
}`, "", http.StatusOK)
	listItems := listResult["data"].(map[string]any)["certificates"].(map[string]any)["items"].([]any)
	var listed map[string]any
	for _, item := range listItems {
		entry := item.(map[string]any)
		if entry["certificateId"] == certificateID {
			listed = entry
		}
	}
	if listed == nil || listed["referenced"] != false || listed["boundProxyId"] != "" {
		t.Fatalf("expected unbound certificate in list, got %+v", listed)
	}

	// proxy create 通过 config.certificateId 建立绑定。
	proxyResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  createProxy(input: {
    userId: "user-1"
    clientId: "client-1"
    name: "secure"
    type: "https"
    config: { entryHost: "secure.example.com", targetHost: "127.0.0.1", targetPort: 8443, certificateId: "`+certificateID+`" }
  }) {
    proxyId
    proxy { id config { certificateId } certificate { certificateId } }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	proxyPayload := proxyResult["data"].(map[string]any)["createProxy"].(map[string]any)
	proxyID := proxyPayload["proxyId"].(string)
	if proxyID == "" {
		t.Fatalf("expected created https proxy id: %+v", proxyPayload)
	}
	boundConfig := proxyPayload["proxy"].(map[string]any)["config"].(map[string]any)
	if boundConfig["certificateId"] != certificateID {
		t.Fatalf("expected proxy bound to certificate via config.certificateId: %+v", boundConfig)
	}

	// proxy update 显式提交空 certificateId，应清空绑定而不是保留旧证书。
	clearBindingResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  updateProxy(input: {
    id: "`+proxyID+`"
    name: "secure"
    config: { entryHost: "secure.example.com", targetHost: "127.0.0.1", targetPort: 8443, certificateId: "" }
  }) {
    proxy { config { certificateId } }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	clearConfig := clearBindingResult["data"].(map[string]any)["updateProxy"].(map[string]any)["proxy"].(map[string]any)["config"].(map[string]any)
	if certificateID, _ := clearConfig["certificateId"].(string); certificateID != "" {
		t.Fatalf("expected updateProxy empty certificateId to clear binding: %+v", clearConfig)
	}

	// 重新绑定，继续覆盖下方「已绑定且可服务」的高风险删除路径。
	rebindResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  bindCertificate(input: { proxyId: "`+proxyID+`", certificateId: "`+certificateID+`" }) {
    proxy { config { certificateId } }
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	rebindConfig := rebindResult["data"].(map[string]any)["bindCertificate"].(map[string]any)["proxy"].(map[string]any)["config"].(map[string]any)
	if rebindConfig["certificateId"] != certificateID {
		t.Fatalf("expected proxy rebound to certificate: %+v", rebindConfig)
	}

	// deleteCertificate 无确认（已绑定且可服务）-> CONFIRMATION_REQUIRED。
	assertGraphQLErrorCodeForQuery(t, client, server.Addr().String(), `mutation {
  deleteCertificate(input: { certificateId: "`+certificateID+`" }) { certificateId requiredConfirm affectedProxyIds }
}`, bootstrap.CSRFToken, "CONFIRMATION_REQUIRED")

	// deleteCertificate 提供匹配 host 确认 -> 成功，requiredConfirm=true，affectedProxyIds 含该代理。
	deleteResult := postAdminGraphQL(t, client, server.Addr().String(), `mutation {
  deleteCertificate(input: { certificateId: "`+certificateID+`", confirmHost: "secure.example.com" }) {
    certificateId
    requiredConfirm
    affectedProxyIds
  }
}`, bootstrap.CSRFToken, http.StatusOK)
	deletePayload := deleteResult["data"].(map[string]any)["deleteCertificate"].(map[string]any)
	if deletePayload["certificateId"] != certificateID || deletePayload["requiredConfirm"] != true {
		t.Fatalf("unexpected delete payload: %+v", deletePayload)
	}
	affected := deletePayload["affectedProxyIds"].([]any)
	if len(affected) != 1 || affected[0].(string) != proxyID {
		t.Fatalf("expected affected proxy %q in delete payload, got %+v", proxyID, affected)
	}

	// 整个交互链路不得泄露私钥文件路径或私钥材料。
	for _, encoded := range [][]byte{mustMarshal(t, createResult), mustMarshal(t, proxyResult), mustMarshal(t, deleteResult), mustMarshal(t, listResult)} {
		for _, secret := range []string{filepath.ToSlash(keyFile), "PRIVATE KEY"} {
			if bytes.Contains(encoded, []byte(secret)) {
				t.Fatalf("graphql response leaked secret material %q: %s", secret, string(encoded))
			}
		}
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal graphql result: %v", err)
	}
	return encoded
}

// writeAdminAPICertPair 在受管证书目录下写入一对覆盖 host 的有效证书/私钥文件，返回其路径。
func writeAdminAPICertPair(t *testing.T, certificateDir string, host string, notAfter time.Time) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	dir := filepath.Join(certificateDir, "static", host)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir managed cert dir: %v", err)
	}
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	return certFile, keyFile
}

func adminAPIOriginCATestCertificateFromCSR(t *testing.T, csrPEM string, hostnames []string, notAfter time.Time) []byte {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		t.Fatal("decode csr")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse csr: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("check csr signature: %v", err)
	}
	if len(hostnames) == 0 {
		hostnames = csr.DNSNames
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: hostnames[0]}, DNSNames: hostnames, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, csr.PublicKey, key)
	if err != nil {
		t.Fatalf("create origin ca certificate from csr: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
