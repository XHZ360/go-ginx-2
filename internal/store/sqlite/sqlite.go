package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	wrapped := &Store{db: db}
	if err := wrapped.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return wrapped, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Users() store.UserRepository { return userRepository{s.db} }

func (s *Store) Clients() store.ClientRepository { return clientRepository{s.db} }

func (s *Store) ClientEnrollments() store.ClientEnrollmentRepository {
	return clientEnrollmentRepository{s.db}
}

func (s *Store) Domains() store.DomainRepository { return domainRepository{s.db} }

func (s *Store) DomainEntries() store.DomainEntryRepository { return domainEntryRepository{s.db} }

func (s *Store) Proxies() store.ProxyRepository { return proxyRepository{s.db} }

func (s *Store) ProxyAccess() store.ProxyAccessRepository { return proxyAccessRepository{s.db} }

func (s *Store) Certificates() store.CertificateRepository { return certificateRepository{s.db} }

func (s *Store) ProviderCredentials() store.ProviderCredentialRepository {
	return providerCredentialRepository{s.db}
}

func (s *Store) Stats() store.StatsRepository { return statsRepository{s.db} }

func (s *Store) AuditEvents() store.AuditRepository { return auditRepository{s.db} }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	if err := addProxyCertificateColumns(ctx, s.db); err != nil {
		return err
	}
	if err := addProxyEntryBindHostColumn(ctx, s.db); err != nil {
		return err
	}
	if err := migrateProxyEntryIndexes(ctx, s.db); err != nil {
		return err
	}
	if err := addClientEnrollmentTokenColumn(ctx, s.db); err != nil {
		return err
	}
	if err := addManagedCertificateLifecycleColumns(ctx, s.db); err != nil {
		return err
	}
	if err := addManagedCertificateProviderColumns(ctx, s.db); err != nil {
		return err
	}
	if err := addCertificateQueryIndexes(ctx, s.db); err != nil {
		return err
	}
	// 证书集中化：先补充代理侧 certificate_id 列与唯一索引，
	// 再重建 managed_certificates 去除级联外键，最后回填代理绑定。
	if err := migrateProxyCertificateBinding(ctx, s.db); err != nil {
		return err
	}
	if err := migrateManagedCertificateProxyOptional(ctx, s.db); err != nil {
		return err
	}
	if err := migrateBindProxyCertificates(ctx, s.db); err != nil {
		return err
	}
	if err := addUserPasswordColumn(ctx, s.db); err != nil {
		return err
	}
	if err := addClientKindColumn(ctx, s.db); err != nil {
		return err
	}
	if err := addProxyAccessAuthColumns(ctx, s.db); err != nil {
		return err
	}
	return migrateDomainPathRouting(ctx, s.db)
}

type proxyAccessRepository struct{ db *sql.DB }

func (r proxyAccessRepository) EnableAuthAndCreateActivation(ctx context.Context, proxyID string, authVersion int64, token domain.ProxyActivationToken) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `update proxies set access_auth_enabled = 1, access_auth_version = ?, updated_at = ? where id = ?`, authVersion, time.Now().UTC(), proxyID); err != nil {
		return err
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	if _, err := tx.ExecContext(ctx, `insert into proxy_activation_tokens (id, proxy_id, auth_version, token_hash, expires_at, used_at, created_at, created_by) values (?, ?, ?, ?, ?, ?, ?, ?)`, token.ID, token.ProxyID, token.AuthVersion, token.TokenHash, token.ExpiresAt, token.UsedAt, token.CreatedAt, token.CreatedBy); err != nil {
		return translateError(err)
	}
	return tx.Commit()
}

func (r proxyAccessRepository) CreateActivationToken(ctx context.Context, token domain.ProxyActivationToken) error {
	if token.ID == "" || token.ProxyID == "" || token.TokenHash == "" || token.ExpiresAt.IsZero() {
		return errors.New("activation token fields are required")
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `insert into proxy_activation_tokens (id, proxy_id, auth_version, token_hash, expires_at, used_at, created_at, created_by) values (?, ?, ?, ?, ?, ?, ?, ?)`, token.ID, token.ProxyID, token.AuthVersion, token.TokenHash, token.ExpiresAt, token.UsedAt, token.CreatedAt, token.CreatedBy)
	return translateError(err)
}

func (r proxyAccessRepository) ActivationToken(ctx context.Context, proxyID string, tokenHash string, now time.Time) (domain.ProxyActivationToken, error) {
	var token domain.ProxyActivationToken
	var usedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `select id, proxy_id, auth_version, token_hash, expires_at, used_at, created_at, created_by from proxy_activation_tokens where proxy_id = ? and token_hash = ? and expires_at > ? and used_at is null`, proxyID, tokenHash, now).Scan(&token.ID, &token.ProxyID, &token.AuthVersion, &token.TokenHash, &token.ExpiresAt, &usedAt, &token.CreatedAt, &token.CreatedBy)
	if usedAt.Valid {
		token.UsedAt = &usedAt.Time
	}
	return token, translateError(err)
}

func (r proxyAccessRepository) ActivationTokenByHash(ctx context.Context, tokenHash string, now time.Time) (domain.ProxyActivationToken, error) {
	var token domain.ProxyActivationToken
	var usedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `select id, proxy_id, auth_version, token_hash, expires_at, used_at, created_at, created_by from proxy_activation_tokens where token_hash = ? and expires_at > ? and used_at is null`, tokenHash, now).Scan(&token.ID, &token.ProxyID, &token.AuthVersion, &token.TokenHash, &token.ExpiresAt, &usedAt, &token.CreatedAt, &token.CreatedBy)
	if usedAt.Valid {
		token.UsedAt = &usedAt.Time
	}
	return token, translateError(err)
}

func (r proxyAccessRepository) RedeemActivationToken(ctx context.Context, tokenID string, secretHash string, credential domain.ProxyAccessCredential, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `update proxy_activation_tokens set used_at = ? where id = ? and used_at is null and expires_at > ?`, now, tokenID, now)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return store.ErrConflict
	}
	if credential.CreatedAt.IsZero() {
		credential.CreatedAt = now
	}
	credential.SecretHash = secretHash
	_, err = tx.ExecContext(ctx, `insert into proxy_access_credentials (id, proxy_id, auth_version, secret_hash, created_at, last_used_at) values (?, ?, ?, ?, ?, ?)`, credential.ID, credential.ProxyID, credential.AuthVersion, credential.SecretHash, credential.CreatedAt, credential.LastUsedAt)
	if err != nil {
		return translateError(err)
	}
	return tx.Commit()
}

func (r proxyAccessRepository) ValidateAccessCredential(ctx context.Context, proxyID string, authVersion int64, secretHash string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `update proxy_access_credentials set last_used_at = ? where proxy_id = ? and auth_version = ? and secret_hash = ?`, now, proxyID, authVersion, secretHash)
	return resultError(result, err)
}

func (r proxyAccessRepository) RevokeAllAccess(ctx context.Context, proxyID string, nextVersion int64) error {
	return r.rewriteAccessState(ctx, proxyID, nextVersion, nil)
}

func (r proxyAccessRepository) DisableAuth(ctx context.Context, proxyID string, nextVersion int64) error {
	enabled := false
	return r.rewriteAccessState(ctx, proxyID, nextVersion, &enabled)
}

func (r proxyAccessRepository) rewriteAccessState(ctx context.Context, proxyID string, nextVersion int64, enabled *bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if enabled == nil {
		if _, err := tx.ExecContext(ctx, `update proxies set access_auth_version = ?, updated_at = ? where id = ?`, nextVersion, time.Now().UTC(), proxyID); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `update proxies set access_auth_enabled = ?, access_auth_version = ?, updated_at = ? where id = ?`, *enabled, nextVersion, time.Now().UTC(), proxyID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from proxy_activation_tokens where proxy_id = ?`, proxyID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from proxy_access_credentials where proxy_id = ?`, proxyID); err != nil {
		return err
	}
	return tx.Commit()
}

type userRepository struct{ db *sql.DB }

func (r userRepository) Create(ctx context.Context, user domain.User) error {
	if err := user.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	_, err := r.db.ExecContext(ctx, `insert into users (id, username, password_hash, role, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?)`, user.ID, user.Username, user.PasswordHash, user.Role, user.Status, user.CreatedAt, user.UpdatedAt)
	return translateError(err)
}

func (r userRepository) ByID(ctx context.Context, id string) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `select id, username, password_hash, role, status, created_at, updated_at from users where id = ?`, id))
}

func (r userRepository) ByUsername(ctx context.Context, username string) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `select id, username, password_hash, role, status, created_at, updated_at from users where username = ?`, username))
}

func (r userRepository) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `select id, username, password_hash, role, status, created_at, updated_at from users order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]domain.User, 0)
	for rows.Next() {
		user, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (r userRepository) SetStatus(ctx context.Context, id string, status domain.UserStatus) error {
	result, err := r.db.ExecContext(ctx, `update users set status = ?, updated_at = ? where id = ?`, status, time.Now().UTC(), id)
	return resultError(result, err)
}

func (r userRepository) SetPassword(ctx context.Context, id string, passwordHash string) error {
	result, err := r.db.ExecContext(ctx, `update users set password_hash = ?, updated_at = ? where id = ?`, passwordHash, time.Now().UTC(), id)
	return resultError(result, err)
}

func (r userRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from users where id = ?`, id)
	return resultError(result, err)
}

type clientRepository struct{ db *sql.DB }

