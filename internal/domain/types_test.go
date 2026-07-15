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

func TestProxyRouteSelectsLongestPathBoundary(t *testing.T) {
	routes := []ProxyRoute{
		{PathPrefix: "/api", Status: ProxyRouteEnabled},
		{PathPrefix: "/api/v2", Status: ProxyRouteEnabled},
	}
	selected, ok := SelectProxyRoute(routes, "/api/v2/users")
	if !ok || selected.PathPrefix != "/api/v2" {
		t.Fatalf("expected longest route, got %+v, %v", selected, ok)
	}
	if _, ok := SelectProxyRoute(routes, "/apix"); ok {
		t.Fatal("expected path segment boundary to reject /apix")
	}
}

func TestRewriteProxyRoutePath(t *testing.T) {
	route := ProxyRoute{PathPrefix: "/api", StripPrefix: true, UpstreamPathPrefix: "/v1"}
	if got := RewriteProxyRoutePath("/api/users", route); got != "/v1/users" {
		t.Fatalf("unexpected rewritten path: %q", got)
	}
	if got := RewriteProxyRoutePath("/api", route); got != "/v1" {
		t.Fatalf("unexpected prefix-only path: %q", got)
	}
}

func TestProxyRouteValidateRejectsReservedPrefix(t *testing.T) {
	route := ProxyRoute{ID: "r1", ProxyID: "p1", ClientID: "c1", PathPrefix: "/.well-known/goginx/activate", TargetHost: "127.0.0.1", TargetPort: 8080, Status: ProxyRouteEnabled}
	if err := route.Validate(); err == nil {
		t.Fatal("expected reserved route prefix error")
	}
}

func TestProxyValidateAcceptsWebProxyWithDomainAndPath(t *testing.T) {
	proxy := Proxy{ID: "p1", UserID: "u1", ClientID: "c1", Name: "web", Type: ProxyWeb, Status: ProxyEnabled, DomainID: "d1", PathPrefix: "/", UpstreamPathPrefix: "/", TargetHost: "127.0.0.1", TargetPort: 8080}

	if err := proxy.Validate(); err != nil {
		t.Fatalf("expected valid proxy: %v", err)
	}
}

func TestSelectWebProxyLongestPrefix(t *testing.T) {
	proxies := []Proxy{
		{PathPrefix: "/api", Status: ProxyEnabled, Type: ProxyWeb},
		{PathPrefix: "/api/v2", Status: ProxyEnabled, Type: ProxyWeb},
	}
	selected, ok := SelectWebProxy(proxies, "/api/v2/users")
	if !ok || selected.PathPrefix != "/api/v2" {
		t.Fatalf("expected longest proxy, got %+v, %v", selected, ok)
	}
	if _, ok := SelectWebProxy(proxies, "/apix"); ok {
		t.Fatal("expected path segment boundary to reject /apix")
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

func TestEffectiveDomainEntryAppliesHTTPDefaults(t *testing.T) {
	domainEntry := DomainEntry{ID: "e1", DomainID: "d1", Protocol: DomainEntryHTTP, Status: DomainEntryEnabled}
	entry, ok := EffectiveDomainEntry(domainEntry, ProxyEntryDefaults{HTTPBindHost: "127.0.0.1", HTTPPort: 18080})
	if !ok {
		t.Fatal("expected effective entry")
	}
	if entry.BindHost != "127.0.0.1" || entry.Port != 18080 || entry.Protocol != ListenerProtocolHTTP {
		t.Fatalf("unexpected effective entry: %+v", entry)
	}
}

func TestClientKindValid(t *testing.T) {
	if !ClientKindProvider.Valid() {
		t.Fatal("expected provider kind to be valid")
	}
	if !ClientKindConsumer.Valid() {
		t.Fatal("expected consumer kind to be valid")
	}
	if ClientKind("").Valid() {
		t.Fatal("expected empty kind to be invalid")
	}
	if ClientKind("unknown").Valid() {
		t.Fatal("expected unknown kind to be invalid")
	}
}

func TestNormalizeClientKindDefaultsToProvider(t *testing.T) {
	if NormalizeClientKind("") != ClientKindProvider {
		t.Fatal("expected empty kind to normalize to provider")
	}
	if NormalizeClientKind(ClientKindConsumer) != ClientKindConsumer {
		t.Fatal("expected consumer kind to pass through")
	}
	if NormalizeClientKind(ClientKindProvider) != ClientKindProvider {
		t.Fatal("expected provider kind to pass through")
	}
}

func TestClientValidateAcceptsEmptyKindAsProvider(t *testing.T) {
	client := Client{ID: "c1", UserID: "u1", Name: "test", CredentialHash: "hash"}
	if err := client.Validate(); err != nil {
		t.Fatalf("expected empty kind to default to provider: %v", err)
	}
}

func TestClientValidateRejectsInvalidKind(t *testing.T) {
	client := Client{ID: "c1", UserID: "u1", Name: "test", Kind: "bogus", CredentialHash: "hash"}
	if err := client.Validate(); err == nil {
		t.Fatal("expected invalid kind to be rejected")
	}
}
