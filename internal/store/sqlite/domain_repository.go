package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

type domainRepository struct{ db *sql.DB }

type domainEntryRepository struct{ db *sql.DB }

const domainSelectColumns = `id, user_id, host, certificate_id, status, created_at, updated_at`
const domainEntrySelectColumns = `id, domain_id, protocol, bind_host, port, status, created_at, updated_at`

func (r domainRepository) Create(ctx context.Context, value domain.Domain) error {
	if err := value.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	if value.UpdatedAt.IsZero() {
		value.UpdatedAt = now
	}
	value.Host = domain.NormalizeRouteHost(value.Host)
	_, err := r.db.ExecContext(ctx, `insert into domains (id, user_id, host, certificate_id, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.UserID, value.Host, value.CertificateID, value.Status, value.CreatedAt, value.UpdatedAt)
	return translateError(err)
}

func (r domainRepository) ByID(ctx context.Context, id string) (domain.Domain, error) {
	return scanDomain(r.db.QueryRowContext(ctx, `select `+domainSelectColumns+` from domains where id = ?`, id))
}

func (r domainRepository) ByHost(ctx context.Context, host string) (domain.Domain, error) {
	return scanDomain(r.db.QueryRowContext(ctx, `select `+domainSelectColumns+` from domains where lower(host) = lower(?)`, domain.NormalizeRouteHost(host)))
}

func (r domainRepository) ByCertificateID(ctx context.Context, certificateID string) (domain.Domain, error) {
	return scanDomain(r.db.QueryRowContext(ctx, `select `+domainSelectColumns+` from domains where certificate_id = ?`, certificateID))
}

func (r domainRepository) List(ctx context.Context) ([]domain.Domain, error) {
	rows, err := r.db.QueryContext(ctx, `select `+domainSelectColumns+` from domains order by host, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Domain, 0)
	for rows.Next() {
		item, err := scanDomainRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r domainRepository) ByUserID(ctx context.Context, userID string) ([]domain.Domain, error) {
	rows, err := r.db.QueryContext(ctx, `select `+domainSelectColumns+` from domains where user_id = ? order by host, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Domain, 0)
	for rows.Next() {
		item, err := scanDomainRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r domainRepository) Update(ctx context.Context, value domain.Domain) error {
	if err := value.Validate(); err != nil {
		return err
	}
	value.Host = domain.NormalizeRouteHost(value.Host)
	result, err := r.db.ExecContext(ctx, `update domains set user_id = ?, host = ?, certificate_id = ?, status = ?, updated_at = ? where id = ?`,
		value.UserID, value.Host, value.CertificateID, value.Status, time.Now().UTC(), value.ID)
	return resultError(result, err)
}

func (r domainRepository) SetStatus(ctx context.Context, id string, status domain.DomainStatus) error {
	result, err := r.db.ExecContext(ctx, `update domains set status = ?, updated_at = ? where id = ?`, status, time.Now().UTC(), id)
	return resultError(result, err)
}

func (r domainRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from domains where id = ?`, id)
	return resultError(result, err)
}

func (r domainEntryRepository) Create(ctx context.Context, entry domain.DomainEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}
	entry.BindHost = domain.NormalizeBindHost(entry.BindHost)
	_, err := r.db.ExecContext(ctx, `insert into domain_entries (id, domain_id, protocol, bind_host, port, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.DomainID, entry.Protocol, entry.BindHost, entry.Port, entry.Status, entry.CreatedAt, entry.UpdatedAt)
	return translateError(err)
}

func (r domainEntryRepository) ByID(ctx context.Context, id string) (domain.DomainEntry, error) {
	return scanDomainEntry(r.db.QueryRowContext(ctx, `select `+domainEntrySelectColumns+` from domain_entries where id = ?`, id))
}

func (r domainEntryRepository) ListByDomainID(ctx context.Context, domainID string) ([]domain.DomainEntry, error) {
	rows, err := r.db.QueryContext(ctx, `select `+domainEntrySelectColumns+` from domain_entries where domain_id = ? order by protocol, port, id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.DomainEntry, 0)
	for rows.Next() {
		item, err := scanDomainEntryRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r domainEntryRepository) ListEnabled(ctx context.Context) ([]domain.DomainEntry, error) {
	rows, err := r.db.QueryContext(ctx, `select `+domainEntrySelectColumns+` from domain_entries where status = ? order by protocol, port, id`, domain.DomainEntryEnabled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.DomainEntry, 0)
	for rows.Next() {
		item, err := scanDomainEntryRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r domainEntryRepository) ByListener(ctx context.Context, protocol domain.DomainEntryProtocol, bindHost string, port int, host string, includeDefault bool) (domain.Domain, domain.DomainEntry, error) {
	bindHost = domain.NormalizeBindHost(bindHost)
	host = domain.NormalizeRouteHost(host)
	query := `select d.id, d.user_id, d.host, d.certificate_id, d.status, d.created_at, d.updated_at,
		e.id, e.domain_id, e.protocol, e.bind_host, e.port, e.status, e.created_at, e.updated_at
		from domain_entries e
		join domains d on d.id = e.domain_id
		where e.protocol = ?
		  and e.status = ?
		  and d.status = ?
		  and lower(d.host) = lower(?)
		  and (
		    (lower(e.bind_host) = lower(?) and e.port = ?)`
	args := []any{protocol, domain.DomainEntryEnabled, domain.DomainEnabled, host, bindHost, port}
	if includeDefault {
		query += ` or (e.bind_host = '' and (e.port = 0 or e.port = ?))`
		args = append(args, port)
	}
	query += `) order by case when lower(e.bind_host) = lower(?) and e.port = ? then 0 else 1 end limit 1`
	args = append(args, bindHost, port)

	var d domain.Domain
	var e domain.DomainEntry
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&d.ID, &d.UserID, &d.Host, &d.CertificateID, &d.Status, &d.CreatedAt, &d.UpdatedAt,
		&e.ID, &e.DomainID, &e.Protocol, &e.BindHost, &e.Port, &e.Status, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return domain.Domain{}, domain.DomainEntry{}, translateError(err)
	}
	return d, e, nil
}

func (r domainEntryRepository) Update(ctx context.Context, entry domain.DomainEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	entry.BindHost = domain.NormalizeBindHost(entry.BindHost)
	result, err := r.db.ExecContext(ctx, `update domain_entries set protocol = ?, bind_host = ?, port = ?, status = ?, updated_at = ? where id = ?`,
		entry.Protocol, entry.BindHost, entry.Port, entry.Status, time.Now().UTC(), entry.ID)
	return resultError(result, err)
}

func (r domainEntryRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `delete from domain_entries where id = ?`, id)
	return resultError(result, err)
}

