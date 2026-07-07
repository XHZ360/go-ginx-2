// Package sdk provides a Go client library for connecting to GoGinX control
// channels as a consumer. It enables third-party applications, Android Go
// components, or other Go programs to securely access remote services behind
// GoGinX without exposing additional public ports.
//
// The SDK connects to goginx-server using consumer client credentials, retrieves
// the list of available proxies, and opens multiplexed data streams that the
// server bridges to the appropriate provider client and its fixed proxy target.
//
// Security boundaries:
//   - The SDK cannot specify arbitrary remote targets; target host and port are
//     determined by the server-side proxy configuration.
//   - The local SOCKS5/HTTP CONNECT entry is a fixed-target tunnel, not an
//     arbitrary destination forward proxy.
//   - Credentials, private keys, and tokens are never exposed in error strings
//     or returned to the caller.
package sdk
