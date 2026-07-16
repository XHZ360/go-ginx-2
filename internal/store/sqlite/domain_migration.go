package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/domain"
)

func migrateDomainPathRouting(ctx context.Context, db *sql.DB) error {
	if err := ensureDomainTables(ctx, db); err != nil {
		return err
	}
	if err := addProxyWebColumns(ctx, db); err != nil {
		return err
	}
	if err := ensureDomainIndexes(ctx, db); err != nil {
		return err
	}
	// Always ensure schema_flags exists and columns are present before any query that uses them.
	if _, err := db.ExecContext(ctx, `create table if not exists schema_flags (name text primary key, value text not null, updated_at timestamp not null)`); err != nil {
		return err
	}
	// Verify proxies.domain_id is queryable after column migration.
	if _, err := db.ExecContext(ctx, `select domain_id, path_prefix, strip_prefix, upstream_path_prefix, stats_legacy_aggregate from proxies limit 0`); err != nil {
		return fmt.Errorf("proxy web columns unavailable after migration: %w", err)
	}
	migrated, err := domainPathRoutingMigrated(ctx, db)
	if err != nil {
		return err
	}
	if !migrated {
		if err := convertLegacyWebProxies(ctx, db); err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `insert or replace into schema_flags (name, value, updated_at) values ('domain_path_routing_v1', 'done', ?)`, time.Now().UTC()); err != nil {
			return err
		}
	}
	// Migration complete: drop empty legacy table. Upgrade-from-old-db still reads
	// proxy_routes inside convertLegacyWebProxies before this point.
	_, err = db.ExecContext(ctx, `drop table if exists proxy_routes`)
	return err
}

