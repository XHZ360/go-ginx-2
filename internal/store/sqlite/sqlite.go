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

func (s *Store) Proxies() store.ProxyRepository { return proxyRepository{s.db} }

func (s *Store) Certificates() store.CertificateRepository { return certificateRepository{s.db} }

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
	return addUserPasswordColumn(ctx, s.db)
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
	_, err := r.db.ExecContext(ctx, `insert into clients (id, user_id, name, status, credential_hash, version, last_online_at, last_offline_at, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, client.ID, client.UserID, client.Name, client.Status, client.CredentialHash, client.Version, client.LastOnlineAt, client.LastOfflineAt, client.CreatedAt, client.UpdatedAt)
	return translateError(err)
}

func (r clientRepository) ByID(ctx context.Context, id string) (domain.Client, error) {
	return scanClient(r.db.QueryRowContext(ctx, `select id, user_id, name, status, credential_hash, version, last_online_at, last_offline_at, created_at, updated_at from clients where id = ?`, id))
}

func (r clientRepository) List(ctx context.Context) ([]domain.Client, error) {
	rows, err := r.db.QueryContext(ctx, `select id, user_id, name, status, credential_hash, version, last_online_at, last_offline_at, created_at, updated_at from clients order by created_at, id`)
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

const proxySelectColumns = `id, user_id, client_id, name, type, status, entry_bind_host, entry_host, entry_port, target_host, target_port, cert_file, key_file, description, created_at, updated_at`

func (r proxyRepository) Create(ctx context.Context, proxy domain.Proxy) error {
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
	_, err := r.db.ExecContext(ctx, `insert into proxies (id, user_id, client_id, name, type, status, entry_bind_host, entry_host, entry_port, target_host, target_port, cert_file, key_file, description, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, proxy.ID, proxy.UserID, proxy.ClientID, proxy.Name, proxy.Type, proxy.Status, domain.NormalizeBindHost(proxy.EntryBindHost), proxy.EntryHost, proxy.EntryPort, proxy.TargetHost, proxy.TargetPort, proxy.CertFile, proxy.KeyFile, proxy.Description, proxy.CreatedAt, proxy.UpdatedAt)
	return translateError(err)
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
	return scanProxy(r.db.QueryRowContext(ctx, `select `+proxySelectColumns+` from proxies where type = ? and lower(entry_host) = lower(?) order by entry_bind_host <> '', entry_port <> 0 limit 1`, domain.ProxyHTTP, host))
}

func (r proxyRepository) ByHTTPSHost(ctx context.Context, host string) (domain.Proxy, error) {
	return scanProxy(r.db.QueryRowContext(ctx, `select `+proxySelectColumns+` from proxies where type = ? and lower(entry_host) = lower(?) order by entry_bind_host <> '', entry_port <> 0 limit 1`, domain.ProxyHTTPS, host))
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
	if err := proxy.Validate(); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `update proxies set name = ?, status = ?, entry_bind_host = ?, entry_host = ?, entry_port = ?, target_host = ?, target_port = ?, cert_file = ?, key_file = ?, description = ?, updated_at = ? where id = ?`, proxy.Name, proxy.Status, domain.NormalizeBindHost(proxy.EntryBindHost), proxy.EntryHost, proxy.EntryPort, proxy.TargetHost, proxy.TargetPort, proxy.CertFile, proxy.KeyFile, proxy.Description, time.Now().UTC(), proxy.ID)
	return resultError(result, err)
}

func (r proxyRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from proxies where id = ?`, id)
	return resultError(result, err)
}

type certificateRepository struct{ db *sql.DB }

func (r certificateRepository) Create(ctx context.Context, certificate domain.ManagedCertificate) error {
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
	_, err := r.db.ExecContext(ctx, `insert into managed_certificates (id, proxy_id, host, status, provider, cert_file, key_file, previous_cert_file, previous_key_file, not_after, last_issued_at, last_renewed_at, last_error, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, certificate.ID, certificate.ProxyID, certificate.Host, certificate.Status, certificate.Provider, certificate.CertFile, certificate.KeyFile, certificate.PreviousCertFile, certificate.PreviousKeyFile, certificate.NotAfter, certificate.LastIssuedAt, certificate.LastRenewedAt, certificate.LastError, certificate.CreatedAt, certificate.UpdatedAt)
	return translateError(err)
}

