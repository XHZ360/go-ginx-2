package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
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

type ClientKind string

const (
	ClientKindProvider ClientKind = "provider"
	ClientKindConsumer ClientKind = "consumer"
)

func (kind ClientKind) Valid() bool {
	switch kind {
	case ClientKindProvider, ClientKindConsumer:
		return true
	default:
		return false
	}
}

// NormalizeClientKind treats empty kind as provider for backward compatibility.
func NormalizeClientKind(kind ClientKind) ClientKind {
	if kind == "" {
		return ClientKindProvider
	}
	return kind
}

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
	ProxyHTTP    ProxyType = "http"  // legacy; migrated to ProxyWeb
	ProxyHTTPS   ProxyType = "https" // legacy; migrated to ProxyWeb
	ProxyWeb     ProxyType = "web"
	ProxyForward ProxyType = "forward"
)

func (proxyType ProxyType) Valid() bool {
	switch proxyType {
	case ProxyTCP, ProxyUDP, ProxyHTTP, ProxyHTTPS, ProxyWeb, ProxyForward:
		return true
	default:
		return false
	}
}

func (proxyType ProxyType) IsWeb() bool {
	return proxyType == ProxyWeb || proxyType == ProxyHTTP || proxyType == ProxyHTTPS
}

type DomainStatus string

const (
	DomainEnabled  DomainStatus = "enabled"
	DomainDisabled DomainStatus = "disabled"
)

type DomainEntryProtocol string

const (
	DomainEntryHTTP  DomainEntryProtocol = "http"
	DomainEntryHTTPS DomainEntryProtocol = "https"
)

func (protocol DomainEntryProtocol) Valid() bool {
	switch protocol {
	case DomainEntryHTTP, DomainEntryHTTPS:
		return true
	default:
		return false
	}
}

type DomainEntryStatus string

const (
	DomainEntryEnabled  DomainEntryStatus = "enabled"
	DomainEntryDisabled DomainEntryStatus = "disabled"
)

