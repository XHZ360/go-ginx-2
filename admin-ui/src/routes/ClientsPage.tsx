import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { queryClients, type ClientFilter } from '../lib/admin-graphql';
import { EmptyState, ErrorState, FilteredEmptyState, PageLoading } from '../components/PageStates';
import { useSession } from '../session';
import { PageHeader, Pagination, StatusBadge, Timestamp } from './shared';

const defaultFilter: ClientFilter = { query: '', userId: '', status: '' };

export function ClientsPage() {
  const session = useSession();
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState<ClientFilter>(defaultFilter);

  const query = useAuthedQuery({
    queryKey: ['clients', page, filter],
    queryFn: () => queryClients({ page: { page, pageSize: 10 }, sort: { field: 'name', direction: 'asc' }, filter }),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });

  if (query.isLoading) {
    return <PageLoading label="Loading clients..." />;
  }
  if (query.error) {
    return <ErrorState title="Clients failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading clients..." />;
  }

  const data = query.data;

  const hasFilter = Boolean(filter.query || filter.userId || filter.status);

  return (
    <section className="page-section">
      <PageHeader
        title="Clients"
        description="Live runtime view for managed clients."
        actions={
          <button type="button" className="button button--secondary" onClick={() => query.refetch()}>
            Refresh
          </button>
        }
      />

      <div className="toolbar-grid">
        <label className="field">
          <span className="field__label">Search</span>
          <input className="input" value={filter.query ?? ''} onChange={(event) => setFilter((current) => ({ ...current, query: event.target.value }))} />
        </label>
        <label className="field">
          <span className="field__label">User ID</span>
          <input className="input" value={filter.userId ?? ''} onChange={(event) => setFilter((current) => ({ ...current, userId: event.target.value }))} />
        </label>
        <label className="field">
          <span className="field__label">Status</span>
          <select className="input" value={filter.status ?? ''} onChange={(event) => setFilter((current) => ({ ...current, status: event.target.value }))}>
            <option value="">All</option>
            <option value="online">Online</option>
            <option value="offline">Offline</option>
            <option value="disabled">Disabled</option>
          </select>
        </label>
      </div>

      {data.items.length === 0 ? (
        hasFilter ? (
          <FilteredEmptyState onClear={() => setFilter(defaultFilter)} />
        ) : (
          <EmptyState title="No clients" message="No managed clients are registered yet." />
        )
      ) : (
        <>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>User</th>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Online</th>
                  <th>Protocol</th>
                  <th>Active proxies</th>
                  <th>Active streams</th>
                  <th>Last heartbeat</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((client) => (
                  <tr key={client.id} className="table-row-link" onClick={() => navigate(`/clients/${client.id}`)}>
                    <td>{client.id}</td>
                    <td>{client.userId}</td>
                    <td>{client.name}</td>
                    <td><StatusBadge value={client.status} /></td>
                    <td>{client.runtime.online ? 'Yes' : 'No'}</td>
                    <td>{client.runtime.protocol ?? 'N/A'}</td>
                    <td>{client.runtime.activeProxies ?? 0}</td>
                    <td>{client.runtime.activeStreams ?? 0}</td>
                    <td><Timestamp value={client.runtime.lastHeartbeat} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination page={data.pageInfo.page} totalPages={data.pageInfo.totalPages} onPageChange={setPage} />
        </>
      )}
    </section>
  );
}
