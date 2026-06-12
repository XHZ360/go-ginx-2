package httpsproxy

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

const (
	managedCertificateDir = "managed"
	activeCertFile        = "active.crt"
	activeKeyFile         = "active.key"
	previousCertFile      = "previous.crt"
	previousKeyFile       = "previous.key"
)

type StoredCertificateFiles struct {
	CertFile         string
	KeyFile          string
	PreviousCertFile string
	PreviousKeyFile  string
	NotAfter         time.Time
	Fingerprint      string
}

type ManagedCertificateStorage struct {
	CertificateDir string
	Now            func() time.Time
}

func (storage ManagedCertificateStorage) Store(host string, certPEM []byte, keyPEM []byte) (StoredCertificateFiles, error) {
	if strings.TrimSpace(storage.CertificateDir) == "" {
		return StoredCertificateFiles{}, errors.New("certificate dir is required")
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return StoredCertificateFiles{}, errors.New("certificate host is required")
	}
	parsed, err := ValidateCertificatePair(certificateValidationHost(host), certPEM, keyPEM, storage.now())
	if err != nil {
		return StoredCertificateFiles{}, err
	}
	dir := filepath.Join(storage.CertificateDir, managedCertificateDir, certificateStorageName(host))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return StoredCertificateFiles{}, err
	}
	certFile := filepath.Join(dir, activeCertFile)
	keyFile := filepath.Join(dir, activeKeyFile)
	previousCert := filepath.Join(dir, previousCertFile)
	previousKey := filepath.Join(dir, previousKeyFile)
	tempCert, tempKey, err := writeTempPair(dir, certPEM, keyPEM)
	if err != nil {
		return StoredCertificateFiles{}, err
	}
	defer os.Remove(tempCert)
	defer os.Remove(tempKey)
	if err := replaceActivePair(certFile, keyFile, previousCert, previousKey, tempCert, tempKey); err != nil {
		return StoredCertificateFiles{}, err
	}
	return StoredCertificateFiles{CertFile: certFile, KeyFile: keyFile, PreviousCertFile: previousCert, PreviousKeyFile: previousKey, NotAfter: parsed.NotAfter, Fingerprint: parsed.Fingerprint}, nil
}

func certificateValidationHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(host, "*.") && len(host) > 2 {
		return "wildcard-validation." + strings.TrimPrefix(host, "*.")
	}
	return host
}

func certificateStorageName(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(host, "*.") && len(host) > 2 {
		return "_wildcard." + strings.TrimPrefix(host, "*.")
	}
	return host
}

func (storage ManagedCertificateStorage) now() time.Time {
	if storage.Now != nil {
		return storage.Now()
	}
	return time.Now().UTC()
}

type ValidatedCertificate struct {
	Certificate tls.Certificate
	Leaf        *x509.Certificate
	NotAfter    time.Time
	Fingerprint string
}

func ValidateCertificatePair(host string, certPEM []byte, keyPEM []byte, now time.Time) (ValidatedCertificate, error) {
	validated, servingStatus, err := InspectCertificatePair(host, certPEM, keyPEM, now, 0)
	if err != nil {
		return ValidatedCertificate{}, err
	}
	if servingStatus == domain.CertificateServingExpired {
		return ValidatedCertificate{}, errors.New("certificate is expired")
	}
	return validated, nil
}

func InspectCertificatePair(host string, certPEM []byte, keyPEM []byte, now time.Time, renewalWindow time.Duration) (ValidatedCertificate, domain.CertificateServingStatus, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ValidatedCertificate{}, domain.CertificateServingInvalid, errors.New("certificate host is required")
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return ValidatedCertificate{}, domain.CertificateServingInvalid, err
	}
	if len(certificate.Certificate) == 0 {
		return ValidatedCertificate{}, domain.CertificateServingInvalid, errors.New("certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return ValidatedCertificate{}, domain.CertificateServingInvalid, err
	}
	if err := leaf.VerifyHostname(host); err != nil {
		return ValidatedCertificate{}, domain.CertificateServingInvalid, err
	}
	status := domain.CertificateServingUsable
	if !leaf.NotAfter.After(now) {
		status = domain.CertificateServingExpired
	} else if renewalWindow > 0 && !leaf.NotAfter.After(now.Add(renewalWindow)) {
		status = domain.CertificateServingExpiringSoon
	}
	certificate.Leaf = leaf
	return ValidatedCertificate{Certificate: certificate, Leaf: leaf, NotAfter: leaf.NotAfter, Fingerprint: LeafFingerprint(leaf)}, status, nil
}

