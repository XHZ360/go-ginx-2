package sqlite

import (
	"context"
	"database/sql"
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

func (s *Store) Proxies() store.ProxyRepository { return proxyRepository{s.db} }

func (s *Store) AuditEvents() store.AuditRepository { return auditRepository{s.db} }

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
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
	_, err := r.db.ExecContext(ctx, `insert into users (id, username, role, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?)`, user.ID, user.Username, user.Role, user.Status, user.CreatedAt, user.UpdatedAt)
	return translateError(err)
}

func (r userRepository) ByID(ctx context.Context, id string) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `select id, username, role, status, created_at, updated_at from users where id = ?`, id))
}

func (r userRepository) ByUsername(ctx context.Context, username string) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `select id, username, role, status, created_at, updated_at from users where username = ?`, username))
}

func (r userRepository) SetStatus(ctx context.Context, id string, status domain.UserStatus) error {
	result, err := r.db.ExecContext(ctx, `update users set status = ?, updated_at = ? where id = ?`, status, time.Now().UTC(), id)
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

func (r clientRepository) SetStatus(ctx context.Context, id string, status domain.ClientStatus) error {
	result, err := r.db.ExecContext(ctx, `update clients set status = ?, updated_at = ? where id = ?`, status, time.Now().UTC(), id)
	return resultError(result, err)
}

func (r clientRepository) RotateCredential(ctx context.Context, id string, credentialHash string) error {
	if strings.TrimSpace(credentialHash) == "" {
		return errors.New("credential hash is required")
	}
	result, err := r.db.ExecContext(ctx, `update clients set credential_hash = ?, version = version + 1, updated_at = ? where id = ?`, credentialHash, time.Now().UTC(), id)
	return resultError(result, err)
}

type proxyRepository struct{ db *sql.DB }

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
	_, err := r.db.ExecContext(ctx, `insert into proxies (id, user_id, client_id, name, type, status, entry_host, entry_port, target_host, target_port, description, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, proxy.ID, proxy.UserID, proxy.ClientID, proxy.Name, proxy.Type, proxy.Status, proxy.EntryHost, proxy.EntryPort, proxy.TargetHost, proxy.TargetPort, proxy.Description, proxy.CreatedAt, proxy.UpdatedAt)
	return translateError(err)
}

func (r proxyRepository) ByID(ctx context.Context, id string) (domain.Proxy, error) {
	return scanProxy(r.db.QueryRowContext(ctx, `select id, user_id, client_id, name, type, status, entry_host, entry_port, target_host, target_port, description, created_at, updated_at from proxies where id = ?`, id))
}

func (r proxyRepository) ByClientID(ctx context.Context, clientID string) ([]domain.Proxy, error) {
	rows, err := r.db.QueryContext(ctx, `select id, user_id, client_id, name, type, status, entry_host, entry_port, target_host, target_port, description, created_at, updated_at from proxies where client_id = ? order by created_at, id`, clientID)
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
	rows, err := r.db.QueryContext(ctx, `select id, user_id, client_id, name, type, status, entry_host, entry_port, target_host, target_port, description, created_at, updated_at from proxies where type = ? and status = ? order by created_at, id`, proxyType, domain.ProxyEnabled)
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

func (r proxyRepository) ByTCPEntryPort(ctx context.Context, port int) (domain.Proxy, error) {
	return scanProxy(r.db.QueryRowContext(ctx, `select id, user_id, client_id, name, type, status, entry_host, entry_port, target_host, target_port, description, created_at, updated_at from proxies where type = ? and entry_port = ?`, domain.ProxyTCP, port))
}

func (r proxyRepository) ByHTTPHost(ctx context.Context, host string) (domain.Proxy, error) {
	return scanProxy(r.db.QueryRowContext(ctx, `select id, user_id, client_id, name, type, status, entry_host, entry_port, target_host, target_port, description, created_at, updated_at from proxies where type = ? and lower(entry_host) = lower(?)`, domain.ProxyHTTP, host))
}

func (r proxyRepository) SetStatus(ctx context.Context, id string, status domain.ProxyStatus) error {
	result, err := r.db.ExecContext(ctx, `update proxies set status = ?, updated_at = ? where id = ?`, status, time.Now().UTC(), id)
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

func scanUser(row *sql.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.Username, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	return user, translateError(err)
}

func scanClient(row *sql.Row) (domain.Client, error) {
	var client domain.Client
	err := row.Scan(&client.ID, &client.UserID, &client.Name, &client.Status, &client.CredentialHash, &client.Version, &client.LastOnlineAt, &client.LastOfflineAt, &client.CreatedAt, &client.UpdatedAt)
	return client, translateError(err)
}

func scanProxy(row *sql.Row) (domain.Proxy, error) {
	var proxy domain.Proxy
	err := row.Scan(&proxy.ID, &proxy.UserID, &proxy.ClientID, &proxy.Name, &proxy.Type, &proxy.Status, &proxy.EntryHost, &proxy.EntryPort, &proxy.TargetHost, &proxy.TargetPort, &proxy.Description, &proxy.CreatedAt, &proxy.UpdatedAt)
	return proxy, translateError(err)
}

func scanProxyRows(rows *sql.Rows) (domain.Proxy, error) {
	var proxy domain.Proxy
	err := rows.Scan(&proxy.ID, &proxy.UserID, &proxy.ClientID, &proxy.Name, &proxy.Type, &proxy.Status, &proxy.EntryHost, &proxy.EntryPort, &proxy.TargetHost, &proxy.TargetPort, &proxy.Description, &proxy.CreatedAt, &proxy.UpdatedAt)
	return proxy, err
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

create table if not exists proxies (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    client_id text not null references clients(id) on delete cascade,
    name text not null,
    type text not null,
    status text not null,
    entry_host text not null default '',
    entry_port integer not null default 0,
    target_host text not null,
    target_port integer not null,
    description text not null default '',
    created_at timestamp not null,
    updated_at timestamp not null
);

create unique index if not exists proxies_tcp_entry_port_unique on proxies(entry_port) where type = 'tcp' and entry_port > 0;
create unique index if not exists proxies_http_entry_host_unique on proxies(lower(entry_host)) where type = 'http' and entry_host <> '';

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
`
