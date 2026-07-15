package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/http"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	tcpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/tcp"
	udpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/udp"
)

type listenerKey struct {
	Protocol string
	BindHost string
	Port     int
}

type desiredProxyListeners struct {
	tcp   map[listenerKey]int
	udp   map[listenerKey]int
	http  map[listenerKey]int
	https map[listenerKey]int
}

func (runtime *ServerRuntime) initProxyListenerRegistry() {
	runtime.tcpListeners = make(map[listenerKey]*tcpproxy.Listener)
	runtime.udpListeners = make(map[listenerKey]*udpproxy.Listener)
	runtime.httpServers = make(map[listenerKey]*httpproxy.Server)
	runtime.httpsListeners = make(map[listenerKey]*httpsproxy.Listener)
}

func (runtime *ServerRuntime) startDefaultProxyListeners() error {
	runtime.proxyListenerMu.Lock()
	defer runtime.proxyListenerMu.Unlock()

	httpKey := listenerKey{Protocol: domain.ListenerProtocolHTTP, BindHost: runtime.proxyEntryDefaults.HTTPBindHost, Port: runtime.proxyEntryDefaults.HTTPPort}
	actualHTTPKey, err := runtime.startHTTPListenerLocked(httpKey, true, 0)
	if err != nil {
		return fmt.Errorf("listen default http proxy: %w", err)
	}
	runtime.defaultHTTPKey = actualHTTPKey
	runtime.proxyEntryDefaults.HTTPBindHost = actualHTTPKey.BindHost
	runtime.proxyEntryDefaults.HTTPPort = actualHTTPKey.Port
	if runtime.httpsEntryEnabled {
		httpsKey := listenerKey{Protocol: domain.ListenerProtocolHTTPS, BindHost: runtime.proxyEntryDefaults.HTTPSBindHost, Port: runtime.proxyEntryDefaults.HTTPSPort}
		actualHTTPSKey, err := runtime.startHTTPSListenerLocked(httpsKey, true, 0)
		if err != nil {
			return fmt.Errorf("listen default https proxy: %w", err)
		}
		runtime.defaultHTTPSKey = actualHTTPSKey
		runtime.proxyEntryDefaults.HTTPSBindHost = actualHTTPSKey.BindHost
		runtime.proxyEntryDefaults.HTTPSPort = actualHTTPSKey.Port
	}
	runtime.refreshLegacyListenerFieldsLocked()
	return nil
}

func (runtime *ServerRuntime) ReconcileProxyListeners(ctx context.Context) error {
	if runtime == nil || runtime.Store == nil {
		return nil
	}
	desired, err := runtime.desiredProxyListeners(ctx)
	if err != nil {
		return err
	}
	runtime.proxyListenerMu.Lock()
	defer runtime.proxyListenerMu.Unlock()

	for key, proxyCount := range desired.tcp {
		if _, ok := runtime.tcpListeners[key]; ok {
			continue
		}
		if err := runtime.startTCPListenerLocked(key, proxyCount); err != nil {
			return err
		}
	}
	for key, proxyCount := range desired.udp {
		if _, ok := runtime.udpListeners[key]; ok {
			continue
		}
		if err := runtime.startUDPListenerLocked(key, proxyCount); err != nil {
			return err
		}
	}
	for key, proxyCount := range desired.http {
		if _, ok := runtime.httpServers[key]; ok {
			continue
		}
		if _, err := runtime.startHTTPListenerLocked(key, runtime.isDefaultHTTPKey(key), proxyCount); err != nil {
			return err
		}
	}
	for key, proxyCount := range desired.https {
		if _, ok := runtime.httpsListeners[key]; ok {
			continue
		}
		if _, err := runtime.startHTTPSListenerLocked(key, runtime.isDefaultHTTPSKey(key), proxyCount); err != nil {
			return err
		}
	}

	for key, listener := range runtime.tcpListeners {
		if _, ok := desired.tcp[key]; ok {
			continue
		}
		log.Printf("proxy listener stopping: protocol=%s bind_host=%s port=%d", key.Protocol, displayListenerBindHost(key.BindHost), key.Port)
		if err := listener.Close(); err != nil {
			return fmt.Errorf("close tcp proxy listener %s: %w", key.address(), err)
		}
		delete(runtime.tcpListeners, key)
	}
	for key, listener := range runtime.udpListeners {
		if _, ok := desired.udp[key]; ok {
			continue
		}
		log.Printf("proxy listener stopping: protocol=%s bind_host=%s port=%d", key.Protocol, displayListenerBindHost(key.BindHost), key.Port)
		if err := listener.Close(); err != nil {
			return fmt.Errorf("close udp proxy listener %s: %w", key.address(), err)
		}
		delete(runtime.udpListeners, key)
	}
	for key, server := range runtime.httpServers {
		if _, ok := desired.http[key]; ok {
			continue
		}
		log.Printf("proxy listener stopping: protocol=%s bind_host=%s port=%d", key.Protocol, displayListenerBindHost(key.BindHost), key.Port)
		if err := server.Close(); err != nil {
			return fmt.Errorf("close http proxy listener %s: %w", key.address(), err)
		}
		delete(runtime.httpServers, key)
	}
	for key, listener := range runtime.httpsListeners {
		if _, ok := desired.https[key]; ok {
			continue
		}
		log.Printf("proxy listener stopping: protocol=%s bind_host=%s port=%d", key.Protocol, displayListenerBindHost(key.BindHost), key.Port)
		if err := listener.Close(); err != nil {
			return fmt.Errorf("close https proxy listener %s: %w", key.address(), err)
		}
		delete(runtime.httpsListeners, key)
	}

	runtime.refreshLegacyListenerFieldsLocked()
	return nil
}

