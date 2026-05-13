package certmanager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestNewCloudflareDNSProviderFromEnvRequiresToken(t *testing.T) {
	t.Setenv("CF_DNS_API_TOKEN", "")
	if _, err := NewCloudflareDNSProviderFromEnv("CF_DNS_API_TOKEN"); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestCloudflareDNSProviderPresentsAndCleansChallenge(t *testing.T) {
	ctx := context.Background()
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		requests = append(requests, r.Method+" "+r.URL.String())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			if r.URL.Query().Get("name") == "example.com" {
				writeJSON(t, w, map[string]any{"success": true, "result": []map[string]string{{"id": "zone-1"}}})
				return
			}
			writeJSON(t, w, map[string]any{"success": true, "result": []map[string]string{}})
		case r.Method == http.MethodPost && r.URL.Path == "/zones/zone-1/dns_records":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["type"] != "TXT" || body["name"] != "_acme-challenge.app.example.com" || body["content"] != "challenge-value" {
				t.Fatalf("unexpected present body: %+v", body)
			}
			writeJSON(t, w, map[string]any{"success": true, "result": map[string]string{"id": "record-1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone-1/dns_records":
			writeJSON(t, w, map[string]any{"success": true, "result": []map[string]string{{"id": "record-1"}}})
		case r.Method == http.MethodDelete && r.URL.Path == "/zones/zone-1/dns_records/record-1":
			writeJSON(t, w, map[string]any{"success": true, "result": map[string]string{"id": "record-1"}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(server.Close)

	provider := CloudflareDNSProvider{APIToken: "secret-token", BaseURL: server.URL, HTTPClient: server.Client()}
	if err := provider.Present(ctx, "_acme-challenge.app.example.com", "challenge-value"); err != nil {
		t.Fatalf("present challenge: %v", err)
	}
	if err := provider.CleanUp(ctx, "_acme-challenge.app.example.com", "challenge-value"); err != nil {
		t.Fatalf("cleanup challenge: %v", err)
	}
	want := []string{
		"GET /zones?name=app.example.com",
		"GET /zones?name=example.com",
		"POST /zones/zone-1/dns_records",
		"GET /zones?name=app.example.com",
		"GET /zones?name=example.com",
		"GET /zones/zone-1/dns_records?content=challenge-value&name=_acme-challenge.app.example.com&type=TXT",
		"DELETE /zones/zone-1/dns_records/record-1",
	}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("unexpected requests:\n got: %#v\nwant: %#v", requests, want)
	}
}

func TestCloudflareErrorsDoNotExposeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"denied"}]}`))
	}))
	t.Cleanup(server.Close)
	provider := CloudflareDNSProvider{APIToken: "secret-token", BaseURL: server.URL, HTTPClient: server.Client()}
	err := provider.Present(context.Background(), "_acme-challenge.app.example.com", "challenge-value")
	if err == nil {
		t.Fatal("expected cloudflare error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestCandidateZones(t *testing.T) {
	got := candidateZones("_acme-challenge.app.example.com.")
	want := []string{"app.example.com", "example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected candidate zones: got %#v want %#v", got, want)
	}
}

func TestNewCloudflareDNSProviderFromEnv(t *testing.T) {
	if err := os.Setenv("CF_DNS_API_TOKEN", "secret-token"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("CF_DNS_API_TOKEN") })
	provider, err := NewCloudflareDNSProviderFromEnv("CF_DNS_API_TOKEN")
	if err != nil {
		t.Fatalf("provider from env: %v", err)
	}
	if provider.APIToken != "secret-token" {
		t.Fatal("expected token from environment")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