func LeafFingerprint(leaf *x509.Certificate) string {
	if leaf == nil {
		return ""
	}
	sum := sha256.Sum256(leaf.Raw)
	return hex.EncodeToString(sum[:])
}

type CertificateMaterialHealth struct {
	ServingStatus domain.CertificateServingStatus
	NotAfter      *time.Time
	Fingerprint   string
	ErrorSummary  string
	Certificate   *tls.Certificate
}

func (health CertificateMaterialHealth) Usable() bool {
	return health.ServingStatus.ServesTLS() && health.Certificate != nil
}

func CheckCertificateFiles(host string, certPath string, keyPath string, certificateDir string, renewalWindow time.Duration, now time.Time) CertificateMaterialHealth {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(certPath) == "" || strings.TrimSpace(keyPath) == "" {
		return certificateHealthError(domain.CertificateServingMissing, errors.New("certificate active material is missing"))
	}
	certFile, err := resolveCertificateFile(certPath, certificateDir)
	if err != nil {
		return certificateHealthError(statusFromFileError(err), err)
	}
	keyFile, err := resolveCertificateFile(keyPath, certificateDir)
	if err != nil {
		return certificateHealthError(statusFromFileError(err), err)
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return certificateHealthError(statusFromFileError(err), err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return certificateHealthError(statusFromFileError(err), err)
	}
	validated, servingStatus, err := InspectCertificatePair(host, certPEM, keyPEM, now, renewalWindow)
	health := CertificateMaterialHealth{ServingStatus: servingStatus, NotAfter: &validated.NotAfter, Fingerprint: validated.Fingerprint}
	if err != nil {
		health.ServingStatus = domain.CertificateServingInvalid
		health.NotAfter = nil
		health.Fingerprint = ""
		health.ErrorSummary = SafeCertificateError(err)
		return health
	}
	if servingStatus == domain.CertificateServingExpired {
		health.ErrorSummary = SafeCertificateError(errors.New("certificate is expired"))
		return health
	}
	certificate := validated.Certificate
	health.Certificate = &certificate
	return health
}

func certificateHealthError(status domain.CertificateServingStatus, err error) CertificateMaterialHealth {
	return CertificateMaterialHealth{ServingStatus: status, ErrorSummary: safeHealthError(status, err)}
}

func statusFromFileError(err error) domain.CertificateServingStatus {
	if errors.Is(err, os.ErrNotExist) {
		return domain.CertificateServingMissing
	}
	return domain.CertificateServingInvalid
}

func SafeCertificateError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 512 {
		message = message[:512]
	}
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\r", " ")
	return message
}

func safeHealthError(status domain.CertificateServingStatus, err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	if status == domain.CertificateServingMissing || errors.Is(err, os.ErrNotExist) {
		return "certificate file is missing"
	}
	if strings.Contains(message, "symlink") {
		return "certificate file symlinks are not allowed"
	}
	if strings.Contains(message, "must be under") {
		return "certificate file is outside certificate directory"
	}
	if strings.Contains(message, "permission") || strings.Contains(message, "access is denied") {
		return "certificate file cannot be read"
	}
	return SafeCertificateError(err)
}

type CertificateResolver struct {
	store          store.Store
	certificateDir string
	mu             sync.Mutex
	cache          map[string]cachedResolvedCertificate
}