func (r domainEntryRepository) DeleteByDomainID(ctx context.Context, domainID string) error {
	_, err := r.db.ExecContext(ctx, `delete from domain_entries where domain_id = ?`, domainID)
	return err
}

func scanDomain(row *sql.Row) (domain.Domain, error) {
	var value domain.Domain
	err := row.Scan(&value.ID, &value.UserID, &value.Host, &value.CertificateID, &value.Status, &value.CreatedAt, &value.UpdatedAt)
	return value, translateError(err)
}

func scanDomainRows(rows *sql.Rows) (domain.Domain, error) {
	var value domain.Domain
	err := rows.Scan(&value.ID, &value.UserID, &value.Host, &value.CertificateID, &value.Status, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func scanDomainEntry(row *sql.Row) (domain.DomainEntry, error) {
	var entry domain.DomainEntry
	err := row.Scan(&entry.ID, &entry.DomainID, &entry.Protocol, &entry.BindHost, &entry.Port, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt)
	return entry, translateError(err)
}

func scanDomainEntryRows(rows *sql.Rows) (domain.DomainEntry, error) {
	var entry domain.DomainEntry
	err := rows.Scan(&entry.ID, &entry.DomainID, &entry.Protocol, &entry.BindHost, &entry.Port, &entry.Status, &entry.CreatedAt, &entry.UpdatedAt)
	return entry, err
}

// Ensure unused import for store.ErrNotFound reference sites compile when needed.
var _ = store.ErrNotFound
