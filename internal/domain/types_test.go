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

func TestManagedCertificateValidateAcceptsSingleLabelWildcard(t *testing.T) {
	certificate := ManagedCertificate{ID: "cert-1", Host: "*.example.com", Status: CertificatePending}

	if err := certificate.Validate(); err != nil {
		t.Fatalf("expected wildcard certificate host to be valid: %v", err)
	}
	certificate.Host = "*.*.example.com"
	if err := certificate.Validate(); err == nil {
		t.Fatal("expected nested wildcard certificate host to be invalid")
	}
}

func TestListenerClaimConflictsOnWildcardBindHost(t *testing.T) {
	wildcard := ListenerClaim{Protocol: ListenerProtocolTCP, Network: ListenerNetworkTCP, BindHost: "0.0.0.0", Port: 10022}
	concrete := ListenerClaim{Protocol: ListenerProtocolTCP, Network: ListenerNetworkTCP, BindHost: "127.0.0.1", Port: 10022}

	if !wildcard.Conflicts(concrete) || !concrete.Conflicts(wildcard) {
		t.Fatal("expected wildcard and concrete bind hosts on the same port to conflict")
	}
}

func TestListenerClaimAllowsSharedHTTPListener(t *testing.T) {
	first := ListenerClaim{Protocol: ListenerProtocolHTTP, Network: ListenerNetworkTCP, BindHost: "127.0.0.1", Port: 8080}
	second := ListenerClaim{Protocol: ListenerProtocolHTTP, Network: ListenerNetworkTCP, BindHost: "127.0.0.1", Port: 8080}
	tcp := ListenerClaim{Protocol: ListenerProtocolTCP, Network: ListenerNetworkTCP, BindHost: "127.0.0.1", Port: 8080}

	if first.Conflicts(second) {
		t.Fatal("expected HTTP listeners with the same socket to be shareable")
	}
	if !first.Conflicts(tcp) {
		t.Fatal("expected HTTP and raw TCP listeners on the same socket to conflict")
	}
}

func TestEffectiveProxyEntryAppliesHTTPDefaults(t *testing.T) {
	proxy := Proxy{ID: "p1", Type: ProxyHTTP, EntryHost: "App.Example.com", TargetHost: "127.0.0.1", TargetPort: 8080}
	entry, ok := EffectiveProxyEntry(proxy, ProxyEntryDefaults{HTTPBindHost: "127.0.0.1", HTTPPort: 18080})
	if !ok {
		t.Fatal("expected effective entry")
	}
	if entry.BindHost != "127.0.0.1" || entry.Port != 18080 || entry.RouteHost != "app.example.com" {
		t.Fatalf("unexpected effective entry: %+v", entry)
	}
}
