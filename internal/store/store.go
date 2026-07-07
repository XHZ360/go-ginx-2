package store

import (
	"context"
	"errors"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
)

type Store interface {
	Users() UserRepository
	Clients() ClientRepository
	ClientEnrollments() ClientEnrollmentRepository
	Proxies() ProxyRepository
	Certificates() CertificateRepository
	ProviderCredentials() ProviderCredentialRepository
	Stats() StatsRepository
	AuditEvents() AuditRepository
	Close() error
}

type ProxyStats struct {
	ProxyID           string
	TCPConnections    int64
	TCPUploadBytes    int64
	TCPDownloadBytes  int64
	TCPErrors         int64
	UDPPackets        int64
	UDPUploadBytes    int64
	UDPDownloadBytes  int64
	UDPErrors         int64
	HTTPRequests      int64
	HTTPUploadBytes   int64
	HTTPDownloadBytes int64
	HTTPErrors        int64
	HTTPStatusCodes   map[int]int64
}

type CertificateSuccess struct {
	CertFile             string
	KeyFile              string
	PreviousCertFile     string
	PreviousKeyFile      string
	NotAfter             time.Time
	ServingStatus        domain.CertificateServingStatus
	ProviderStatus       domain.CertificateProviderStatus
	ProviderType         domain.CertificateProviderType
	ProviderName         string
	CredentialID         string
	CloudflareID         string
	PreviousCloudflareID string
	Hostnames            []string
	RequestType          string
	RequestedValidity    int
	Fingerprint          string
	LastCheckedAt        time.Time
	LastAttemptedAt      time.Time
	LastSyncedAt         *time.Time
	CompletedAt          time.Time
}

type CertificateFailure struct {
	Status          domain.CertificateStatus
	ServingStatus   domain.CertificateServingStatus
	OperationStatus domain.CertificateOperationStatus
	ProviderStatus  domain.CertificateProviderStatus
	LastError       string
	LastCheckedAt   time.Time
	LastAttemptedAt time.Time
	LastSyncedAt    *time.Time
	NextAttemptAt   *time.Time
	FailureCount    int
	CompletedAt     time.Time
}

type CertificateHealth struct {
	ServingStatus domain.CertificateServingStatus
	NotAfter      *time.Time
	Fingerprint   string
	LastError     string
	CheckedAt     time.Time
}

type CertificateProviderSync struct {
	ProviderStatus domain.CertificateProviderStatus
	LastError      string
	SyncedAt       time.Time
	UpdatedAt      time.Time
}

type CertificateLifecycleCandidateQuery struct {
	Now            time.Time
	ACMEBefore     *time.Time
	OriginCABefore *time.Time
}

type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	ByID(ctx context.Context, id string) (domain.User, error)
	ByUsername(ctx context.Context, username string) (domain.User, error)
	List(ctx context.Context) ([]domain.User, error)
	SetStatus(ctx context.Context, id string, status domain.UserStatus) error
	SetPassword(ctx context.Context, id string, passwordHash string) error
	Delete(ctx context.Context, id string) error
}

type ClientRepository interface {
	Create(ctx context.Context, client domain.Client) error
	ByID(ctx context.Context, id string) (domain.Client, error)
	List(ctx context.Context) ([]domain.Client, error)
	SetStatus(ctx context.Context, id string, status domain.ClientStatus) error
	RotateCredential(ctx context.Context, id string, credentialHash string) error
	Delete(ctx context.Context, id string) error
}

type ClientEnrollmentRepository interface {
	Create(ctx context.Context, enrollment domain.ClientEnrollment) error
	ByID(ctx context.Context, id string) (domain.ClientEnrollment, error)
	LatestReviewableByClientID(ctx context.Context, clientID string, now time.Time) (domain.ClientEnrollment, error)
	LatestUnusedByClientID(ctx context.Context, clientID string) (domain.ClientEnrollment, error)
	MarkUsed(ctx context.Context, id string, usedAt time.Time) error
}

type ProxyRepository interface {
	Create(ctx context.Context, proxy domain.Proxy) error
	ByID(ctx context.Context, id string) (domain.Proxy, error)
	List(ctx context.Context) ([]domain.Proxy, error)
	ByClientID(ctx context.Context, clientID string) ([]domain.Proxy, error)
	ByUserID(ctx context.Context, userID string) ([]domain.Proxy, error)
	EnabledByType(ctx context.Context, proxyType domain.ProxyType) ([]domain.Proxy, error)
	ByTCPEntry(ctx context.Context, bindHost string, port int, includeDefault bool) (domain.Proxy, error)
	ByUDPEntry(ctx context.Context, bindHost string, port int, includeDefault bool) (domain.Proxy, error)
	ByTCPEntryPort(ctx context.Context, port int) (domain.Proxy, error)
	ByUDPEntryPort(ctx context.Context, port int) (domain.Proxy, error)
	ByHTTPRoute(ctx context.Context, bindHost string, port int, host string, includeDefault bool) (domain.Proxy, error)
	ByHTTPSRoute(ctx context.Context, bindHost string, port int, host string, includeDefault bool) (domain.Proxy, error)
	ByHTTPHost(ctx context.Context, host string) (domain.Proxy, error)
	ByHTTPSHost(ctx context.Context, host string) (domain.Proxy, error)
	ByCertificateID(ctx context.Context, certificateID string) (domain.Proxy, error)
	Update(ctx context.Context, proxy domain.Proxy) error
	SetStatus(ctx context.Context, id string, status domain.ProxyStatus) error
	Delete(ctx context.Context, id string) error
}

type CertificateRepository interface {
	Create(ctx context.Context, certificate domain.ManagedCertificate) error
	ByID(ctx context.Context, id string) (domain.ManagedCertificate, error)
	ByProxyID(ctx context.Context, proxyID string) (domain.ManagedCertificate, error)
	ByHost(ctx context.Context, host string) (domain.ManagedCertificate, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]domain.ManagedCertificate, error)
	ListByProxyIDs(ctx context.Context, proxyIDs []string) ([]domain.ManagedCertificate, error)
	ListRenewable(ctx context.Context, before time.Time, now time.Time) ([]domain.ManagedCertificate, error)
	ListLifecycleCandidates(ctx context.Context, query CertificateLifecycleCandidateQuery) ([]domain.ManagedCertificate, error)
	UpdateSuccess(ctx context.Context, id string, result CertificateSuccess) error
	UpdateFailure(ctx context.Context, id string, failure CertificateFailure) error
	UpdateHealth(ctx context.Context, id string, health CertificateHealth) error
	UpdateProviderSync(ctx context.Context, id string, sync CertificateProviderSync) error
}

type ProviderCredentialRepository interface {
	Create(ctx context.Context, credential domain.ProviderCredential) error
	ByID(ctx context.Context, id string) (domain.ProviderCredential, error)
	List(ctx context.Context) ([]domain.ProviderCredential, error)
	ListByProviderType(ctx context.Context, providerType domain.CertificateProviderType, statuses []domain.ProviderCredentialStatus) ([]domain.ProviderCredential, error)
	Update(ctx context.Context, credential domain.ProviderCredential) error
	SetStatus(ctx context.Context, id string, status domain.ProviderCredentialStatus, lastVerifiedAt *time.Time, lastError string) error
	Delete(ctx context.Context, id string) error
}

type AuditRepository interface {
	Create(ctx context.Context, event domain.AuditEvent) error
	ListRecent(ctx context.Context, limit int) ([]domain.AuditEvent, error)
}

type StatsRepository interface {
	Save(ctx context.Context, stats []ProxyStats) error
	List(ctx context.Context) ([]ProxyStats, error)
}
