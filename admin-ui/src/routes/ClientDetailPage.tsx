import { useParams } from 'react-router-dom';
import { useAuthedQuery } from '../hooks/useAuthedQuery';
import { queryClient } from '../lib/admin-graphql';
import { formatBytes } from '../lib/format';
import { ErrorState, NotFoundState, PageLoading } from '../components/PageStates';
import { isNotFoundError } from '../lib/contracts';
import { useSession } from '../session';
import { DetailBackLink, PageHeader, StatusBadge, Timestamp } from './shared';

export function ClientDetailPage() {
  const { id = '' } = useParams();
  const session = useSession();
  const query = useAuthedQuery({
    queryKey: ['client', id],
    queryFn: () => queryClient(id),
    refetchInterval: session.pollIntervalSeconds * 1000,
  });

  if (query.isLoading) {
    return <PageLoading label="Loading client..." />;
  }
  if (query.error) {
    if (isNotFoundError(query.error)) {
      return <NotFoundState resource="Client" />;
    }
    return <ErrorState title="Client failed" message={query.error.message} retry={() => query.refetch()} />;
  }
  if (!query.data) {
    return <PageLoading label="Loading client..." />;
  }

  const client = query.data;

  return (
    <section className="page-section">
      <DetailBackLink to="/clients" label="Back to clients" />
      <PageHeader title={client.name} description={`Client ID: ${client.id}`} actions={<StatusBadge value={client.status} />} />
      <div className="detail-grid">
        <article className="panel">
          <h2>Runtime</h2>
          <dl className="detail-list">
            <div><dt>Online</dt><dd>{client.runtime.online ? 'Yes' : 'No'}</dd></div>
            <div><dt>Protocol</dt><dd>{client.runtime.protocol ?? 'N/A'}</dd></div>
            <div><dt>Connected</dt><dd><Timestamp value={client.runtime.connectedAt} /></dd></div>
            <div><dt>Last heartbeat</dt><dd><Timestamp value={client.runtime.lastHeartbeat} /></dd></div>
            <div><dt>Error summary</dt><dd>{client.runtime.errorSummary || 'None'}</dd></div>
          </dl>
        </article>
        <article className="panel">
          <h2>Stats</h2>
          <dl className="detail-list">
            <div><dt>Active proxies</dt><dd>{client.runtime.activeProxies ?? 0}</dd></div>
            <div><dt>Active streams</dt><dd>{client.runtime.activeStreams ?? 0}</dd></div>
            <div><dt>Upload</dt><dd>{formatBytes(client.runtime.uploadBytes)}</dd></div>
            <div><dt>Download</dt><dd>{formatBytes(client.runtime.downloadBytes)}</dd></div>
            <div><dt>Version</dt><dd>{client.version}</dd></div>
          </dl>
        </article>
      </div>

      <article className="panel">
        <h2>Managed proxies</h2>
        {client.managedProxies.length === 0 ? (
          <p className="muted">No proxies assigned to this client.</p>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Status</th>
                  <th>Runtime</th>
                  <th>Entry</th>
                </tr>
              </thead>
              <tbody>
                {client.managedProxies.map((proxy) => (
                  <tr key={proxy.id}>
                    <td>{proxy.id}</td>
                    <td>{proxy.name}</td>
                    <td>{proxy.type}</td>
                    <td><StatusBadge value={proxy.status} /></td>
                    <td><StatusBadge value={proxy.runtimeStatus} /></td>
                    <td>{proxy.entryHost ?? '0.0.0.0'}:{proxy.entryPort ?? '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </article>
    </section>
  );
}