type cachedResolvedCertificate struct {
	certFile    string
	keyFile     string
	certModTime time.Time
	keyModTime  time.Time
	certificate tls.Certificate
}

func NewCertificateResolver(store store.Store, certificateDir string) *CertificateResolver {
	return &CertificateResolver{store: store, certificateDir: certificateDir, cache: make(map[string]cachedResolvedCertificate)}
}

func (resolver *CertificateResolver) Reload(host string) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	delete(resolver.cache, strings.ToLower(strings.TrimSpace(host)))
}

func (resolver *CertificateResolver) Certificate(ctx context.Context, host string, proxy domain.Proxy) (*tls.Certificate, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	certFile, keyFile, _, err := resolver.activeFiles(ctx, host, proxy)
	if err != nil {
		return nil, err
	}
	if certFile == "" || keyFile == "" {
		return nil, errors.New("certificate active material is missing")
	}
	certificate, _, err := resolver.fileCertificate(host, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &certificate, nil
}

func (resolver *CertificateResolver) activeFiles(ctx context.Context, host string, proxy domain.Proxy) (string, string, *domain.ManagedCertificate, error) {
	// 主路径：代理通过 CertificateID 显式绑定证书资源（权威绑定）。
	if strings.TrimSpace(proxy.CertificateID) != "" && resolver.store != nil {
		managed, err := resolver.store.Certificates().ByID(ctx, proxy.CertificateID)
		if err == nil {
			if managed.ProviderStatus.BlocksServing() {
				return "", "", &managed, errors.New("certificate provider marked active material unavailable")
			}
			return managed.CertFile, managed.KeyFile, &managed, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return "", "", nil, err
		}
		// 绑定的证书已不存在：落入遗留回退路径（迁移期容错）。
	}
	// 遗留回退（仅迁移期）：未绑定 CertificateID 或绑定查找失败时，沿用旧解析顺序。
	if proxy.CertFile != "" || proxy.KeyFile != "" {
		if proxy.CertFile == "" || proxy.KeyFile == "" {
			return "", "", nil, errors.New("static certificate and key files must both be configured")
		}
		return proxy.CertFile, proxy.KeyFile, nil, nil
	}
	if resolver.store == nil {
		return "", "", nil, nil
	}
	managed, err := resolver.store.Certificates().ByHost(ctx, host)
	if errors.Is(err, store.ErrNotFound) {
		return "", "", nil, nil
	}
	if err != nil {
		return "", "", nil, err
	}
	if proxy.ID != "" && managed.ProxyID != proxy.ID {
		return "", "", nil, nil
	}
	if managed.ProviderType == domain.CertificateProviderCloudflareOriginCA && managed.ProviderStatus.BlocksServing() {
		return "", "", &managed, errors.New("certificate provider marked active material unavailable")
	}
	return managed.CertFile, managed.KeyFile, &managed, nil
}

func (resolver *CertificateResolver) fileCertificate(host string, certPath string, keyPath string) (tls.Certificate, CertificateMaterialHealth, error) {
	certFile, err := resolveCertificateFile(certPath, resolver.certificateDir)
	if err != nil {
		health := certificateHealthError(statusFromFileError(err), err)
		return tls.Certificate{}, health, errors.New(health.ErrorSummary)
	}
	keyFile, err := resolveCertificateFile(keyPath, resolver.certificateDir)
	if err != nil {
		health := certificateHealthError(statusFromFileError(err), err)
		return tls.Certificate{}, health, errors.New(health.ErrorSummary)
	}
	certInfo, err := os.Stat(certFile)
	if err != nil {
		health := certificateHealthError(statusFromFileError(err), err)
		return tls.Certificate{}, health, errors.New(health.ErrorSummary)
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		health := certificateHealthError(statusFromFileError(err), err)
		return tls.Certificate{}, health, errors.New(health.ErrorSummary)
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if cached, ok := resolver.cache[host]; ok && cached.certFile == certFile && cached.keyFile == keyFile && cached.certModTime.Equal(certInfo.ModTime()) && cached.keyModTime.Equal(keyInfo.ModTime()) {
		if cached.certificate.Leaf != nil && cached.certificate.Leaf.NotAfter.After(time.Now().UTC()) {
			notAfter := cached.certificate.Leaf.NotAfter
			health := CertificateMaterialHealth{ServingStatus: domain.CertificateServingUsable, NotAfter: &notAfter, Fingerprint: LeafFingerprint(cached.certificate.Leaf), Certificate: &cached.certificate}
			return cached.certificate, health, nil
		}
		delete(resolver.cache, host)
	}
	health := CheckCertificateFiles(host, certFile, keyFile, resolver.certificateDir, 0, time.Now().UTC())
	if !health.Usable() {
		if health.ErrorSummary == "" {
			health.ErrorSummary = "certificate active material is not usable"
		}
		return tls.Certificate{}, health, errors.New(health.ErrorSummary)
	}
	certificate := *health.Certificate
	resolver.cache[host] = cachedResolvedCertificate{certFile: certFile, keyFile: keyFile, certModTime: certInfo.ModTime(), keyModTime: keyInfo.ModTime(), certificate: certificate}
	return certificate, health, nil
}

func resolveCertificateFile(path string, certificateDir string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("certificate file symlinks are not allowed")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(certificateDir) == "" {
		return resolved, nil
	}
	certificateDir, err = filepath.EvalSymlinks(certificateDir)
	if err != nil {
		return "", err
	}
	certificateDir, err = filepath.Abs(certificateDir)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(certificateDir, resolved)
	if err != nil {
		return "", err
	}
	if relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return "", fmt.Errorf("certificate file must be under %s", certificateDir)
	}
	return resolved, nil
}

func writeTempPair(dir string, certPEM []byte, keyPEM []byte) (string, string, error) {
	tempCert, err := os.CreateTemp(dir, "active-*.crt")
	if err != nil {
		return "", "", err
	}
	tempCertName := tempCert.Name()
	if _, err := tempCert.Write(certPEM); err != nil {
		_ = tempCert.Close()
		return "", "", err
	}
	if err := tempCert.Close(); err != nil {
		return "", "", err
	}
	if err := os.Chmod(tempCertName, 0o600); err != nil {
		return "", "", err
	}
	tempKey, err := os.CreateTemp(dir, "active-*.key")
	if err != nil {
		return "", "", err
	}
	tempKeyName := tempKey.Name()
	if _, err := tempKey.Write(keyPEM); err != nil {
		_ = tempKey.Close()
		return "", "", err
	}
	if err := tempKey.Close(); err != nil {
		return "", "", err
	}
	if err := os.Chmod(tempKeyName, 0o600); err != nil {
		return "", "", err
	}
	return tempCertName, tempKeyName, nil
}

func replaceActivePair(certFile string, keyFile string, previousCert string, previousKey string, tempCert string, tempKey string) error {
	hasActive, err := bothFilesExist(certFile, keyFile)
	if err != nil {
		return err
	}
	if hasActive {
		if err := os.Remove(previousCert); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Remove(previousKey); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(certFile, previousCert); err != nil {
			return err
		}
		if err := os.Rename(keyFile, previousKey); err != nil {
			_ = os.Rename(previousCert, certFile)
			return err
		}
	}
	if err := os.Rename(tempCert, certFile); err != nil {
		return err
	}
	if err := os.Rename(tempKey, keyFile); err != nil {
		_ = os.Remove(certFile)
		if hasActive {
			_ = os.Rename(previousCert, certFile)
			_ = os.Rename(previousKey, keyFile)
		}
		return err
	}
	return nil
}

func bothFilesExist(certFile string, keyFile string) (bool, error) {
	certExists, err := fileExists(certFile)
	if err != nil {
		return false, err
	}
	keyExists, err := fileExists(keyFile)
	if err != nil {
		return false, err
	}
	if certExists != keyExists {
		return false, fmt.Errorf("active certificate pair is incomplete")
	}
	return certExists, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
