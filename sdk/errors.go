package sdk

import (
	"errors"
	"fmt"
)

var (
	// ErrNotConnected is returned when an operation requires an active control
	// connection but the client has not been connected or has been closed.
	ErrNotConnected = errors.New("sdk: not connected")

	// ErrAlreadyConnected is returned when Connect is called on a client that
	// is already connected.
	ErrAlreadyConnected = errors.New("sdk: already connected")

	// ErrAuthenticationFailed is returned when the server rejects the client
	// credentials. The underlying reason is not included to avoid leaking
	// credential material.
	ErrAuthenticationFailed = errors.New("sdk: authentication failed")

	// ErrProxyNotFound is returned when a dial targets a proxy ID that does
	// not exist, is not accessible by this consumer, or is disabled.
	ErrProxyNotFound = errors.New("sdk: proxy not found or not accessible")

	// ErrDialFailed is returned when a data stream cannot be established to
	// the target proxy (e.g., provider offline, timeout, or stream error).
	ErrDialFailed = errors.New("sdk: dial failed")
)

// ConfigError describes a missing or invalid configuration field.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("sdk: config error on %s: %s", e.Field, e.Message)
}
