package clientjoin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
)

func TestJoinRedeemsTokenAndReturnsClientConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client/enroll" || r.Method != http.MethodPost {
			t.Fatalf("unexpected enrollment request %s %s", r.Method, r.URL.Path)
		}
		var request enrollment.RedeemRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Token == "" {
			t.Fatal("expected token")
		}
		_ = json.NewEncoder(w).Encode(enrollment.RedeemResponse{ServerAddress: "127.0.0.1:8443", ServerTLSAddress: "127.0.0.1:9443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect})
	}))
	defer server.Close()
	token, err := enrollment.EncodeToken(enrollment.TokenPayload{EnrollmentID: "join-1", Secret: "join-secret", EnrollmentURL: server.URL + "/api/client/enroll", ServerAddress: "127.0.0.1:8443", ServerName: "go-ginx-control.test", CAPEM: "ca-pem", ClientID: "client-1", Credential: "secret", AllowedProtocols: []domain.Protocol{domain.ProtocolQUIC}, Reconnect: config.DefaultClient().Reconnect, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}

	cfg, caPEM, err := Join(context.Background(), token, nil)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if cfg.ClientID != "client-1" || cfg.Credential != "secret" || cfg.ServerName != "go-ginx-control.test" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if string(caPEM) != "ca-pem" {
		t.Fatalf("unexpected ca pem %q", string(caPEM))
	}
}
