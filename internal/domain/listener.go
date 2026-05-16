package domain

import (
	"errors"
	"fmt"
)

const (
	ListenerNetworkTCP = "tcp"
	ListenerNetworkUDP = "udp"
)

var ErrEntryConflict = errors.New("entry conflict")

type ListenerClaim struct {
	Network    string
	Port       int
	Source     string
	ResourceID string
}

type ListenerAdmissionError struct {
	Proposed ListenerClaim
	Conflict ListenerClaim
}

func (claim ListenerClaim) Conflicts(other ListenerClaim) bool {
	return claim.Network == other.Network && claim.Port == other.Port
}

func (err *ListenerAdmissionError) Error() string {
	if err == nil {
		return ErrEntryConflict.Error()
	}
	return fmt.Sprintf("%s listener on port %d conflicts with %s", err.Proposed.Network, err.Proposed.Port, err.Conflict.Source)
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

func ListenerClaimForProxy(proxy Proxy) (ListenerClaim, bool) {
	if proxy.EntryPort <= 0 {
		return ListenerClaim{}, false
	}
	claim := ListenerClaim{Port: proxy.EntryPort, Source: fmt.Sprintf("proxy %s", proxy.ID), ResourceID: proxy.ID}
	switch proxy.Type {
	case ProxyTCP:
		claim.Network = ListenerNetworkTCP
	case ProxyUDP:
		claim.Network = ListenerNetworkUDP
	default:
		return ListenerClaim{}, false
	}
	return claim, true
}