func (r certificateRepository) ByProxyID(ctx context.Context, proxyID string) (domain.ManagedCertificate, error) {
	return scanManagedCertificate(r.db.QueryRowContext(ctx, managedCertificateSelect+` where proxy_id = ?`, proxyID))
}

func (r certificateRepository) ByHost(ctx context.Context, host string) (domain.ManagedCertificate, error) {
	return scanManagedCertificate(r.db.QueryRowContext(ctx, managedCertificateSelect+` where lower(host) = lower(?)`, host))
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

func (r certificateRepository) ListRenewable(ctx context.Context, before time.Time) ([]domain.ManagedCertificate, error) {
	rows, err := r.db.QueryContext(ctx, managedCertificateSelect+` where status in (?, ?) and not_after is not null and not_after <= ? order by not_after, host`, domain.CertificateValid, domain.CertificateExpiringSoon, before)
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
	query := `update managed_certificates set status = ?, cert_file = ?, key_file = ?, previous_cert_file = ?, previous_key_file = ?, not_after = ?, last_error = '', updated_at = ?`
	args := []any{domain.CertificateValid, result.CertFile, result.KeyFile, result.PreviousCertFile, result.PreviousKeyFile, result.NotAfter, completedAt}
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
	result, err := r.db.ExecContext(ctx, `update managed_certificates set status = ?, last_error = ?, updated_at = ? where id = ?`, status, failure.LastError, completedAt, id)
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
	err := row.Scan(&client.ID, &client.UserID, &client.Name, &client.Status, &client.CredentialHash, &client.Version, &client.LastOnlineAt, &client.LastOfflineAt, &client.CreatedAt, &client.UpdatedAt)
	return client, translateError(err)
}

func scanClientRows(rows *sql.Rows) (domain.Client, error) {
	var client domain.Client
	err := rows.Scan(&client.ID, &client.UserID, &client.Name, &client.Status, &client.CredentialHash, &client.Version, &client.LastOnlineAt, &client.LastOfflineAt, &client.CreatedAt, &client.UpdatedAt)
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
	err := row.Scan(&proxy.ID, &proxy.UserID, &proxy.ClientID, &proxy.Name, &proxy.Type, &proxy.Status, &proxy.EntryBindHost, &proxy.EntryHost, &proxy.EntryPort, &proxy.TargetHost, &proxy.TargetPort, &proxy.CertFile, &proxy.KeyFile, &proxy.Description, &proxy.CreatedAt, &proxy.UpdatedAt)
	return proxy, translateError(err)
}

func scanProxyRows(rows *sql.Rows) (domain.Proxy, error) {
	var proxy domain.Proxy
	err := rows.Scan(&proxy.ID, &proxy.UserID, &proxy.ClientID, &proxy.Name, &proxy.Type, &proxy.Status, &proxy.EntryBindHost, &proxy.EntryHost, &proxy.EntryPort, &proxy.TargetHost, &proxy.TargetPort, &proxy.CertFile, &proxy.KeyFile, &proxy.Description, &proxy.CreatedAt, &proxy.UpdatedAt)
	return proxy, err
}

const managedCertificateSelect = `select id, proxy_id, host, status, provider, cert_file, key_file, previous_cert_file, previous_key_file, not_after, last_issued_at, last_renewed_at, last_error, created_at, updated_at from managed_certificates`

func scanManagedCertificate(row *sql.Row) (domain.ManagedCertificate, error) {
	var certificate domain.ManagedCertificate
	var notAfter sql.NullTime
	var lastIssuedAt sql.NullTime
	var lastRenewedAt sql.NullTime
	err := row.Scan(&certificate.ID, &certificate.ProxyID, &certificate.Host, &certificate.Status, &certificate.Provider, &certificate.CertFile, &certificate.KeyFile, &certificate.PreviousCertFile, &certificate.PreviousKeyFile, &notAfter, &lastIssuedAt, &lastRenewedAt, &certificate.LastError, &certificate.CreatedAt, &certificate.UpdatedAt)
	applyManagedCertificateTimes(&certificate, notAfter, lastIssuedAt, lastRenewedAt)
	return certificate, translateError(err)
}

func scanManagedCertificateRows(rows *sql.Rows) (domain.ManagedCertificate, error) {
	var certificate domain.ManagedCertificate
	var notAfter sql.NullTime
	var lastIssuedAt sql.NullTime
	var lastRenewedAt sql.NullTime
	err := rows.Scan(&certificate.ID, &certificate.ProxyID, &certificate.Host, &certificate.Status, &certificate.Provider, &certificate.CertFile, &certificate.KeyFile, &certificate.PreviousCertFile, &certificate.PreviousKeyFile, &notAfter, &lastIssuedAt, &lastRenewedAt, &certificate.LastError, &certificate.CreatedAt, &certificate.UpdatedAt)
	applyManagedCertificateTimes(&certificate, notAfter, lastIssuedAt, lastRenewedAt)
	return certificate, err
}

func applyManagedCertificateTimes(certificate *domain.ManagedCertificate, notAfter sql.NullTime, lastIssuedAt sql.NullTime, lastRenewedAt sql.NullTime) {
	if notAfter.Valid {
		certificate.NotAfter = &notAfter.Time
	}
	if lastIssuedAt.Valid {
		certificate.LastIssuedAt = &lastIssuedAt.Time
	}
	if lastRenewedAt.Valid {
		certificate.LastRenewedAt = &lastRenewedAt.Time
	}
}

func scanAuditEventRows(rows *sql.Rows) (domain.AuditEvent, error) {
	var event domain.AuditEvent
	err := rows.Scan(&event.ID, &event.ActorUserID, &event.ResourceType, &event.ResourceID, &event.Action, &event.Result, &event.SourceIP, &event.ErrorSummary, &event.CreatedAt)
	return event, err
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

func addClientEnrollmentTokenColumn(ctx context.Context, db *sql.DB) error {
	return addColumnIfMissing(ctx, db, "client_enrollments", "token", "text not null default ''")
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

create table if not exists proxies (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    client_id text not null references clients(id) on delete cascade,
    name text not null,
    type text not null,
    status text not null,
    entry_bind_host text not null default '',
    entry_host text not null default '',
    entry_port integer not null default 0,
    target_host text not null,
    target_port integer not null,
    cert_file text not null default '',
    key_file text not null default '',
    description text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);

create unique index if not exists proxies_tcp_entry_unique on proxies(lower(entry_bind_host), entry_port) where type = 'tcp' and entry_port > 0;
create unique index if not exists proxies_udp_entry_unique on proxies(lower(entry_bind_host), entry_port) where type = 'udp' and entry_port > 0;
create unique index if not exists proxies_http_route_unique on proxies(lower(entry_bind_host), entry_port, lower(entry_host)) where type = 'http' and entry_host <> '';
create unique index if not exists proxies_https_route_unique on proxies(lower(entry_bind_host), entry_port, lower(entry_host)) where type = 'https' and entry_host <> '';

create table if not exists managed_certificates (
    id text primary key,
    proxy_id text not null references proxies(id) on delete cascade,
    host text not null,
    status text not null,
    provider text not null default '',
    cert_file text not null default '',
    key_file text not null default '',
    previous_cert_file text not null default '',
    previous_key_file text not null default '',
    not_after timestamp,
    last_issued_at timestamp,
    last_renewed_at timestamp,
    last_error text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);

create unique index if not exists managed_certificates_proxy_unique on managed_certificates(proxy_id);
create unique index if not exists managed_certificates_host_unique on managed_certificates(lower(host));

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