func (r clientRepository) Create(ctx context.Context, client domain.Client) error {
	if err := client.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if client.CreatedAt.IsZero() {
		client.CreatedAt = now
	}
	if client.UpdatedAt.IsZero() {
		client.UpdatedAt = now
	}
	kind := domain.NormalizeClientKind(client.Kind)
	_, err := r.db.ExecContext(ctx, `insert into clients (id, user_id, name, kind, status, credential_hash, version, last_online_at, last_offline_at, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, client.ID, client.UserID, client.Name, kind, client.Status, client.CredentialHash, client.Version, client.LastOnlineAt, client.LastOfflineAt, client.CreatedAt, client.UpdatedAt)
	return translateError(err)
}

func (r clientRepository) ByID(ctx context.Context, id string) (domain.Client, error) {
	return scanClient(r.db.QueryRowContext(ctx, `select id, user_id, name, kind, status, credential_hash, version, last_online_at, last_offline_at, created_at, updated_at from clients where id = ?`, id))
}

func (r clientRepository) List(ctx context.Context) ([]domain.Client, error) {
	rows, err := r.db.QueryContext(ctx, `select id, user_id, name, kind, status, credential_hash, version, last_online_at, last_offline_at, created_at, updated_at from clients order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	clients := make([]domain.Client, 0)
	for rows.Next() {
		client, err := scanClientRows(rows)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return clients, nil
}

func (r clientRepository) SetStatus(ctx context.Context, id string, status domain.ClientStatus) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
update clients
set status = ?,
    last_online_at = case when ? = ? then ? else last_online_at end,
    last_offline_at = case when ? in (?, ?) then ? else last_offline_at end,
    updated_at = ?
where id = ?`,
		status,
		status, domain.ClientOnline, now,
		status, domain.ClientOffline, domain.ClientDisconnected, now,
		now,
		id)
	return resultError(result, err)
}

func (r clientRepository) RotateCredential(ctx context.Context, id string, credentialHash string) error {
	if strings.TrimSpace(credentialHash) == "" {
		return errors.New("credential hash is required")
	}
	result, err := r.db.ExecContext(ctx, `update clients set credential_hash = ?, version = version + 1, updated_at = ? where id = ?`, credentialHash, time.Now().UTC(), id)
	return resultError(result, err)
}

func (r clientRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from clients where id = ?`, id)
	return resultError(result, err)
}

type clientEnrollmentRepository struct{ db *sql.DB }

func (r clientEnrollmentRepository) Create(ctx context.Context, enrollment domain.ClientEnrollment) error {
	if err := enrollment.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if enrollment.CreatedAt.IsZero() {
		enrollment.CreatedAt = now
	}
	if enrollment.UpdatedAt.IsZero() {
		enrollment.UpdatedAt = now
	}
	_, err := r.db.ExecContext(ctx, `insert into client_enrollments (id, client_id, secret_hash, token_hash, token, expires_at, used_at, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?)`, enrollment.ID, enrollment.ClientID, enrollment.SecretHash, enrollment.TokenHash, enrollment.Token, enrollment.ExpiresAt, enrollment.UsedAt, enrollment.CreatedAt, enrollment.UpdatedAt)
	return translateError(err)
}

func (r clientEnrollmentRepository) ByID(ctx context.Context, id string) (domain.ClientEnrollment, error) {
	return scanClientEnrollment(r.db.QueryRowContext(ctx, `select id, client_id, secret_hash, token_hash, token, expires_at, used_at, created_at, updated_at from client_enrollments where id = ?`, id))
}

func (r clientEnrollmentRepository) LatestReviewableByClientID(ctx context.Context, clientID string, now time.Time) (domain.ClientEnrollment, error) {
	return scanClientEnrollment(r.db.QueryRowContext(ctx, `select id, client_id, secret_hash, token_hash, token, expires_at, used_at, created_at, updated_at from client_enrollments where client_id = ? and token <> '' and used_at is null and expires_at > ? order by created_at desc, id desc limit 1`, clientID, now))
}

func (r clientEnrollmentRepository) LatestUnusedByClientID(ctx context.Context, clientID string) (domain.ClientEnrollment, error) {
	return scanClientEnrollment(r.db.QueryRowContext(ctx, `select id, client_id, secret_hash, token_hash, token, expires_at, used_at, created_at, updated_at from client_enrollments where client_id = ? and token <> '' and used_at is null order by created_at desc, id desc limit 1`, clientID))
}

func (r clientEnrollmentRepository) MarkUsed(ctx context.Context, id string, usedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `update client_enrollments set used_at = ?, updated_at = ? where id = ? and used_at is null`, usedAt, usedAt, id)
	return resultError(result, err)
}

type proxyRepository struct{ db *sql.DB }

const proxySelectColumns = `id, user_id, client_id, name, type, status, domain_id, path_prefix, strip_prefix, upstream_path_prefix, entry_bind_host, entry_host, entry_port, target_host, target_port, cert_file, key_file, certificate_id, access_auth_enabled, access_auth_version, stats_legacy_aggregate, description, created_at, updated_at`

