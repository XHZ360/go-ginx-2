package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
)

type ListenerReconciler interface {
	ReconcileProxyListeners(ctx context.Context) error
}

type ProxyListenerReconciler = ListenerReconciler

type CreateUserInput struct {
	ID, Username, Password string
	Role                   domain.Role
	ActorID                string
}
type CreateClientInput struct {
	ID, UserID, Name    string
	Kind                domain.ClientKind
	Credential, ActorID string
}
type CreateClientResult struct {
	Client     domain.Client
	Credential string
}
type CreateClientJoinInput struct {
	ID, UserID, Name, ActorID                                                string
	EnrollmentURL, ServerAddress, ServerTLSAddress, ServerName, ServerCAFile string
	AllowedProtocols                                                         []domain.Protocol
	Reconnect                                                                config.Reconnect
	TTL                                                                      time.Duration
}
type CreateClientJoinResult struct {
	Client domain.Client
	Token  string
}
type ReviewClientJoinTokenResult struct {
	Client    domain.Client
	Token     string
	ExpiresAt time.Time
}

type CreateProxyInput struct {
	ID, UserID, ClientID, Name                             string
	Type                                                   domain.ProxyType
	DomainID, PathPrefix                                   string
	StripPrefix                                            bool
	UpstreamPathPrefix, EntryBindHost, EntryHost           string
	EntryPort                                              int
	TargetHost                                             string
	TargetPort                                             int
	CertFile, KeyFile, CertificateID, Description, ActorID string
}
type UpdateProxyInput struct {
	ID                       string
	Type                     domain.ProxyType
	Name, DomainID           string
	DomainIDSet              bool
	PathPrefix               string
	PathPrefixSet            bool
	StripPrefix              bool
	StripPrefixSet           bool
	UpstreamPathPrefix       string
	UpstreamPathPrefixSet    bool
	EntryBindHost, EntryHost string
	EntryPort                int
	TargetHost               string
	TargetPort               int
	CertFile, KeyFile        string
	CertificateID            string
	CertificateIDSet         bool
	Description, ActorID     string
}
type ProxyActivationResult struct {
	URL       string
	ExpiresAt time.Time
}

type CertificateInput struct {
	CertificateID, ProxyID    string
	ProviderType              domain.CertificateProviderType
	CredentialID, RequestType string
	RequestedValidity         int
	ActorID                   string
}
type CreateCertificateInput struct {
	Host                       string
	ProviderType               domain.CertificateProviderType
	CredentialID, RequestType  string
	RequestedValidity          int
	CertFile, KeyFile, ActorID string
}
type DeleteCertificateInput struct{ CertificateID, ConfirmHost, ConfirmCertificateID, ActorID string }
type DeleteCertificateResult struct {
	CertificateID    string
	AffectedProxyIDs []string
	RequiredConfirm  bool
}
type BindCertificateInput struct{ ProxyID, CertificateID, ActorID string }
type UnbindCertificateInput struct{ ProxyID, ActorID string }
type ProviderCredentialInput struct{ ID, Name, Scope, Token, ActorID string }
type UpdateProviderCredentialInput struct{ ID, Name, Scope, Token, ActorID string }
type RevokeOriginCACertificateInput struct{ CertificateID, ProxyID, Host, CloudflareCertificateID, ActorID string }
type RotateClientCredentialInput struct{ ClientID, ActorID string }
type RotateClientCredentialResult struct {
	Client     domain.Client
	Credential string
}

func hashAccessValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func proxyActivationURL(webDomain domain.Domain, proxy domain.Proxy, token string) string {
	_ = proxy
	return fmt.Sprintf("https://%s/.well-known/goginx/activate/%s", webDomain.Host, token)
}
func unavailableJoinTokenError() error {
	return contracterr.Conflict("join token is not available; generate a new join token", nil)
}

func providerReadinessError(err error) error {
	readinessErr, ok := certmanager.IsProviderNotReady(err)
	if !ok {
		return err
	}
	return contracterr.ProviderNotReady("certificate provider is not ready", map[string]string{"missingRequirements": strings.Join(readinessErr.Readiness.MissingRequirements, ","), "guidance": readinessErr.Readiness.Guidance})
}
func providerCredentialStorageUnavailableError() error {
	return contracterr.Unsupported("cloudflare origin ca credential storage is not configured; enable origin_ca_enabled and set origin_ca_secret_store_path")
}
func providerCredentialVerificationError(err error) error {
	if errors.Is(err, certmanager.ErrProviderCredentialDisabled) {
		return contracterr.Conflict("provider credential is disabled", err)
	}
	message := httpsproxy.SafeCertificateError(err)
	if strings.TrimSpace(message) == "" {
		message = "provider credential verification failed"
	}
	return contracterr.Conflict("provider credential verification failed: "+message, err)
}

