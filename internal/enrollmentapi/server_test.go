package enrollmentapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestServerRedeemsClientEnrollmentToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, token := enrollmentAPITestStore(t)
	server, err := Listen(Entry{ListenAddress: "127.0.0.1:0", Enrollment: enrollment.Service{Store: db}})
	if err != nil {
		t.Fatalf("listen enrollment server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(ctx) }()

	response, err := postJSON("http://"+server.Addr().String()+ClientEnrollmentPath, enrollment.RedeemRequest{Token: token})
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

	duplicate, err := postJSON("http://"+server.Addr().String()+ClientEnrollmentPath, enrollment.RedeemRequest{Token: token})
	if err != nil {
		t.Fatalf("post duplicate enrollment: %v", err)
	}
	defer duplicate.Body.Close()
	if duplicate.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(duplicate.Body)
		t.Fatalf("unexpected duplicate enrollment status %d body=%s", duplicate.StatusCode, string(body))
	}
}

func TestServerOnlyExposesEnrollmentPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, _ := enrollmentAPITestStore(t)
	server, err := Listen(Entry{ListenAddress: "127.0.0.1:0", Enrollment: enrollment.Service{Store: db}})
	if err != nil {
		t.Fatalf("listen enrollment server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(ctx) }()

	for _, path := range []string{"/", "/dashboard", "/api/admin/graphql"} {
		response, err := http.Get("http://" + server.Addr().String() + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("expected %s to be unavailable, got %d", path, response.StatusCode)
		}
	}
}

func enrollmentAPITestStore(t *testing.T) (*sqlite.Store, string) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "enrollmentapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	payload := enrollment.TokenPayload{EnrollmentID: "join-1", Secret: "join-secret", EnrollmentURL: "http://127.0.0.1:8081/api/client/enroll", ServerAddress: "127.0.0.1:8443", ServerTLSAddress: "127.0.0.1:9443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS}, Reconnect: config.DefaultClient().Reconnect, ExpiresAt: expiresAt}
	token, err := enrollment.EncodeToken(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ClientEnrollments().Create(ctx, domain.ClientEnrollment{ID: payload.EnrollmentID, ClientID: payload.ClientID, SecretHash: enrollment.HashSecret(payload.Secret), TokenHash: enrollment.HashToken(token), Token: token, ExpiresAt: expiresAt}); err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	return db, token
}

func postJSON(url string, payload any) (*http.Response, error) {
	content, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return http.Post(url, "application/json", bytes.NewReader(content))
}
