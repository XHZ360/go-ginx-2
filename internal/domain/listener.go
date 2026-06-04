package domain

import (
	"errors"
	"fmt"
)

var ErrEntryConflict = errors.New("entry conflict")

type ListenerClaim struct {
	Protocol   string
	Network    string
	BindHost   string
	Port       int
	Source     string
	ResourceID string
}

type ListenerAdmissionError struct {
	Proposed ListenerClaim
	Conflict ListenerClaim
}

func (claim ListenerClaim) Conflicts(other ListenerClaim) bool {
	if claim.Network != other.Network || claim.Port != other.Port {
		return false
	}
	if !bindHostsConflict(claim.BindHost, other.BindHost) {
		return false
	}
	return !shareableListenerProtocol(claim.Protocol, other.Protocol)
}

func (err *ListenerAdmissionError) Error() string {
	if err == nil {
		return ErrEntryConflict.Error()
	}
	return fmt.Sprintf("%s listener on %s:%d conflicts with %s", err.Proposed.Protocol, displayBindHost(err.Proposed.BindHost), err.Proposed.Port, err.Conflict.Source)
}

func (err *ListenerAdmissionError) Unwrap() error {
	return ErrEntryConflict
}

func FindListenerConflict(existing []ListenerClaim, proposed ListenerClaim) (ListenerClaim, bool) {
	for _, claim := range existing {
		if claim.Conflicts(proposed) {
			return claim, true
		}
	}
	return ListenerClaim{}, false
}

func ListenerClaimForProxy(proxy Proxy, defaults ...ProxyEntryDefaults) (ListenerClaim, bool) {
	var selectedDefaults ProxyEntryDefaults
	if len(defaults) > 0 {
		selectedDefaults = defaults[0]
	}
	entry, ok := EffectiveProxyEntry(proxy, selectedDefaults)
	if !ok {
		return ListenerClaim{}, false
	}
	return ListenerClaim{Protocol: entry.Protocol, Network: entry.Network, BindHost: entry.BindHost, Port: entry.Port, Source: fmt.Sprintf("proxy %s", proxy.ID), ResourceID: proxy.ID}, true
}

func bindHostsConflict(left string, right string) bool {
	left = NormalizeBindHost(left)
	right = NormalizeBindHost(right)
	return left == right || IsWildcardBindHost(left) || IsWildcardBindHost(right)
}

func shareableListenerProtocol(left string, right string) bool {
	return left == right && (left == ListenerProtocolHTTP || left == ListenerProtocolHTTPS)
}

func displayBindHost(host string) string {
	if NormalizeBindHost(host) == "" {
		return "*"
	}
	return NormalizeBindHost(host)
}