func (runtime *ServerRuntime) desiredProxyListeners(ctx context.Context) (desiredProxyListeners, error) {
	desired := desiredProxyListeners{
		tcp:   make(map[listenerKey]int),
		udp:   make(map[listenerKey]int),
		http:  make(map[listenerKey]int),
		https: make(map[listenerKey]int),
	}
	if runtime.defaultHTTPKey.Port > 0 {
		desired.http[runtime.defaultHTTPKey] = 0
	}
	if runtime.httpsEntryEnabled && runtime.defaultHTTPSKey.Port > 0 {
		desired.https[runtime.defaultHTTPSKey] = 0
	}
	proxies, err := runtime.Store.Proxies().List(ctx)
	if err != nil {
		return desiredProxyListeners{}, err
	}
	for _, proxy := range proxies {
		if proxy.Status != domain.ProxyEnabled {
			continue
		}
		if proxy.Type.IsWeb() {
			// Web listeners are driven by DomainEntry, not Proxy entry fields.
			continue
		}
		entry, ok := domain.EffectiveProxyEntry(proxy, runtime.proxyEntryDefaults)
		if !ok {
			if runtime.proxyUsesUnavailableDefault(proxy) {
				continue
			}
			return desiredProxyListeners{}, fmt.Errorf("%s proxy %s entry listener is invalid", proxy.Type, proxy.ID)
		}
		key := listenerKey{Protocol: entry.Protocol, BindHost: entry.BindHost, Port: entry.Port}
		switch key.Protocol {
		case domain.ListenerProtocolTCP:
			desired.tcp[key]++
		case domain.ListenerProtocolUDP:
			desired.udp[key]++
		case domain.ListenerProtocolHTTP:
			desired.http[key]++
		case domain.ListenerProtocolHTTPS:
			desired.https[key]++
		}
	}
	entries, err := runtime.Store.DomainEntries().ListEnabled(ctx)
	if err != nil {
		return desiredProxyListeners{}, err
	}
	for _, domainEntry := range entries {
		webDomain, err := runtime.Store.Domains().ByID(ctx, domainEntry.DomainID)
		if err != nil || webDomain.Status != domain.DomainEnabled {
			continue
		}
		entry, ok := domain.EffectiveDomainEntry(domainEntry, runtime.proxyEntryDefaults)
		if !ok {
			if domainEntry.Port == 0 {
				continue
			}
			return desiredProxyListeners{}, fmt.Errorf("domain entry %s listener is invalid", domainEntry.ID)
		}
		key := listenerKey{Protocol: entry.Protocol, BindHost: entry.BindHost, Port: entry.Port}
		switch key.Protocol {
		case domain.ListenerProtocolHTTP:
			desired.http[key]++
		case domain.ListenerProtocolHTTPS:
			desired.https[key]++
		}
	}
	return desired, nil
}

func (runtime *ServerRuntime) proxyUsesUnavailableDefault(proxy domain.Proxy) bool {
	switch proxy.Type {
	case domain.ProxyHTTP:
		return proxy.EntryPort == 0 && runtime.proxyEntryDefaults.HTTPPort == 0
	case domain.ProxyHTTPS:
		return proxy.EntryPort == 0 && runtime.proxyEntryDefaults.HTTPSPort == 0
	default:
		return false
	}
}

