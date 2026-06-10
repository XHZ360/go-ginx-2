package certmanager

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestBuildOriginCACSRGeneratesLocalKeyAndNormalizedSANs(t *testing.T) {
	csrPEM, keyPEM, err := BuildOriginCACSR([]string{" App.Example.com ", "app.example.com", "api.example.com"}, OriginCARequestTypeECC)
	if err != nil {
		t.Fatalf("build csr: %v", err)
	}
	if strings.Contains(string(csrPEM), "PRIVATE KEY") {
		t.Fatal("csr output must not include private key material")
	}
	if !strings.Contains(string(keyPEM), "PRIVATE KEY") {
		t.Fatal("expected local private key material to be returned separately")
	}
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		t.Fatal("decode csr pem")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse csr: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("check csr signature: %v", err)
	}
	if !slices.Equal(csr.DNSNames, []string{"app.example.com", "api.example.com"}) {
		t.Fatalf("unexpected csr dns names: %+v", csr.DNSNames)
	}
	if _, _, err := BuildOriginCACSR([]string{"app.example.com"}, "origin-unknown"); err == nil {
		t.Fatal("expected unsupported request type error")
	}
}

func TestCloudflareOriginCAClientRequestsAndSanitizesErrors(t *testing.T) {
	ctx := context.Background()
	const token = "cf-token-secret"
	calls := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		key := r.Method + " " + r.URL.Path
		calls[key]++
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "POST /certificates":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create body: %v", err)
			}
			if body["csr"] != "csr-pem" || body["request_type"] != OriginCARequestTypeECC || int(body["requested_validity"].(float64)) != 5475 {
				t.Errorf("unexpected create body: %+v", body)
			}
			hostnames, _ := body["hostnames"].([]any)
			if len(hostnames) != 2 || hostnames[0] != "app.example.com" || hostnames[1] != "api.example.com" {
				t.Errorf("unexpected hostnames in create body: %+v", hostnames)
			}
			if _, ok := body["private_key"]; ok {
				t.Error("create body must not include private_key")
			}
			_, _ = w.Write([]byte(`{"success":true,"result":{"id":"cert-1","certificate":"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n","hostnames":["app.example.com","api.example.com"],"request_type":"origin-ecc","requested_validity":5475,"expires_on":"2030-01-02T03:04:05Z","status":"active"}}`))
		case "GET /certificates/cert-1":
			_, _ = w.Write([]byte(`{"success":true,"result":{"id":"cert-1","hostnames":["app.example.com"],"status":"active"}}`))
		case "GET /certificates":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"cert-1","hostnames":["app.example.com"],"status":"active"}]}`))
		case "DELETE /certificates/cert-1":
			_, _ = w.Write([]byte(`{"success":true,"result":{"id":"cert-1"}}`))
		case "GET /user/tokens/verify":
			_, _ = w.Write([]byte(`{"success":true,"result":{"status":"active"}}`))
		default:
			t.Errorf("unexpected request %s", key)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := CloudflareOriginCAClient{HTTPClient: server.Client(), BaseURL: server.URL}
	created, err := client.Create(ctx, token, OriginCACreateRequest{CSR: "csr-pem", Hostnames: []string{"App.Example.com", "api.example.com"}, RequestType: OriginCARequestTypeECC, RequestedValidity: 5475})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID != "cert-1" || created.RequestType != OriginCARequestTypeECC || created.ExpiresOn == nil || !slices.Equal(created.Hostnames, []string{"app.example.com", "api.example.com"}) {
		t.Fatalf("unexpected created certificate: %+v", created)
	}
	got, err := client.Get(ctx, token, "cert-1")
	if err != nil || got.ID != "cert-1" {
		t.Fatalf("get certificate: cert=%+v err=%v", got, err)
	}
	listed, err := client.List(ctx, token)
	if err != nil || len(listed) != 1 || listed[0].ID != "cert-1" {
		t.Fatalf("list certificates: certs=%+v err=%v", listed, err)
	}
	if err := client.Revoke(ctx, token, "cert-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := client.VerifyToken(ctx, token); err != nil {
		t.Fatalf("verify token: %v", err)
	}
	for _, key := range []string{"POST /certificates", "GET /certificates/cert-1", "GET /certificates", "DELETE /certificates/cert-1", "GET /user/tokens/verify"} {
		if calls[key] != 1 {
			t.Fatalf("expected one %s call, got %d", key, calls[key])
		}
	}
}

func TestCloudflareOriginCAClientErrorDoesNotLeakTokenOrResponseBody(t *testing.T) {
	const token = "cf-token-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"success":false,"errors":[{"message":"body has cf-token-secret"}]}`, http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	client := CloudflareOriginCAClient{HTTPClient: server.Client(), BaseURL: server.URL}
	err := client.VerifyToken(context.Background(), token)
	if err == nil {
		t.Fatal("expected verify failure")
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "body has") {
		t.Fatalf("origin ca error leaked sensitive context: %v", err)
	}
}

func TestCloudflareOriginCAClientMapsMissingCertificate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	client := CloudflareOriginCAClient{HTTPClient: server.Client(), BaseURL: server.URL}
	_, err := client.Get(context.Background(), "token", url.PathEscape("missing/cert"))
	if err != ErrOriginCACertificateMissing {
		t.Fatalf("expected missing certificate error, got %v", err)
	}
}
