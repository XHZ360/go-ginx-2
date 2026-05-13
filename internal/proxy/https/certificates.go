package httpsproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
}

type ManagedCertificateStorage struct {
	CertificateDir string
	Now            func() time.Time
}

func (storage ManagedCertificateStorage) Store(host string, certPEM []byte, keyPEM []byte) (StoredCertificateFiles, error) {
	if strings.TrimSpace(storage.CertificateDir) == "" {
		return StoredCertificateFiles{}, errors.New("certificate dir is required")
	}
	parsed, err := ValidateCertificatePair(host, certPEM, keyPEM, storage.now())
	if err != nil {
		return StoredCertificateFiles{}, err
	}
	host = strings.ToLower(strings.TrimSpace(host))
	dir := filepath.Join(storage.CertificateDir, managedCertificateDir, host)
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
	return StoredCertificateFiles{CertFile: certFile, KeyFile: keyFile, PreviousCertFile: previousCert, PreviousKeyFile: previousKey, NotAfter: parsed.NotAfter}, nil
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
}

func ValidateCertificatePair(host string, certPEM []byte, keyPEM []byte, now time.Time) (ValidatedCertificate, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ValidatedCertificate{}, errors.New("certificate host is required")
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return ValidatedCertificate{}, err
	}
	if len(certificate.Certificate) == 0 {
		return ValidatedCertificate{}, errors.New("certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return ValidatedCertificate{}, err
	}
	if err := leaf.VerifyHostname(host); err != nil {
		return ValidatedCertificate{}, err
	}
	if !leaf.NotAfter.After(now) {
		return ValidatedCertificate{}, errors.New("certificate is expired")
	}
	certificate.Leaf = leaf
	return ValidatedCertificate{Certificate: certificate, Leaf: leaf, NotAfter: leaf.NotAfter}, nil
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
	certFile, keyFile, active, err := resolver.activeFiles(ctx, host, proxy)
	if err != nil || !active {
		return nil, err
	}
	certificate, err := resolver.fileCertificate(host, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &certificate, nil
}

func (resolver *CertificateResolver) activeFiles(ctx context.Context, host string, proxy domain.Proxy) (string, string, bool, error) {
	if proxy.CertFile != "" || proxy.KeyFile != "" {
		if proxy.CertFile == "" || proxy.KeyFile == "" {
			return "", "", false, errors.New("static certificate and key files must both be configured")
		}
		return proxy.CertFile, proxy.KeyFile, true, nil
	}
	if resolver.store == nil {
		return "", "", false, nil
	}
	managed, err := resolver.store.Certificates().ByHost(ctx, host)
	if errors.Is(err, store.ErrNotFound) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if proxy.ID != "" && managed.ProxyID != proxy.ID {
		return "", "", false, nil
	}
	if !managedCertificateActive(managed) {
		return "", "", false, nil
	}
	return managed.CertFile, managed.KeyFile, true, nil
}

func (resolver *CertificateResolver) fileCertificate(host string, certPath string, keyPath string) (tls.Certificate, error) {
	listener := &Listener{entry: Entry{CertificateDir: resolver.certificateDir}}
	certFile, err := listener.certificateFile(certPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyFile, err := listener.certificateFile(keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	certInfo, err := os.Stat(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if cached, ok := resolver.cache[host]; ok && cached.certFile == certFile && cached.keyFile == keyFile && cached.certModTime.Equal(certInfo.ModTime()) && cached.keyModTime.Equal(keyInfo.ModTime()) {
		return cached.certificate, nil
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	validated, err := ValidateCertificatePair(host, certPEM, keyPEM, time.Now().UTC())
	if err != nil {
		return tls.Certificate{}, err
	}
	resolver.cache[host] = cachedResolvedCertificate{certFile: certFile, keyFile: keyFile, certModTime: certInfo.ModTime(), keyModTime: keyInfo.ModTime(), certificate: validated.Certificate}
	return validated.Certificate, nil
}

func managedCertificateActive(certificate domain.ManagedCertificate) bool {
	if certificate.Status != domain.CertificateValid && certificate.Status != domain.CertificateExpiringSoon {
		return false
	}
	return certificate.CertFile != "" && certificate.KeyFile != ""
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