func (runtime *ServerRuntime) startTCPListenerLocked(key listenerKey, proxyCount int) error {
	includeDefault := domain.NormalizeBindHost(key.BindHost) == domain.NormalizeBindHost(runtime.proxyEntryDefaults.TCPBindHost)
	listener, err := tcpproxy.Listen(tcpproxy.Entry{Store: runtime.Store, Sessions: runtime.Sessions, ListenAddress: key.address(), EntryBindHost: key.BindHost, EntryPort: key.Port, IncludeDefaultEntry: includeDefault, Stats: runtime.persistentStats})
	if err != nil {
		log.Printf("proxy listener start failed: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
		return fmt.Errorf("listen tcp proxy on %s: %w", key.address(), err)
	}
	runtime.tcpListeners[key] = listener
	log.Printf("proxy listener started: protocol=%s bind_host=%s port=%d proxies=%d", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, proxyCount)
	go runtime.serveTCPListener(listener, key)
	return nil
}

func (runtime *ServerRuntime) startUDPListenerLocked(key listenerKey, proxyCount int) error {
	includeDefault := domain.NormalizeBindHost(key.BindHost) == domain.NormalizeBindHost(runtime.proxyEntryDefaults.TCPBindHost)
	listener, err := udpproxy.Listen(udpproxy.Entry{Store: runtime.Store, Sessions: runtime.Sessions, ListenAddress: key.address(), EntryBindHost: key.BindHost, EntryPort: key.Port, IncludeDefaultEntry: includeDefault, Stats: runtime.persistentStats})
	if err != nil {
		log.Printf("proxy listener start failed: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
		return fmt.Errorf("listen udp proxy on %s: %w", key.address(), err)
	}
	runtime.udpListeners[key] = listener
	log.Printf("proxy listener started: protocol=%s bind_host=%s port=%d proxies=%d", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, proxyCount)
	go runtime.serveUDPListener(listener, key)
	return nil
}

func (runtime *ServerRuntime) startHTTPListenerLocked(key listenerKey, includeDefaultRoutes bool, proxyCount int) (listenerKey, error) {
	server, err := httpproxy.Listen(httpproxy.Entry{Store: runtime.Store, Sessions: runtime.Sessions, ListenAddress: key.address(), EntryBindHost: key.BindHost, EntryPort: key.Port, IncludeDefaultRoutes: includeDefaultRoutes, Stats: runtime.persistentStats})
	if err != nil {
		log.Printf("proxy listener start failed: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
		return listenerKey{}, fmt.Errorf("listen http proxy on %s: %w", key.address(), err)
	}
	actualKey := key.withActualPort(server.Addr())
	server.SetEntryPort(actualKey.Port)
	runtime.httpServers[actualKey] = server
	log.Printf("proxy listener started: protocol=%s bind_host=%s port=%d proxies=%d", actualKey.Protocol, displayListenerBindHost(actualKey.BindHost), actualKey.Port, proxyCount)
	go runtime.serveHTTPServer(server, actualKey)
	return actualKey, nil
}

func (runtime *ServerRuntime) startHTTPSListenerLocked(key listenerKey, includeDefaultRoutes bool, proxyCount int) (listenerKey, error) {
	listener, err := httpsproxy.Listen(httpsproxy.Entry{Store: runtime.Store, Sessions: runtime.Sessions, ListenAddress: key.address(), EntryBindHost: key.BindHost, EntryPort: key.Port, IncludeDefaultRoutes: includeDefaultRoutes, CertificateDir: runtime.certificateDir})
	if err != nil {
		log.Printf("proxy listener start failed: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
		return listenerKey{}, fmt.Errorf("listen https proxy on %s: %w", key.address(), err)
	}
	actualKey := key.withActualPort(listener.Addr())
	listener.SetEntryPort(actualKey.Port)
	runtime.httpsListeners[actualKey] = listener
	log.Printf("proxy listener started: protocol=%s bind_host=%s port=%d proxies=%d", actualKey.Protocol, displayListenerBindHost(actualKey.BindHost), actualKey.Port, proxyCount)
	go runtime.serveHTTPSListener(listener, actualKey)
	return actualKey, nil
}

func (runtime *ServerRuntime) serveTCPListener(listener *tcpproxy.Listener, key listenerKey) {
	if err := listener.Serve(runtime.runtimeCtx); err != nil && runtime.runtimeCtx.Err() == nil {
		log.Printf("proxy listener stopped with error: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
	}
}

func (runtime *ServerRuntime) serveUDPListener(listener *udpproxy.Listener, key listenerKey) {
	if err := listener.Serve(runtime.runtimeCtx); err != nil && runtime.runtimeCtx.Err() == nil {
		log.Printf("proxy listener stopped with error: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
	}
}

func (runtime *ServerRuntime) serveHTTPServer(server *httpproxy.Server, key listenerKey) {
	if err := server.Serve(runtime.runtimeCtx); err != nil && runtime.runtimeCtx.Err() == nil {
		log.Printf("proxy listener stopped with error: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
	}
}

func (runtime *ServerRuntime) serveHTTPSListener(listener *httpsproxy.Listener, key listenerKey) {
	if err := listener.Serve(runtime.runtimeCtx); err != nil && runtime.runtimeCtx.Err() == nil {
		log.Printf("proxy listener stopped with error: protocol=%s bind_host=%s port=%d error=%v", key.Protocol, displayListenerBindHost(key.BindHost), key.Port, err)
	}
}

func (runtime *ServerRuntime) refreshLegacyListenerFieldsLocked() {
	runtime.TCPListeners = runtime.TCPListeners[:0]
	for _, listener := range runtime.tcpListeners {
		runtime.TCPListeners = append(runtime.TCPListeners, listener)
	}
	runtime.UDPListeners = runtime.UDPListeners[:0]
	for _, listener := range runtime.udpListeners {
		runtime.UDPListeners = append(runtime.UDPListeners, listener)
	}
	runtime.HTTPServer = nil
	if server := runtime.httpServers[runtime.defaultHTTPKey]; server != nil {
		runtime.HTTPServer = server
	} else {
		for _, server := range runtime.httpServers {
			runtime.HTTPServer = server
			break
		}
	}
	runtime.HTTPSListener = nil
	if listener := runtime.httpsListeners[runtime.defaultHTTPSKey]; listener != nil {
		runtime.HTTPSListener = listener
	} else {
		for _, listener := range runtime.httpsListeners {
			runtime.HTTPSListener = listener
			break
		}
	}
}

func (runtime *ServerRuntime) closeProxyListeners() error {
	runtime.proxyListenerMu.Lock()
	defer runtime.proxyListenerMu.Unlock()
	var closeErr error
	for key, listener := range runtime.tcpListeners {
		closeErr = errors.Join(closeErr, listener.Close())
		delete(runtime.tcpListeners, key)
	}
	for key, listener := range runtime.udpListeners {
		closeErr = errors.Join(closeErr, listener.Close())
		delete(runtime.udpListeners, key)
	}
	for key, server := range runtime.httpServers {
		closeErr = errors.Join(closeErr, server.Close())
		delete(runtime.httpServers, key)
	}
	for key, listener := range runtime.httpsListeners {
		closeErr = errors.Join(closeErr, listener.Close())
		delete(runtime.httpsListeners, key)
	}
	runtime.refreshLegacyListenerFieldsLocked()
	return closeErr
}

func (runtime *ServerRuntime) TCPProxyListenerCount() int {
	runtime.proxyListenerMu.Lock()
	defer runtime.proxyListenerMu.Unlock()
	return len(runtime.tcpListeners)
}

func (runtime *ServerRuntime) UDPProxyListenerCount() int {
	runtime.proxyListenerMu.Lock()
	defer runtime.proxyListenerMu.Unlock()
	return len(runtime.udpListeners)
}

func (runtime *ServerRuntime) HTTPProxyListenerCount() int {
	runtime.proxyListenerMu.Lock()
	defer runtime.proxyListenerMu.Unlock()
	return len(runtime.httpServers)
}

func (runtime *ServerRuntime) HTTPSProxyListenerCount() int {
	runtime.proxyListenerMu.Lock()
	defer runtime.proxyListenerMu.Unlock()
	return len(runtime.httpsListeners)
}

func (runtime *ServerRuntime) isDefaultHTTPKey(key listenerKey) bool {
	return key == runtime.defaultHTTPKey
}

func (runtime *ServerRuntime) isDefaultHTTPSKey(key listenerKey) bool {
	return key == runtime.defaultHTTPSKey
}

func (key listenerKey) address() string {
	return domain.ListenAddress(key.BindHost, key.Port)
}

func (key listenerKey) withActualPort(addr net.Addr) listenerKey {
	actualPort := portFromAddr(addr)
	if actualPort > 0 {
		key.Port = actualPort
	}
	return key
}

func portFromAddr(addr net.Addr) int {
	if addr == nil {
		return 0
	}
	_, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0
	}
	return port
}

func displayListenerBindHost(host string) string {
	if domain.NormalizeBindHost(host) == "" {
		return "*"
	}
	return domain.NormalizeBindHost(host)
}