func (r proxyRepository) Create(ctx context.Context, proxy domain.Proxy) error {
	if err := ensureLegacyWebProxyDomain(ctx, r.db, &proxy); err != nil {
		return err
	}
	if err := normalizeProxyForStore(&proxy); err != nil {
		return err
	}
	if err := proxy.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if proxy.CreatedAt.IsZero() {
		proxy.CreatedAt = now
	}
	if proxy.UpdatedAt.IsZero() {
		proxy.UpdatedAt = now
	}
	_, err := r.db.ExecContext(ctx, `insert into proxies (id, user_id, client_id, name, type, status, domain_id, path_prefix, strip_prefix, upstream_path_prefix, entry_bind_host, entry_host, entry_port, target_host, target_port, cert_file, key_file, certificate_id, access_auth_enabled, access_auth_version, stats_legacy_aggregate, description, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, proxy.ID, proxy.UserID, proxy.ClientID, proxy.Name, proxy.Type, proxy.Status, proxy.DomainID, proxy.PathPrefix, proxy.StripPrefix, proxy.UpstreamPathPrefix, domain.NormalizeBindHost(proxy.EntryBindHost), proxy.EntryHost, proxy.EntryPort, proxy.TargetHost, proxy.TargetPort, proxy.CertFile, proxy.KeyFile, proxy.CertificateID, proxy.AccessAuthEnabled, proxy.AccessAuthVersion, proxy.StatsLegacyAggregate, proxy.Description, proxy.CreatedAt, proxy.UpdatedAt)
	return translateError(err)
}

// ensureLegacyWebProxyDomain converts create-time ProxyHTTP/HTTPS fixtures into Domain + Web Proxy.
func ensureLegacyWebProxyDomain(ctx context.Context, db *sql.DB, proxy *domain.Proxy) error {
	if proxy.Type != domain.ProxyHTTP && proxy.Type != domain.ProxyHTTPS {
		return nil
	}
	host := domain.NormalizeRouteHost(proxy.EntryHost)
	if host == "" {
		return errors.New("web proxy host is required")
	}
	now := time.Now().UTC()
	domainID := ""
	var existing domain.Domain
	err := db.QueryRowContext(ctx, `select id, user_id, host, certificate_id, status, created_at, updated_at from domains where lower(host) = lower(?)`, host).Scan(&existing.ID, &existing.UserID, &existing.Host, &existing.CertificateID, &existing.Status, &existing.CreatedAt, &existing.UpdatedAt)
	if err == nil {
		if existing.UserID != proxy.UserID {
			return fmt.Errorf("%w: domain host belongs to another user", store.ErrConflict)
		}
		domainID = existing.ID
		if proxy.CertificateID != "" && existing.CertificateID == "" {
			if _, err := db.ExecContext(ctx, `update domains set certificate_id = ?, updated_at = ? where id = ?`, proxy.CertificateID, now, domainID); err != nil {
				return err
			}
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		domainID = newMigrationID("domain")
		certID := proxy.CertificateID
		if _, err := db.ExecContext(ctx, `insert into domains (id, user_id, host, certificate_id, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?)`, domainID, proxy.UserID, host, certID, domain.DomainEnabled, now, now); err != nil {
			return translateError(err)
		}
	} else {
		return err
	}
	protocol := domain.DomainEntryHTTP
	if proxy.Type == domain.ProxyHTTPS {
		protocol = domain.DomainEntryHTTPS
	}
	bindHost := domain.NormalizeBindHost(proxy.EntryBindHost)
	port := proxy.EntryPort
	var entryCount int
	if err := db.QueryRowContext(ctx, `select count(*) from domain_entries where domain_id = ? and protocol = ? and lower(bind_host) = lower(?) and port = ?`, domainID, protocol, bindHost, port).Scan(&entryCount); err != nil {
		return err
	}
	if entryCount == 0 {
		if _, err := db.ExecContext(ctx, `insert into domain_entries (id, domain_id, protocol, bind_host, port, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?)`, newMigrationID("dentry"), domainID, protocol, bindHost, port, domain.DomainEntryEnabled, now, now); err != nil {
			return translateError(err)
		}
	}
	// static cert files: register as managed certificate bound to domain if no certificate id
	if protocol == domain.DomainEntryHTTPS && proxy.CertificateID == "" && proxy.CertFile != "" && proxy.KeyFile != "" {
		var domainCert string
		_ = db.QueryRowContext(ctx, `select certificate_id from domains where id = ?`, domainID).Scan(&domainCert)
		if domainCert == "" {
			certID := newMigrationID("cert")
			if _, err := db.ExecContext(ctx, `insert into managed_certificates (id, proxy_id, host, status, serving_status, operation_status, provider, provider_type, provider_name, credential_id, provider_status, cloudflare_certificate_id, previous_cloudflare_certificate_id, hostnames, request_type, requested_validity, cert_file, key_file, previous_cert_file, previous_key_file, failure_count, fingerprint, last_error, created_at, updated_at) values (?, '', ?, ?, ?, '', 'file', 'file', 'static', '', '', '', '', '[]', '', 0, ?, ?, '', '', 0, '', '', ?, ?)`,
				certID, host, domain.CertificateValid, domain.CertificateServingUsable, proxy.CertFile, proxy.KeyFile, now, now); err != nil {
				return translateError(err)
			}
			if _, err := db.ExecContext(ctx, `update domains set certificate_id = ?, updated_at = ? where id = ?`, certID, now, domainID); err != nil {
				return err
			}
		}
	}
	pathPrefix := proxy.PathPrefix
	if pathPrefix == "" {
		pathPrefix = "/"
	}
	upstream := proxy.UpstreamPathPrefix
	if upstream == "" {
		upstream = "/"
	}
	proxy.Type = domain.ProxyWeb
	proxy.DomainID = domainID
	proxy.PathPrefix = pathPrefix
	proxy.UpstreamPathPrefix = upstream
	proxy.EntryBindHost = ""
	proxy.EntryHost = ""
	proxy.EntryPort = 0
	proxy.CertificateID = ""
	proxy.CertFile = ""
	proxy.KeyFile = ""
	return nil
}

func normalizeProxyForStore(proxy *domain.Proxy) error {
	switch proxy.Type {
	case domain.ProxyWeb:
		pathPrefix, err := domain.NormalizeProxyRoutePrefix(proxy.PathPrefix)
		if err != nil {
			return err
		}
		upstream, err := domain.NormalizeProxyUpstreamPathPrefix(proxy.UpstreamPathPrefix)
		if err != nil {
			return err
		}
		proxy.PathPrefix = pathPrefix
		proxy.UpstreamPathPrefix = upstream
		proxy.EntryBindHost = ""
		proxy.EntryHost = ""
		proxy.EntryPort = 0
		proxy.CertificateID = ""
		// CertFile/KeyFile may remain temporarily for legacy static-file migration.
	case domain.ProxyTCP, domain.ProxyUDP, domain.ProxyForward:
		proxy.DomainID = ""
		proxy.PathPrefix = ""
		proxy.StripPrefix = false
		proxy.UpstreamPathPrefix = ""
	}
	// legacy ProxyHTTP/ProxyHTTPS keep entry fields for migration fixtures
	return nil
}

func (r proxyRepository) ByID(ctx context.Context, id string) (domain.Proxy, error) {
	return scanProxy(r.db.QueryRowContext(ctx, `select `+proxySelectColumns+` from proxies where id = ?`, id))
}

func (r proxyRepository) List(ctx context.Context) ([]domain.Proxy, error) {
	rows, err := r.db.QueryContext(ctx, `select `+proxySelectColumns+` from proxies order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	proxies := make([]domain.Proxy, 0)
	for rows.Next() {
		proxy, err := scanProxyRows(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return proxies, nil
}

func (r proxyRepository) ByClientID(ctx context.Context, clientID string) ([]domain.Proxy, error) {
	rows, err := r.db.QueryContext(ctx, `select `+proxySelectColumns+` from proxies where client_id = ? order by created_at, id`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	proxies := make([]domain.Proxy, 0)
	for rows.Next() {
		proxy, err := scanProxyRows(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return proxies, nil
}

func (r proxyRepository) ByUserID(ctx context.Context, userID string) ([]domain.Proxy, error) {
	rows, err := r.db.QueryContext(ctx, `select `+proxySelectColumns+` from proxies where user_id = ? order by created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	proxies := make([]domain.Proxy, 0)
	for rows.Next() {
		proxy, err := scanProxyRows(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return proxies, nil
}

func (r proxyRepository) ByDomainID(ctx context.Context, domainID string) ([]domain.Proxy, error) {
	rows, err := r.db.QueryContext(ctx, `select `+proxySelectColumns+` from proxies where domain_id = ? order by length(path_prefix) desc, path_prefix, id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	proxies := make([]domain.Proxy, 0)
	for rows.Next() {
		proxy, err := scanProxyRows(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	return proxies, rows.Err()
}

func (r proxyRepository) EnabledWebByDomainID(ctx context.Context, domainID string) ([]domain.Proxy, error) {
	rows, err := r.db.QueryContext(ctx, `select `+proxySelectColumns+` from proxies where domain_id = ? and status = ? and type = ? order by length(path_prefix) desc, path_prefix, id`, domainID, domain.ProxyEnabled, domain.ProxyWeb)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	proxies := make([]domain.Proxy, 0)
	for rows.Next() {
		proxy, err := scanProxyRows(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	return proxies, rows.Err()
}

func (r proxyRepository) ByDomainAndPath(ctx context.Context, domainID string, path string) (domain.Proxy, error) {
	proxies, err := r.EnabledWebByDomainID(ctx, domainID)
	if err != nil {
		return domain.Proxy{}, err
	}
	selected, ok := domain.SelectWebProxy(proxies, path)
	if !ok {
		return domain.Proxy{}, store.ErrNotFound
	}
	return selected, nil
}

func (r proxyRepository) EnabledByType(ctx context.Context, proxyType domain.ProxyType) ([]domain.Proxy, error) {
	rows, err := r.db.QueryContext(ctx, `select `+proxySelectColumns+` from proxies where type = ? and status = ? order by created_at, id`, proxyType, domain.ProxyEnabled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	proxies := make([]domain.Proxy, 0)
	for rows.Next() {
		proxy, err := scanProxyRows(rows)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return proxies, nil
}

func (r proxyRepository) ByTCPEntry(ctx context.Context, bindHost string, port int, includeDefault bool) (domain.Proxy, error) {
	return r.byEntry(ctx, domain.ProxyTCP, domain.NormalizeBindHost(bindHost), port, "", includeDefault)
}

func (r proxyRepository) ByUDPEntry(ctx context.Context, bindHost string, port int, includeDefault bool) (domain.Proxy, error) {
	return r.byEntry(ctx, domain.ProxyUDP, domain.NormalizeBindHost(bindHost), port, "", includeDefault)
}

func (r proxyRepository) ByTCPEntryPort(ctx context.Context, port int) (domain.Proxy, error) {
	return r.ByTCPEntry(ctx, "", port, true)
}

func (r proxyRepository) ByUDPEntryPort(ctx context.Context, port int) (domain.Proxy, error) {
	return r.ByUDPEntry(ctx, "", port, true)
}

func (r proxyRepository) ByHTTPRoute(ctx context.Context, bindHost string, port int, host string, includeDefault bool) (domain.Proxy, error) {
	return r.byEntry(ctx, domain.ProxyHTTP, domain.NormalizeBindHost(bindHost), port, domain.NormalizeRouteHost(host), includeDefault)
}

func (r proxyRepository) ByHTTPSRoute(ctx context.Context, bindHost string, port int, host string, includeDefault bool) (domain.Proxy, error) {
	return r.byEntry(ctx, domain.ProxyHTTPS, domain.NormalizeBindHost(bindHost), port, domain.NormalizeRouteHost(host), includeDefault)
}

func (r proxyRepository) ByHTTPHost(ctx context.Context, host string) (domain.Proxy, error) {
	// Prefer Domain-based web root proxy; fall back to legacy http row.
	var domainID string
	err := r.db.QueryRowContext(ctx, `select id from domains where lower(host) = lower(?)`, domain.NormalizeRouteHost(host)).Scan(&domainID)
	if err == nil {
		proxy, pathErr := r.ByDomainAndPath(ctx, domainID, "/")
		if pathErr == nil {
			return proxy, nil
		}
	}
	return scanProxy(r.db.QueryRowContext(ctx, `select `+proxySelectColumns+` from proxies where type = ? and lower(entry_host) = lower(?) order by entry_bind_host <> '', entry_port <> 0 limit 1`, domain.ProxyHTTP, host))
}

func (r proxyRepository) ByHTTPSHost(ctx context.Context, host string) (domain.Proxy, error) {
	var domainID string
	err := r.db.QueryRowContext(ctx, `select id from domains where lower(host) = lower(?)`, domain.NormalizeRouteHost(host)).Scan(&domainID)
	if err == nil {
		proxy, pathErr := r.ByDomainAndPath(ctx, domainID, "/")
		if pathErr == nil {
			return proxy, nil
		}
	}
	return scanProxy(r.db.QueryRowContext(ctx, `select `+proxySelectColumns+` from proxies where type = ? and lower(entry_host) = lower(?) order by entry_bind_host <> '', entry_port <> 0 limit 1`, domain.ProxyHTTPS, host))
}

// ByCertificateID 返回绑定到指定证书资源的代理；若无绑定则返回 store.ErrNotFound。
func (r proxyRepository) ByCertificateID(ctx context.Context, certificateID string) (domain.Proxy, error) {
	return scanProxy(r.db.QueryRowContext(ctx, `select `+proxySelectColumns+` from proxies where certificate_id = ?`, certificateID))
}

func (r proxyRepository) byEntry(ctx context.Context, proxyType domain.ProxyType, bindHost string, port int, routeHost string, includeDefault bool) (domain.Proxy, error) {
	args := []any{proxyType, domain.NormalizeBindHost(bindHost), port}
	routeClause := ""
	if proxyType == domain.ProxyHTTP || proxyType == domain.ProxyHTTPS {
		routeClause = " and lower(entry_host) = lower(?)"
		args = append(args, routeHost)
	}
	defaultClause := ""
	if includeDefault {
		if proxyType == domain.ProxyHTTP || proxyType == domain.ProxyHTTPS {
			defaultClause = " or (entry_bind_host = '' and (entry_port = 0 or entry_port = ?)"
		} else {
			defaultClause = " or (entry_bind_host = '' and entry_port = ?"
		}
		args = append(args, port)
		if routeClause != "" {
			defaultClause += " and lower(entry_host) = lower(?)"
			args = append(args, routeHost)
		}
		defaultClause += ")"
	}
	query := `select ` + proxySelectColumns + ` from proxies where type = ? and ((lower(entry_bind_host) = lower(?) and entry_port = ?` + routeClause + `)` + defaultClause + `) order by case when lower(entry_bind_host) = lower(?) and entry_port = ? then 0 else 1 end limit 1`
	args = append(args, bindHost, port)
	return scanProxy(r.db.QueryRowContext(ctx, query, args...))
}

func (r proxyRepository) SetStatus(ctx context.Context, id string, status domain.ProxyStatus) error {
	result, err := r.db.ExecContext(ctx, `update proxies set status = ?, updated_at = ? where id = ?`, status, time.Now().UTC(), id)
	return resultError(result, err)
}

func (r proxyRepository) Update(ctx context.Context, proxy domain.Proxy) error {
	if err := normalizeProxyForStore(&proxy); err != nil {
		return err
	}
	if err := proxy.Validate(); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `update proxies set name = ?, status = ?, domain_id = ?, path_prefix = ?, strip_prefix = ?, upstream_path_prefix = ?, entry_bind_host = ?, entry_host = ?, entry_port = ?, target_host = ?, target_port = ?, cert_file = ?, key_file = ?, certificate_id = ?, access_auth_enabled = ?, access_auth_version = ?, stats_legacy_aggregate = ?, description = ?, updated_at = ? where id = ?`, proxy.Name, proxy.Status, proxy.DomainID, proxy.PathPrefix, proxy.StripPrefix, proxy.UpstreamPathPrefix, domain.NormalizeBindHost(proxy.EntryBindHost), proxy.EntryHost, proxy.EntryPort, proxy.TargetHost, proxy.TargetPort, proxy.CertFile, proxy.KeyFile, proxy.CertificateID, proxy.AccessAuthEnabled, proxy.AccessAuthVersion, proxy.StatsLegacyAggregate, proxy.Description, time.Now().UTC(), proxy.ID)
	return resultError(result, err)
}

func (r proxyRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from proxies where id = ?`, id)
	return resultError(result, err)
}

type certificateRepository struct{ db *sql.DB }

func (r certificateRepository) Create(ctx context.Context, certificate domain.ManagedCertificate) error {
	applyManagedCertificateDefaults(&certificate)
	if err := certificate.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if certificate.CreatedAt.IsZero() {
		certificate.CreatedAt = now
	}
	if certificate.UpdatedAt.IsZero() {
		certificate.UpdatedAt = now
	}
	hostnames, err := encodeStringList(certificate.Hostnames)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `insert into managed_certificates (id, proxy_id, host, status, serving_status, operation_status, provider, provider_type, provider_name, credential_id, provider_status, cloudflare_certificate_id, previous_cloudflare_certificate_id, hostnames, request_type, requested_validity, cert_file, key_file, previous_cert_file, previous_key_file, not_after, last_issued_at, last_renewed_at, last_checked_at, last_synced_at, last_attempted_at, next_attempt_at, failure_count, fingerprint, last_error, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, certificate.ID, certificate.ProxyID, certificate.Host, certificate.Status, certificate.ServingStatus, certificate.OperationStatus, certificate.Provider, certificate.ProviderType, certificate.ProviderName, certificate.CredentialID, certificate.ProviderStatus, certificate.CloudflareCertificateID, certificate.PreviousCloudflareCertificateID, hostnames, certificate.RequestType, certificate.RequestedValidity, certificate.CertFile, certificate.KeyFile, certificate.PreviousCertFile, certificate.PreviousKeyFile, certificate.NotAfter, certificate.LastIssuedAt, certificate.LastRenewedAt, certificate.LastCheckedAt, certificate.LastSyncedAt, certificate.LastAttemptedAt, certificate.NextAttemptAt, certificate.FailureCount, certificate.Fingerprint, certificate.LastError, certificate.CreatedAt, certificate.UpdatedAt)
	return translateError(err)
}

func (r certificateRepository) ByID(ctx context.Context, id string) (domain.ManagedCertificate, error) {
	return scanManagedCertificate(r.db.QueryRowContext(ctx, managedCertificateSelect+` where id = ?`, id))
}

func (r certificateRepository) ByProxyID(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	return scanManagedCertificate(r.db.QueryRowContext(ctx, managedCertificateSelect+` where proxy_id = ?`, proxyID))
}

func (r certificateRepository) ByHost(ctx context.Context, host string) (domain.ManagedCertificate, error) {
	return scanManagedCertificate(r.db.QueryRowContext(ctx, managedCertificateSelect+` where lower(host) = lower(?)`, host))
}

// Delete 按 ID 删除证书资源；若不存在则返回 store.ErrNotFound。
func (r certificateRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from managed_certificates where id = ?`, id)
	return resultError(result, err)
}

func (r certificateRepository) List(ctx context.Context) ([]domain.ManagedCertificate, error) {
	rows, err := r.db.QueryContext(ctx, managedCertificateSelect+` order by host, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	certificates := make([]domain.ManagedCertificate, 0)
	for rows.Next() {
		certificate, err := scanManagedCertificateRows(rows)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, certificate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return certificates, nil
}

func (r certificateRepository) ListByProxyIDs(ctx context.Context, proxyIDs []string) ([]domain.ManagedCertificate, error) {
	proxyIDs = compactStrings(proxyIDs)
	if len(proxyIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(proxyIDs))
	args := make([]any, len(proxyIDs))
	for index, proxyID := range proxyIDs {
		placeholders[index] = "?"
		args[index] = proxyID
	}
	rows, err := r.db.QueryContext(ctx, managedCertificateSelect+` where proxy_id in (`+strings.Join(placeholders, ",")+`) order by host, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	certificates := make([]domain.ManagedCertificate, 0)
	for rows.Next() {
		certificate, err := scanManagedCertificateRows(rows)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, certificate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return certificates, nil
}

func (r certificateRepository) ListRenewable(ctx context.Context, before time.Time, now time.Time) ([]domain.ManagedCertificate, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return r.ListLifecycleCandidates(ctx, store.CertificateLifecycleCandidateQuery{Now: now, ACMEBefore: &before, OriginCABefore: &before})
}

func (r certificateRepository) ListLifecycleCandidates(ctx context.Context, query store.CertificateLifecycleCandidateQuery) ([]domain.ManagedCertificate, error) {
	now := query.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	providerClauses := make([]string, 0, 2)
	args := []any{
		now,
		domain.CertificateServingUsable,
		domain.CertificateServingExpiringSoon,
		domain.CertificateValid,
		domain.CertificateExpiringSoon,
	}
	if query.ACMEBefore != nil {
		providerClauses = append(providerClauses, `((provider_type = '' or provider_type <> ?) and not_after <= ?)`)
		args = append(args, domain.CertificateProviderCloudflareOriginCA, *query.ACMEBefore)
	}
	if query.OriginCABefore != nil {
		providerClauses = append(providerClauses, `(provider_type = ? and not_after <= ?)`)
		args = append(args, domain.CertificateProviderCloudflareOriginCA, *query.OriginCABefore)
	}
	if len(providerClauses) == 0 {
		return nil, nil
	}
	statement := managedCertificateSelect + ` where not_after is not null and (next_attempt_at is null or next_attempt_at <= ?) and (serving_status in (?, ?) or (serving_status = '' and status in (?, ?))) and (` + strings.Join(providerClauses, ` or `) + `) order by not_after, host`
	rows, err := r.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	certificates := make([]domain.ManagedCertificate, 0)
	for rows.Next() {
		certificate, err := scanManagedCertificateRows(rows)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, certificate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return certificates, nil
}

func (r certificateRepository) UpdateSuccess(ctx context.Context, id string, result store.CertificateSuccess) error {
	completedAt := result.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	servingStatus := result.ServingStatus
	if servingStatus == "" {
		servingStatus = domain.CertificateServingUsable
	}
	lastCheckedAt := result.LastCheckedAt
	if lastCheckedAt.IsZero() {
		lastCheckedAt = completedAt
	}
	lastAttemptedAt := result.LastAttemptedAt
	if lastAttemptedAt.IsZero() {
		lastAttemptedAt = completedAt
	}
	query := `update managed_certificates set status = ?, serving_status = ?, operation_status = ?, cert_file = ?, key_file = ?, previous_cert_file = ?, previous_key_file = ?, not_after = ?, last_checked_at = ?, last_attempted_at = ?, next_attempt_at = null, failure_count = 0, fingerprint = ?, last_error = '', updated_at = ?`
	args := []any{certificateStatusFromServing(servingStatus), servingStatus, domain.CertificateOperationIdle, result.CertFile, result.KeyFile, result.PreviousCertFile, result.PreviousKeyFile, result.NotAfter, lastCheckedAt, lastAttemptedAt, strings.ToLower(result.Fingerprint), completedAt}
	if result.ProviderStatus != "" {
		query += `, provider_status = ?`
		args = append(args, result.ProviderStatus)
	}
	if result.ProviderType != "" {
		query += `, provider_type = ?`
		args = append(args, result.ProviderType)
	}
	if result.ProviderName != "" {
		query += `, provider_name = ?, provider = ?`
		args = append(args, result.ProviderName, result.ProviderName)
	}
	if result.CredentialID != "" {
		query += `, credential_id = ?`
		args = append(args, result.CredentialID)
	}
	if result.CloudflareID != "" {
		query += `, cloudflare_certificate_id = ?`
		args = append(args, result.CloudflareID)
	}
	if result.PreviousCloudflareID != "" {
		query += `, previous_cloudflare_certificate_id = ?`
		args = append(args, result.PreviousCloudflareID)
	}
	if result.Hostnames != nil {
		hostnames, err := encodeStringList(result.Hostnames)
		if err != nil {
			return err
		}
		query += `, hostnames = ?`
		args = append(args, hostnames)
	}
	if result.RequestType != "" {
		query += `, request_type = ?`
		args = append(args, result.RequestType)
	}
	if result.RequestedValidity > 0 {
		query += `, requested_validity = ?`
		args = append(args, result.RequestedValidity)
	}
	if result.LastSyncedAt != nil {
		query += `, last_synced_at = ?`
		args = append(args, result.LastSyncedAt)
	}
	if result.PreviousCertFile == "" && result.PreviousKeyFile == "" {
		query += `, last_issued_at = ?`
		args = append(args, completedAt)
	} else {
		query += `, last_renewed_at = ?`
		args = append(args, completedAt)
	}
	query += ` where id = ?`
	args = append(args, id)
	resultSQL, err := r.db.ExecContext(ctx, query, args...)
	return resultError(resultSQL, err)
}

func (r certificateRepository) UpdateFailure(ctx context.Context, id string, failure store.CertificateFailure) error {
	completedAt := failure.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	status := failure.Status
	if status == "" {
		status = domain.CertificateIssueFailed
	}
	operationStatus := failure.OperationStatus
	if operationStatus == "" {
		operationStatus = operationStatusFromCertificateStatus(status)
	}
	lastAttemptedAt := failure.LastAttemptedAt
	if lastAttemptedAt.IsZero() {
		lastAttemptedAt = completedAt
	}
	query := `update managed_certificates set status = ?, operation_status = ?, last_error = ?, last_attempted_at = ?, next_attempt_at = ?, failure_count = case when ? > 0 then ? else failure_count + 1 end, updated_at = ?`
	args := []any{status, operationStatus, failure.LastError, lastAttemptedAt, failure.NextAttemptAt, failure.FailureCount, failure.FailureCount, completedAt}
	if failure.ServingStatus != "" {
		query += `, serving_status = ?`
		args = append(args, failure.ServingStatus)
	}
	if failure.ProviderStatus != "" {
		query += `, provider_status = ?`
		args = append(args, failure.ProviderStatus)
	}
	if !failure.LastCheckedAt.IsZero() {
		query += `, last_checked_at = ?`
		args = append(args, failure.LastCheckedAt)
	}
	if failure.LastSyncedAt != nil {
		query += `, last_synced_at = ?`
		args = append(args, failure.LastSyncedAt)
	}
	query += ` where id = ?`
	args = append(args, id)
	result, err := r.db.ExecContext(ctx, query, args...)
	return resultError(result, err)
}

func (r certificateRepository) UpdateHealth(ctx context.Context, id string, health store.CertificateHealth) error {
	checkedAt := health.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	servingStatus := health.ServingStatus
	if servingStatus == "" {
		servingStatus = domain.CertificateServingInvalid
	}
	result, err := r.db.ExecContext(ctx, `update managed_certificates set status = case when status in (?, ?) then status else ? end, serving_status = ?, not_after = ?, fingerprint = ?, last_error = case when status in (?, ?) then last_error else ? end, last_checked_at = ?, updated_at = ? where id = ?`, domain.CertificateIssueFailed, domain.CertificateRenewalFailed, certificateStatusFromServing(servingStatus), servingStatus, health.NotAfter, strings.ToLower(health.Fingerprint), domain.CertificateIssueFailed, domain.CertificateRenewalFailed, health.LastError, checkedAt, checkedAt, id)
	return resultError(result, err)
}

func (r certificateRepository) UpdateProviderSync(ctx context.Context, id string, sync store.CertificateProviderSync) error {
	syncedAt := sync.SyncedAt
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}
	updatedAt := sync.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = syncedAt
	}
	status := sync.ProviderStatus
	if status == "" {
		status = domain.CertificateProviderStatusUnknown
	}
	lastError := sync.LastError
	if lastError == "" {
		lastError = providerStatusError(status)
	}
	result, err := r.db.ExecContext(ctx, `update managed_certificates set provider_status = ?, last_synced_at = ?, last_error = case when ? <> '' then ? when status in (?, ?) then last_error else '' end, updated_at = ? where id = ?`, status, syncedAt, lastError, lastError, domain.CertificateIssueFailed, domain.CertificateRenewalFailed, updatedAt, id)
	return resultError(result, err)
}

func providerStatusError(status domain.CertificateProviderStatus) string {
	switch status {
	case domain.CertificateProviderStatusRevoked:
		return "certificate provider marked active material revoked"
	case domain.CertificateProviderStatusMissingRemote:
		return "certificate provider active material is missing remotely"
	default:
		return ""
	}
}

type providerCredentialRepository struct{ db *sql.DB }

func (r providerCredentialRepository) Create(ctx context.Context, credential domain.ProviderCredential) error {
	if err := credential.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if credential.CreatedAt.IsZero() {
		credential.CreatedAt = now
	}
	if credential.UpdatedAt.IsZero() {
		credential.UpdatedAt = now
	}
	_, err := r.db.ExecContext(ctx, `insert into provider_credentials (id, name, provider_type, scope, token_fingerprint, secret_ref, status, last_verified_at, last_error, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, credential.ID, credential.Name, credential.ProviderType, credential.Scope, credential.TokenFingerprint, credential.SecretRef, credential.Status, credential.LastVerifiedAt, credential.LastError, credential.CreatedAt, credential.UpdatedAt)
	return translateError(err)
}

func (r providerCredentialRepository) ByID(ctx context.Context, id string) (domain.ProviderCredential, error) {
	return scanProviderCredential(r.db.QueryRowContext(ctx, providerCredentialSelect+` where id = ?`, id))
}

func (r providerCredentialRepository) List(ctx context.Context) ([]domain.ProviderCredential, error) {
	rows, err := r.db.QueryContext(ctx, providerCredentialSelect+` order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	credentials := make([]domain.ProviderCredential, 0)
	for rows.Next() {
		credential, err := scanProviderCredentialRows(rows)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return credentials, nil
}

func (r providerCredentialRepository) ListByProviderType(ctx context.Context, providerType domain.CertificateProviderType, statuses []domain.ProviderCredentialStatus) ([]domain.ProviderCredential, error) {
	providerType = domain.CertificateProviderType(strings.TrimSpace(string(providerType)))
	if !providerType.Valid() {
		return nil, errors.New("provider credential type is invalid")
	}
	args := []any{providerType}
	statement := providerCredentialSelect + ` where provider_type = ?`
	if len(statuses) > 0 {
		placeholders := make([]string, 0, len(statuses))
		for _, status := range statuses {
			if !status.Valid() {
				return nil, errors.New("provider credential status is invalid")
			}
			placeholders = append(placeholders, "?")
			args = append(args, status)
		}
		statement += ` and status in (` + strings.Join(placeholders, ",") + `)`
	}
	rows, err := r.db.QueryContext(ctx, statement+` order by created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	credentials := make([]domain.ProviderCredential, 0)
	for rows.Next() {
		credential, err := scanProviderCredentialRows(rows)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return credentials, nil
}

func (r providerCredentialRepository) Update(ctx context.Context, credential domain.ProviderCredential) error {
	if err := credential.Validate(); err != nil {
		return err
	}
	if credential.UpdatedAt.IsZero() {
		credential.UpdatedAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, `update provider_credentials set name = ?, provider_type = ?, scope = ?, token_fingerprint = ?, secret_ref = ?, status = ?, last_verified_at = ?, last_error = ?, updated_at = ? where id = ?`, credential.Name, credential.ProviderType, credential.Scope, credential.TokenFingerprint, credential.SecretRef, credential.Status, credential.LastVerifiedAt, credential.LastError, credential.UpdatedAt, credential.ID)
	return resultError(result, err)
}

func (r providerCredentialRepository) SetStatus(ctx context.Context, id string, status domain.ProviderCredentialStatus, lastVerifiedAt *time.Time, lastError string) error {
	result, err := r.db.ExecContext(ctx, `update provider_credentials set status = ?, last_verified_at = ?, last_error = ?, updated_at = ? where id = ?`, status, lastVerifiedAt, lastError, time.Now().UTC(), id)
	return resultError(result, err)
}

func (r providerCredentialRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from provider_credentials where id = ?`, id)
	return resultError(result, err)
}

type auditRepository struct{ db *sql.DB }

func (r auditRepository) Create(ctx context.Context, event domain.AuditEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		return errors.New("audit event id is required")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `insert into audit_events (id, actor_user_id, resource_type, resource_id, action, result, source_ip, error_summary, created_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.ActorUserID, event.ResourceType, event.ResourceID, event.Action, event.Result, event.SourceIP, event.ErrorSummary, event.CreatedAt)
	return translateError(err)
}

func (r auditRepository) ListRecent(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `select id, actor_user_id, resource_type, resource_id, action, result, source_ip, error_summary, created_at from audit_events order by created_at desc, id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		event, err := scanAuditEventRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

type statsRepository struct{ db *sql.DB }

func (r statsRepository) Save(ctx context.Context, snapshots []store.ProxyStats) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, snapshot := range snapshots {
		statusCodes, err := json.Marshal(snapshot.HTTPStatusCodes)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `insert into proxy_stats (proxy_id, tcp_connections, tcp_upload_bytes, tcp_download_bytes, tcp_errors, udp_packets, udp_upload_bytes, udp_download_bytes, udp_errors, http_requests, http_upload_bytes, http_download_bytes, http_errors, http_status_codes, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
on conflict(proxy_id) do update set
    tcp_connections = excluded.tcp_connections,
    tcp_upload_bytes = excluded.tcp_upload_bytes,
    tcp_download_bytes = excluded.tcp_download_bytes,
    tcp_errors = excluded.tcp_errors,
    udp_packets = excluded.udp_packets,
    udp_upload_bytes = excluded.udp_upload_bytes,
    udp_download_bytes = excluded.udp_download_bytes,
    udp_errors = excluded.udp_errors,
    http_requests = excluded.http_requests,
    http_upload_bytes = excluded.http_upload_bytes,
    http_download_bytes = excluded.http_download_bytes,
    http_errors = excluded.http_errors,
    http_status_codes = excluded.http_status_codes,
    updated_at = excluded.updated_at`, snapshot.ProxyID, snapshot.TCPConnections, snapshot.TCPUploadBytes, snapshot.TCPDownloadBytes, snapshot.TCPErrors, snapshot.UDPPackets, snapshot.UDPUploadBytes, snapshot.UDPDownloadBytes, snapshot.UDPErrors, snapshot.HTTPRequests, snapshot.HTTPUploadBytes, snapshot.HTTPDownloadBytes, snapshot.HTTPErrors, string(statusCodes), time.Now().UTC())
		if err != nil {
			return translateError(err)
		}
	}
	return tx.Commit()
}

func (r statsRepository) List(ctx context.Context) ([]store.ProxyStats, error) {
	rows, err := r.db.QueryContext(ctx, `select proxy_id, tcp_connections, tcp_upload_bytes, tcp_download_bytes, tcp_errors, udp_packets, udp_upload_bytes, udp_download_bytes, udp_errors, http_requests, http_upload_bytes, http_download_bytes, http_errors, http_status_codes from proxy_stats order by proxy_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := make([]store.ProxyStats, 0)
	for rows.Next() {
		var snapshot store.ProxyStats
		var statusCodes string
		if err := rows.Scan(&snapshot.ProxyID, &snapshot.TCPConnections, &snapshot.TCPUploadBytes, &snapshot.TCPDownloadBytes, &snapshot.TCPErrors, &snapshot.UDPPackets, &snapshot.UDPUploadBytes, &snapshot.UDPDownloadBytes, &snapshot.UDPErrors, &snapshot.HTTPRequests, &snapshot.HTTPUploadBytes, &snapshot.HTTPDownloadBytes, &snapshot.HTTPErrors, &statusCodes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(statusCodes), &snapshot.HTTPStatusCodes); err != nil {
			return nil, err
		}
		if snapshot.HTTPStatusCodes == nil {
			snapshot.HTTPStatusCodes = make(map[int]int64)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func scanUser(row *sql.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	return user, translateError(err)
}

func scanUserRows(rows *sql.Rows) (domain.User, error) {
	var user domain.User
	err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func scanClient(row *sql.Row) (domain.Client, error) {
	var client domain.Client
	err := row.Scan(&client.ID, &client.UserID, &client.Name, &client.Kind, &client.Status, &client.CredentialHash, &client.Version, &client.LastOnlineAt, &client.LastOfflineAt, &client.CreatedAt, &client.UpdatedAt)
	if client.Kind == "" {
		client.Kind = domain.ClientKindProvider
	}
	return client, translateError(err)
}

func scanClientRows(rows *sql.Rows) (domain.Client, error) {
	var client domain.Client
	err := rows.Scan(&client.ID, &client.UserID, &client.Name, &client.Kind, &client.Status, &client.CredentialHash, &client.Version, &client.LastOnlineAt, &client.LastOfflineAt, &client.CreatedAt, &client.UpdatedAt)
	if client.Kind == "" {
		client.Kind = domain.ClientKindProvider
	}
	return client, err
}

func scanClientEnrollment(row *sql.Row) (domain.ClientEnrollment, error) {
	var enrollment domain.ClientEnrollment
	var usedAt sql.NullTime
	err := row.Scan(&enrollment.ID, &enrollment.ClientID, &enrollment.SecretHash, &enrollment.TokenHash, &enrollment.Token, &enrollment.ExpiresAt, &usedAt, &enrollment.CreatedAt, &enrollment.UpdatedAt)
	if usedAt.Valid {
		enrollment.UsedAt = &usedAt.Time
	}
	return enrollment, translateError(err)
}

func scanProxy(row *sql.Row) (domain.Proxy, error) {
	var proxy domain.Proxy
	err := row.Scan(&proxy.ID, &proxy.UserID, &proxy.ClientID, &proxy.Name, &proxy.Type, &proxy.Status, &proxy.DomainID, &proxy.PathPrefix, &proxy.StripPrefix, &proxy.UpstreamPathPrefix, &proxy.EntryBindHost, &proxy.EntryHost, &proxy.EntryPort, &proxy.TargetHost, &proxy.TargetPort, &proxy.CertFile, &proxy.KeyFile, &proxy.CertificateID, &proxy.AccessAuthEnabled, &proxy.AccessAuthVersion, &proxy.StatsLegacyAggregate, &proxy.Description, &proxy.CreatedAt, &proxy.UpdatedAt)
	return proxy, translateError(err)
}

func scanProxyRows(rows *sql.Rows) (domain.Proxy, error) {
	var proxy domain.Proxy
	err := rows.Scan(&proxy.ID, &proxy.UserID, &proxy.ClientID, &proxy.Name, &proxy.Type, &proxy.Status, &proxy.DomainID, &proxy.PathPrefix, &proxy.StripPrefix, &proxy.UpstreamPathPrefix, &proxy.EntryBindHost, &proxy.EntryHost, &proxy.EntryPort, &proxy.TargetHost, &proxy.TargetPort, &proxy.CertFile, &proxy.KeyFile, &proxy.CertificateID, &proxy.AccessAuthEnabled, &proxy.AccessAuthVersion, &proxy.StatsLegacyAggregate, &proxy.Description, &proxy.CreatedAt, &proxy.UpdatedAt)
	return proxy, err
}

const managedCertificateSelect = `select id, proxy_id, host, status, serving_status, operation_status, provider, provider_type, provider_name, credential_id, provider_status, cloudflare_certificate_id, previous_cloudflare_certificate_id, hostnames, request_type, requested_validity, cert_file, key_file, previous_cert_file, previous_key_file, not_after, last_issued_at, last_renewed_at, last_checked_at, last_synced_at, last_attempted_at, next_attempt_at, failure_count, fingerprint, last_error, created_at, updated_at from managed_certificates`

func scanManagedCertificate(row *sql.Row) (domain.ManagedCertificate, error) {
	var certificate domain.ManagedCertificate
	var notAfter sql.NullTime
	var lastIssuedAt sql.NullTime
	var lastRenewedAt sql.NullTime
	var lastCheckedAt sql.NullTime
	var lastSyncedAt sql.NullTime
	var lastAttemptedAt sql.NullTime
	var nextAttemptAt sql.NullTime
	var hostnames string
	err := row.Scan(&certificate.ID, &certificate.ProxyID, &certificate.Host, &certificate.Status, &certificate.ServingStatus, &certificate.OperationStatus, &certificate.Provider, &certificate.ProviderType, &certificate.ProviderName, &certificate.CredentialID, &certificate.ProviderStatus, &certificate.CloudflareCertificateID, &certificate.PreviousCloudflareCertificateID, &hostnames, &certificate.RequestType, &certificate.RequestedValidity, &certificate.CertFile, &certificate.KeyFile, &certificate.PreviousCertFile, &certificate.PreviousKeyFile, &notAfter, &lastIssuedAt, &lastRenewedAt, &lastCheckedAt, &lastSyncedAt, &lastAttemptedAt, &nextAttemptAt, &certificate.FailureCount, &certificate.Fingerprint, &certificate.LastError, &certificate.CreatedAt, &certificate.UpdatedAt)
	if err == nil {
		certificate.Hostnames, err = decodeStringList(hostnames)
	}
	applyManagedCertificateTimes(&certificate, notAfter, lastIssuedAt, lastRenewedAt, lastCheckedAt, lastSyncedAt, lastAttemptedAt, nextAttemptAt)
	applyManagedCertificateDefaults(&certificate)
	return certificate, translateError(err)
}

func scanManagedCertificateRows(rows *sql.Rows) (domain.ManagedCertificate, error) {
	var certificate domain.ManagedCertificate
	var notAfter sql.NullTime
	var lastIssuedAt sql.NullTime
	var lastRenewedAt sql.NullTime
	var lastCheckedAt sql.NullTime
	var lastSyncedAt sql.NullTime
	var lastAttemptedAt sql.NullTime
	var nextAttemptAt sql.NullTime
	var hostnames string
	err := rows.Scan(&certificate.ID, &certificate.ProxyID, &certificate.Host, &certificate.Status, &certificate.ServingStatus, &certificate.OperationStatus, &certificate.Provider, &certificate.ProviderType, &certificate.ProviderName, &certificate.CredentialID, &certificate.ProviderStatus, &certificate.CloudflareCertificateID, &certificate.PreviousCloudflareCertificateID, &hostnames, &certificate.RequestType, &certificate.RequestedValidity, &certificate.CertFile, &certificate.KeyFile, &certificate.PreviousCertFile, &certificate.PreviousKeyFile, &notAfter, &lastIssuedAt, &lastRenewedAt, &lastCheckedAt, &lastSyncedAt, &lastAttemptedAt, &nextAttemptAt, &certificate.FailureCount, &certificate.Fingerprint, &certificate.LastError, &certificate.CreatedAt, &certificate.UpdatedAt)
	if err == nil {
		certificate.Hostnames, err = decodeStringList(hostnames)
	}
	applyManagedCertificateTimes(&certificate, notAfter, lastIssuedAt, lastRenewedAt, lastCheckedAt, lastSyncedAt, lastAttemptedAt, nextAttemptAt)
	applyManagedCertificateDefaults(&certificate)
	return certificate, err
}

func applyManagedCertificateTimes(certificate *domain.ManagedCertificate, notAfter sql.NullTime, lastIssuedAt sql.NullTime, lastRenewedAt sql.NullTime, lastCheckedAt sql.NullTime, lastSyncedAt sql.NullTime, lastAttemptedAt sql.NullTime, nextAttemptAt sql.NullTime) {
	if notAfter.Valid {
		certificate.NotAfter = &notAfter.Time
	}
	if lastIssuedAt.Valid {
		certificate.LastIssuedAt = &lastIssuedAt.Time
	}
	if lastRenewedAt.Valid {
		certificate.LastRenewedAt = &lastRenewedAt.Time
	}
	if lastCheckedAt.Valid {
		certificate.LastCheckedAt = &lastCheckedAt.Time
	}
	if lastSyncedAt.Valid {
		certificate.LastSyncedAt = &lastSyncedAt.Time
	}
	if lastAttemptedAt.Valid {
		certificate.LastAttemptedAt = &lastAttemptedAt.Time
	}
	if nextAttemptAt.Valid {
		certificate.NextAttemptAt = &nextAttemptAt.Time
	}
}

func applyManagedCertificateDefaults(certificate *domain.ManagedCertificate) {
	if certificate.ProviderName == "" {
		certificate.ProviderName = certificate.Provider
	}
	if certificate.ProviderType == "" {
		certificate.ProviderType = domain.CertificateProviderACMEDNS01
	}
	if certificate.Provider == "" {
		certificate.Provider = certificate.ProviderName
	}
	if certificate.ProviderName == "" {
		certificate.ProviderName = certificate.Provider
	}
	if certificate.ProviderStatus == "" {
		certificate.ProviderStatus = domain.CertificateProviderStatusUnknown
	}
	if certificate.ServingStatus == "" {
		certificate.ServingStatus = servingStatusFromLegacyStatus(*certificate)
	}
	if certificate.OperationStatus == "" {
		certificate.OperationStatus = operationStatusFromCertificateStatus(certificate.Status)
	}
	certificate.Fingerprint = strings.ToLower(certificate.Fingerprint)
}

const providerCredentialSelect = `select id, name, provider_type, scope, token_fingerprint, secret_ref, status, last_verified_at, last_error, created_at, updated_at from provider_credentials`

func scanProviderCredential(row *sql.Row) (domain.ProviderCredential, error) {
	var credential domain.ProviderCredential
	var lastVerifiedAt sql.NullTime
	err := row.Scan(&credential.ID, &credential.Name, &credential.ProviderType, &credential.Scope, &credential.TokenFingerprint, &credential.SecretRef, &credential.Status, &lastVerifiedAt, &credential.LastError, &credential.CreatedAt, &credential.UpdatedAt)
	if lastVerifiedAt.Valid {
		credential.LastVerifiedAt = &lastVerifiedAt.Time
	}
	return credential, translateError(err)
}

func scanProviderCredentialRows(rows *sql.Rows) (domain.ProviderCredential, error) {
	var credential domain.ProviderCredential
	var lastVerifiedAt sql.NullTime
	err := rows.Scan(&credential.ID, &credential.Name, &credential.ProviderType, &credential.Scope, &credential.TokenFingerprint, &credential.SecretRef, &credential.Status, &lastVerifiedAt, &credential.LastError, &credential.CreatedAt, &credential.UpdatedAt)
	if lastVerifiedAt.Valid {
		credential.LastVerifiedAt = &lastVerifiedAt.Time
	}
	return credential, err
}

func servingStatusFromLegacyStatus(certificate domain.ManagedCertificate) domain.CertificateServingStatus {
	if certificate.CertFile == "" || certificate.KeyFile == "" {
		return domain.CertificateServingMissing
	}
	switch certificate.Status {
	case domain.CertificateValid:
		return domain.CertificateServingUsable
	case domain.CertificateExpiringSoon:
		return domain.CertificateServingExpiringSoon
	case domain.CertificateExpired:
		return domain.CertificateServingExpired
	default:
		return domain.CertificateServingMissing
	}
}

func operationStatusFromCertificateStatus(status domain.CertificateStatus) domain.CertificateOperationStatus {
	switch status {
	case domain.CertificateIssueFailed:
		return domain.CertificateOperationIssueFailed
	case domain.CertificateRenewalFailed:
		return domain.CertificateOperationRenewalFailed
	default:
		return domain.CertificateOperationIdle
	}
}

func certificateStatusFromServing(status domain.CertificateServingStatus) domain.CertificateStatus {
	switch status {
	case domain.CertificateServingUsable:
		return domain.CertificateValid
	case domain.CertificateServingExpiringSoon:
		return domain.CertificateExpiringSoon
	case domain.CertificateServingExpired:
		return domain.CertificateExpired
	default:
		return domain.CertificatePending
	}
}

func scanAuditEventRows(rows *sql.Rows) (domain.AuditEvent, error) {
	var event domain.AuditEvent
	err := rows.Scan(&event.ID, &event.ActorUserID, &event.ResourceType, &event.ResourceID, &event.Action, &event.Result, &event.SourceIP, &event.ErrorSummary, &event.CreatedAt)
	return event, err
}

func encodeStringList(values []string) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	content, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func decodeStringList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func addProxyCertificateColumns(ctx context.Context, db *sql.DB) error {
	if err := addColumnIfMissing(ctx, db, "proxies", "cert_file", "text not null default ''"); err != nil {
		return err
	}
	return addColumnIfMissing(ctx, db, "proxies", "key_file", "text not null default ''")
}

func addProxyEntryBindHostColumn(ctx context.Context, db *sql.DB) error {
	return addColumnIfMissing(ctx, db, "proxies", "entry_bind_host", "text not null default ''")
}

func migrateProxyEntryIndexes(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`drop index if exists proxies_tcp_entry_port_unique`,
		`drop index if exists proxies_udp_entry_port_unique`,
		`drop index if exists proxies_http_entry_host_unique`,
		`drop index if exists proxies_https_entry_host_unique`,
		`create unique index if not exists proxies_tcp_entry_unique on proxies(lower(entry_bind_host), entry_port) where type = 'tcp' and entry_port > 0`,
		`create unique index if not exists proxies_udp_entry_unique on proxies(lower(entry_bind_host), entry_port) where type = 'udp' and entry_port > 0`,
		`create unique index if not exists proxies_http_route_unique on proxies(lower(entry_bind_host), entry_port, lower(entry_host)) where type = 'http' and entry_host <> ''`,
		`create unique index if not exists proxies_https_route_unique on proxies(lower(entry_bind_host), entry_port, lower(entry_host)) where type = 'https' and entry_host <> ''`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func addUserPasswordColumn(ctx context.Context, db *sql.DB) error {
	return addColumnIfMissing(ctx, db, "users", "password_hash", "text not null default ''")
}

func addClientKindColumn(ctx context.Context, db *sql.DB) error {
	return addColumnIfMissing(ctx, db, "clients", "kind", "text not null default 'provider'")
}

func addProxyAccessAuthColumns(ctx context.Context, db *sql.DB) error {
	if err := addColumnIfMissing(ctx, db, "proxies", "access_auth_enabled", "integer not null default 0"); err != nil {
		return err
	}
	return addColumnIfMissing(ctx, db, "proxies", "access_auth_version", "integer not null default 0")
}

func addClientEnrollmentTokenColumn(ctx context.Context, db *sql.DB) error {
	return addColumnIfMissing(ctx, db, "client_enrollments", "token", "text not null default ''")
}

func addManagedCertificateLifecycleColumns(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		name       string
		definition string
	}{
		{"serving_status", "text not null default ''"},
		{"operation_status", "text not null default ''"},
		{"last_checked_at", "timestamp"},
		{"last_attempted_at", "timestamp"},
		{"next_attempt_at", "timestamp"},
		{"failure_count", "integer not null default 0"},
		{"fingerprint", "text not null default ''"},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(ctx, db, "managed_certificates", column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
update managed_certificates
set serving_status = case
    when cert_file = '' or key_file = '' then ?
    when status = ? then ?
    when status = ? then ?
    when status = ? then ?
    else ?
end
where serving_status = ''`,
		domain.CertificateServingMissing,
		domain.CertificateValid, domain.CertificateServingUsable,
		domain.CertificateExpiringSoon, domain.CertificateServingExpiringSoon,
		domain.CertificateExpired, domain.CertificateServingExpired,
		domain.CertificateServingMissing); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
update managed_certificates
set operation_status = case
    when status = ? then ?
    when status = ? then ?
    else ?
end
where operation_status = ''`,
		domain.CertificateIssueFailed, domain.CertificateOperationIssueFailed,
		domain.CertificateRenewalFailed, domain.CertificateOperationRenewalFailed,
		domain.CertificateOperationIdle)
	return err
}

func addManagedCertificateProviderColumns(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		name       string
		definition string
	}{
		{"provider_type", "text not null default ''"},
		{"provider_name", "text not null default ''"},
		{"credential_id", "text not null default ''"},
		{"provider_status", "text not null default ''"},
		{"cloudflare_certificate_id", "text not null default ''"},
		{"previous_cloudflare_certificate_id", "text not null default ''"},
		{"hostnames", "text not null default '[]'"},
		{"request_type", "text not null default ''"},
		{"requested_validity", "integer not null default 0"},
		{"last_synced_at", "timestamp"},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(ctx, db, "managed_certificates", column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
update managed_certificates
set provider_type = ?
where provider_type = ''`,
		domain.CertificateProviderACMEDNS01); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
update managed_certificates
set provider_name = case when provider <> '' then provider else ? end
where provider_name = ''`,
		"cloudflare"); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
update managed_certificates
set provider_status = ?
where provider_status = ''`,
		domain.CertificateProviderStatusUnknown)
	return err
}

func addCertificateQueryIndexes(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`create index if not exists managed_certificates_lifecycle_provider_idx on managed_certificates(provider_type, not_after, next_attempt_at)`,
		`create index if not exists provider_credentials_provider_status_idx on provider_credentials(provider_type, status)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// migrateProxyCertificateBinding 为已有数据库的 proxies 表补充 certificate_id 列，
// 并建立“一证一代理”的部分唯一索引（仅对非空 certificate_id 生效）。幂等。
func migrateProxyCertificateBinding(ctx context.Context, db *sql.DB) error {
	if err := addColumnIfMissing(ctx, db, "proxies", "certificate_id", "text not null default ''"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `create unique index if not exists proxies_certificate_id_unique on proxies(certificate_id) where certificate_id != ''`); err != nil {
		return err
	}
	return nil
}

// migrateManagedCertificateProxyOptional 重建 managed_certificates 表，
// 去除 proxy_id 上的 on delete cascade 外键以及 managed_certificates_proxy_unique 唯一索引，
// 使证书成为独立资源（proxy_id 仅作为遗留反向引用，可为空）。
// 幂等保护：通过 PRAGMA foreign_key_list 检测是否仍存在指向 proxies 的外键，
// 若已无外键则说明重建已完成，直接返回。
func migrateManagedCertificateProxyOptional(ctx context.Context, db *sql.DB) error {
	hasProxyFK, err := managedCertificatesHasProxyForeignKey(ctx, db)
	if err != nil {
		return err
	}
	if !hasProxyFK {
		// 已经重建过（或属于全新数据库，建表时即无外键），无需处理。
		return nil
	}

	// 关闭外键约束以便安全重建表（必须在事务之外执行）。
	if _, err := db.ExecContext(ctx, `pragma foreign_keys=off`); err != nil {
		return err
	}
	// 无论成功与否，结束时恢复外键约束。
	defer func() { _, _ = db.ExecContext(ctx, `pragma foreign_keys=on`) }()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 新表列顺序/类型必须与当前表完全一致，仅将 proxy_id 改为无外键、默认 '' 的普通列。
	statements := []string{
		`create table managed_certificates_new (
    id text primary key,
    proxy_id text not null default '',
    host text not null,
    status text not null,
    serving_status text not null default '',
    operation_status text not null default '',
    provider text not null default '',
    provider_type text not null default '',
    provider_name text not null default '',
    credential_id text not null default '',
    provider_status text not null default '',
    cloudflare_certificate_id text not null default '',
    previous_cloudflare_certificate_id text not null default '',
    hostnames text not null default '[]',
    request_type text not null default '',
    requested_validity integer not null default 0,
    cert_file text not null default '',
    key_file text not null default '',
    previous_cert_file text not null default '',
    previous_key_file text not null default '',
    not_after timestamp,
    last_issued_at timestamp,
    last_renewed_at timestamp,
    last_checked_at timestamp,
    last_synced_at timestamp,
    last_attempted_at timestamp,
    next_attempt_at timestamp,
    failure_count integer not null default 0,
    fingerprint text not null default '',
    last_error text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
)`,
		`insert into managed_certificates_new (id, proxy_id, host, status, serving_status, operation_status, provider, provider_type, provider_name, credential_id, provider_status, cloudflare_certificate_id, previous_cloudflare_certificate_id, hostnames, request_type, requested_validity, cert_file, key_file, previous_cert_file, previous_key_file, not_after, last_issued_at, last_renewed_at, last_checked_at, last_synced_at, last_attempted_at, next_attempt_at, failure_count, fingerprint, last_error, created_at, updated_at)
select id, proxy_id, host, status, serving_status, operation_status, provider, provider_type, provider_name, credential_id, provider_status, cloudflare_certificate_id, previous_cloudflare_certificate_id, hostnames, request_type, requested_validity, cert_file, key_file, previous_cert_file, previous_key_file, not_after, last_issued_at, last_renewed_at, last_checked_at, last_synced_at, last_attempted_at, next_attempt_at, failure_count, fingerprint, last_error, created_at, updated_at from managed_certificates`,
		`drop table managed_certificates`,
		`alter table managed_certificates_new rename to managed_certificates`,
		// 重建表会丢弃旧索引：恢复 host 唯一索引与生命周期查询索引；
		// 不再重建 proxy 唯一索引（绑定关系已迁移到代理侧）。
		`create unique index if not exists managed_certificates_host_unique on managed_certificates(lower(host))`,
		`create index if not exists managed_certificates_lifecycle_provider_idx on managed_certificates(provider_type, not_after, next_attempt_at)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}

	// 校验外键完整性（此时应不再有指向 proxies 的外键违例）。
	if _, err := tx.ExecContext(ctx, `pragma foreign_key_check`); err != nil {
		return err
	}
	return tx.Commit()
}

// managedCertificatesHasProxyForeignKey 检测 managed_certificates 是否仍持有指向 proxies 的外键。
func managedCertificatesHasProxyForeignKey(ctx context.Context, db *sql.DB) (bool, error) {
	rows, err := db.QueryContext(ctx, `pragma foreign_key_list(managed_certificates)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id       int
			seq      int
			table    string
			from     string
			to       sql.NullString
			onUpdate string
			onDelete string
			match    string
		)
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return false, err
		}
		if strings.EqualFold(table, "proxies") {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
}

// migrateBindProxyCertificates 将旧的“证书反向引用 proxy_id”回填为代理侧的权威绑定
// proxies.certificate_id（设计迁移步骤 2：旧 managed cert -> proxy.certificate_id）。
// 仅当代理尚未绑定证书（certificate_id = ''）且存在对应反向引用证书时才回填。幂等。
// TODO(phase2): 旧代理静态证书（cert_file/key_file）迁移为文件型证书资源
// （provider_type=file）需要 ID 生成与 certmanager 逻辑，留待 Phase 2 服务层实现。
func migrateBindProxyCertificates(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `update proxies set certificate_id = (select mc.id from managed_certificates mc where mc.proxy_id = proxies.id) where certificate_id = '' and exists(select 1 from managed_certificates mc where mc.proxy_id = proxies.id)`)
	return err
}

func addColumnIfMissing(ctx context.Context, db *sql.DB, table string, column string, definition string) error {
	rows, err := db.QueryContext(ctx, `pragma table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `alter table `+table+` add column `+column+` `+definition)
	return err
}

func resultError(result sql.Result, err error) error {
	if err != nil {
		return translateError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return store.ErrNotFound
	}
	return nil
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "constraint failed") || strings.Contains(message, "unique constraint") {
		return fmt.Errorf("%w: %v", store.ErrAlreadyExists, err)
	}
	return err
}

func compactStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		compacted = append(compacted, value)
	}
	return compacted
}

const schema = `
pragma foreign_keys = on;

create table if not exists users (
    id text primary key,
    username text not null unique,
    password_hash text not null default '',
    role text not null,
    status text not null,
    created_at timestamp not null,
    updated_at timestamp not null
);

create table if not exists clients (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    name text not null,
    kind text not null default 'provider',
    status text not null,
    credential_hash text not null,
    version integer not null default 0,
    last_online_at timestamp,
    last_offline_at timestamp,
    created_at timestamp not null,
    updated_at timestamp not null
);

create table if not exists client_enrollments (
    id text primary key,
    client_id text not null references clients(id) on delete cascade,
    secret_hash text not null,
    token_hash text not null,
    token text not null default '',
    expires_at timestamp not null,
    used_at timestamp,
    created_at timestamp not null,
    updated_at timestamp not null
);

create unique index if not exists client_enrollments_token_hash_unique on client_enrollments(token_hash);

create table if not exists domains (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    host text not null,
    certificate_id text not null default '',
    status text not null,
    created_at timestamp not null,
    updated_at timestamp not null
);

create unique index if not exists domains_host_unique on domains(lower(host));
create index if not exists domains_certificate_id_idx on domains(certificate_id) where certificate_id <> '';

create table if not exists domain_entries (
    id text primary key,
    domain_id text not null references domains(id) on delete cascade,
    protocol text not null,
    bind_host text not null default '',
    port integer not null,
    status text not null,
    created_at timestamp not null,
    updated_at timestamp not null
);

create unique index if not exists domain_entries_listener_unique on domain_entries(domain_id, protocol, lower(bind_host), port);
create index if not exists domain_entries_listener_lookup_idx on domain_entries(protocol, lower(bind_host), port, status);

create table if not exists proxies (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    client_id text not null references clients(id) on delete cascade,
    name text not null,
    type text not null,
    status text not null,
    domain_id text not null default '',
    path_prefix text not null default '',
    strip_prefix integer not null default 0,
    upstream_path_prefix text not null default '/',
    entry_bind_host text not null default '',
    entry_host text not null default '',
    entry_port integer not null default 0,
    target_host text not null,
    target_port integer not null,
    cert_file text not null default '',
    key_file text not null default '',
    certificate_id text not null default '',
    access_auth_enabled integer not null default 0,
    access_auth_version integer not null default 0,
    stats_legacy_aggregate integer not null default 0,
    description text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);

create table if not exists schema_flags (
    name text primary key,
    value text not null,
    updated_at timestamp not null
);

create table if not exists proxy_activation_tokens (
    id text primary key,
    proxy_id text not null references proxies(id) on delete cascade,
    auth_version integer not null,
    token_hash text not null unique,
    expires_at timestamp not null,
    used_at timestamp,
    created_at timestamp not null,
    created_by text not null default ''
);

create index if not exists proxy_activation_tokens_lookup_idx on proxy_activation_tokens(proxy_id, auth_version, expires_at);

create table if not exists proxy_access_credentials (
    id text primary key,
    proxy_id text not null references proxies(id) on delete cascade,
    auth_version integer not null,
    secret_hash text not null unique,
    created_at timestamp not null,
    last_used_at timestamp
);

create index if not exists proxy_access_credentials_lookup_idx on proxy_access_credentials(proxy_id, auth_version, secret_hash);

create table if not exists managed_certificates (
    id text primary key,
    proxy_id text not null default '',
    host text not null,
    status text not null,
    serving_status text not null default '',
    operation_status text not null default '',
    provider text not null default '',
    provider_type text not null default '',
    provider_name text not null default '',
    credential_id text not null default '',
    provider_status text not null default '',
    cloudflare_certificate_id text not null default '',
    previous_cloudflare_certificate_id text not null default '',
    hostnames text not null default '[]',
    request_type text not null default '',
    requested_validity integer not null default 0,
    cert_file text not null default '',
    key_file text not null default '',
    previous_cert_file text not null default '',
    previous_key_file text not null default '',
    not_after timestamp,
    last_issued_at timestamp,
    last_renewed_at timestamp,
    last_checked_at timestamp,
    last_synced_at timestamp,
    last_attempted_at timestamp,
    next_attempt_at timestamp,
    failure_count integer not null default 0,
    fingerprint text not null default '',
    last_error text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);

create unique index if not exists managed_certificates_host_unique on managed_certificates(lower(host));

create table if not exists provider_credentials (
    id text primary key,
    name text not null,
    provider_type text not null,
    scope text not null default '',
    token_fingerprint text not null default '',
    secret_ref text not null,
    status text not null,
    last_verified_at timestamp,
    last_error text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);

create index if not exists provider_credentials_provider_type_idx on provider_credentials(provider_type);

create table if not exists audit_events (
    id text primary key,
    actor_user_id text not null,
    resource_type text not null,
    resource_id text not null,
    action text not null,
    result text not null,
    source_ip text not null default '',
    error_summary text not null default '',
    created_at timestamp not null
);

create table if not exists proxy_stats (
    proxy_id text primary key references proxies(id) on delete cascade,
    tcp_connections integer not null default 0,
    tcp_upload_bytes integer not null default 0,
    tcp_download_bytes integer not null default 0,
    tcp_errors integer not null default 0,
    udp_packets integer not null default 0,
    udp_upload_bytes integer not null default 0,
    udp_download_bytes integer not null default 0,
    udp_errors integer not null default 0,
    http_requests integer not null default 0,
    http_upload_bytes integer not null default 0,
    http_download_bytes integer not null default 0,
    http_errors integer not null default 0,
    http_status_codes text not null default '{}',
    updated_at timestamp not null
);
`
