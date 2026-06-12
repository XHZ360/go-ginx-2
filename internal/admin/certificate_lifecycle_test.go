package admin

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/contracterr"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

// writeManagedCertPair 在受管证书目录下写入一对覆盖 host 的证书/私钥文件，返回其路径。
func writeManagedCertPair(t *testing.T, certificateDir string, host string, notAfter time.Time) (string, string) {
	t.Helper()
	certPEM, keyPEM, _ := adminTestCertificatePEM(host, notAfter)
	dir := filepath.Join(certificateDir, "static", host)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir managed cert dir: %v", err)
	}
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	return certFile, keyFile
}

func TestServiceCreatesUnboundFileCertificate(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-file-1", nil }, Now: func() time.Time { return time.Now().UTC() }}}

	certFile, keyFile := writeManagedCertPair(t, certificateDir, "app.example.com", time.Now().Add(24*time.Hour))
	certificate, err := service.CreateCertificate(ctx, CreateCertificateInput{Host: "app.example.com", ProviderType: domain.CertificateProviderFile, CertFile: certFile, KeyFile: keyFile, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create file certificate: %v", err)
	}
	if certificate.ProxyID != "" {
		t.Fatalf("expected unbound certificate, got proxy %q", certificate.ProxyID)
	}
	if certificate.ProviderType != domain.CertificateProviderFile || certificate.CertFile != certFile || certificate.KeyFile != keyFile {
		t.Fatalf("unexpected file certificate metadata: %+v", certificate)
	}
	if certificate.ServingStatus != domain.CertificateServingUsable {
		t.Fatalf("expected usable serving status, got %+v", certificate)
	}
	// 私钥内容不入库（仅路径）。
	if certificate.Fingerprint == "" {
		t.Fatalf("expected fingerprint metadata, got %+v", certificate)
	}
}

func TestServiceDeleteCertificateRiskBasedConfirmation(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-file-1", nil }, Now: func() time.Time { return time.Now().UTC() }}, ListenerReconciler: &fakeProxyListenerReconciler{}}
	user, client := createAdminTestOwnership(ctx, t, service)

	certFile, keyFile := writeManagedCertPair(t, certificateDir, "app.example.com", time.Now().Add(24*time.Hour))
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, CertFile: certFile, KeyFile: keyFile, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create https proxy with legacy files: %v", err)
	}
	if proxy.CertificateID == "" {
		t.Fatalf("expected legacy files migrated into bound certificate, got %+v", proxy)
	}
	certificateID := proxy.CertificateID

	// 已绑定且可服务：无确认 -> CONFIRMATION_REQUIRED。
	_, err = service.DeleteCertificate(ctx, DeleteCertificateInput{CertificateID: certificateID, ActorID: "admin-1"})
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeConfirmationRequired {
		t.Fatalf("expected confirmation required, got %v", err)
	}
	if _, err := db.Certificates().ByID(ctx, certificateID); err != nil {
		t.Fatalf("certificate should survive rejected delete: %v", err)
	}

	// 提供匹配 host 确认 -> 成功，解绑代理并清理受管文件。
	result, err := service.DeleteCertificate(ctx, DeleteCertificateInput{CertificateID: certificateID, ConfirmHost: "app.example.com", ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("confirmed delete: %v", err)
	}
	if !result.RequiredConfirm || len(result.AffectedProxyIDs) != 1 || result.AffectedProxyIDs[0] != proxy.ID {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	reloaded, err := db.Proxies().ByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("reload proxy: %v", err)
	}
	if reloaded.CertificateID != "" || reloaded.Status != domain.ProxyNeedsConf {
		t.Fatalf("expected proxy unbound and needs_config, got %+v", reloaded)
	}
	if _, statErr := os.Stat(certFile); !os.IsNotExist(statErr) {
		t.Fatalf("expected managed cert file removed, stat err=%v", statErr)
	}
}

func TestServiceDeleteUnboundCertificateNoConfirmation(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-file-1", nil }, Now: func() time.Time { return time.Now().UTC() }}, ListenerReconciler: &fakeProxyListenerReconciler{}}

	certFile, keyFile := writeManagedCertPair(t, certificateDir, "free.example.com", time.Now().Add(24*time.Hour))
	certificate, err := service.CreateCertificate(ctx, CreateCertificateInput{Host: "free.example.com", ProviderType: domain.CertificateProviderFile, CertFile: certFile, KeyFile: keyFile, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create unbound file certificate: %v", err)
	}
	result, err := service.DeleteCertificate(ctx, DeleteCertificateInput{CertificateID: certificate.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("delete unbound certificate: %v", err)
	}
	if result.RequiredConfirm || len(result.AffectedProxyIDs) != 0 {
		t.Fatalf("unexpected unbound delete result: %+v", result)
	}
	if _, err := db.Certificates().ByID(ctx, certificate.ID); err == nil {
		t.Fatalf("expected certificate to be deleted")
	}
}

// TestServiceDeleteBoundButUnservableCertificateNoConfirmation 覆盖低风险删除分支：
// 证书虽绑定代理但活跃材料已过期（不可服务），删除无需二次确认，
// 仍解绑代理、标记 needs_config，并在 AffectedProxyIDs 中返回该代理。
func TestServiceDeleteBoundButUnservableCertificateNoConfirmation(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-expired-1", nil }, Now: func() time.Time { return time.Now().UTC() }}, ListenerReconciler: &fakeProxyListenerReconciler{}}
	user, client := createAdminTestOwnership(ctx, t, service)

	// 写入一对已过期的受管证书文件并以遗留路径方式绑定到代理。
	certFile, keyFile := writeManagedCertPair(t, certificateDir, "expired.example.com", time.Now().Add(-time.Hour))
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "expired.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, CertFile: certFile, KeyFile: keyFile, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create https proxy with expired legacy files: %v", err)
	}
	if proxy.CertificateID == "" {
		t.Fatalf("expected legacy files migrated into bound certificate, got %+v", proxy)
	}
	certificateID := proxy.CertificateID

	// 绑定但不可服务（过期）-> 低风险，无需确认即可删除。
	result, err := service.DeleteCertificate(ctx, DeleteCertificateInput{CertificateID: certificateID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("delete bound-but-unservable certificate: %v", err)
	}
	if result.RequiredConfirm {
		t.Fatalf("expected low-risk delete without confirmation, got %+v", result)
	}
	if len(result.AffectedProxyIDs) != 1 || result.AffectedProxyIDs[0] != proxy.ID {
		t.Fatalf("expected affected proxy in low-risk delete, got %+v", result)
	}
	reloaded, err := db.Proxies().ByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("reload proxy: %v", err)
	}
	if reloaded.CertificateID != "" || reloaded.Status != domain.ProxyNeedsConf {
		t.Fatalf("expected proxy unbound and needs_config, got %+v", reloaded)
	}
	if _, err := db.Certificates().ByID(ctx, certificateID); err == nil {
		t.Fatalf("expected certificate to be deleted")
	}
}

// TestServiceDeleteCertificateAcceptsCertificateIdConfirmation 覆盖强确认成功的另一条路径：
// 通过匹配的 ConfirmCertificateId（而非 ConfirmHost）确认删除已绑定且可服务的证书。
func TestServiceDeleteCertificateAcceptsCertificateIdConfirmation(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-confirm-1", nil }, Now: func() time.Time { return time.Now().UTC() }}, ListenerReconciler: &fakeProxyListenerReconciler{}}
	user, client := createAdminTestOwnership(ctx, t, service)

	certFile, keyFile := writeManagedCertPair(t, certificateDir, "app.example.com", time.Now().Add(24*time.Hour))
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, CertFile: certFile, KeyFile: keyFile, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create https proxy with legacy files: %v", err)
	}
	certificateID := proxy.CertificateID
	if certificateID == "" {
		t.Fatalf("expected legacy files migrated into bound certificate, got %+v", proxy)
	}

	// 不匹配的 cert id 确认 -> CONFIRMATION_REQUIRED。
	_, err = service.DeleteCertificate(ctx, DeleteCertificateInput{CertificateID: certificateID, ConfirmCertificateID: "wrong-cert-id", ActorID: "admin-1"})
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeConfirmationRequired {
		t.Fatalf("expected confirmation required for mismatched cert id, got %v", err)
	}

	// 匹配的 cert id 确认 -> 成功删除并解绑。
	result, err := service.DeleteCertificate(ctx, DeleteCertificateInput{CertificateID: certificateID, ConfirmCertificateID: certificateID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("confirmed delete via certificate id: %v", err)
	}
	if !result.RequiredConfirm || len(result.AffectedProxyIDs) != 1 || result.AffectedProxyIDs[0] != proxy.ID {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	reloaded, err := db.Proxies().ByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("reload proxy: %v", err)
	}
	if reloaded.CertificateID != "" || reloaded.Status != domain.ProxyNeedsConf {
		t.Fatalf("expected proxy unbound and needs_config, got %+v", reloaded)
	}
	if _, err := db.Certificates().ByID(ctx, certificateID); err == nil {
		t.Fatalf("expected certificate to be deleted")
	}
}

func TestServiceBindUnbindCertificateAndOneToOneConflict(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-bind-1", nil }, Now: func() time.Time { return time.Now().UTC() }}, ListenerReconciler: &fakeProxyListenerReconciler{}}
	user, client := createAdminTestOwnership(ctx, t, service)

	certFile, keyFile := writeManagedCertPair(t, certificateDir, "app.example.com", time.Now().Add(24*time.Hour))
	certificate, err := service.CreateCertificate(ctx, CreateCertificateInput{Host: "app.example.com", ProviderType: domain.CertificateProviderFile, CertFile: certFile, KeyFile: keyFile, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create file certificate: %v", err)
	}
	proxyA, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-a", UserID: user.ID, ClientID: client.ID, Name: "a", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy a: %v", err)
	}
	proxyB, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-b", UserID: user.ID, ClientID: client.ID, Name: "b", Type: domain.ProxyHTTPS, EntryBindHost: "127.0.0.2", EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8081, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy b: %v", err)
	}

	bound, err := service.BindCertificate(ctx, BindCertificateInput{ProxyID: proxyA.ID, CertificateID: certificate.ID, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("bind certificate: %v", err)
	}
	if bound.CertificateID != certificate.ID {
		t.Fatalf("expected proxy bound, got %+v", bound)
	}

	// 一对一：第二个代理绑定同一证书 -> CERTIFICATE_INCOMPATIBLE。
	_, err = service.BindCertificate(ctx, BindCertificateInput{ProxyID: proxyB.ID, CertificateID: certificate.ID, ActorID: "admin-1"})
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeCertificateIncompatible {
		t.Fatalf("expected one-to-one binding conflict, got %v", err)
	}

	// 解绑 proxyA 后 proxyB 可绑定。
	if _, err := service.UnbindCertificate(ctx, UnbindCertificateInput{ProxyID: proxyA.ID, ActorID: "admin-1"}); err != nil {
		t.Fatalf("unbind certificate: %v", err)
	}
	reloadedA, err := db.Proxies().ByID(ctx, proxyA.ID)
	if err != nil {
		t.Fatalf("reload proxy a: %v", err)
	}
	if reloadedA.CertificateID != "" || reloadedA.Status != domain.ProxyNeedsConf {
		t.Fatalf("expected proxy a unbound and needs_config, got %+v", reloadedA)
	}
	if _, err := service.BindCertificate(ctx, BindCertificateInput{ProxyID: proxyB.ID, CertificateID: certificate.ID, ActorID: "admin-1"}); err != nil {
		t.Fatalf("rebind after unbind: %v", err)
	}
}