func ensureDomainTables(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`create table if not exists domains (
			id text primary key,
			user_id text not null references users(id) on delete cascade,
			host text not null,
			certificate_id text not null default '',
			status text not null,
			created_at timestamp not null,
			updated_at timestamp not null
		)`,
		`create table if not exists domain_entries (
			id text primary key,
			domain_id text not null references domains(id) on delete cascade,
			protocol text not null,
			bind_host text not null default '',
			port integer not null,
			status text not null,
			created_at timestamp not null,
			updated_at timestamp not null
		)`,
		`create table if not exists schema_flags (
			name text primary key,
			value text not null,
			updated_at timestamp not null
		)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func addProxyWebColumns(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		name       string
		definition string
	}{
		{"domain_id", "text not null default ''"},
		{"path_prefix", "text not null default ''"},
		{"strip_prefix", "integer not null default 0"},
		{"upstream_path_prefix", "text not null default '/'"},
		{"stats_legacy_aggregate", "integer not null default 0"},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(ctx, db, "proxies", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func ensureDomainIndexes(ctx context.Context, db *sql.DB) error {
	// Certificate → Domain is 1:n (one cert may cover many domains). Drop the old 1:1 unique index.
	if _, err := db.ExecContext(ctx, `drop index if exists domains_certificate_id_unique`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `drop index if exists proxies_domain_path_unique`); err != nil {
		return err
	}
	statements := []string{
		`create unique index if not exists domains_host_unique on domains(lower(host))`,
		`create index if not exists domains_certificate_id_idx on domains(certificate_id) where certificate_id <> ''`,
		`create unique index if not exists domain_entries_listener_unique on domain_entries(domain_id, protocol, lower(bind_host), port)`,
		`create index if not exists domain_entries_listener_lookup_idx on domain_entries(protocol, lower(bind_host), port, status)`,
		`create index if not exists proxies_domain_status_idx on proxies(domain_id, status) where domain_id <> ''`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func domainPathRoutingMigrated(ctx context.Context, db *sql.DB) (bool, error) {
	var value string
	err := db.QueryRowContext(ctx, `select value from schema_flags where name = 'domain_path_routing_v1'`).Scan(&value)
	if err == sql.ErrNoRows {
		// also treat as migrated when no legacy http/https proxies remain
		var legacyCount int
		if err := db.QueryRowContext(ctx, `select count(*) from proxies where type in ('http', 'https')`).Scan(&legacyCount); err != nil {
			return false, err
		}
		if legacyCount == 0 {
			var webCount int
			if err := db.QueryRowContext(ctx, `select count(*) from proxies where type = 'web'`).Scan(&webCount); err != nil {
				return false, err
			}
			if webCount > 0 {
				return true, nil
			}
			// empty DB or only tcp/udp: mark ready without conversion
			return true, nil
		}
		return false, nil
	}
	if err != nil {
		// schema_flags may not exist yet on very first open before ensureDomainTables; treat as not migrated
		if strings.Contains(err.Error(), "no such table") {
			return false, nil
		}
		return false, err
	}
	return value == "done", nil
}

type legacyProxyRow struct {
	ID                string
	UserID            string
	ClientID          string
	Name              string
	Type              string
	Status            string
	EntryBindHost     string
	EntryHost         string
	EntryPort         int
	TargetHost        string
	TargetPort        int
	CertFile          string
	KeyFile           string
	CertificateID     string
	AccessAuthEnabled bool
	AccessAuthVersion int64
	Description       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type legacyRouteRow struct {
	ID                 string
	ProxyID            string
	ClientID           string
	PathPrefix         string
	StripPrefix        bool
	UpstreamPathPrefix string
	TargetHost         string
	TargetPort         int
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type legacyPathOwner struct {
	ProxyID    string
	ClientID   string
	TargetHost string
	TargetPort int
	Strip      bool
	Upstream   string
	Source     string
}

func convertLegacyWebProxies(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `select id, user_id, client_id, name, type, status, entry_bind_host, entry_host, entry_port, target_host, target_port, cert_file, key_file, certificate_id, access_auth_enabled, access_auth_version, description, created_at, updated_at from proxies where type in ('http', 'https') order by created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	legacyProxies := make([]legacyProxyRow, 0)
	for rows.Next() {
		var proxy legacyProxyRow
		if err := rows.Scan(&proxy.ID, &proxy.UserID, &proxy.ClientID, &proxy.Name, &proxy.Type, &proxy.Status, &proxy.EntryBindHost, &proxy.EntryHost, &proxy.EntryPort, &proxy.TargetHost, &proxy.TargetPort, &proxy.CertFile, &proxy.KeyFile, &proxy.CertificateID, &proxy.AccessAuthEnabled, &proxy.AccessAuthVersion, &proxy.Description, &proxy.CreatedAt, &proxy.UpdatedAt); err != nil {
			return err
		}
		legacyProxies = append(legacyProxies, proxy)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(legacyProxies) == 0 {
		return tx.Commit()
	}

	type domainAgg struct {
		ID            string
		UserID        string
		Host          string
		CertificateID string
		Status        domain.DomainStatus
		CreatedAt     time.Time
		UpdatedAt     time.Time
		entries       map[string]domain.DomainEntry
		pathOwners    map[string]legacyPathOwner
	}

	domainsByHost := map[string]*domainAgg{}
	now := time.Now().UTC()

	for _, proxy := range legacyProxies {
		host := domain.NormalizeRouteHost(proxy.EntryHost)
		if host == "" {
			return fmt.Errorf("domain path migration conflict: proxy %s missing entry host", proxy.ID)
		}
		agg, ok := domainsByHost[host]
		if !ok {
			agg = &domainAgg{
				ID:         newMigrationID("domain"),
				UserID:     proxy.UserID,
				Host:       host,
				Status:     domain.DomainEnabled,
				CreatedAt:  proxy.CreatedAt,
				UpdatedAt:  proxy.UpdatedAt,
				entries:    map[string]domain.DomainEntry{},
				pathOwners: map[string]legacyPathOwner{},
			}
			domainsByHost[host] = agg
		}
		if agg.UserID != proxy.UserID {
			return fmt.Errorf("domain path migration conflict: host %s owned by multiple users", host)
		}
		if proxy.CertificateID != "" {
			if agg.CertificateID != "" && agg.CertificateID != proxy.CertificateID {
				return fmt.Errorf("domain path migration conflict: host %s has multiple certificate bindings", host)
			}
			agg.CertificateID = proxy.CertificateID
		}
		if proxy.Status != string(domain.ProxyEnabled) && agg.Status == domain.DomainEnabled {
			// keep domain enabled if any proxy was enabled; otherwise leave as enabled default
		}
		protocol := domain.DomainEntryHTTP
		if proxy.Type == string(domain.ProxyHTTPS) {
			protocol = domain.DomainEntryHTTPS
		}
		bindHost := domain.NormalizeBindHost(proxy.EntryBindHost)
		port := proxy.EntryPort
		entryKey := fmt.Sprintf("%s|%s|%d", protocol, bindHost, port)
		if _, exists := agg.entries[entryKey]; !exists {
			status := domain.DomainEntryEnabled
			if proxy.Status != string(domain.ProxyEnabled) {
				status = domain.DomainEntryDisabled
			}
			agg.entries[entryKey] = domain.DomainEntry{
				ID:        newMigrationID("dentry"),
				DomainID:  agg.ID,
				Protocol:  protocol,
				BindHost:  bindHost,
				Port:      port,
				Status:    status,
				CreatedAt: proxy.CreatedAt,
				UpdatedAt: proxy.UpdatedAt,
			}
		} else if proxy.Status == string(domain.ProxyEnabled) {
			entry := agg.entries[entryKey]
			entry.Status = domain.DomainEntryEnabled
			agg.entries[entryKey] = entry
		}

		// parent becomes /
		parentOwner := legacyPathOwner{
			ProxyID:    proxy.ID,
			ClientID:   proxy.ClientID,
			TargetHost: proxy.TargetHost,
			TargetPort: proxy.TargetPort,
			Strip:      false,
			Upstream:   "/",
			Source:     "parent:" + proxy.ID,
		}
		if existing, exists := agg.pathOwners["/"]; exists {
			if !samePathBackend(existing, parentOwner) {
				return fmt.Errorf("domain path migration conflict: host %s path / maps to different backends", host)
			}
		} else {
			agg.pathOwners["/"] = parentOwner
		}

		routeRows, err := queryLegacyProxyRoutes(ctx, tx, proxy.ID)
		if err != nil {
			return err
		}
		for _, route := range routeRows {
			pathPrefix, err := domain.NormalizeProxyRoutePrefix(route.PathPrefix)
			if err != nil {
				return fmt.Errorf("domain path migration conflict: route %s invalid path: %w", route.ID, err)
			}
			owner := legacyPathOwner{
				ProxyID:    route.ID, // new proxy will use route id
				ClientID:   route.ClientID,
				TargetHost: route.TargetHost,
				TargetPort: route.TargetPort,
				Strip:      route.StripPrefix,
				Upstream:   route.UpstreamPathPrefix,
				Source:     "route:" + route.ID,
			}
			if existing, exists := agg.pathOwners[pathPrefix]; exists {
				if !samePathBackend(existing, owner) {
					return fmt.Errorf("domain path migration conflict: host %s path %s maps to different backends", host, pathPrefix)
				}
			} else {
				agg.pathOwners[pathPrefix] = owner
			}
		}
	}

	// insert domains and entries
	for _, agg := range domainsByHost {
		if _, err := tx.ExecContext(ctx, `insert into domains (id, user_id, host, certificate_id, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?)`,
			agg.ID, agg.UserID, agg.Host, agg.CertificateID, agg.Status, agg.CreatedAt, agg.UpdatedAt); err != nil {
			return translateError(err)
		}
		for _, entry := range agg.entries {
			if _, err := tx.ExecContext(ctx, `insert into domain_entries (id, domain_id, protocol, bind_host, port, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?)`,
				entry.ID, entry.DomainID, entry.Protocol, entry.BindHost, entry.Port, entry.Status, entry.CreatedAt, entry.UpdatedAt); err != nil {
				return translateError(err)
			}
		}
	}

	// convert proxies: update parents to web, insert routes as web proxies
	for _, proxy := range legacyProxies {
		host := domain.NormalizeRouteHost(proxy.EntryHost)
		agg := domainsByHost[host]
		upstream, err := domain.NormalizeProxyUpstreamPathPrefix("/")
		if err != nil {
			return err
		}
		// revoke access for parent; bump version if auth was enabled so old cookies fail
		nextAuthVersion := proxy.AccessAuthVersion
		if proxy.AccessAuthEnabled {
			nextAuthVersion = proxy.AccessAuthVersion + 1
			if _, err := tx.ExecContext(ctx, `delete from proxy_activation_tokens where proxy_id = ?`, proxy.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `delete from proxy_access_credentials where proxy_id = ?`, proxy.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `update proxies set type = ?, domain_id = ?, path_prefix = ?, strip_prefix = 0, upstream_path_prefix = ?, entry_bind_host = '', entry_port = 0, access_auth_version = ?, stats_legacy_aggregate = 1, updated_at = ? where id = ?`,
			domain.ProxyWeb, agg.ID, "/", upstream, nextAuthVersion, now, proxy.ID); err != nil {
			// Older DBs still have entry_host/cert columns; clear those when present.
			if _, fallbackErr := tx.ExecContext(ctx, `update proxies set type = ?, domain_id = ?, path_prefix = ?, strip_prefix = 0, upstream_path_prefix = ?, entry_bind_host = '', entry_host = '', entry_port = 0, certificate_id = '', cert_file = '', key_file = '', access_auth_version = ?, stats_legacy_aggregate = 1, updated_at = ? where id = ?`,
				domain.ProxyWeb, agg.ID, "/", upstream, nextAuthVersion, now, proxy.ID); fallbackErr != nil {
				return err
			}
		}

		routes, err := queryLegacyProxyRoutes(ctx, tx, proxy.ID)
		if err != nil {
			return err
		}
		for _, route := range routes {
			pathPrefix, err := domain.NormalizeProxyRoutePrefix(route.PathPrefix)
			if err != nil {
				return err
			}
			upstreamPrefix, err := domain.NormalizeProxyUpstreamPathPrefix(route.UpstreamPathPrefix)
			if err != nil {
				return err
			}
			status := domain.ProxyEnabled
			if route.Status == "disabled" || proxy.Status != string(domain.ProxyEnabled) {
				status = domain.ProxyDisabled
			}
			authEnabled := proxy.AccessAuthEnabled
			authVersion := int64(1)
			if !authEnabled {
				authVersion = 0
			}
			name := proxy.Name + " " + pathPrefix
			if _, err := tx.ExecContext(ctx, `insert into proxies (id, user_id, client_id, name, type, status, domain_id, path_prefix, strip_prefix, upstream_path_prefix, entry_bind_host, entry_port, target_host, target_port, access_auth_enabled, access_auth_version, stats_legacy_aggregate, description, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, ?, ?, ?, ?, 0, ?, ?, ?)`,
				route.ID, proxy.UserID, route.ClientID, name, domain.ProxyWeb, status, agg.ID, pathPrefix, route.StripPrefix, upstreamPrefix, route.TargetHost, route.TargetPort, authEnabled, authVersion, "migrated from proxy route "+route.ID, route.CreatedAt, route.UpdatedAt); err != nil {
				return translateError(err)
			}
		}
		if _, err := tx.ExecContext(ctx, `delete from proxy_routes where proxy_id = ?`, proxy.ID); err != nil {
			// Table may already be absent on partially cleaned installs; ignore missing table.
			if !isMissingTableError(err) {
				return err
			}
		}
	}

	return tx.Commit()
}

// queryLegacyProxyRoutes reads pre-Domain path routes. Missing table means no legacy routes.
func queryLegacyProxyRoutes(ctx context.Context, tx *sql.Tx, proxyID string) ([]legacyRouteRow, error) {
	rows, err := tx.QueryContext(ctx, `select id, proxy_id, client_id, path_prefix, strip_prefix, upstream_path_prefix, target_host, target_port, status, created_at, updated_at from proxy_routes where proxy_id = ? order by length(path_prefix) desc, path_prefix, id`, proxyID)
	if err != nil {
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	routes := make([]legacyRouteRow, 0)
	for rows.Next() {
		var route legacyRouteRow
		if err := rows.Scan(&route.ID, &route.ProxyID, &route.ClientID, &route.PathPrefix, &route.StripPrefix, &route.UpstreamPathPrefix, &route.TargetHost, &route.TargetPort, &route.Status, &route.CreatedAt, &route.UpdatedAt); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") && strings.Contains(message, "proxy_routes")
}

func samePathBackend(a, b legacyPathOwner) bool {
	return a.ClientID == b.ClientID && a.TargetHost == b.TargetHost && a.TargetPort == b.TargetPort && a.Strip == b.Strip && a.Upstream == b.Upstream
}

func newMigrationID(prefix string) string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(value[:])
}
