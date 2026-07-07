package sdk

import (
	"strings"
)

// Config holds the configuration for an SDK client. All fields are required
// unless noted otherwise.
type Config struct {
	// ServerAddress is the primary control channel address, typically a QUIC
	// endpoint (e.g., "control.example.com:8443").
	ServerAddress string

	// ServerTLSAddress is the fallback TCP+TLS control channel address.
	// If empty, only the primary address (QUIC) is used.
	ServerTLSAddress string

	// ServerName is the TLS server name for certificate verification.
	ServerName string

	// ServerCAFile is the path to the CA certificate file for verifying the
	// server's TLS certificate.
	ServerCAFile string

	// ClientID is the consumer client ID issued by the GoGinX admin.
	ClientID string

	// Credential is the consumer client credential.
	Credential string

	// AllowedProtocols lists the protocols the SDK may attempt. Supported
	// values are "quic" and "tcp_tls". If empty, both are attempted in order.
	AllowedProtocols []string
}

func (c Config) validate() error {
	if strings.TrimSpace(c.ServerAddress) == "" && strings.TrimSpace(c.ServerTLSAddress) == "" {
		return &ConfigError{Field: "ServerAddress", Message: "at least one of ServerAddress or ServerTLSAddress is required"}
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return &ConfigError{Field: "ClientID", Message: "is required"}
	}
	if strings.TrimSpace(c.Credential) == "" {
		return &ConfigError{Field: "Credential", Message: "is required"}
	}
	if strings.TrimSpace(c.ServerName) == "" {
		return &ConfigError{Field: "ServerName", Message: "is required"}
	}
	if strings.TrimSpace(c.ServerCAFile) == "" {
		return &ConfigError{Field: "ServerCAFile", Message: "is required"}
	}
	return nil
}
