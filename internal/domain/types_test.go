package domain

import "testing"

func TestHashCredentialIsStableAndNotPlaintext(t *testing.T) {
	first := HashCredential("secret")
	second := HashCredential("secret")

	if first != second {
		t.Fatal("expected stable credential hash")
	}
	if first == "secret" || first == "" {
		t.Fatal("expected non-empty hash distinct from plaintext")
	}
}

func TestProxyValidateRejectsInvalidTarget(t *testing.T) {
	proxy := Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "bad", Type: ProxyTCP, Status: ProxyEnabled, EntryPort: 10000, TargetHost: "bad host", TargetPort: 8080}

	if err := proxy.Validate(); err == nil {
		t.Fatal("expected invalid target host error")
	}
}

func TestProxyValidateAcceptsHTTPHostRouteWithoutPort(t *testing.T) {
	proxy := Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "web", Type: ProxyHTTP, Status: ProxyEnabled, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080}

	if err := proxy.Validate(); err != nil {
		t.Fatalf("expected valid proxy: %v", err)
	}
}