func removeManagedFile(certificateDir, path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	absDir, err := filepath.Abs(certificateDir)
	if err != nil {
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}
	relative, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return
	}
	if relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return
	}
	_ = os.Remove(absPath)
}
func hostnameWithinCertificate(certificate domain.ManagedCertificate, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, candidate := range append([]string{certificate.Host}, certificate.Hostnames...) {
		if hostnameMatchesPattern(host, candidate) {
			return true
		}
	}
	return false
}
func hostnameMatchesPattern(host, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		if !strings.HasSuffix(host, suffix) {
			return false
		}
		label := host[:len(host)-len(suffix)]
		return label != "" && !strings.Contains(label, ".")
	}
	return false
}
func proxyRequiresListenerAdmission(proxyType domain.ProxyType) bool {
	return proxyType == domain.ProxyTCP || proxyType == domain.ProxyUDP
}
func displayBindHost(host string) string {
	if domain.NormalizeBindHost(host) == "" {
		return "*"
	}
	return domain.NormalizeBindHost(host)
}

func newCredential() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate credential: %v", err))
	}
	return hex.EncodeToString(bytes[:])
}
func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate %s id: %v", prefix, err))
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

func validateCreateUserInput(input CreateUserInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.Username) == "" {
		fields["username"] = "username is required"
	}
	if input.Role != "" && !input.Role.Valid() {
		fields["role"] = "role is invalid"
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}
func validateSetUserPassword(userID, password string) error {
	fields := map[string]string{}
	if strings.TrimSpace(userID) == "" {
		fields["id"] = "user id is required"
	}
	if strings.TrimSpace(password) == "" {
		fields["password"] = "password is required"
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}
func validateCreateClientInput(input CreateClientInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.UserID) == "" {
		fields["userId"] = "user id is required"
	}
	if strings.TrimSpace(input.Name) == "" {
		fields["name"] = "client name is required"
	}
	if !domain.NormalizeClientKind(input.Kind).Valid() {
		fields["kind"] = "client kind is invalid"
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}
func validateCreateProxyInput(input CreateProxyInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.UserID) == "" {
		fields["userId"] = "user id is required"
	}
	if strings.TrimSpace(input.ClientID) == "" {
		fields["clientId"] = "client id is required"
	}
	if strings.TrimSpace(input.Name) == "" {
		fields["name"] = "proxy name is required"
	}
	if !input.Type.Valid() {
		fields["type"] = "proxy type is invalid"
	}
	if strings.TrimSpace(input.TargetHost) == "" {
		fields["targetHost"] = "proxy target host is required"
	}
	if input.TargetPort <= 0 || input.TargetPort > 65535 {
		fields["targetPort"] = "proxy target port is invalid"
	}
	_ = collectProxyEntryFieldErrors(fields, input.Type, input.EntryBindHost, input.EntryHost, input.EntryPort, input.CertFile, input.KeyFile)
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}
func validateUpdateProxyInput(input UpdateProxyInput) error {
	fields := map[string]string{}
	if strings.TrimSpace(input.ID) == "" {
		fields["id"] = "proxy id is required"
	}
	if strings.TrimSpace(input.Name) == "" {
		fields["name"] = "proxy name is required"
	}
	if strings.TrimSpace(input.TargetHost) == "" {
		fields["targetHost"] = "proxy target host is required"
	}
	if input.TargetPort <= 0 || input.TargetPort > 65535 {
		fields["targetPort"] = "proxy target port is invalid"
	}
	if input.Type != "" && !input.Type.Valid() {
		fields["type"] = "proxy type is invalid"
	}
	if input.Type.Valid() {
		_ = collectProxyEntryFieldErrors(fields, input.Type, input.EntryBindHost, input.EntryHost, input.EntryPort, input.CertFile, input.KeyFile)
	}
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}
func validateProxyEntryFields(proxyType domain.ProxyType, entryBindHost, entryHost string, entryPort int, certFile, keyFile string) error {
	fields := map[string]string{}
	_ = collectProxyEntryFieldErrors(fields, proxyType, entryBindHost, entryHost, entryPort, certFile, keyFile)
	if len(fields) > 0 {
		return contracterr.Validation("validation failed", fields)
	}
	return nil
}
func collectProxyEntryFieldErrors(fields map[string]string, proxyType domain.ProxyType, entryBindHost, entryHost string, entryPort int, certFile, keyFile string) error {
	if strings.TrimSpace(entryBindHost) != "" && !domain.ValidBindHost(entryBindHost) {
		fields["entryBindHost"] = "proxy entry bind host is invalid"
	}
	switch proxyType {
	case domain.ProxyTCP, domain.ProxyUDP:
		if entryPort <= 0 || entryPort > 65535 {
			fields["entryPort"] = fmt.Sprintf("%s proxy entry port is required", proxyType)
		}
	case domain.ProxyHTTP, domain.ProxyHTTPS:
		if strings.TrimSpace(entryHost) == "" {
			fields["entryHost"] = fmt.Sprintf("%s proxy route host is required", proxyType)
		} else if !domain.ValidBindHost(entryHost) {
			fields["entryHost"] = fmt.Sprintf("%s proxy route host is invalid", proxyType)
		}
		if entryPort < 0 || entryPort > 65535 {
			fields["entryPort"] = fmt.Sprintf("%s proxy entry port is invalid", proxyType)
		}
	}
	if proxyType == domain.ProxyHTTPS && (strings.TrimSpace(certFile) == "") != (strings.TrimSpace(keyFile) == "") {
		fields["certFile"] = "https proxy cert file and key file must be provided together"
		fields["keyFile"] = "https proxy cert file and key file must be provided together"
	}
	return nil
}