type Domain struct {
	ID            string
	UserID        string
	Host          string
	CertificateID string
	Status        DomainStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DomainEntry struct {
	ID        string
	DomainID  string
	Protocol  DomainEntryProtocol
	BindHost  string
	Port      int
	Status    DomainEntryStatus
	CreatedAt time.Time
	UpdatedAt time.Time
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

type CertificateStatus string

const (
	CertificatePending       CertificateStatus = "pending"
	CertificateValid         CertificateStatus = "valid"
	CertificateExpiringSoon  CertificateStatus = "expiring_soon"
	CertificateExpired       CertificateStatus = "expired"
	CertificateIssueFailed   CertificateStatus = "issue_failed"
	CertificateRenewalFailed CertificateStatus = "renewal_failed"
	CertificateDisabled      CertificateStatus = "disabled"
)

type CertificateServingStatus string

const (
	CertificateServingUsable       CertificateServingStatus = "usable"
	CertificateServingExpiringSoon CertificateServingStatus = "expiring_soon"
	CertificateServingExpired      CertificateServingStatus = "expired"
	CertificateServingMissing      CertificateServingStatus = "missing"
	CertificateServingInvalid      CertificateServingStatus = "invalid"
)

type CertificateOperationStatus string

const (
	CertificateOperationIdle          CertificateOperationStatus = "idle"
	CertificateOperationIssuing       CertificateOperationStatus = "issuing"
	CertificateOperationRenewing      CertificateOperationStatus = "renewing"
	CertificateOperationIssueFailed   CertificateOperationStatus = "issue_failed"
	CertificateOperationRenewalFailed CertificateOperationStatus = "renewal_failed"
)

type CertificateProviderType string

const (
	CertificateProviderACMEDNS01          CertificateProviderType = "acme_dns01"
	CertificateProviderCloudflareOriginCA CertificateProviderType = "cloudflare_origin_ca"
	// CertificateProviderFile 表示由静态证书文件（cert_file/key_file）迁移而来的文件型证书。
	CertificateProviderFile CertificateProviderType = "file"
)

type CertificateProviderStatus string

const (
	CertificateProviderStatusActive        CertificateProviderStatus = "active"
	CertificateProviderStatusRevoked       CertificateProviderStatus = "revoked"
	CertificateProviderStatusMissingRemote CertificateProviderStatus = "missing_remote"
	CertificateProviderStatusUnknown       CertificateProviderStatus = "unknown"
)

type ProviderCredentialStatus string

const (
	ProviderCredentialPending            ProviderCredentialStatus = "pending"
	ProviderCredentialVerified           ProviderCredentialStatus = "verified"
	ProviderCredentialVerificationFailed ProviderCredentialStatus = "verification_failed"
	ProviderCredentialDisabled           ProviderCredentialStatus = "disabled"
)

func (status CertificateStatus) Valid() bool {
	switch status {
	case CertificatePending, CertificateValid, CertificateExpiringSoon, CertificateExpired, CertificateIssueFailed, CertificateRenewalFailed, CertificateDisabled:
		return true
	default:
		return false
	}
}

func (status CertificateServingStatus) Valid() bool {
	switch status {
	case CertificateServingUsable, CertificateServingExpiringSoon, CertificateServingExpired, CertificateServingMissing, CertificateServingInvalid:
		return true
	default:
		return false
	}
}

func (status CertificateServingStatus) ServesTLS() bool {
	return status == CertificateServingUsable || status == CertificateServingExpiringSoon
}

func (status CertificateOperationStatus) Valid() bool {
	switch status {
	case CertificateOperationIdle, CertificateOperationIssuing, CertificateOperationRenewing, CertificateOperationIssueFailed, CertificateOperationRenewalFailed:
		return true
	default:
		return false
	}
}

func (provider CertificateProviderType) Valid() bool {
	switch provider {
	case CertificateProviderACMEDNS01, CertificateProviderCloudflareOriginCA, CertificateProviderFile:
		return true
	default:
		return false
	}
}

func (status CertificateProviderStatus) Valid() bool {
	switch status {
	case CertificateProviderStatusActive, CertificateProviderStatusRevoked, CertificateProviderStatusMissingRemote, CertificateProviderStatusUnknown:
		return true
	default:
		return false
	}
}

func (status CertificateProviderStatus) BlocksServing() bool {
	return status == CertificateProviderStatusRevoked || status == CertificateProviderStatusMissingRemote
}

func (status ProviderCredentialStatus) Valid() bool {
	switch status {
	case ProviderCredentialPending, ProviderCredentialVerified, ProviderCredentialVerificationFailed, ProviderCredentialDisabled:
		return true
	default:
		return false
	}
}

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         Role
	Status       UserStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Client struct {
	ID             string
	UserID         string
	Name           string
	Kind           ClientKind
	Status         ClientStatus
	CredentialHash string
	Version        int64
	LastOnlineAt   *time.Time
	LastOfflineAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ClientEnrollment struct {
	ID         string
	ClientID   string
	SecretHash string
	TokenHash  string
	Token      string
	ExpiresAt  time.Time
	UsedAt     *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Proxy struct {
	ID                 string
	UserID             string
	ClientID           string
	Name               string
	Type               ProxyType
	Status             ProxyStatus
	DomainID           string // Web Proxy 必填
	PathPrefix         string // Web Proxy 必填
	StripPrefix        bool
	UpstreamPathPrefix string
	EntryBindHost      string // TCP/UDP
	EntryHost          string // legacy only
	EntryPort          int    // TCP/UDP
	TargetHost         string
	TargetPort         int
	CertFile           string // legacy only
	KeyFile            string // legacy only
	CertificateID      string // legacy only; Domain.CertificateID 为权威绑定
	AccessAuthEnabled  bool
	AccessAuthVersion  int64
	// StatsLegacyAggregate 标记迁移后保留在 / Proxy 上的历史累计统计含全部旧路径流量。
	StatsLegacyAggregate bool
	Description          string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ProxyActivationToken struct {
	ID          string
	ProxyID     string
	AuthVersion int64
	TokenHash   string
	ExpiresAt   time.Time
	UsedAt      *time.Time
	CreatedAt   time.Time
	CreatedBy   string
}

type ProxyAccessCredential struct {
	ID          string
	ProxyID     string
	AuthVersion int64
	SecretHash  string
	CreatedAt   time.Time
	LastUsedAt  *time.Time
}

const ReservedProxyRoutePrefix = "/.well-known/goginx/"

func (domain Domain) Validate() error {
	if strings.TrimSpace(domain.ID) == "" {
		return errors.New("domain id is required")
	}
	if strings.TrimSpace(domain.UserID) == "" {
		return errors.New("domain user id is required")
	}
	host := NormalizeRouteHost(domain.Host)
	if host == "" || !validHostname(host) {
		return errors.New("domain host is invalid")
	}
	if domain.Status != DomainEnabled && domain.Status != DomainDisabled {
		return errors.New("domain status is invalid")
	}
	return nil
}

func (entry DomainEntry) Validate() error {
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.DomainID) == "" {
		return errors.New("domain entry identifiers are required")
	}
	if !entry.Protocol.Valid() {
		return errors.New("domain entry protocol is invalid")
	}
	if !ValidBindHost(entry.BindHost) {
		return errors.New("domain entry bind host is invalid")
	}
	if entry.Port <= 0 || entry.Port > 65535 {
		return errors.New("domain entry port is invalid")
	}
	if entry.Status != DomainEntryEnabled && entry.Status != DomainEntryDisabled {
		return errors.New("domain entry status is invalid")
	}
	return nil
}

func NormalizeProxyRoutePrefix(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#%\\\x00") || strings.Contains(value, "//") {
		return "", errors.New("proxy route path prefix is invalid")
	}
	if len(value) > 1 {
		value = strings.TrimRight(value, "/")
	}
	return value, nil
}

func NormalizeProxyUpstreamPathPrefix(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/", nil
	}
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#%\\\x00") || strings.Contains(value, "//") {
		return "", errors.New("proxy route upstream path prefix is invalid")
	}
	if len(value) > 1 {
		value = strings.TrimRight(value, "/")
	}
	return value, nil
}

func ProxyRouteMatches(path string, prefix string) bool {
	return prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/")
}

func SelectWebProxy(proxies []Proxy, path string) (Proxy, bool) {
	var selected Proxy
	found := false
	for _, proxy := range proxies {
		if proxy.Status != ProxyEnabled || !proxy.Type.IsWeb() || !ProxyRouteMatches(path, proxy.PathPrefix) {
			continue
		}
		if !found || len(proxy.PathPrefix) > len(selected.PathPrefix) {
			selected = proxy
			found = true
		}
	}
	return selected, found
}

func ValidProxyRequestPath(path string, rawPath string) bool {
	if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsRune(path, '\x00') {
		return false
	}
	rawPath = strings.ToLower(rawPath)
	return !strings.Contains(rawPath, "%2f") && !strings.Contains(rawPath, "%5c") && !strings.Contains(rawPath, "%00")
}

func RewriteWebProxyPath(path string, pathPrefix string, stripPrefix bool, upstreamPathPrefix string) string {
	if !stripPrefix {
		return path
	}
	remainder := strings.TrimPrefix(path, pathPrefix)
	if remainder == "" {
		return upstreamPathPrefix
	}
	if upstreamPathPrefix == "/" {
		return remainder
	}
	return strings.TrimRight(upstreamPathPrefix, "/") + "/" + strings.TrimLeft(remainder, "/")
}

type ManagedCertificate struct {
	ID                              string
	ProxyID                         string
	Host                            string
	Status                          CertificateStatus
	ServingStatus                   CertificateServingStatus
	OperationStatus                 CertificateOperationStatus
	Provider                        string
	ProviderType                    CertificateProviderType
	ProviderName                    string
	CredentialID                    string
	ProviderStatus                  CertificateProviderStatus
	CloudflareCertificateID         string
	PreviousCloudflareCertificateID string
	Hostnames                       []string
	RequestType                     string
	RequestedValidity               int
	CertFile                        string
	KeyFile                         string
	PreviousCertFile                string
	PreviousKeyFile                 string
	NotAfter                        *time.Time
	LastIssuedAt                    *time.Time
	LastRenewedAt                   *time.Time
	LastCheckedAt                   *time.Time
	LastSyncedAt                    *time.Time
	LastAttemptedAt                 *time.Time
	NextAttemptAt                   *time.Time
	FailureCount                    int
	Fingerprint                     string
	LastError                       string
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

type ACMEProviderSettings struct {
	DirectoryURL        string
	AccountEmail        string
	TermsAccepted       bool
	RenewalWindow       time.Duration
	DNSProvider         string
	DNSProviderTokenEnv string
}

type OriginCAProviderSettings struct {
	Enabled            bool
	SecretStorePath    string
	DefaultRequestType string
	RequestedValidity  int
	RotationWindow     time.Duration
}

type ProviderCredential struct {
	ID               string
	Name             string
	ProviderType     CertificateProviderType
	Scope            string
	TokenFingerprint string
	SecretRef        string
	Status           ProviderCredentialStatus
	LastVerifiedAt   *time.Time
	LastError        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CertificateOperationResult struct {
	Status           CertificateStatus
	ServingStatus    CertificateServingStatus
	OperationStatus  CertificateOperationStatus
	ProviderStatus   CertificateProviderStatus
	CertFile         string
	KeyFile          string
	PreviousCertFile string
	PreviousKeyFile  string
	NotAfter         *time.Time
	LastCheckedAt    *time.Time
	LastAttemptedAt  *time.Time
	NextAttemptAt    *time.Time
	FailureCount     int
	Fingerprint      string
	ErrorSummary     string
	CompletedAt      time.Time
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
	kind := NormalizeClientKind(client.Kind)
	if !kind.Valid() {
		return errors.New("client kind is invalid")
	}
	return nil
}

func (enrollment ClientEnrollment) Validate() error {
	if strings.TrimSpace(enrollment.ID) == "" {
		return errors.New("client enrollment id is required")
	}
	if strings.TrimSpace(enrollment.ClientID) == "" {
		return errors.New("client enrollment client id is required")
	}
	if strings.TrimSpace(enrollment.SecretHash) == "" {
		return errors.New("client enrollment secret hash is required")
	}
	if strings.TrimSpace(enrollment.TokenHash) == "" {
		return errors.New("client enrollment token hash is required")
	}
	if strings.TrimSpace(enrollment.Token) == "" {
		return errors.New("client enrollment token is required")
	}
	if enrollment.ExpiresAt.IsZero() {
		return errors.New("client enrollment expiry is required")
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
	if proxy.Type == ProxyWeb {
		if strings.TrimSpace(proxy.DomainID) == "" {
			return errors.New("web proxy domain id is required")
		}
		pathPrefix, err := NormalizeProxyRoutePrefix(proxy.PathPrefix)
		if err != nil {
			return err
		}
		if strings.HasPrefix(pathPrefix, ReservedProxyRoutePrefix) || pathPrefix == strings.TrimSuffix(ReservedProxyRoutePrefix, "/") {
			return errors.New("proxy path prefix is reserved")
		}
		if _, err := NormalizeProxyUpstreamPathPrefix(proxy.UpstreamPathPrefix); err != nil {
			return err
		}
	}
	return nil
}

func (certificate ManagedCertificate) Validate() error {
	if strings.TrimSpace(certificate.ID) == "" {
		return errors.New("certificate id is required")
	}
	// ProxyID 允许为空：证书现在是独立资源，可在未被任何代理绑定前存在（未绑定证书合法）。
	if strings.TrimSpace(certificate.Host) == "" || !validCertificateHost(certificate.Host) {
		return errors.New("certificate host is invalid")
	}
	if !certificate.Status.Valid() {
		return errors.New("certificate status is invalid")
	}
	if certificate.ServingStatus != "" && !certificate.ServingStatus.Valid() {
		return errors.New("certificate serving status is invalid")
	}
	if certificate.OperationStatus != "" && !certificate.OperationStatus.Valid() {
		return errors.New("certificate operation status is invalid")
	}
	if certificate.ProviderType != "" && !certificate.ProviderType.Valid() {
		return errors.New("certificate provider type is invalid")
	}
	if certificate.ProviderStatus != "" && !certificate.ProviderStatus.Valid() {
		return errors.New("certificate provider status is invalid")
	}
	if certificate.FailureCount < 0 {
		return errors.New("certificate failure count is invalid")
	}
	return nil
}

func (credential ProviderCredential) Validate() error {
	if strings.TrimSpace(credential.ID) == "" {
		return errors.New("provider credential id is required")
	}
	if strings.TrimSpace(credential.Name) == "" {
		return errors.New("provider credential name is required")
	}
	if !credential.ProviderType.Valid() {
		return errors.New("provider credential type is invalid")
	}
	if !credential.Status.Valid() {
		return errors.New("provider credential status is invalid")
	}
	if strings.TrimSpace(credential.SecretRef) == "" {
		return errors.New("provider credential secret ref is required")
	}
	return nil
}

func HashCredential(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPasswordHash(password string, hash string) bool {
	if strings.TrimSpace(password) == "" || strings.TrimSpace(hash) == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
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

func validCertificateHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "*.") {
		return validHostname(strings.TrimPrefix(host, "*."))
	}
	return validHostname(host)
}
