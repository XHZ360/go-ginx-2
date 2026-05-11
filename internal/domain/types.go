package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"
)

type Protocol string

const (
	ProtocolQUIC   Protocol = "quic"
	ProtocolTCPTLS Protocol = "tcp_tls"
)

func (protocol Protocol) Valid() bool {
	switch protocol {
	case ProtocolQUIC, ProtocolTCPTLS:
		return true
	default:
		return false
	}
}

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

func (role Role) Valid() bool {
	switch role {
	case RoleAdmin, RoleUser:
		return true
	default:
		return false
	}
}

type UserStatus string

const (
	UserEnabled  UserStatus = "enabled"
	UserDisabled UserStatus = "disabled"
)

type ClientStatus string

const (
	ClientOffline      ClientStatus = "offline"
	ClientOnline       ClientStatus = "online"
	ClientAuthFailed   ClientStatus = "auth_failed"
	ClientDisabled     ClientStatus = "disabled"
	ClientConnecting   ClientStatus = "connecting"
	ClientDisconnected ClientStatus = "disconnected"
)

type ProxyType string

const (
	ProxyTCP     ProxyType = "tcp"
	ProxyUDP     ProxyType = "udp"
	ProxyHTTP    ProxyType = "http"
	ProxyHTTPS   ProxyType = "https"
	ProxyForward ProxyType = "forward"
)

func (proxyType ProxyType) Valid() bool {
	switch proxyType {
	case ProxyTCP, ProxyUDP, ProxyHTTP, ProxyHTTPS, ProxyForward:
		return true
	default:
		return false
	}
}

type ProxyStatus string

const (
	ProxyDraft     ProxyStatus = "draft"
	ProxyPending   ProxyStatus = "pending"
	ProxyEnabled   ProxyStatus = "enabled"
	ProxyOnline    ProxyStatus = "online"
	ProxyOffline   ProxyStatus = "offline"
	ProxyDisabled  ProxyStatus = "disabled"
	ProxyError     ProxyStatus = "error"
	ProxyNeedsConf ProxyStatus = "needs_config"
)

type User struct {
	ID        string
	Username  string
	Role      Role
	Status    UserStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Client struct {
	ID             string
	UserID         string
	Name           string
	Status         ClientStatus
	CredentialHash string
	Version        int64
	LastOnlineAt   *time.Time
	LastOfflineAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Proxy struct {
	ID          string
	UserID      string
	ClientID    string
	Name        string
	Type        ProxyType
	Status      ProxyStatus
	EntryHost   string
	EntryPort   int
	TargetHost  string
	TargetPort  int
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type AuditEvent struct {
	ID           string
	ActorUserID  string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	SourceIP     string
	ErrorSummary string
	CreatedAt    time.Time
}

func (user User) Validate() error {
	if strings.TrimSpace(user.ID) == "" {
		return errors.New("user id is required")
	}
	if strings.TrimSpace(user.Username) == "" {
		return errors.New("username is required")
	}
	if !user.Role.Valid() {
		return errors.New("user role is invalid")
	}
	if user.Status != UserEnabled && user.Status != UserDisabled {
		return errors.New("user status is invalid")
	}
	return nil
}

func (client Client) Validate() error {
	if strings.TrimSpace(client.ID) == "" {
		return errors.New("client id is required")
	}
	if strings.TrimSpace(client.UserID) == "" {
		return errors.New("client user id is required")
	}
	if strings.TrimSpace(client.Name) == "" {
		return errors.New("client name is required")
	}
	if strings.TrimSpace(client.CredentialHash) == "" {
		return errors.New("client credential hash is required")
	}
	return nil
}

func (proxy Proxy) Validate() error {
	if strings.TrimSpace(proxy.ID) == "" {
		return errors.New("proxy id is required")
	}
	if strings.TrimSpace(proxy.UserID) == "" {
		return errors.New("proxy user id is required")
	}
	if strings.TrimSpace(proxy.ClientID) == "" {
		return errors.New("proxy client id is required")
	}
	if strings.TrimSpace(proxy.Name) == "" {
		return errors.New("proxy name is required")
	}
	if !proxy.Type.Valid() {
		return errors.New("proxy type is invalid")
	}
	if proxy.EntryPort < 0 || proxy.EntryPort > 65535 {
		return errors.New("proxy entry port is invalid")
	}
	if proxy.TargetPort <= 0 || proxy.TargetPort > 65535 {
		return errors.New("proxy target port is invalid")
	}
	if strings.TrimSpace(proxy.TargetHost) == "" {
		return errors.New("proxy target host is required")
	}
	if ip := net.ParseIP(proxy.TargetHost); ip == nil && !validHostname(proxy.TargetHost) {
		return errors.New("proxy target host is invalid")
	}
	return nil
}

func HashCredential(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

func validHostname(hostname string) bool {
	if len(hostname) > 253 {
		return false
	}
	for part := range strings.SplitSeq(hostname, ".") {
		if part == "" || len(part) > 63 {
			return false
		}
		for index, char := range part {
			isLetter := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
			isDigit := char >= '0' && char <= '9'
			if !(isLetter || isDigit || char == '-') {
				return false
			}
			if char == '-' && (index == 0 || index == len(part)-1) {
				return false
			}
		}
	}
	return true
}