func TestServiceBindCertificateRejectsIncompatibleHost(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-host-1", nil }, Now: func() time.Time { return time.Now().UTC() }}, ListenerReconciler: &fakeProxyListenerReconciler{}}
	user, client := createAdminTestOwnership(ctx, t, service)

	certFile, keyFile := writeManagedCertPair(t, certificateDir, "other.example.com", time.Now().Add(24*time.Hour))
	certificate, err := service.CreateCertificate(ctx, CreateCertificateInput{Host: "other.example.com", ProviderType: domain.CertificateProviderFile, CertFile: certFile, KeyFile: keyFile, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create file certificate: %v", err)
	}
	proxy, err := service.CreateProxy(ctx, CreateProxyInput{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, EntryHost: "app.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, ActorID: "admin-1"})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	_, err = service.BindCertificate(ctx, BindCertificateInput{ProxyID: proxy.ID, CertificateID: certificate.ID, ActorID: "admin-1"})
	var contractError *contracterr.Error
	if !errors.As(err, &contractError) || contractError.Code != contracterr.CodeCertificateIncompatible {
		t.Fatalf("expected incompatible host error, got %v", err)
	}
}

func TestServiceMigrateLegacyFileCertificatesIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-migrated-1", nil }, Now: func() time.Time { return time.Now().UTC() }}}

	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	certFile, keyFile := writeManagedCertPair(t, certificateDir, "legacy.example.com", time.Now().Add(24*time.Hour))
	// 直接写入带有遗留静态证书路径、未绑定 certificate_id 的代理（模拟 Phase 1 之前的数据）。
	legacy := domain.Proxy{ID: "proxy-legacy", UserID: user.ID, ClientID: client.ID, Name: "legacy", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "legacy.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, CertFile: certFile, KeyFile: keyFile}
	if err := db.Proxies().Create(ctx, legacy); err != nil {
		t.Fatalf("create legacy proxy: %v", err)
	}

	migrated, err := service.MigrateLegacyFileCertificates(ctx)
	if err != nil {
		t.Fatalf("migrate legacy file certificates: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("expected one migrated certificate, got %d", migrated)
	}
	bound, err := db.Proxies().ByID(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload legacy proxy: %v", err)
	}
	if bound.CertificateID == "" {
		t.Fatalf("expected legacy proxy bound to migrated certificate, got %+v", bound)
	}
	certificate, err := db.Certificates().ByID(ctx, bound.CertificateID)
	if err != nil {
		t.Fatalf("reload migrated certificate: %v", err)
	}
	if certificate.ProviderType != domain.CertificateProviderFile || certificate.CertFile != certFile || certificate.Host != "legacy.example.com" {
		t.Fatalf("unexpected migrated certificate: %+v", certificate)
	}

	// 幂等：再次运行不应迁移更多。
	again, err := service.MigrateLegacyFileCertificates(ctx)
	if err != nil {
		t.Fatalf("re-run migration: %v", err)
	}
	if again != 0 {
		t.Fatalf("expected idempotent migration, got %d", again)
	}
}

// TestServiceMigrateLegacyFileCertificatesStoresOnlyPathsNotPrivateKeyBytes 覆盖
// 旧静态文件路径迁移（cert_file/key_file -> file-backed certificate resource + binding）
// 的核心安全约束：迁移后 SQLite 中只登记文件路径，绝不写入私钥字节。
func TestServiceMigrateLegacyFileCertificatesStoresOnlyPathsNotPrivateKeyBytes(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	certificateDir := t.TempDir()
	service := Service{Store: db, Certificates: certmanager.Service{Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: certificateDir}, NewID: func() (string, error) { return "cert-migrated-1", nil }, Now: func() time.Time { return time.Now().UTC() }}}

	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	certFile, keyFile := writeManagedCertPair(t, certificateDir, "legacy.example.com", time.Now().Add(24*time.Hour))
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("read key material: %v", err)
	}
	legacy := domain.Proxy{ID: "proxy-legacy", UserID: user.ID, ClientID: client.ID, Name: "legacy", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "legacy.example.com", TargetHost: "127.0.0.1", TargetPort: 8080, CertFile: certFile, KeyFile: keyFile}
	if err := db.Proxies().Create(ctx, legacy); err != nil {
		t.Fatalf("create legacy proxy: %v", err)
	}

	migrated, err := service.MigrateLegacyFileCertificates(ctx)
	if err != nil {
		t.Fatalf("migrate legacy file certificates: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("expected one migrated certificate, got %d", migrated)
	}

	bound, err := db.Proxies().ByID(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload legacy proxy: %v", err)
	}
	if bound.CertificateID == "" {
		t.Fatalf("expected legacy proxy bound to migrated certificate, got %+v", bound)
	}
	certificate, err := db.Certificates().ByID(ctx, bound.CertificateID)
	if err != nil {
		t.Fatalf("reload migrated certificate: %v", err)
	}
	// provider_type=file，仅登记文件路径。
	if certificate.ProviderType != domain.CertificateProviderFile || certificate.CertFile != certFile || certificate.KeyFile != keyFile || certificate.Host != "legacy.example.com" {
		t.Fatalf("unexpected migrated certificate metadata: %+v", certificate)
	}

	// 私钥字节绝不入库：扫描整个 SQLite 文件中的所有文本/二进制列，确认没有任何私钥 PEM 材料。
	if err := db.Close(); err != nil {
		t.Fatalf("close store before raw scan: %v", err)
	}
	assertSQLiteContainsNoBytes(t, ctx, dbPath, [][]byte{keyBytes, []byte("PRIVATE KEY")})
}

// assertSQLiteContainsNoBytes 打开 SQLite 文件并扫描所有用户表中每一列的文本/二进制值，
// 断言其中不包含给定的任何字节片段（用于验证私钥材料未入库）。
func assertSQLiteContainsNoBytes(t *testing.T, ctx context.Context, dbPath string, forbidden [][]byte) {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer raw.Close()

	tableRows, err := raw.QueryContext(ctx, `select name from sqlite_master where type = 'table' and name not like 'sqlite_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	tables := make([]string, 0)
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := tableRows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	if err := tableRows.Close(); err != nil {
		t.Fatalf("close table rows: %v", err)
	}

	for _, table := range tables {
		rows, err := raw.QueryContext(ctx, `select * from "`+table+`"`)
		if err != nil {
			t.Fatalf("select from %s: %v", table, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatalf("columns of %s: %v", table, err)
		}
		for rows.Next() {
			cells := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range cells {
				pointers[index] = &cells[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatalf("scan row of %s: %v", table, err)
			}
			for index, cell := range cells {
				var value []byte
				switch typed := cell.(type) {
				case []byte:
					value = typed
				case string:
					value = []byte(typed)
				default:
					continue
				}
				for _, needle := range forbidden {
					if bytes.Contains(value, needle) {
						t.Fatalf("sqlite table %s column %s leaked forbidden material %q", table, columns[index], string(needle))
					}
				}
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate rows of %s: %v", table, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close rows of %s: %v", table, err)
		}
	}
}
