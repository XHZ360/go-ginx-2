package domain

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	ListenerNetworkTCP = "tcp"
	ListenerNetworkUDP = "udp"

	ListenerProtocolTCP   = "tcp"
	ListenerProtocolUDP   = "udp"
	ListenerProtocolHTTP  = "http"
	ListenerProtocolHTTPS = "https"
)

type ProxyEntryDefaults struct {
	TCPBindHost   string
	HTTPBindHost  string
	HTTPPort      int
	HTTPSBindHost string
	HTTPSPort     int
}

type ProxyEntry struct {
	Protocol  string
	Network   string
	BindHost  string
	Port      int
	RouteHost string
}

func EffectiveProxyEntry(proxy Proxy, defaults ProxyEntryDefaults) (ProxyEntry, bool) {
	entry := ProxyEntry{
		BindHost: NormalizeBindHost(proxy.EntryBindHost),
		Port:     proxy.EntryPort,
	}
	switch proxy.Type {
	case ProxyTCP:
		entry.Protocol = ListenerProtocolTCP
		entry.Network = ListenerNetworkTCP
		if entry.BindHost == "" {
			entry.BindHost = NormalizeBindHost(defaults.TCPBindHost)
		}
	case ProxyUDP:
		entry.Protocol = ListenerProtocolUDP
		entry.Network = ListenerNetworkUDP
		if entry.BindHost == "" {
			entry.BindHost = NormalizeBindHost(defaults.TCPBindHost)
		}
	case ProxyHTTP:
		// legacy path retained for pre-migration fixtures only
		entry.Protocol = ListenerProtocolHTTP
		entry.Network = ListenerNetworkTCP
		entry.RouteHost = NormalizeRouteHost(proxy.EntryHost)
		if entry.BindHost == "" {
			entry.BindHost = NormalizeBindHost(defaults.HTTPBindHost)
		}
		if entry.Port == 0 {
			entry.Port = defaults.HTTPPort
		}
	case ProxyHTTPS:
		entry.Protocol = ListenerProtocolHTTPS
		entry.Network = ListenerNetworkTCP
		entry.RouteHost = NormalizeRouteHost(proxy.EntryHost)
		if entry.BindHost == "" {
			entry.BindHost = NormalizeBindHost(defaults.HTTPSBindHost)
		}
		if entry.Port == 0 {
			entry.Port = defaults.HTTPSPort
		}
	case ProxyWeb:
		// Web listeners are driven by DomainEntry, not Proxy entry fields.
		return ProxyEntry{}, false
	default:
		return ProxyEntry{}, false
	}
	if entry.Port <= 0 || entry.Port > 65535 {
		return ProxyEntry{}, false
	}
	return entry, true
}

func EffectiveDomainEntry(entry DomainEntry, defaults ProxyEntryDefaults) (ProxyEntry, bool) {
	result := ProxyEntry{
		BindHost: NormalizeBindHost(entry.BindHost),
		Port:     entry.Port,
		Network:  ListenerNetworkTCP,
	}
	switch entry.Protocol {
	case DomainEntryHTTP:
		result.Protocol = ListenerProtocolHTTP
		if result.BindHost == "" {
			result.BindHost = NormalizeBindHost(defaults.HTTPBindHost)
		}
		if result.Port == 0 {
			result.Port = defaults.HTTPPort
		}
	case DomainEntryHTTPS:
		result.Protocol = ListenerProtocolHTTPS
		if result.BindHost == "" {
			result.BindHost = NormalizeBindHost(defaults.HTTPSBindHost)
		}
		if result.Port == 0 {
			result.Port = defaults.HTTPSPort
		}
	default:
		return ProxyEntry{}, false
	}
	if result.Port <= 0 || result.Port > 65535 {
		return ProxyEntry{}, false
	}
	return result, true
}

func NormalizeBindHost(host string) string {
	host = trimIPv6Brackets(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.ToLower(host)
}

func NormalizeRouteHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func ValidBindHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	if strings.Contains(host, "://") || strings.Contains(host, "/") {
		return false
	}
	host = trimIPv6Brackets(host)
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	if strings.Contains(host, ":") {
		return false
	}
	return validHostname(host)
}

func ListenAddress(host string, port int) string {
	return net.JoinHostPort(NormalizeBindHost(host), strconv.Itoa(port))
}

func ParseListenAddress(address string) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, fmt.Errorf("port is invalid")
	}
	return NormalizeBindHost(host), port, nil
}

func IsWildcardBindHost(host string) bool {
	host = NormalizeBindHost(host)
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

func trimIPv6Brackets(host string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "["), "]")
}
